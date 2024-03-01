package webapi

import (
	"geevly/gen/go/eda"
	"net/http"
	"strconv"

	templates "geevly/internal/webapi/templates/admin/student"
	components "geevly/internal/webapi/templates/components"
	layouts "geevly/internal/webapi/templates/layouts"

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
}

func (s *Server) adminViewStudent(w http.ResponseWriter, r *http.Request) {
	studentID := chi.URLParam(r, "studentID")

	student, err := s.StudentSvc.GetStudent(r.Context(), studentID)
	if err != nil {
		s.errorPage(w, r, "Error getting student", err)
		return
	}

	s.renderTempl(w, r, templates.AdminViewStudent(studentID, student.GetStudent(), student.GetVersion()))
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
	err := r.ParseForm()
	if err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	dob, err := s.formAsDate(r, "date_of_birth")
	if err != nil {
		s.errorPage(w, r, "Error parsing date of birth", err)
		return
	}

	student := eda.Student_Create{
		FirstName:   r.Form.Get("first_name"),
		LastName:    r.Form.Get("last_name"),
		DateOfBirth: dob,
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

	ver, err := s.formAsInt64(r, "version")
	if err != nil {
		s.errorPage(w, r, "Invalid version", nil)
		return
	}

	err = r.ParseForm()
	if err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	dob, err := s.formAsDate(r, "date_of_birth")
	if err != nil {
		s.errorPage(w, r, "Error parsing date of birth", err)
		return
	}

	student := eda.Student_Update{
		StudentId:   studentID,
		FirstName:   r.Form.Get("first_name"),
		LastName:    r.Form.Get("last_name"),
		DateOfBirth: dob,
		Version:     uint64(ver),
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
	sVer := r.URL.Query().Get("ver")
	active := r.URL.Query().Get("active") == "true"

	ver, err := strconv.ParseInt(sVer, 10, 64)
	if err != nil {
		s.errorPage(w, r, "Invalid version", err)
		return
	}

	newStatus := eda.Student_ACTIVE
	if !active {
		newStatus = eda.Student_INACTIVE
	}

	res, err := s.StudentSvc.SetStatus(r.Context(), &eda.Student_SetStatus{
		StudentId: sID,
		Version:   uint64(ver),
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

	err := r.ParseForm()
	if err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	courseID := r.Form.Get("course_id")

	_, err = s.StudentSvc.EnrollStudent(r.Context(), &eda.Student_Enroll{
		StudentId: studentID,
		CourseId:  courseID,
	})
	if err != nil {
		s.errorPage(w, r, "Error enrolling student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect("/admin/student/"+studentID, "Student enrolled"))
}
