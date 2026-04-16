package pipeline

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/thomas-tahk/job-app-dispatch/internal/ai"
	"github.com/thomas-tahk/job-app-dispatch/internal/connector"
	"github.com/thomas-tahk/job-app-dispatch/internal/discord"
	"github.com/thomas-tahk/job-app-dispatch/internal/models"
	"github.com/thomas-tahk/job-app-dispatch/internal/scorer"
	"gorm.io/gorm"
)

// Config holds everything the pipeline needs that isn't a service dependency.
type Config struct {
	WebAddr       string
	LinkedInURL   string
	GitHubURL     string
	DevResumePath string
	ITResumePath  string
	SamplesDir    string
	Scorer        scorer.Config
}

// Runner owns the full scrape → score → store → notify pipeline,
// and the approve → generate → submit pipeline.
type Runner struct {
	DB         *gorm.DB
	Scrapers   []connector.Scraper
	Submitters map[string]connector.Submitter
	AI         *ai.Client
	Discord    *discord.Bot
	Config     Config
}

// Run executes one full scrape cycle. Called by the scheduler.
func (r *Runner) Run(ctx context.Context) {
	log.Println("pipeline: starting scrape run")

	// 1. Scrape all sources; continue past individual failures.
	var rawJobs []connector.ScrapedJob
	for _, s := range r.Scrapers {
		jobs, err := s.Scrape(ctx)
		if err != nil {
			log.Printf("pipeline: scraper %q error: %v", s.Source(), err)
			continue
		}
		log.Printf("pipeline: %q returned %d jobs", s.Source(), len(jobs))
		rawJobs = append(rawJobs, jobs...)
	}

	if len(rawJobs) == 0 {
		log.Println("pipeline: no jobs scraped")
		return
	}

	// 2. Deduplicate against the DB.
	newJobs := r.deduplicate(rawJobs)
	log.Printf("pipeline: %d new after deduplication", len(newJobs))
	if len(newJobs) == 0 {
		return
	}

	// 3. Load resume data for scoring (best-effort; scoring degrades gracefully if empty).
	var devResume, itResume models.Resume
	r.DB.Where("role_type = ?", models.RoleDev).First(&devResume)
	r.DB.Where("role_type = ?", models.RoleIT).First(&itResume)

	// 4. Filter, score, enrich.
	var toStore []models.Job
	for _, raw := range newJobs {
		resume := pickResume(raw, devResume, itResume, r.Config.Scorer)
		result := scorer.Score(raw, resume, r.Config.Scorer)

		if result.ShouldSkip {
			log.Printf("pipeline: skip %q at %q: %s", raw.Title, raw.Company, result.SkipReason)
			continue
		}

		requiresRelocation := !raw.IsRemote &&
			!isAllowedLocation(raw.Location, r.Config.Scorer.OnsiteAllowedLocations)

		rationale, err := r.AI.GenerateMatchRationale(
			ctx, raw.Title, raw.Company, raw.Description, resume.ParsedText, result.Score,
		)
		if err != nil {
			log.Printf("pipeline: rationale error for %q: %v", raw.Title, err)
		}

		postedAt, _ := time.Parse("2006-01-02", raw.PostedAt)
		if postedAt.IsZero() {
			postedAt = time.Now()
		}

		toStore = append(toStore, models.Job{
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
			ApplyURL:           raw.ApplyURL,
			IsEasyApply:        raw.IsEasyApply,
			RoleType:           result.RoleType,
			Archetype:          result.Archetype,
			LegitimacyTier:     result.LegitimacyTier,
			MatchScore:         result.Score,
			MatchReason:        rationale,
			Status:             models.StatusNew,
			ResumeVersion:      resume.Filename,
			PostedAt:           postedAt,
		})
	}

	// 5. Persist.
	stored := 0
	for i := range toStore {
		if err := r.DB.Create(&toStore[i]).Error; err != nil {
			log.Printf("pipeline: db error for %q: %v", toStore[i].Title, err)
			continue
		}
		stored++
	}
	log.Printf("pipeline: stored %d new jobs", stored)

	// 6. Notify Discord.
	if stored > 0 {
		addr := "http://localhost" + r.Config.WebAddr
		if err := r.Discord.NotifyDigestReady(stored, addr); err != nil {
			log.Printf("pipeline: discord notify error: %v", err)
		}
	}

	log.Println("pipeline: run complete")
}

// deduplicate returns only jobs not already present in the DB (keyed on ExternalID + Source).
func (r *Runner) deduplicate(jobs []connector.ScrapedJob) []connector.ScrapedJob {
	var out []connector.ScrapedJob
	for _, j := range jobs {
		var count int64
		r.DB.Model(&models.Job{}).
			Where("external_id = ? AND source = ?", j.ExternalID, j.Source).
			Count(&count)
		if count == 0 {
			out = append(out, j)
		}
	}
	return out
}

// pickResume selects the resume that best matches the scraped job's role type.
func pickResume(job connector.ScrapedJob, dev, it models.Resume, cfg scorer.Config) models.Resume {
	lower := strings.ToLower(job.Title + " " + job.Description)
	for _, kw := range cfg.ITKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return it
		}
	}
	return dev
}

// isAllowedLocation does a case-insensitive substring check against the allowed list.
func isAllowedLocation(location string, allowed []string) bool {
	lower := strings.ToLower(location)
	for _, a := range allowed {
		if strings.Contains(lower, strings.ToLower(a)) {
			return true
		}
	}
	return false
}
