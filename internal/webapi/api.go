package webapi

import (
	"bytes"
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

	"geevly/internal/bulk_upload"
	"geevly/internal/file"
	"geevly/internal/school"
	"geevly/internal/student"
	"geevly/internal/webapi/bulk_domains"
	"geevly/internal/webapi/templates"
	"geevly/internal/webapi/templates/admin"
	"geevly/internal/webapi/templates/layouts"
)

// ServiceRegistry provides centralized access to all application services
type ServiceRegistry struct {
	StudentSvc    *student.StudentService
	SchoolSvc     *school.Service
	FileSvc       *file.Service
	BulkUploadSvc *bulk_upload.Service
}

// NewServiceRegistry creates a new service registry with the provided services
func NewServiceRegistry(
	studentSvc *student.StudentService,
	schoolSvc *school.Service,
	fileSvc *file.Service,
	bulkUploadSvc *bulk_upload.Service,
) *ServiceRegistry {
	return &ServiceRegistry{
		StudentSvc:    studentSvc,
		SchoolSvc:     schoolSvc,
		FileSvc:       fileSvc,
		BulkUploadSvc: bulkUploadSvc,
	}
}

type Server struct {
	ctx                context.Context
	ListenAddress      string
	StaticFS           fs.FS
	Services           *ServiceRegistry
	Clerk              clerk.Client // Admin Clerk instance
	SponsorClerk       clerk.Client // Sponsor/User frontend Clerk instance
	bulkDomainRegistry *bulk_domains.DomainRegistry
}

// NewServer creates a new Server instance with the provided services
func NewServer(
	listenAddress string,
	staticFS fs.FS,
	studentSvc *student.StudentService,
	schoolSvc *school.Service,
	fileSvc *file.Service,
	bulkUploadSvc *bulk_upload.Service,
	adminClerk clerk.Client,
	sponsorClerk clerk.Client,
) *Server {
	return &Server{
		ListenAddress: listenAddress,
		StaticFS:      staticFS,
		Services: NewServiceRegistry(
			studentSvc,
			schoolSvc,
			fileSvc,
			bulkUploadSvc,
		),
		Clerk:        adminClerk,
		SponsorClerk: sponsorClerk,
	}
}

type Roles struct {
	Admin      bool
	IsSignedIn bool
	IsFeeder   bool
	IsNurse    bool
}

