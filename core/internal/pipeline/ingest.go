package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/thomas-tahk/job-app-dispatch/internal/connector"
	"github.com/thomas-tahk/job-app-dispatch/internal/models"
	"github.com/thomas-tahk/job-app-dispatch/internal/scorer"
)

// IngestURL fetches a single job from a URL, scores it, stores it in the DB,
// and immediately triggers cover letter generation. Returns the job ID so the
// caller can redirect the user straight to the cover letter editor.
//
// Hard score filters are bypassed — the user explicitly chose this job.
// If the job already exists in the DB, the existing record's ID is returned.
func (r *Runner) IngestURL(ctx context.Context, rawURL string) (uint, error) {
	// 1. Fetch via Python subprocess.
	cmd := exec.CommandContext(ctx, "python3",
		"../connectors/ats/fetch_single.py",
		"--url", rawURL,
		"--config", "../config.yaml",
	)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		return 0, fmt.Errorf("fetch_single failed: %w\n%s", err, stderr)
	}

	var raw connector.ScrapedJob
	if err := json.Unmarshal(out, &raw); err != nil {
		return 0, fmt.Errorf("fetch_single returned invalid JSON: %w", err)
	}
	raw.Source = coalesce(raw.Source, "manual")

	// 2. Return existing job if already in DB.
	var existing models.Job
	if err := r.DB.Where("external_id = ? AND source = ?", raw.ExternalID, raw.Source).
		First(&existing).Error; err == nil {
		return existing.ID, nil
	}

	// 3. Score — ShouldSkip is ignored for manually ingested jobs.
	var devResume, itResume models.Resume
	r.DB.Where("role_type = ?", models.RoleDev).First(&devResume)
	r.DB.Where("role_type = ?", models.RoleIT).First(&itResume)
	resume := pickResume(raw, devResume, itResume, r.Config.Scorer)
	result := scorer.Score(raw, resume, r.Config.Scorer)

	requiresRelocation := !raw.IsRemote &&
		!isAllowedLocation(raw.Location, r.Config.Scorer.OnsiteAllowedLocations)

	postedAt, _ := time.Parse("2006-01-02", raw.PostedAt)
	if postedAt.IsZero() {
		postedAt = time.Now()
	}

	job := models.Job{
		ExternalID:         raw.ExternalID,
		Source:             raw.Source,
		Title:              raw.Title,
		Company:            raw.Company,
		Location:           raw.Location,
		IsRemote:           raw.IsRemote,
		RequiresRelocation: requiresRelocation,
		SalaryMin:          raw.SalaryMin,
		SalaryMax:          raw.SalaryMax,
		SalaryRaw:          raw.SalaryRaw,
		HasHealthcare:      raw.HasHealthcare,
		Benefits:           raw.Benefits,
		Description:        raw.Description,
		ApplyURL:           coalesce(raw.ApplyURL, rawURL),
		IsEasyApply:        raw.IsEasyApply,
		RoleType:           result.RoleType,
		Archetype:          result.Archetype,
		LegitimacyTier:     result.LegitimacyTier,
		MatchScore:         result.Score,
		Status:             models.StatusNew,
		ResumeVersion:      resume.Filename,
		PostedAt:           postedAt,
	}

	// 4. Persist.
	if err := r.DB.Create(&job).Error; err != nil {
		return 0, fmt.Errorf("store job: %w", err)
	}

	// 5. Immediately generate cover letter — user already chose this job.
	if err := r.ProcessApproval(ctx, job.ID); err != nil {
		return job.ID, fmt.Errorf("cover letter generation failed: %w", err)
	}

	return job.ID, nil
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
