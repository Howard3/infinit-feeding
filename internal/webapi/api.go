package webapi

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"geevly/internal/student"
	"geevly/internal/webapi/templates"
)

type Server struct {
	ctx           context.Context
	ListenAddress string
	StaticFS      fs.FS
	StudentRepo   student.Repository
}

func (s *Server) verifyConfig() {
	if s.StaticFS == nil {
		panic("StaticFS is required")
	}
	if s.StudentRepo == nil {
		panic("StudentRepo is required")
	}
}

func (s *Server) getListenAddress() string {
	if s.ListenAddress == "" {
		return ":3000"
	}
	return s.ListenAddress
}

// TODO: more secure error page, anything could be dumped here!
func (s *Server) errorPage(w http.ResponseWriter, r *http.Request, title string, err error) {
	page := templates.Layout(templates.SystemError(title, err.Error()))
	if err := page.Render(r.Context(), w); err != nil {
		slog.Error("failed to render error page", "error", err)
	}
}

func (s *Server) renderInlayout(w http.ResponseWriter, r *http.Request, component templ.Component) {
	var page templ.Component

	isHTMX := r.Header.Get("HX-Request") == "true"

	if isHTMX {
		page = templates.HTMXLayout(component)
	} else {
		page = templates.Layout(component)
	}
	if err := page.Render(r.Context(), w); err != nil {
		slog.Error("failed to render component", "error", err)
	}
}

func (s *Server) Start(ctx context.Context) {
	s.ctx = ctx

	s.verifyConfig()

	// start chi
	c := chi.NewRouter()
	c.Use(middleware.Logger)
	c.Use(middleware.Recoverer)
	c.Use(middleware.Compress(5))
	c.Use(middleware.Logger)

	// serve static files
	c.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(s.StaticFS))))

	c.Get("/", func(w http.ResponseWriter, r *http.Request) {
		// TODO: run in parallel
		students, err := s.StudentRepo.ListStudents(r.Context(), 10, 0)
		if err != nil {
			s.errorPage(w, r, "Failed to list students", err)
			return
		}
		count, err := s.StudentRepo.CountStudents(r.Context())
		if err != nil {
			s.errorPage(w, r, "Failed to count students", err)
			return
		}

		s.renderInlayout(w, r, templates.StudentList(students, count))
	})

	c.Get("/admin/student/create", func(w http.ResponseWriter, r *http.Request) {
		s.renderInlayout(w, r, templates.CreateStudent())
	})

	slog.Info("Starting server", "listen_address", s.getListenAddress())
	if err := http.ListenAndServe(s.getListenAddress(), c); err != nil {
		panic(fmt.Errorf("failed to start server: %w", err))
	}
}
