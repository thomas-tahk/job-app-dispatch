package scorer

import (
	"strings"
	"time"

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
	SalaryFloorHourly      float64
	OnsiteAllowedLocations []string // e.g. ["Albuquerque, NM", "Santa Fe, NM"]
	DevKeywords            []string
	ITKeywords             []string
}

// Result is the output of scoring a single job.
type Result struct {
	Score          float64
	RoleType       models.RoleType
	Archetype      string // e.g. "fullstack", "backend", "helpdesk"
	LegitimacyTier string // "high" | "caution" | "suspicious"
	ShouldSkip     bool
	SkipReason     string
}

// Score evaluates a raw scraped job against the user's resume and config.
//
// Hard skips:
//   - Salary is listed AND below the configured floor.
//   - No benefits info AND salary is also missing (completely data-free listing).
//
// Jobs without salary listed are kept — many legitimate ATS postings omit it.
func Score(job connector.ScrapedJob, resume models.Resume, cfg Config) Result {
	// --- Hard filters ---
	hasSalary := job.SalaryMin > 0 || job.SalaryMax > 0
	if hasSalary {
		effective := job.SalaryMin
		if effective == 0 {
			effective = job.SalaryMax
		}
		if effective < cfg.SalaryFloorHourly {
			return Result{ShouldSkip: true, SkipReason: "salary below floor"}
		}
	}
	// Drop completely data-free listings (no salary AND no benefits text at all).
	if !hasSalary && !job.HasHealthcare && job.Benefits == "" && job.Description == "" {
		return Result{ShouldSkip: true, SkipReason: "no job data"}
	}

	// --- Role classification ---
	roleType := classifyRole(job.Title, job.Description, cfg)

	// --- Dimension scores (0.0–1.0 each) ---
	roleScore := scoreRoleAlignment(roleType, resume.RoleType)
	salaryScore := scoreSalary(job.SalaryMin, cfg.SalaryFloorHourly)
	locationScore := scoreLocation(job, cfg)
	skillScore := scoreSkills(job.Description, resume.Skills)
	seniorityScore := scoreSeniority(job.Title, job.Description)

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
		Score:          total,
		RoleType:       roleType,
		Archetype:      detectArchetype(job.Title, job.Description, roleType),
		LegitimacyTier: assessLegitimacy(job),
	}
}

// ── Role classification ───────────────────────────────────────────────────────

func classifyRole(title, description string, cfg Config) models.RoleType {
	lower := strings.ToLower(title + " " + description)
	for _, kw := range cfg.ITKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return models.RoleIT
		}
	}
	for _, kw := range cfg.DevKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return models.RoleDev
		}
	}
	return models.RoleDev // default
}

// ── Archetype detection ───────────────────────────────────────────────────────

var devArchetypes = []struct {
	name     string
	keywords []string
}{
	{"ai_ml", []string{"machine learning", "artificial intelligence", "llm", "ml engineer", "data scientist", "nlp", "computer vision", "neural", "deep learning", "ai engineer"}},
	{"devops", []string{"devops", "dev ops", "platform engineer", "site reliability", "sre", "infrastructure", "kubernetes", "k8s", "terraform", "ci/cd", "cloud engineer"}},
	{"mobile", []string{"mobile", "ios", "android", "react native", "flutter", "swift", "kotlin"}},
	{"frontend", []string{"frontend", "front end", "front-end", "ui engineer", "react developer", "vue", "angular", "next.js", "svelte"}},
	{"backend", []string{"backend", "back end", "back-end", "api engineer", "microservices", "distributed systems", "golang", "rust engineer"}},
	{"fullstack", []string{"fullstack", "full stack", "full-stack", "full stack engineer"}},
}

var itArchetypes = []struct {
	name     string
	keywords []string
}{
	{"security", []string{"security", "infosec", "cybersecurity", "soc analyst", "penetration", "vulnerability", "cyber"}},
	{"network", []string{"network engineer", "networking", "cisco", "firewall", "vpn", "switching", "routing", "network admin"}},
	{"sysadmin", []string{"sysadmin", "system administrator", "windows server", "active directory", "exchange", "systems engineer"}},
	{"field_tech", []string{"field technician", "field tech", "deskside", "desktop technician", "on-site support"}},
	{"helpdesk", []string{"help desk", "helpdesk", "service desk", "tier 1", "tier 2", "l1 support", "l2 support", "it support specialist"}},
}

func detectArchetype(title, description string, roleType models.RoleType) string {
	lower := strings.ToLower(title + " " + description)
	archetypes := devArchetypes
	if roleType == models.RoleIT {
		archetypes = itArchetypes
	}
	for _, a := range archetypes {
		for _, kw := range a.keywords {
			if strings.Contains(lower, kw) {
				return a.name
			}
		}
	}
	return ""
}

// ── Legitimacy assessment ─────────────────────────────────────────────────────

func assessLegitimacy(job connector.ScrapedJob) string {
	// Suspicious: obvious ghost-job signals.
	if len(job.Description) < 200 {
		return "suspicious"
	}
	if job.ApplyURL == "" {
		return "suspicious"
	}
	age := daysOld(job.PostedAt)
	if age > 60 {
		return "suspicious"
	}
	// Caution: weaker signals.
	if len(job.Description) < 600 {
		return "caution"
	}
	if age > 21 {
		return "caution"
	}
	return "high"
}

func daysOld(postedAt string) int {
	if postedAt == "" {
		return 0 // unknown age — assume fresh
	}
	t, err := time.Parse("2006-01-02", postedAt)
	if err != nil || t.IsZero() {
		return 0
	}
	return int(time.Since(t).Hours() / 24)
}

// ── Dimension scorers ─────────────────────────────────────────────────────────

func scoreRoleAlignment(jobRole, resumeRole models.RoleType) float64 {
	if jobRole == resumeRole {
		return 1.0
	}
	return 0.3
}

func scoreSalary(salaryMin, floor float64) float64 {
	if salaryMin <= 0 {
		return 0.4 // salary unknown — neutral, not penalised
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
