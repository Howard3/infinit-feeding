package webapi

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"geevly/gen/go/eda"
	"geevly/internal/student"
	"geevly/internal/webapi/templates"
)

type Server struct {
	ctx           context.Context
	ListenAddress string
	StaticFS      fs.FS
	StudentSvc    *student.StudentService
}

func (s *Server) verifyConfig() {
	if s.StaticFS == nil {
		panic("StaticFS is required")
	}
	if s.StudentSvc == nil {
		panic("StudentSvc is required")
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
	s.renderInlayout(w, r, templates.SystemError(title, err.Error()))
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

func (s *Server) pageQuery(r *http.Request) uint {
	sPage := r.URL.Query().Get("page")
	if sPage == "" {
		return 1
	}

	page, err := strconv.ParseUint(sPage, 10, 32)
	if err != nil {
		return 1
	}

	return uint(page)
}

func (s *Server) limitQuery(r *http.Request) uint {
	sLimit := r.URL.Query().Get("limit")
	if sLimit == "" {
		return 10
	}

	limit, err := strconv.ParseUint(sLimit, 10, 32)
	if err != nil {
		return 10
	}

	return uint(limit)
}

func (s *Server) formAsDate(r *http.Request, key string) (*eda.Date, error) {
	sDate := r.FormValue(key)
	if sDate == "" {
		return nil, fmt.Errorf("no date provided")
	}

	date, err := time.Parse("2006-01-02", sDate)
	if err != nil {
		return nil, fmt.Errorf("invalid date format")
	}

	return &eda.Date{Year: int32(date.Year()), Month: int32(date.Month()), Day: int32(date.Day())}, nil
}

// formAsInt32 returns the value of the form field as an int64.
func (s *Server) formAsInt64(r *http.Request, key string) (int64, error) {
	sValue := r.FormValue(key)
	if sValue == "" {
		return 0, fmt.Errorf("no value provided")
	}

	value, err := strconv.ParseInt(sValue, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number format")
	}

	return value, nil
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

	c.Route("/admin", func(r chi.Router) {
		r.Route("/student", s.studentAdminRoutes)
	})

	slog.Info("Starting server", "listen_address", s.getListenAddress())
	if err := http.ListenAndServe(s.getListenAddress(), c); err != nil {
		panic(fmt.Errorf("failed to start server: %w", err))
	}
}
