package webapi

import (
	"fmt"
	"geevly/gen/go/eda"
	"net/http"

	studenttempl "geevly/internal/webapi/templates/admin/student"
	templates "geevly/internal/webapi/templates/admin/student"
	components "geevly/internal/webapi/templates/components"
	layouts "geevly/internal/webapi/templates/layouts"

	"github.com/Howard3/valueextractor"
	"github.com/go-chi/chi/v5"
)

func (s *Server) studentAdminRoutes(r chi.Router) {
	r.Get("/", s.adminListStudents)
	r.Get("/create", s.adminCreateStudentForm)
	r.Post("/create", s.adminCreateStudent)
	r.Get("/{studentID}", s.adminViewStudent)
	r.Post("/{studentID}", s.adminUpdateStudent)
	r.Get("/{studentID}/history", s.adminStudentHistory)
	r.Put("/{studentID}/toggleStatus", s.toggleStudentStatus)
	r.Post("/{studentID}/enroll", s.adminEnrollStudent)
	r.Delete("/{studentID}/enrollment", s.adminUnenrollStudent)
}

func (s *Server) adminViewStudent(w http.ResponseWriter, r *http.Request) {
	studentID := chi.URLParam(r, "studentID")

	student, err := s.StudentSvc.GetStudent(r.Context(), studentID)
	if err != nil {
		s.errorPage(w, r, "Error getting student", err)
		return
	}

	schools, err := s.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		s.errorPage(w, r, "Error getting schools", err)
		return
	}

	schoolsMap := make(map[string]string)
	for id, school := range schools {
		schoolsMap[fmt.Sprintf("%d", id)] = school
	}

	viewParams := studenttempl.ViewParams{
		SchoolMap: schoolsMap,
		Student:   student.GetStudent(),
		ID:        studentID,
		Version:   student.Version,
	}

	s.renderTempl(w, r, templates.AdminViewStudent(viewParams))
}

func (s *Server) adminListStudents(w http.ResponseWriter, r *http.Request) {
	page := s.pageQuery(r)
	limit := s.limitQuery(r)

	students, err := s.StudentSvc.ListStudents(r.Context(), limit, page)
	if err != nil {
		s.errorPage(w, r, "Error listing students", err)
		return
	}

	pagination := components.NewPagination(page, limit, students.Count)

	s.renderTempl(w, r, templates.StudentList(students, pagination))
}

func (s *Server) adminCreateStudentForm(w http.ResponseWriter, r *http.Request) {
	s.renderTempl(w, r, templates.CreateStudent())
}

func (s *Server) adminCreateStudent(w http.ResponseWriter, r *http.Request) {
	var dob eda.Date
	var firstName, lastName string

	ex := valueextractor.Using(&valueextractor.FormExtractor{Request: r})
	ex.With("date_of_birth", AsProtoDate(&dob))
	ex.With("first_name", valueextractor.AsString(&firstName))
	ex.With("last_name", valueextractor.AsString(&lastName))

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	student := eda.Student_Create{
		FirstName:   firstName,
		LastName:    lastName,
		DateOfBirth: &dob,
	}

	res, err := s.StudentSvc.CreateStudent(r.Context(), &student)
	if err != nil {
		// TODO: handle error on-form
		s.errorPage(w, r, "Error creating student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect("/admin/student/"+res.StudentId, "Student created"))
}

func (s *Server) adminUpdateStudent(w http.ResponseWriter, r *http.Request) {
	studentID := chi.URLParam(r, "studentID")
	var ver uint64
	var firstName, lastName string
	var dob eda.Date

	ex := valueextractor.Using(&valueextractor.FormExtractor{Request: r})
	ex.With("version", valueextractor.AsUint64(&ver))
	ex.With("first_name", valueextractor.AsString(&firstName))
	ex.With("last_name", valueextractor.AsString(&lastName))
	ex.With("date_of_birth", AsProtoDate(&dob))

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	student := eda.Student_Update{
		StudentId:   studentID,
		FirstName:   firstName,
		LastName:    lastName,
		DateOfBirth: &dob,
		Version:     ver,
	}

	res, err := s.StudentSvc.UpdateStudent(r.Context(), &student)
	if err != nil {
		s.errorPage(w, r, "Error updating student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect("/admin/student/"+res.StudentId, "Student updated"))
}

func (s *Server) toggleStudentStatus(w http.ResponseWriter, r *http.Request) {
	sID := chi.URLParam(r, "studentID")
	var ver uint64
	active := r.URL.Query().Get("active") == "true"

	ex := valueextractor.Using(&valueextractor.QueryExtractor{Query: r.URL.Query()})
	ex.With("ver", valueextractor.AsUint64(&ver))

	newStatus := eda.Student_ACTIVE
	if !active {
		newStatus = eda.Student_INACTIVE
	}

	res, err := s.StudentSvc.SetStatus(r.Context(), &eda.Student_SetStatus{
		StudentId: sID,
		Version:   ver,
		Status:    newStatus,
	})
	if err != nil {
		s.errorPage(w, r, "Error setting status", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect("/admin/student/"+res.StudentId, "Status updated"))
}

func (s *Server) adminStudentHistory(w http.ResponseWriter, r *http.Request) {
	studentID := chi.URLParam(r, "studentID")

	history, err := s.StudentSvc.GetHistory(r.Context(), studentID)
	if err != nil {
		s.errorPage(w, r, "Error getting student history", err)
		return
	}

	s.renderTempl(w, r, templates.StudentHistorySection(history))
}

func (s *Server) adminEnrollStudent(w http.ResponseWriter, r *http.Request) {
	studentID := chi.URLParam(r, "studentID")

	var dateOfEnrollment eda.Date
	var version uint64
	var schoolID string

	ex := valueextractor.Using(&valueextractor.FormExtractor{Request: r})
	ex.With("enrollment_date", AsProtoDate(&dateOfEnrollment))
	ex.With("version", valueextractor.AsUint64(&version))
	ex.With("school_id", valueextractor.AsString(&schoolID))

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	_, err := s.StudentSvc.EnrollStudent(r.Context(), &eda.Student_Enroll{
		StudentId:        studentID,
		SchoolId:         r.Form.Get("school_id"),
		DateOfEnrollment: &dateOfEnrollment,
		Version:          version,
	})
	if err != nil {
		s.errorPage(w, r, "Error enrolling student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect("/admin/student/"+studentID, "Student enrolled"))
}

func (s *Server) adminUnenrollStudent(w http.ResponseWriter, r *http.Request) {
	var version uint64
	qe := valueextractor.QueryExtractor{Query: r.URL.Query()}
	ex := valueextractor.Using(qe)
	ex.With("version", valueextractor.AsUint64(&version))

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	studentID := chi.URLParam(r, "studentID")

	_, err := s.StudentSvc.UnenrollStudent(r.Context(), &eda.Student_Unenroll{
		StudentId: studentID,
		Version:   version,
	})
	if err != nil {
		s.errorPage(w, r, "Error unenrolling student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect("/admin/student/"+studentID, "Student unenrolled"))
}
