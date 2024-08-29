package webapi

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/clerkinc/clerk-sdk-go/clerk"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"geevly/internal/file"
	"geevly/internal/school"
	"geevly/internal/student"
	"geevly/internal/user"
	"geevly/internal/webapi/templates"
	"geevly/internal/webapi/templates/admin"
	"geevly/internal/webapi/templates/layouts"
)

type Server struct {
	ctx           context.Context
	ListenAddress string
	StaticFS      fs.FS
	StudentSvc    *student.StudentService
	SchoolSvc     *school.Service
	UserSvc       *user.Service
	FileSvc       *file.Service
	Clerk         clerk.Client
}

type Roles struct {
	Admin      bool
	IsSignedIn bool
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
	s.renderTempl(w, r, templates.SystemError(title, err.Error()))
}

func (s *Server) renderTempl(w http.ResponseWriter, r *http.Request, page templ.Component) {
	// add roles to the context for the layout
	ctx := r.Context()
	roles, ok := ctx.Value("roles").(Roles)
	params := layouts.Params{}
	if ok {
		params.IsAdmin = roles.Admin
		params.IsSignedIn = roles.IsSignedIn
	}

	page = layouts.Layout(r, page, params)

	if err := page.Render(ctx, w); err != nil {
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

func (s *Server) Start(ctx context.Context) {
	s.ctx = ctx

	s.verifyConfig()

	// start chi
	c := chi.NewRouter()
	c.Use(middleware.Logger)
	c.Use(middleware.Recoverer)
	c.Use(middleware.Compress(5))
	c.Use(middleware.Logger)
	c.Use(clerk.WithSessionV2(s.Clerk))
	c.Use(s.AddRolesToContext)

	// serve static files
	c.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(s.StaticFS))))

	c.Get("/", func(w http.ResponseWriter, r *http.Request) {
		s.renderTempl(w, r, templates.Home())
	})

	c.Get("/about", func(w http.ResponseWriter, r *http.Request) {
		s.renderTempl(w, r, templates.About())
	})

	c.Get("/how-it-works", func(w http.ResponseWriter, r *http.Request) {
		s.renderTempl(w, r, templates.HowItWorks())
	})

	c.Route("/student", func(r chi.Router) {
		r.Get("/profile/photo/{ID}", s.studentProfilePhoto)
		r.Get("/feeding/photo/{ID}", s.studentFeedingPhoto)
	})

	c.Route("/admin", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Use(s.requireAdmin)
		r.Route("/student", s.studentAdminRoutes)
		r.Route("/school", s.schoolAdminRoutes)
		r.Route("/user", s.userAdminRouter)
		r.Route("/reports", s.adminReports)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			s.renderTempl(w, r, admin.AdminHome())
		})
	})

	c.Route("/feeding", s.feedingRoutes)

	c.Get("/sign-in", s.signIn)

	slog.Info("Starting server", "listen_address", s.getListenAddress())
	if err := http.ListenAndServe(s.getListenAddress(), c); err != nil {
		panic(fmt.Errorf("failed to start server: %w", err))
	}
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, _ := clerk.SessionFromContext(r.Context())
		if session == nil {
			http.Redirect(w, r, "/sign-in", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) signIn(w http.ResponseWriter, r *http.Request) {
	s.renderTempl(w, r, templates.SignIn())
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roles, ok := r.Context().Value("roles").(Roles)
		if !ok || !roles.Admin {
			s.renderTempl(w, r, templates.PermissionDenied())
			return
		}
		next.ServeHTTP(w, r)
	})
}

func getMetadataValue[T any](metadata any, key string) (out T, err error) {
	m, ok := metadata.(map[string]interface{})
	if !ok {
		return out, fmt.Errorf("metadata is not a map")
	}

	value, ok := m[key]
	if !ok {
		return out, fmt.Errorf("key not found")
	}

	v, ok := value.(T)
	if !ok {
		return out, fmt.Errorf("value is not of type %T", out)
	}

	return v, nil
}

func (s *Server) AddRolesToContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, _ := clerk.SessionFromContext(r.Context())
		if session == nil {
			next.ServeHTTP(w, r)
			return
		}

		userID := session.Claims.Subject
		user, err := s.Clerk.Users().Read(userID)
		if err != nil {
			http.Error(w, "Failed to read user data", http.StatusInternalServerError)
			return
		}

		roles := Roles{
			Admin: false,
		}

		isAdmin, err := getMetadataValue[bool](user.PrivateMetadata, "admin")
		if err == nil {
			roles.Admin = isAdmin
		}

		roles.IsSignedIn = true

		ctx := context.WithValue(r.Context(), "roles", roles)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
