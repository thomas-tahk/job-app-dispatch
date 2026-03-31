package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/thomas-tahk/job-app-dispatch/internal/models"
	"gorm.io/gorm"
)

//go:embed templates/*
var templateFS embed.FS

// Server is the local web UI for reviewing jobs and editing cover letters.
type Server struct {
	router    *chi.Mux
	db        *gorm.DB
	addr      string
	templates *template.Template
	// onApprove generates a cover letter and creates an Application record.
	onApprove func(ctx context.Context, jobID uint) error
	// onSubmit runs the actual form submission for an approved job (blocking).
	onSubmit func(ctx context.Context, jobID uint)
}

func New(
	db *gorm.DB,
	addr string,
	onApprove func(context.Context, uint) error,
	onSubmit func(context.Context, uint),
) (*Server, error) {
	funcs := template.FuncMap{
		// pct converts a 0.0–1.0 score to a 0–100 integer for display.
		"pct": func(f float64) int { return int(f * 100) },
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &Server{
		db:        db,
		addr:      addr,
		templates: tmpl,
		onApprove: onApprove,
		onSubmit:  onSubmit,
	}
	s.router = chi.NewRouter()
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	s.router.Get("/", s.handleDigest)
	s.router.Post("/jobs/{id}/approve", s.handleApprove)
	s.router.Post("/jobs/{id}/reject", s.handleReject)
	s.router.Get("/jobs/{id}/cover", s.handleCoverView)
	s.router.Post("/jobs/{id}/cover", s.handleCoverSave)
	s.router.Post("/jobs/{id}/submit", s.handleSubmit)
}

func (s *Server) Start() error {
	return http.ListenAndServe(s.addr, s.router)
}

// handleDigest renders the main job review page, sorted by score descending.
func (s *Server) handleDigest(w http.ResponseWriter, r *http.Request) {
	var jobs []models.Job
	s.db.Where("status = ?", models.StatusNew).Order("match_score desc").Find(&jobs)
	s.templates.ExecuteTemplate(w, "digest.html", map[string]any{"Jobs": jobs})
}

// handleApprove generates a cover letter for the job and redirects to the editor.
// The user reviews and edits before triggering actual submission.
func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := s.onApprove(r.Context(), uint(id)); err != nil {
		http.Error(w, "Failed to generate cover letter: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/jobs/%d/cover", id), http.StatusSeeOther)
}

// handleReject marks a job as rejected — never shown again.
func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	s.db.Model(&models.Job{}).Where("id = ?", id).Update("status", models.StatusRejected)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleCoverView renders the cover letter editor for a job.
func (s *Server) handleCoverView(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var app models.Application
	s.db.Preload("Job").Where("job_id = ?", id).First(&app)
	s.templates.ExecuteTemplate(w, "cover_edit.html", map[string]any{
		"Application": app,
		"JobID":       id,
	})
}

// handleCoverSave persists edits to the cover letter.
func (s *Server) handleCoverSave(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.db.Model(&models.Application{}).
		Where("job_id = ?", id).
		Update("cover_letter", r.FormValue("cover_letter"))
	http.Redirect(w, r, fmt.Sprintf("/jobs/%d/cover", id), http.StatusSeeOther)
}

// handleSubmit saves the latest cover letter text, then triggers submission
// in a background goroutine so the redirect is immediate.
func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// Save the cover letter from the form before kicking off submission.
	if cl := r.FormValue("cover_letter"); cl != "" {
		s.db.Model(&models.Application{}).
			Where("job_id = ?", id).
			Update("cover_letter", cl)
	}
	go s.onSubmit(context.Background(), uint(id))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
