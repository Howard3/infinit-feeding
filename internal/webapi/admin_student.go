package webapi

import (
	"encoding/base64"
	"fmt"
	"geevly/gen/go/eda"
	"net/http"

	studenttempl "geevly/internal/webapi/templates/admin/student"
	templates "geevly/internal/webapi/templates/admin/student"
	components "geevly/internal/webapi/templates/components"
	layouts "geevly/internal/webapi/templates/layouts"

	vex "github.com/Howard3/valueextractor"
	"github.com/go-chi/chi/v5"

	qrcode "github.com/skip2/go-qrcode"
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
	r.Post("/{studentID}/regenerateCode", s.adminRegenerateCode)
	r.Get("/QRCode", s.adminQRCode)
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
	ex := vex.Using(&vex.FormExtractor{Request: r})
	student := eda.Student_Create{
		FirstName:       *vex.ReturnString(ex, "first_name"),
		LastName:        *vex.ReturnString(ex, "last_name"),
		DateOfBirth:     ReturnProtoDate(ex, "date_of_birth"),
		StudentSchoolId: *vex.ReturnString(ex, "student_school_id"),
	}

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
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
	ex := vex.Using(&vex.FormExtractor{Request: r})
	student := eda.Student_Update{
		StudentId:       studentID,
		FirstName:       *vex.ReturnString(ex, "first_name"),
		LastName:        *vex.ReturnString(ex, "last_name"),
		DateOfBirth:     ReturnProtoDate(ex, "date_of_birth"),
		Version:         *vex.ReturnUint64(ex, "version"),
		StudentSchoolId: *vex.ReturnString(ex, "student_school_id"),
	}

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
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
	ex := vex.Using(&vex.QueryExtractor{Query: r.URL.Query()})
	var newStatus eda.Student_Status
	switch *vex.ReturnString(ex, "active") {
	case "true":
		newStatus = eda.Student_ACTIVE
	case "false":
		newStatus = eda.Student_INACTIVE
	default:
		s.errorPage(w, r, "Error parsing form", fmt.Errorf("Invalid status"))
		return
	}

	res, err := s.StudentSvc.SetStatus(r.Context(), &eda.Student_SetStatus{
		StudentId: sID,
		Version:   *vex.ReturnUint64(ex, "ver"),
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

	ex := vex.Using(&vex.FormExtractor{Request: r})
	cmd := eda.Student_Enroll{
		StudentId:        studentID,
		SchoolId:         *vex.ReturnString(ex, "school_id"),
		DateOfEnrollment: ReturnProtoDate(ex, "enrollment_date"),
		Version:          *vex.ReturnUint64(ex, "version"),
	}

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	_, err := s.StudentSvc.EnrollStudent(r.Context(), &cmd)
	if err != nil {
		s.errorPage(w, r, "Error enrolling student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect("/admin/student/"+studentID, "Student enrolled"))
}

func (s *Server) adminUnenrollStudent(w http.ResponseWriter, r *http.Request) {
	qe := vex.QueryExtractor{Query: r.URL.Query()}
	ex := vex.Using(qe)
	version := vex.Result(ex, "version", vex.AsUint64)

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
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

func (s *Server) adminRegenerateCode(w http.ResponseWriter, r *http.Request) {
	studentID := chi.URLParam(r, "studentID")
	fe := vex.FormExtractor{Request: r}
	ex := vex.Using(&fe)
	ver := vex.Result(ex, "version", vex.AsUint64)

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	_, err := s.StudentSvc.GenerateCode(r.Context(), &eda.Student_GenerateCode{
		StudentId: studentID,
		Version:   ver,
	})

	if err != nil {
		s.errorPage(w, r, "Error regenerating code", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect("/admin/student/"+studentID, "Code regenerated"))
}

func (s *Server) adminQRCode(w http.ResponseWriter, r *http.Request) {
	qe := vex.QueryExtractor{Query: r.URL.Query()}
	ex := vex.Using(&qe)
	data := vex.Result(ex, "data", vex.AsString)

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(data)
	if err != nil {
		s.errorPage(w, r, "Error decoding data", err)
		return
	}

	png, err := qrcode.Encode(string(decoded), qrcode.Highest, 256)
	if err != nil {
		s.errorPage(w, r, "Error generating QR code", err)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	if _, err := w.Write(png); err != nil {
		s.errorPage(w, r, "Error writing QR code", err)
	}
}
