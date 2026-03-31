package connector

import "context"

// Scraper is the interface every job board scraper must satisfy.
// Each connector implements this by running a Python subprocess.
type Scraper interface {
	Source() string
	Scrape(ctx context.Context) ([]ScrapedJob, error)
}

// Submitter is the interface every job board submitter must satisfy.
type Submitter interface {
	Source() string
	Submit(ctx context.Context, req SubmitRequest) (SubmitResult, error)
}

// ScrapedJob is the raw, unfiltered output from a scraper script.
// Field names match the Python Pydantic model for direct JSON round-tripping.
type ScrapedJob struct {
	ExternalID    string  `json:"external_id"`
	Source        string  `json:"source"` // set by the Go runner from Scraper.Source()
	Title         string  `json:"title"`
	Company       string  `json:"company"`
	Location      string  `json:"location"`
	IsRemote      bool    `json:"is_remote"`
	SalaryRaw     string  `json:"salary_raw"`
	SalaryMin     float64 `json:"salary_min"`
	SalaryMax     float64 `json:"salary_max"`
	HasHealthcare bool    `json:"has_healthcare"`
	Benefits      string  `json:"benefits"`
	Description   string  `json:"description"`
	ApplyURL      string  `json:"apply_url"`
	IsEasyApply   bool    `json:"is_easy_apply"`
	PostedAt      string  `json:"posted_at"`
}

// SubmitRequest is passed to a submitter script via stdin as JSON.
type SubmitRequest struct {
	Job         ScrapedJob `json:"job"`
	CoverLetter string     `json:"cover_letter"`
	ResumeFile  string     `json:"resume_file"` // absolute path to the PDF
	LinkedInURL string     `json:"linkedin_url"`
	GitHubURL   string     `json:"github_url"`
}

// SubmitResult is returned by a submitter script via stdout as JSON.
type SubmitResult struct {
	Success       bool   `json:"success"`
	FailureReason string `json:"failure_reason,omitempty"`
	ManualURL     string `json:"manual_url,omitempty"` // set when automation cannot handle the form
}
