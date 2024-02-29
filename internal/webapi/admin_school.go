package webapi

import (
	"fmt"
	"geevly/gen/go/eda"
	schooltempl "geevly/internal/webapi/templates/admin/school"
	components "geevly/internal/webapi/templates/components"
	layouts "geevly/internal/webapi/templates/layouts"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) schoolAdminRoutes(r chi.Router) {
	r.Get("/", s.adminListSchools)
	r.Get("/create", s.adminCreateSchoolForm)
	r.Post("/create", s.adminCreateSchool)
	r.Get("/{studentID}", s.adminViewSchool)
	r.Post("/{studentID}", s.adminUpdateSchool)
	r.Get("/{studentID}/history", s.adminSchoolHistory)
	r.Put("/{studentID}/toggleStatus", s.toggleSchoolStatus)
}

func (s *Server) adminListSchools(w http.ResponseWriter, r *http.Request) {
	page := s.pageQuery(r)
	limit := s.limitQuery(r)

	students, err := s.StudentSvc.ListStudents(r.Context(), limit, page)
	if err != nil {
		s.errorPage(w, r, "Error listing students", err)
		return
	}

	pagination := components.NewPagination(page, limit, students.Count)
	s.renderTempl(w, r, schooltempl.List(nil, pagination))
}

func (s *Server) adminCreateSchoolForm(w http.ResponseWriter, r *http.Request) {
	s.renderTempl(w, r, schooltempl.Create())
}

func (s *Server) adminCreateSchool(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	cmd := eda.School_Create{
		Name:      r.FormValue("name"),
		Principal: r.FormValue("principal"),
		Contact:   r.FormValue("contact"),
	}

	res, err := s.SchoolSvc.Create(r.Context(), &cmd)
	if err != nil {
		// TODO: handle error on-form
		s.errorPage(w, r, "Error creating student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/school/%d", res.Id), "School created"))
}

func (s *Server) adminViewSchool(w http.ResponseWriter, r *http.Request) {
	// ...
}

func (s *Server) adminUpdateSchool(w http.ResponseWriter, r *http.Request) {
	// ...
}

func (s *Server) adminSchoolHistory(w http.ResponseWriter, r *http.Request) {
	// ...
}

func (s *Server) toggleSchoolStatus(w http.ResponseWriter, r *http.Request) {
	// ...
}