func (s *Server) verifyConfig() {
	if s.StaticFS == nil {
		panic("StaticFS is required")
	}
	if s.Services == nil {
		panic("Services is required")
	}
	if s.Services.StudentSvc == nil {
		panic("StudentSvc is required")
	}
	if s.Services.SchoolSvc == nil {
		panic("SchoolSvc is required")
	}
	if s.Services.FileSvc == nil {
		panic("FileSvc is required")
	}
	if s.Services.BulkUploadSvc == nil {
		panic("BulkUploadSvc is required")
	}

	// Initialize the bulk domain registry if not already set
	if s.bulkDomainRegistry == nil {
		serviceRegistry := &bulk_domains.ServiceRegistry{
			SchoolService:  s.Services.SchoolSvc,
			StudentService: s.Services.StudentSvc,
			FileService:    s.Services.FileSvc,
		}
		s.bulkDomainRegistry = bulk_domains.NewDomainRegistry(serviceRegistry)
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
		params.IsFeeder = roles.IsFeeder
		params.IsNurse = roles.IsNurse
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
		return 15
	}

	limit, err := strconv.ParseUint(sLimit, 10, 32)
	if err != nil {
		return 15
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
	c.Use(logServerErrors)
	c.Use(PrometheusMiddleware)
	c.Use(middleware.Compress(5))
	c.Use(clerk.WithSessionV2(s.Clerk))
	c.Use(s.AddRolesToContext)

	// serve static files
	c.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(s.StaticFS))))

	// Health check endpoint - no auth required
	c.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Prometheus metrics endpoint
	c.Handle("/metrics", MetricsHandler())

	// Redirect root based on user role
	c.Get("/", func(w http.ResponseWriter, r *http.Request) {
		roles, ok := r.Context().Value("roles").(Roles)
		if !ok || !roles.IsSignedIn {
			http.Redirect(w, r, "/sign-in", http.StatusTemporaryRedirect)
			return
		}
		switch {
		case roles.Admin:
			http.Redirect(w, r, "/admin", http.StatusTemporaryRedirect)
		case roles.IsNurse:
			http.Redirect(w, r, "/nurse", http.StatusTemporaryRedirect)
		case roles.IsFeeder:
			http.Redirect(w, r, "/staff", http.StatusTemporaryRedirect)
		default:
			http.Redirect(w, r, "/sign-in", http.StatusTemporaryRedirect)
		}
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
		r.Route("/bulk-upload", s.bulkUploadAdminRoutes)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			s.renderTempl(w, r, admin.AdminHome())
		})
	})

	c.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Route("/staff", s.staffRoutes)
	})

	c.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Use(s.requireNurse)
		r.Route("/nurse", s.nurseRoutes)
	})

	c.Route("/feeding", s.feedingRoutes)

	c.Get("/sign-in", s.signIn)

	s.apiRoutes(c)

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
			w.WriteHeader(http.StatusForbidden)
			s.renderTempl(w, r, templates.PermissionDenied(s.permissionDeniedParams(roles, ok)))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireFeeder(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roles, ok := r.Context().Value("roles").(Roles)
		if !ok || !roles.IsFeeder {
			w.WriteHeader(http.StatusForbidden)
			s.renderTempl(w, r, templates.PermissionDenied(s.permissionDeniedParams(roles, ok)))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireNurse(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roles, ok := r.Context().Value("roles").(Roles)
		if !ok || !roles.IsNurse {
			w.WriteHeader(http.StatusForbidden)
			s.renderTempl(w, r, templates.PermissionDenied(s.permissionDeniedParams(roles, ok)))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) permissionDeniedParams(roles Roles, ok bool) templates.PermissionDeniedParams {
	if !ok {
		return templates.PermissionDeniedParams{}
	}
	return templates.PermissionDeniedParams{
		IsAdmin:  roles.Admin,
		IsFeeder: roles.IsFeeder,
		IsNurse:  roles.IsNurse,
	}
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
		return out, fmt.Errorf("value is not of type %T, value: %v and type: %T", out, value, value)
	}

	return v, nil
}

func setMetadataValue(metadata any, key string, value any) (any, error) {
	m, ok := metadata.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("metadata is not a map")
	}

	m[key] = value
	return m, nil
}

func (s *Server) getSessionUserID(r *http.Request) (string, error) {
	session, _ := clerk.SessionFromContext(r.Context())
	if session == nil {
		return "", fmt.Errorf("session not found in context")
	}
	return session.Claims.Subject, nil
}

func (s *Server) getSessionUser(r *http.Request) (*clerk.User, error) {
	userID, err := s.getSessionUserID(r)
	if err != nil {
		return nil, err
	}

	return s.Clerk.Users().Read(userID)
}

func (s *Server) AddRolesToContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := s.getSessionUser(r)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		roles := Roles{
			Admin:    false,
			IsFeeder: false,
		}

		isAdmin, err := getMetadataValue[bool](user.PrivateMetadata, "admin")
		if err == nil {
			roles.Admin = isAdmin
		}

		feederEnrollments, err := getMetadataValue[string](user.PrivateMetadata, "feeder_enrollments")
		if err == nil {
			roles.IsFeeder = feederEnrollments != ""
		}

		nurseEnrollments, err := getMetadataValue[string](user.PrivateMetadata, "nurse_enrollments")
		if err == nil {
			roles.IsNurse = nurseEnrollments != ""
		}

		roles.IsSignedIn = true

		ctx := context.WithValue(r.Context(), "roles", roles)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// logServerErrors captures handler responses and emits a slog.Error for any
// 5xx status, including the first chunk of the response body. Handlers that
// use http.Error to surface failures tend to write the real error message
// into the body without ever calling slog themselves, so middleware.Logger
// only gives us the status line. This fills in the gap so we never have to
// guess what caused a 500 again.
func logServerErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &errorCapturingWriter{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status >= 500 {
			body := rec.body.String()
			if len(body) > 1024 {
				body = body[:1024] + "...(truncated)"
			}
			slog.Error("server error response",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"body", body,
			)
		}
	})
}

type errorCapturingWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	body        bytes.Buffer
}

func (w *errorCapturingWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *errorCapturingWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	// Only retain the body for error responses so we don't bloat memory on
	// large successful payloads.
	if w.status >= 500 && w.body.Len() < 2048 {
		w.body.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// Flush, Hijack, Push passthroughs so the wrapper doesn't break HTMX
// streaming / websocket upgrades.
func (w *errorCapturingWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
