package ai

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Client wraps the Anthropic SDK for cover letter generation and match rationale.
type Client struct {
	client anthropic.Client
}

func New(apiKey string) *Client {
	return &Client{client: anthropic.NewClient(option.WithAPIKey(apiKey))}
}

// CoverLetterRequest carries all context needed to generate a tailored cover letter.
type CoverLetterRequest struct {
	JobTitle       string
	Company        string
	JobDescription string
	ResumeText     string
	StyleSamples   []string // past cover letters — tone/voice reference only
	LinkedInURL    string
	GitHubURL      string
}

// GenerateCoverLetter produces a tailored cover letter matching the user's voice.
func (c *Client) GenerateCoverLetter(ctx context.Context, req CoverLetterRequest) (string, error) {
	samplesBlock := ""
	for i, s := range req.StyleSamples {
		samplesBlock += fmt.Sprintf("\n--- Sample %d ---\n%s\n", i+1, s)
	}

	prompt := fmt.Sprintf(`You are writing a cover letter on behalf of the applicant.

STYLE REFERENCE (past letters written by the applicant — match tone and voice, do not reuse content verbatim):
%s

JOB POSTING:
Title: %s
Company: %s
Description:
%s

APPLICANT RESUME SUMMARY:
%s

APPLICANT LINKS:
LinkedIn: %s
GitHub: %s

Write a concise, genuine cover letter (3–4 paragraphs) in the applicant's voice.
Be specific to this role and company. Include the LinkedIn and GitHub links where natural.
Do not use generic filler phrases. Do not start with "I am writing to apply for".`,
		samplesBlock,
		req.JobTitle,
		req.Company,
		req.JobDescription,
		req.ResumeText,
		req.LinkedInURL,
		req.GitHubURL,
	)

	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_6,
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("cover letter generation: %w", err)
	}
	if len(msg.Content) == 0 {
		return "", fmt.Errorf("empty response from Claude")
	}
	return msg.Content[0].Text, nil
}

// GenerateMatchRationale produces a short one-liner explaining the match for the digest.
func (c *Client) GenerateMatchRationale(ctx context.Context, jobTitle, company, jobDescription, resumeText string, score float64) (string, error) {
	prompt := fmt.Sprintf(`In one sentence (max 20 words), explain why this job is a good match for this applicant.
Be specific — mention a skill or experience that aligns. Score: %.0f/100.

Job: %s at %s
Description excerpt: %.500s
Resume excerpt: %.500s`,
		score*100, jobTitle, company, jobDescription, resumeText,
	)

	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_6,
		MaxTokens: 100,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("match rationale generation: %w", err)
	}
	if len(msg.Content) == 0 {
		return "", nil
	}
	return msg.Content[0].Text, nil
}
