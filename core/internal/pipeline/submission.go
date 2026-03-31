package pipeline

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/thomas-tahk/job-app-dispatch/internal/ai"
	"github.com/thomas-tahk/job-app-dispatch/internal/connector"
	"github.com/thomas-tahk/job-app-dispatch/internal/models"
)

// ProcessApproval is called when the user approves a job in the web UI.
// It generates a cover letter and creates an Application record.
// Actual submission is triggered separately by the user after reviewing the letter.
func (r *Runner) ProcessApproval(ctx context.Context, jobID uint) error {
	var job models.Job
	if err := r.DB.First(&job, jobID).Error; err != nil {
		return fmt.Errorf("load job %d: %w", jobID, err)
	}

	// Load the matching resume for this role type.
	var resume models.Resume
	r.DB.Where("role_type = ?", job.RoleType).First(&resume)

	// Load style samples from the cover_letter_samples directory.
	samples, err := LoadStyleSamples(r.Config.SamplesDir)
	if err != nil {
		log.Printf("submission: load style samples: %v (continuing without samples)", err)
	}

	// Generate the cover letter.
	coverLetter, err := r.AI.GenerateCoverLetter(ctx, ai.CoverLetterRequest{
		JobTitle:       job.Title,
		Company:        job.Company,
		JobDescription: job.Description,
		ResumeText:     resume.ParsedText,
		StyleSamples:   samples,
		LinkedInURL:    r.Config.LinkedInURL,
		GitHubURL:      r.Config.GitHubURL,
	})
	if err != nil {
		return fmt.Errorf("generate cover letter: %w", err)
	}

	// Choose the correct resume PDF.
	resumeFile := r.Config.DevResumePath
	if job.RoleType == models.RoleIT {
		resumeFile = r.Config.ITResumePath
	}

	// Create the Application record. Status stays "approved" until Submit is called.
	app := models.Application{
		JobID:       job.ID,
		CoverLetter: coverLetter,
		ResumeFile:  resumeFile,
	}
	if err := r.DB.Create(&app).Error; err != nil {
		return fmt.Errorf("create application record: %w", err)
	}

	log.Printf("submission: application prepared for job %d (%s at %s)", job.ID, job.Title, job.Company)
	return nil
}

// Submit runs the actual form submission for an approved job.
// Called by the web handler after the user reviews and saves the cover letter.
// Runs synchronously; caller should invoke in a goroutine if needed.
func (r *Runner) Submit(ctx context.Context, jobID uint) {
	var job models.Job
	if err := r.DB.First(&job, jobID).Error; err != nil {
		log.Printf("submission: load job %d: %v", jobID, err)
		return
	}

	var app models.Application
	if err := r.DB.Where("job_id = ?", jobID).First(&app).Error; err != nil {
		log.Printf("submission: load application for job %d: %v", jobID, err)
		return
	}

	submitter, ok := r.Submitters[job.Source]
	if !ok {
		log.Printf("submission: no submitter registered for source %q", job.Source)
		r.DB.Model(&models.Job{}).Where("id = ?", job.ID).Update("status", models.StatusFailed)
		r.DB.Model(&models.Application{}).Where("id = ?", app.ID).Update("failure_reason", "no submitter for source: "+job.Source)
		r.Discord.NotifySubmissionFailed(job, job.ApplyURL)
		return
	}

	result, err := submitter.Submit(ctx, connector.SubmitRequest{
		Job:         jobToScrapedJob(job),
		CoverLetter: app.CoverLetter,
		ResumeFile:  app.ResumeFile,
		LinkedInURL: r.Config.LinkedInURL,
		GitHubURL:   r.Config.GitHubURL,
	})
	if err != nil {
		log.Printf("submission: submitter error for job %d: %v", job.ID, err)
		r.DB.Model(&models.Job{}).Where("id = ?", job.ID).Update("status", models.StatusFailed)
		r.Discord.NotifySubmissionFailed(job, job.ApplyURL)
		return
	}

	if result.Success {
		now := time.Now()
		r.DB.Model(&models.Application{}).Where("id = ?", app.ID).Update("submitted_at", &now)
		r.DB.Model(&models.Job{}).Where("id = ?", job.ID).Update("status", models.StatusSubmitted)
		r.Discord.NotifySubmissionSuccess(job)
		log.Printf("submission: success for job %d (%s at %s)", job.ID, job.Title, job.Company)
	} else {
		manualURL := result.ManualURL
		if manualURL == "" {
			manualURL = job.ApplyURL
		}
		r.DB.Model(&models.Application{}).Where("id = ?", app.ID).Update("failure_reason", result.FailureReason)
		r.DB.Model(&models.Job{}).Where("id = ?", job.ID).Update("status", models.StatusFailed)
		r.Discord.NotifySubmissionFailed(job, manualURL)
		log.Printf("submission: failed for job %d: %s", job.ID, result.FailureReason)
	}
}

// jobToScrapedJob maps a stored Job back to the ScrapedJob shape the submitter expects.
func jobToScrapedJob(j models.Job) connector.ScrapedJob {
	return connector.ScrapedJob{
		ExternalID:    j.ExternalID,
		Source:        j.Source,
		Title:         j.Title,
		Company:       j.Company,
		Location:      j.Location,
		IsRemote:      j.IsRemote,
		SalaryRaw:     j.SalaryRaw,
		SalaryMin:     j.SalaryMin,
		SalaryMax:     j.SalaryMax,
		HasHealthcare: j.HasHealthcare,
		Benefits:      j.Benefits,
		Description:   j.Description,
		ApplyURL:      j.ApplyURL,
		IsEasyApply:   j.IsEasyApply,
		PostedAt:      j.PostedAt.Format("2006-01-02"),
	}
}
