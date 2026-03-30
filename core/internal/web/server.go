package web

import (
	"embed"
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
}

func New(db *gorm.DB, addr string) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &Server{db: db, addr: addr, templates: tmpl}
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
}

func (s *Server) Start() error {
	return http.ListenAndServe(s.addr, s.router)
}

// handleDigest renders the main job review page with all new jobs, sorted by score.
func (s *Server) handleDigest(w http.ResponseWriter, r *http.Request) {
	var jobs []models.Job
	s.db.Where("status = ?", models.StatusNew).Order("match_score desc").Find(&jobs)
	s.templates.ExecuteTemplate(w, "digest.html", map[string]any{"Jobs": jobs})
}

// handleApprove marks a job as approved and queues it for submission.
func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	s.db.Model(&models.Job{}).Where("id = ?", id).Update("status", models.StatusApproved)
	// TODO: trigger cover letter generation if not already done, then queue submission
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleReject marks a job as rejected so it is never shown again.
func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	s.db.Model(&models.Job{}).Where("id = ?", id).Update("status", models.StatusRejected)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleCoverView renders the cover letter editor for a specific job.
func (s *Server) handleCoverView(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var app models.Application
	s.db.Preload("Job").Where("job_id = ?", id).First(&app)
	s.templates.ExecuteTemplate(w, "cover_edit.html", map[string]any{"Application": app})
}

// handleCoverSave saves the edited cover letter back to the application record.
func (s *Server) handleCoverSave(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	coverLetter := r.FormValue("cover_letter")
	s.db.Model(&models.Application{}).Where("job_id = ?", id).Update("cover_letter", coverLetter)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
