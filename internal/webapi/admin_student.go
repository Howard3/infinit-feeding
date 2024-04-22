package webapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"geevly/gen/go/eda"
	"io"
	"net/http"
	"strconv"

	studenttempl "geevly/internal/webapi/templates/admin/student"
	templates "geevly/internal/webapi/templates/admin/student"
	components "geevly/internal/webapi/templates/components"
	layouts "geevly/internal/webapi/templates/layouts"

	vex "github.com/Howard3/valueextractor"
	"github.com/go-chi/chi/v5"

	qrcode "github.com/skip2/go-qrcode"
)

type DomainReference string

const DRStudents DomainReference = "students"
const DRFeedingTemporary DomainReference = "feeding_temporary"
const DRFeedingPermanent DomainReference = "feeding"

func (s *Server) studentAdminRoutes(r chi.Router) {
	r.Get("/", s.adminListStudents)
	r.Get("/create", s.adminCreateStudentForm)
	r.Post("/create", s.adminCreateStudent)
	r.Get("/QRCode", s.adminQRCode)

	r.Group(func(r chi.Router) {
		r.Use(s.setStudentIDMiddleware)
		r.Get(`/{ID:(^\d+)}`, s.adminViewStudent)
		r.Post(`/{ID:(^\d+)}`, s.adminUpdateStudent)
		r.Get(`/{ID:(^\d+)}/history`, s.adminStudentHistory)
		r.Put(`/{ID:(^\d+)}/toggleStatus`, s.toggleStudentStatus)
		r.Post(`/{ID:(^\d+)}/enroll`, s.adminEnrollStudent)
		r.Post(`/{ID:(^\d+)}/profilePhoto`, s.adminUploadProfilePhoto)
		r.Delete(`/{ID:(^\d+)}/enrollment`, s.adminUnenrollStudent)
		r.Post(`/{ID:(^\d+)}/regenerateCode`, s.adminRegenerateCode)
	})
}

func (s *Server) setStudentIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "ID")
		uintID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			s.errorPage(w, r, "Invalid ID", err)
			return
		}

		ctx := context.WithValue(r.Context(), "studentID", uintID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) getStudentIDFromContext(ctx context.Context) uint64 {
	id, ok := ctx.Value("studentID").(uint64)
	if !ok {
		// acceptable to be an internal panic because this should not be called unless the
		// middleware was called.
		panic("No student ID in context")
	}
	return id
}

func (s *Server) adminViewStudent(w http.ResponseWriter, r *http.Request) {
	studentID := s.getStudentIDFromContext(r.Context())
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
	ex := vex.Using(&vex.FormExtractor{Request: r}, vex.WithOptionalKeys("grade_level"))
	student := eda.Student_Create{
		FirstName:       *vex.ReturnString(ex, "first_name"),
		LastName:        *vex.ReturnString(ex, "last_name"),
		DateOfBirth:     ReturnProtoDate(ex, "date_of_birth"),
		GradeLevel:      *vex.ReturnUint64(ex, "grade_level"),
		StudentSchoolId: *vex.ReturnString(ex, "student_school_id"),
		Sex:             eda.Student_Sex(eda.Student_Sex_value[*vex.ReturnString(ex, "sex")]),
	}

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	agg, err := s.StudentSvc.CreateStudent(r.Context(), &student)
	if err != nil {
		// TODO: handle error on-form
		s.errorPage(w, r, "Error creating student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/student/%s", agg.GetID()), "Student created"))
}

func (s *Server) adminUpdateStudent(w http.ResponseWriter, r *http.Request) {
	studentID := s.getStudentIDFromContext(r.Context())
	ex := vex.Using(&vex.FormExtractor{Request: r}, vex.WithOptionalKeys("student_school_id"))
	cmd := eda.Student_Update{
		FirstName:       *vex.ReturnString(ex, "first_name"),
		LastName:        *vex.ReturnString(ex, "last_name"),
		DateOfBirth:     ReturnProtoDate(ex, "date_of_birth"),
		Version:         *vex.ReturnUint64(ex, "version"),
		StudentSchoolId: *vex.ReturnString(ex, "student_school_id"),
		GradeLevel:      *vex.ReturnUint64(ex, "grade_level"),
		Sex:             eda.Student_Sex(eda.Student_Sex_value[*vex.ReturnString(ex, "sex")]),
	}

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	_, err := s.StudentSvc.RunCommand(r.Context(), studentID, &cmd)
	if err != nil {
		s.errorPage(w, r, "Error updating student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/student/%d", studentID), "Student updated"))
}

