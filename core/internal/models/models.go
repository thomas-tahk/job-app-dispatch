package models

import (
	"time"

	"gorm.io/gorm"
)

type JobStatus string

const (
	StatusNew       JobStatus = "new"
	StatusApproved  JobStatus = "approved"
	StatusRejected  JobStatus = "rejected"
	StatusSubmitted JobStatus = "submitted"
	StatusFailed    JobStatus = "failed"
	StatusManual    JobStatus = "manual" // automation failed; user must apply manually
)

type RoleType string

const (
	RoleDev RoleType = "dev"
	RoleIT  RoleType = "it"
)

// Job represents a single job listing, from first scrape through final disposition.
type Job struct {
	gorm.Model
	ExternalID         string    `gorm:"uniqueIndex"` // platform-native ID for deduplication
	Source             string    // "linkedin" | "indeed"
	Title              string
	Company            string
	Location           string
	IsRemote           bool
	RequiresRelocation bool      // true if on-site outside ABQ/Santa Fe
	SalaryMin          float64
	SalaryMax          float64
	SalaryRaw          string    // original text, kept for display
	HasHealthcare      bool
	Benefits           string
	Description        string
	ApplyURL           string
	IsEasyApply        bool      // true if the platform handles the form (LinkedIn Easy Apply, Indeed Apply)
	RoleType           RoleType
	MatchScore         float64
	MatchReason        string    // AI-generated one-liner for digest display
	Status             JobStatus `gorm:"default:'new'"`
	ResumeVersion      string    // filename of the resume chosen for this role
	PostedAt           time.Time
}

// Application tracks a single submission attempt for an approved job.
type Application struct {
	gorm.Model
	JobID           uint
	Job             Job
	CoverLetter     string     // final version (after user edits)
	ResumeFile      string     // path to PDF submitted
	ResumeDiff      string     // AI-generated tailoring suggestions (plain text)
	DiffGeneratedAt *time.Time // when the diff was generated; used to detect resume file updates
	SubmittedAt     *time.Time
	FailureReason   string
}

// Resume stores parsed resume data, keyed by filename.
// Re-populated automatically when files change in materials/resumes/.
type Resume struct {
	gorm.Model
	Filename   string   `gorm:"uniqueIndex"`
	RoleType   RoleType // which role type this version targets
	ParsedText string   // full plain text, used for scoring
	Skills     string   // JSON array of extracted skill strings
	Titles     string   // JSON array of job titles from experience section
}
