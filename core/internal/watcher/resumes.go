package watcher

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// ResumeWatcher monitors a directory and calls onChanged whenever a .pdf or .docx
// file is created or modified. Used to trigger automatic resume re-parsing.
type ResumeWatcher struct {
	dir       string
	onChanged func(path string)
	w         *fsnotify.Watcher
}

func NewResumeWatcher(dir string, onChanged func(path string)) (*ResumeWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &ResumeWatcher{dir: dir, onChanged: onChanged, w: w}, nil
}

// Start begins watching in a background goroutine. Stops when ctx is cancelled.
func (r *ResumeWatcher) Start(ctx context.Context) error {
	if err := r.w.Add(r.dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext == ".pdf" || ext == ".docx" {
			path := filepath.Join(r.dir, e.Name())
			log.Printf("watcher: initial parse: %s", path)
			r.onChanged(path)
		}
	}
	go func() {
		for {
			select {
			case event, ok := <-r.w.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
					ext := filepath.Ext(event.Name)
					if ext == ".pdf" || ext == ".docx" {
						log.Printf("watcher: resume changed: %s", event.Name)
						r.onChanged(event.Name)
					}
				}
			case err, ok := <-r.w.Errors:
				if !ok {
					return
				}
				log.Printf("watcher: error: %v", err)
			case <-ctx.Done():
				r.w.Close()
				return
			}
		}
	}()
	return nil
}