func (s *Server) toggleStudentStatus(w http.ResponseWriter, r *http.Request) {
	studentID := s.getStudentIDFromContext(r.Context())
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

	_, err := s.StudentSvc.RunCommand(r.Context(), studentID, &eda.Student_SetStatus{
		Version: *vex.ReturnUint64(ex, "ver"),
		Status:  newStatus,
	})
	if err != nil {
		s.errorPage(w, r, "Error setting status", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/student/%d", studentID), "Status updated"))
}

func (s *Server) adminStudentHistory(w http.ResponseWriter, r *http.Request) {
	studentID := s.getStudentIDFromContext(r.Context())

	history, err := s.StudentSvc.GetHistory(r.Context(), studentID)
	if err != nil {
		s.errorPage(w, r, "Error getting student history", err)
		return
	}

	s.renderTempl(w, r, templates.StudentHistorySection(history))
}

func (s *Server) adminEnrollStudent(w http.ResponseWriter, r *http.Request) {
	studentID := s.getStudentIDFromContext(r.Context())
	ex := vex.Using(&vex.FormExtractor{Request: r})
	cmd := eda.Student_Enroll{
		SchoolId:         *vex.ReturnString(ex, "school_id"),
		DateOfEnrollment: ReturnProtoDate(ex, "enrollment_date"),
		Version:          *vex.ReturnUint64(ex, "version"),
	}

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	_, err := s.StudentSvc.RunCommand(r.Context(), studentID, &cmd)
	if err != nil {
		s.errorPage(w, r, "Error enrolling student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/student/%d", studentID), "Student enrolled"))
}

func (s *Server) adminUnenrollStudent(w http.ResponseWriter, r *http.Request) {
	studentID := s.getStudentIDFromContext(r.Context())
	qe := vex.QueryExtractor{Query: r.URL.Query()}
	ex := vex.Using(qe)
	version := vex.Result(ex, "version", vex.AsUint64)

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	_, err := s.StudentSvc.RunCommand(r.Context(), studentID, &eda.Student_Unenroll{
		Version: version,
	})
	if err != nil {
		s.errorPage(w, r, "Error unenrolling student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/student/%d", studentID), "Student unenrolled"))
}

func (s *Server) adminRegenerateCode(w http.ResponseWriter, r *http.Request) {
	studentID := s.getStudentIDFromContext(r.Context())
	fe := vex.FormExtractor{Request: r}
	ex := vex.Using(&fe)
	ver := vex.Result(ex, "version", vex.AsUint64)

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	// generate a new code
	code, err := generateRandomBytes(10)
	if err != nil {
		s.errorPage(w, r, "Error generating code", err)
		return
	}

	// TODO: retry on fail
	_, err = s.StudentSvc.RunCommand(r.Context(), studentID, &eda.Student_SetLookupCode{
		CodeUniqueId: code,
		Version:      ver,
	})

	if err != nil {
		s.errorPage(w, r, "Error regenerating code", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/student/%d", studentID), "Code regenerated"))
}

// adminQRCode - render a qr code from an input value
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

// TODO: check the file size, type, etc
func (s *Server) adminUploadProfilePhoto(w http.ResponseWriter, r *http.Request) {
	studentID := s.getStudentIDFromContext(r.Context())

	r.ParseMultipartForm(10 << 20) // 10 MB
	file, _, err := r.FormFile("file")
	if err != nil {
		s.errorPage(w, r, "Error parsing file", err)
		return
	}
	defer file.Close()

	// read "version" from the form
	ver, err := strconv.ParseUint(r.FormValue("version"), 10, 64)
	if err != nil {
		s.errorPage(w, r, "Error parsing version", err)
		return
	}

	// read all the file content
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		s.errorPage(w, r, "Error reading file", err)
		return
	}

	fileID, err := s.FileSvc.CreateFile(r.Context(), fileBytes, &eda.File{
		DomainReference: eda.File_DomainReference(eda.File_DomainReference_value[string(DRStudents)]),
	})

	if err != nil {
		s.errorPage(w, r, "Error saving file", err)
		return
	}

	_, err = s.StudentSvc.RunCommand(r.Context(), studentID, &eda.Student_SetProfilePhoto{
		FileId:  fileID,
		Version: ver,
	})

	if err != nil {
		s.errorPage(w, r, "Error setting profile photo", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/student/%d", studentID), "Profile photo updated"))
}
