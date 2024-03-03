package webapi

import (
	"fmt"
	"geevly/gen/go/eda"
	schooltempl "geevly/internal/webapi/templates/admin/school"
	components "geevly/internal/webapi/templates/components"
	layouts "geevly/internal/webapi/templates/layouts"
	"net/http"
	"strconv"

	"github.com/Howard3/valueextractor"
	"github.com/go-chi/chi/v5"
)

func (s *Server) schoolAdminRoutes(r chi.Router) {
	r.Get("/", s.adminListSchools)
	r.Get("/create", s.adminCreateSchoolForm)
	r.Post("/create", s.adminCreateSchool)
	r.Get("/{ID}", s.adminViewSchool)
	r.Post("/{ID}", s.adminUpdateSchool)
	r.Get("/{ID}/history", s.adminSchoolHistory)
}

func (s *Server) adminListSchools(w http.ResponseWriter, r *http.Request) {
	page := s.pageQuery(r)
	limit := s.limitQuery(r)

	schools, err := s.SchoolSvc.List(r.Context(), limit, page)
	if err != nil {
		s.errorPage(w, r, "Error listing schools", err)
		return
	}

	pagination := components.NewPagination(page, limit, schools.Count)
	s.renderTempl(w, r, schooltempl.List(schools, pagination))
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
	id := chi.URLParam(r, "ID")
	uintID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		s.errorPage(w, r, "Invalid ID", err)
		return
	}

	agg, err := s.SchoolSvc.Get(r.Context(), uintID)
	if err != nil {
		s.errorPage(w, r, "Error getting school", err)
		return
	}

	s.renderTempl(w, r, schooltempl.View(uintID, agg.GetData(), agg.GetVersion()))
}

func (s *Server) readSchoolIDFromURL(w http.ResponseWriter, r *http.Request) (uint64, error) {
	id := chi.URLParam(r, "ID")
	uintID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		s.errorPage(w, r, "Invalid ID", err)
		return 0, err
	}
	return uintID, nil
}

func (s *Server) adminUpdateSchool(w http.ResponseWriter, r *http.Request) {
	id, err := s.readSchoolIDFromURL(w, r)
	if err != nil {
		return
	}

	var version uint64
	var name, principal, contact string

	ex := valueextractor.Using(&valueextractor.FormExtractor{Request: r})
	ex.With("version", valueextractor.AsUint64(&version))
	ex.With("name", valueextractor.AsString(&name))
	ex.With("principal", valueextractor.AsString(&principal))
	ex.With("contact", valueextractor.AsString(&contact))

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	cmd := eda.School_Update{
		Id:        id,
		Name:      name,
		Principal: principal,
		Contact:   contact,
		Version:   version,
	}

	if _, err = s.SchoolSvc.Update(r.Context(), &cmd); err != nil {
		s.errorPage(w, r, "Error updating school", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/school/%d", id), "School updated"))
}

func (s *Server) adminSchoolHistory(w http.ResponseWriter, r *http.Request) {
	id, err := s.readSchoolIDFromURL(w, r)
	if err != nil {
		return
	}

	history, err := s.SchoolSvc.GetHistory(r.Context(), id)
	if err != nil {
		s.errorPage(w, r, "Error getting history", err)
		return
	}

	s.renderTempl(w, r, schooltempl.EventHistory(history))
}

func (s *Server) toggleSchoolStatus(w http.ResponseWriter, r *http.Request) {
	// ...
}
