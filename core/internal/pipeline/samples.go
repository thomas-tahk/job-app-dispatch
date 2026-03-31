package pipeline

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadStyleSamples reads all .txt and .md files from dir and returns their text contents.
// Used to supply cover letter tone/voice reference to the AI client.
// PDF cover letters are not parsed here; save samples as .txt or .md.
func LoadStyleSamples(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var samples []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".txt", ".md":
			content, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			samples = append(samples, string(content))
		}
	}
	return samples, nil
}
