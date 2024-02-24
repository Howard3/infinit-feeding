package webapi

import (
	"geevly/gen/go/eda"
	"geevly/internal/webapi/templates"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) studentAdminRoutes(r chi.Router) {
	r.Get("/", s.adminListStudents)
	r.Get("/create", s.adminCreateStudentForm)
	r.Post("/create", s.adminCreateStudent)
	r.Get("/{studentID}", s.adminViewStudent)
}

func (s *Server) adminViewStudent(w http.ResponseWriter, r *http.Request) {
	studentID := chi.URLParam(r, "studentID")

	student, err := s.StudentSvc.GetStudent(r.Context(), studentID)
	if err != nil {
		s.errorPage(w, r, "Error getting student", err)
		return
	}

	s.renderInlayout(w, r, templates.AdminViewStudent(studentID, student))
}

func (s *Server) adminListStudents(w http.ResponseWriter, r *http.Request) {
	page := s.pageQuery(r)
	limit := s.limitQuery(r)

	students, err := s.StudentSvc.ListStudents(r.Context(), page, limit)
	if err != nil {
		s.errorPage(w, r, "Error listing students", err)
		return
	}

	pagination := templates.NewPagination(page, limit, students.Count)

	s.renderInlayout(w, r, templates.StudentList(students, pagination))
}

func (s *Server) adminCreateStudentForm(w http.ResponseWriter, r *http.Request) {
	s.renderInlayout(w, r, templates.CreateStudent())
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

	s.renderInlayout(w, r, templates.HTMXRedirect("/admin/student/"+res.StudentId, "Student created"))
}
