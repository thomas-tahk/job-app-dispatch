package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// PythonScraper satisfies Scraper by running a Python script as a subprocess.
// The script receives --config <path> and writes a JSON array of ScrapedJob to stdout.
type PythonScraper struct {
	source     string
	scriptPath string
	configPath string
}

func NewPythonScraper(source, scriptPath, configPath string) *PythonScraper {
	return &PythonScraper{source: source, scriptPath: scriptPath, configPath: configPath}
}

func (p *PythonScraper) Source() string { return p.source }

func (p *PythonScraper) Scrape(ctx context.Context) ([]ScrapedJob, error) {
	cmd := exec.CommandContext(ctx, "python", p.scriptPath, "--config", p.configPath)
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		return nil, fmt.Errorf("scraper %s failed: %w\nstderr: %s", p.source, err, stderr)
	}
	var jobs []ScrapedJob
	if err := json.Unmarshal(out, &jobs); err != nil {
		return nil, fmt.Errorf("scraper %s returned invalid JSON: %w", p.source, err)
	}
	return jobs, nil
}

// PythonSubmitter satisfies Submitter by running a Python script as a subprocess.
// The script reads a SubmitRequest JSON from stdin and writes a SubmitResult JSON to stdout.
type PythonSubmitter struct {
	source     string
	scriptPath string
}

func NewPythonSubmitter(source, scriptPath string) *PythonSubmitter {
	return &PythonSubmitter{source: source, scriptPath: scriptPath}
}

func (p *PythonSubmitter) Source() string { return p.source }

func (p *PythonSubmitter) Submit(ctx context.Context, req SubmitRequest) (SubmitResult, error) {
	input, err := json.Marshal(req)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("marshal submit request: %w", err)
	}
	cmd := exec.CommandContext(ctx, "python", p.scriptPath)
	cmd.Stdin = strings.NewReader(string(input))
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		return SubmitResult{Success: false, FailureReason: fmt.Sprintf("%v: %s", err, stderr)}, nil
	}
	var result SubmitResult
	if err := json.Unmarshal(out, &result); err != nil {
		return SubmitResult{}, fmt.Errorf("submitter %s returned invalid JSON: %w", p.source, err)
	}
	return result, nil
}
