package webapi

import (
	"fmt"
	"geevly/gen/go/eda"
	schooltempl "geevly/internal/webapi/templates/admin/school"
	components "geevly/internal/webapi/templates/components"
	layouts "geevly/internal/webapi/templates/layouts"
	"net/http"
	"sort"
	"strconv"

	vex "github.com/Howard3/valueextractor"
	"github.com/go-chi/chi/v5"
)

func (s *Server) schoolAdminRoutes(r chi.Router) {
	r.Get("/", s.adminListSchools)
	r.Get("/create", s.adminCreateSchoolForm)
	r.Post("/create", s.adminCreateSchool)
	r.Get("/{ID}", s.adminViewSchool)
	r.Post("/{ID}", s.adminUpdateSchool)
	r.Get("/{ID}/history", s.adminSchoolHistory)
	r.Get("/{ID}/period", s.adminSchoolPeriodForm)
	r.Post("/{ID}/period", s.adminSetSchoolPeriod)
	r.Get("/locations", s.getSchoolLocations)
}

func (s *Server) adminListSchools(w http.ResponseWriter, r *http.Request) {
	page := s.pageQuery(r)
	limit := s.limitQuery(r)

	schools, err := s.Services.SchoolSvc.List(r.Context(), limit, page)
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
		Country:   r.FormValue("country"),
		City:      r.FormValue("city"),
	}

	res, err := s.Services.SchoolSvc.Create(r.Context(), &cmd)
	if err != nil {
		// TODO: handle error on-form
		s.errorPage(w, r, "Error creating student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/school/%d", res.Id), "School created"))
}

func (s *Server) adminViewSchool(w http.ResponseWriter, r *http.Request) {
	id, err := s.readSchoolIDFromURL(w, r)
	if err != nil {
		s.errorPage(w, r, "Invalid ID", err)
		return
	}

	agg, err := s.Services.SchoolSvc.Get(r.Context(), id)
	if err != nil {
		s.errorPage(w, r, "Error getting school", err)
		return
	}

	s.renderTempl(w, r, schooltempl.View(id, agg.GetData(), agg.GetVersion()))
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

	ex := vex.Using(&vex.FormExtractor{Request: r})
	version := vex.Result(ex, "version", vex.AsUint64)
	name := vex.Result(ex, "name", vex.AsString)
	principal := vex.Result(ex, "principal", vex.AsString)
	contact := vex.Result(ex, "contact", vex.AsString)
	country := vex.Result(ex, "country", vex.AsString)
	city := vex.Result(ex, "city", vex.AsString)

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	cmd := eda.School_Update{
		Id:        id,
		Name:      name,
		Principal: principal,
		Contact:   contact,
		Version:   version,
		Country:   country,
		City:      city,
	}

	if _, err = s.Services.SchoolSvc.Update(r.Context(), &cmd); err != nil {
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

	history, err := s.Services.SchoolSvc.GetHistory(r.Context(), id)
	if err != nil {
		s.errorPage(w, r, "Error getting history", err)
		return
	}

	s.renderTempl(w, r, schooltempl.EventHistory(history))
}

func (s *Server) toggleSchoolStatus(w http.ResponseWriter, r *http.Request) {
	// ...
}

func (s *Server) getSchoolLocations(w http.ResponseWriter, r *http.Request) {
	locations, err := s.Services.SchoolSvc.ListLocations(r.Context())
	if err != nil {
		s.errorPage(w, r, "Error getting locations", err)
		return
	}

	// Group locations by country
	locationMap := make(map[string][]string)
	for _, loc := range locations {
		locationMap[loc.Country] = append(locationMap[loc.Country], loc.City)
	}

	// Convert to format needed by template
	countries := make([]string, 0, len(locationMap))
	for country := range locationMap {
		countries = append(countries, country)
	}
	sort.Strings(countries)

	// If no locations exist yet, provide empty selection option
	if len(countries) == 0 {
		countries = append(countries, "")
		locationMap[""] = []string{""}
	}

	// TODO: Return the data in appropriate format (JSON/Template)
	// You can now use countries and locationMap[country] in your templates
}

func (s *Server) adminSchoolPeriodForm(w http.ResponseWriter, r *http.Request) {
	id, err := s.readSchoolIDFromURL(w, r)
	if err != nil {
		return
	}

	agg, err := s.Services.SchoolSvc.Get(r.Context(), id)
	if err != nil {
		s.errorPage(w, r, "Error getting school", err)
		return
	}

	s.renderTempl(w, r, schooltempl.SetPeriod(id, agg.GetData(), agg.GetVersion()))
}

func (s *Server) adminSetSchoolPeriod(w http.ResponseWriter, r *http.Request) {
	id, err := s.readSchoolIDFromURL(w, r)
	if err != nil {
		return
	}

	ex := vex.Using(&vex.FormExtractor{Request: r})
	version := vex.Result(ex, "version", vex.AsUint64)
	startMonth := uint32(vex.Result(ex, "school_start_month", vex.AsUint64))
	startDay := uint32(vex.Result(ex, "school_start_day", vex.AsUint64))
	endMonth := uint32(vex.Result(ex, "school_end_month", vex.AsUint64))
	endDay := uint32(vex.Result(ex, "school_end_day", vex.AsUint64))

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	cmd := eda.School_SetSchoolPeriod{
		Id:      id,
		Version: version,
		SchoolStart: &eda.School_MonthDay{
			Month: startMonth,
			Day:   startDay,
		},
		SchoolEnd: &eda.School_MonthDay{
			Month: endMonth,
			Day:   endDay,
		},
	}

	if _, err = s.Services.SchoolSvc.SetSchoolPeriod(r.Context(), &cmd); err != nil {
		s.errorPage(w, r, "Error setting school period", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/school/%d", id), "School period updated"))
}
