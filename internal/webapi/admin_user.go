package webapi

import (
	"context"
	"geevly/gen/go/eda"
	"net/http"
	"strconv"

	usertempl "geevly/internal/webapi/templates/admin/user"
	components "geevly/internal/webapi/templates/components"
	layouts "geevly/internal/webapi/templates/layouts"

	vex "github.com/Howard3/valueextractor"
	"github.com/go-chi/chi/v5"
)

func (s *Server) userAdminRouter(r chi.Router) {
	r.Get("/", s.adminListUsers)
	r.Get("/create", s.adminCreateUserForm)
	r.Post(`/create`, s.adminCreateUser)

	r.Group(func(r chi.Router) {
		r.Use(s.setUserIDMiddleware)
		r.Get(`/{ID:^\d+}`, s.adminViewUser)
		r.Post(`/{ID:^\d+}`, s.adminUpdateUser)
		r.Get(`/{ID:^\d+}/history`, s.adminUserHistory)
		r.Put(`/{ID:^\d+}/toggleStatus`, s.toggleUserStatus)

	})
}

func (s *Server) setUserIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "ID")
		uintID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			s.errorPage(w, r, "Invalid ID", err)
			return
		}

		ctx := context.WithValue(r.Context(), "userID", uintID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) getUserIDFromContext(ctx context.Context) uint64 {
	id, ok := ctx.Value("userID").(uint64)
	if !ok {
		// acceptable to be an internal panic because this should not be called unless the
		// middleware was called.
		panic("No user ID in context")
	}
	return id
}

func (s *Server) adminViewUser(w http.ResponseWriter, r *http.Request) {
	agg, err := s.UserSvc.Get(r.Context(), s.getUserIDFromContext(r.Context()))
	if err != nil {
		s.errorPage(w, r, "Error getting user", err)
		return

	}

	s.renderTempl(w, r, usertempl.View(agg.GetIDUint64(), agg.GetData(), agg.GetVersion()))
}

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) {
	page := s.pageQuery(r)
	limit := s.limitQuery(r)

	students, err := s.UserSvc.List(r.Context(), limit, page)
	if err != nil {
		s.errorPage(w, r, "Error listing", err)
		return
	}

	pagination := components.NewPagination(page, limit, students.Count)

	s.renderTempl(w, r, usertempl.List(students, pagination))
}

func (s *Server) adminCreateUserForm(w http.ResponseWriter, r *http.Request) {
	s.renderTempl(w, r, usertempl.Create())

}

func (s *Server) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	ex := vex.Using(&vex.FormExtractor{Request: r})
	cmd := eda.User_Create{
		FirstName: *vex.ReturnString(ex, "first_name"),
		LastName:  *vex.ReturnString(ex, "last_name"),
		Email:     *vex.ReturnString(ex, "email"),
	}

	// TODO: generate password

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	user, err := s.UserSvc.CreateUser(r.Context(), &cmd)
	if err != nil {
		// TODO: handle error on-form
		s.errorPage(w, r, "Error creating user", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect("/admin/user/"+user.GetID(), "User created"))
}

func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r.Context())
	ex := vex.Using(&vex.FormExtractor{Request: r})
	cmd := eda.User_Update{
		FirstName: *vex.ReturnString(ex, "first_name"),
		LastName:  *vex.ReturnString(ex, "last_name"),
		Email:     *vex.ReturnString(ex, "email"),
		Version:   *vex.ReturnUint64(ex, "version"),
	}

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	user, err := s.UserSvc.RunCommand(r.Context(), userID, &cmd)
	if err != nil {
		s.errorPage(w, r, "Error updating user", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect("/admin/user/"+user.GetID(), "User updated"))
}

func (s *Server) toggleUserStatus(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r.Context())
	ex := vex.Using(&vex.FormExtractor{Request: r})
	cmd := eda.User_SetActiveState{
		Active:  *vex.ReturnBool(ex, "active"),
		Version: *vex.ReturnUint64(ex, "version"),
	}

	user, err := s.UserSvc.RunCommand(r.Context(), userID, &cmd)
	if err != nil {
		s.errorPage(w, r, "Error setting status", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect("/admin/user/"+user.GetID(), "Status updated"))
}

func (s *Server) adminUserHistory(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r.Context())

	history, err := s.UserSvc.GetHistory(r.Context(), userID)
	if err != nil {
		s.errorPage(w, r, "Error getting history", err)
		return
	}

	// s.renderTempl(w, r, templates.UserHistorySection(history))
	// TODO:
	_ = history
}
