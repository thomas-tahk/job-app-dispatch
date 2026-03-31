package scorer

import (
	"strings"

	"github.com/thomas-tahk/job-app-dispatch/internal/connector"
	"github.com/thomas-tahk/job-app-dispatch/internal/models"
)

// Scoring dimension weights — must sum to 1.0.
const (
	weightRoleAlignment = 0.30
	weightSalary        = 0.20
	weightLocation      = 0.25
	weightSkills        = 0.20
	weightSeniority     = 0.05 // soft signal only; never causes a skip
)

// Config holds the scoring parameters loaded from config.yaml.
type Config struct {
	SalaryFloorHourly     float64
	OnsiteAllowedLocations []string // e.g. ["Albuquerque, NM", "Santa Fe, NM"]
	DevKeywords           []string
	ITKeywords            []string
}

// Result is the output of scoring a single job.
type Result struct {
	Score      float64
	RoleType   models.RoleType
	ShouldSkip bool
	SkipReason string
	// MatchReason is a short human-readable rationale; populated later by the AI client.
}

// Score evaluates a raw scraped job against the user's resume and config.
// Hard skips: salary not listed, salary below floor, no healthcare info.
// Everything else is a scored signal. Seniority is never a hard skip.
func Score(job connector.ScrapedJob, resume models.Resume, cfg Config) Result {
	// --- Hard filters ---
	if job.SalaryMin == 0 && job.SalaryMax == 0 {
		return Result{ShouldSkip: true, SkipReason: "salary not listed"}
	}
	effectiveSalary := job.SalaryMin
	if effectiveSalary == 0 {
		effectiveSalary = job.SalaryMax
	}
	if effectiveSalary < cfg.SalaryFloorHourly {
		return Result{ShouldSkip: true, SkipReason: "salary below floor"}
	}
	if !job.HasHealthcare && job.Benefits == "" {
		return Result{ShouldSkip: true, SkipReason: "no benefits information"}
	}

	// --- Role classification ---
	roleType := classifyRole(job.Title, job.Description, cfg)

	// --- Dimension scores (0.0–1.0 each) ---
	roleScore := scoreRoleAlignment(roleType, resume.RoleType)
	salaryScore := scoreSalary(effectiveSalary, cfg.SalaryFloorHourly)
	locationScore := scoreLocation(job, cfg)
	skillScore := scoreSkills(job.Description, resume.Skills)
	seniorityScore := scoreSeniority(job.Title, job.Description) // soft, low weight

	total := roleScore*weightRoleAlignment +
		salaryScore*weightSalary +
		locationScore*weightLocation +
		skillScore*weightSkills +
		seniorityScore*weightSeniority

	// Obvious mismatch: score too low AND role type is wrong.
	if total < 0.15 && roleType != resume.RoleType {
		return Result{ShouldSkip: true, SkipReason: "clear role mismatch and very low score"}
	}

	return Result{
		Score:    total,
		RoleType: roleType,
	}
}

func classifyRole(title, description string, cfg Config) models.RoleType {
	lower := strings.ToLower(title + " " + description)
	for _, kw := range cfg.DevKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return models.RoleDev
		}
	}
	for _, kw := range cfg.ITKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return models.RoleIT
		}
	}
	return models.RoleDev // default
}

func scoreRoleAlignment(jobRole, resumeRole models.RoleType) float64 {
	if jobRole == resumeRole {
		return 1.0
	}
	return 0.3
}

func scoreSalary(salaryMin, floor float64) float64 {
	if salaryMin <= 0 {
		return 0
	}
	ratio := (salaryMin - floor) / floor
	if ratio >= 1.0 {
		return 1.0
	}
	if ratio < 0 {
		return 0
	}
	return ratio
}

func scoreLocation(job connector.ScrapedJob, cfg Config) float64 {
	if job.IsRemote {
		return 1.0
	}
	lower := strings.ToLower(job.Location)
	for _, allowed := range cfg.OnsiteAllowedLocations {
		if strings.Contains(lower, strings.ToLower(allowed)) {
			return 0.8 // local on-site/hybrid
		}
	}
	return 0.4 // requires relocation — penalised but not excluded
}

func scoreSkills(description, resumeSkillsJSON string) float64 {
	// TODO: parse resumeSkillsJSON, count keyword matches in description
	_ = description
	_ = resumeSkillsJSON
	return 0.5 // placeholder until implemented
}

func scoreSeniority(title, description string) float64 {
	lower := strings.ToLower(title + " " + description)
	switch {
	case strings.Contains(lower, "senior") || strings.Contains(lower, "lead") || strings.Contains(lower, "principal"):
		return 0.6
	case strings.Contains(lower, "junior") || strings.Contains(lower, "entry"):
		return 1.0
	default:
		return 0.8
	}
}
