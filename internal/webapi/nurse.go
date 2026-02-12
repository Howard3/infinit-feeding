package webapi

import (
	"fmt"
	"geevly/gen/go/eda"
	nursetempl "geevly/internal/webapi/templates/nurse"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Server) nurseRoutes(r chi.Router) {
	r.Get("/", s.nurseHome)
	r.Get("/school/{schoolID}", s.nurseSchoolStudents)
	r.Get("/school/{schoolID}/student/{studentID}", s.nurseStudentAssessment)
	r.Get("/scan", s.nurseScan)
	r.Get("/code/{code}", s.nurseConfirmCode)
	r.Get("/studentBy/studentSchoolID", s.nurseConfirmStudentByLRN)
	r.Post("/record", s.nurseRecordAssessment)
	r.Post("/record/undo", s.nurseUndoAssessment)
}

func (s *Server) getNurseEnrollments(r *http.Request) ([]uint64, error) {
	user, err := s.getSessionUser(r)
	if err != nil {
		return nil, err
	}
	nurseEnrollments, err := getMetadataValue[string](user.PrivateMetadata, "nurse_enrollments")
	if err != nil {
		return nil, err
	}

	if len(nurseEnrollments) == 0 {
		return nil, nil
	}

	nurseEnrollmentsStrSlice := strings.Split(nurseEnrollments, ",")
	nurseEnrollmentsSlice := make([]uint64, 0)
	for _, enrollment := range nurseEnrollmentsStrSlice {
		schoolID, err := strconv.ParseUint(enrollment, 10, 64)
		if err != nil {
			return nil, err
		}
		nurseEnrollmentsSlice = append(nurseEnrollmentsSlice, schoolID)
	}

	return nurseEnrollmentsSlice, nil
}

// isNurseEnrolledInSchool checks whether the nurse is enrolled in the given school
func (s *Server) isNurseEnrolledInSchool(r *http.Request, schoolID uint64) (bool, error) {
	enrollments, err := s.getNurseEnrollments(r)
	if err != nil {
		return false, err
	}
	return slices.Contains(enrollments, schoolID), nil
}

func (s *Server) nurseHome(w http.ResponseWriter, r *http.Request) {
	enrollments, err := s.getNurseEnrollments(r)
	if err != nil {
		s.errorPage(w, r, "Error fetching nurse enrollments", err)
		return
	}

	if len(enrollments) == 0 {
		s.renderTempl(w, r, nursetempl.NoSchoolAssigned())
		return
	}

	schools, err := s.Services.SchoolSvc.GetSchoolsByIDs(r.Context(), enrollments)
	if err != nil {
		s.errorPage(w, r, "Error fetching schools", err)
		return
	}

	s.renderTempl(w, r, nursetempl.Index(schools))
}

func (s *Server) nurseSchoolStudents(w http.ResponseWriter, r *http.Request) {
	schoolID := chi.URLParam(r, "schoolID")
	schoolIDUint, err := strconv.ParseUint(schoolID, 10, 64)
	if err != nil {
		s.errorPage(w, r, "Error parsing school ID", err)
		return
	}

	enrolled, err := s.isNurseEnrolledInSchool(r, schoolIDUint)
	if err != nil {
		s.errorPage(w, r, "Error checking enrollment", err)
		return
	}
	if !enrolled {
		s.errorPage(w, r, "Access denied", fmt.Errorf("you are not enrolled as a nurse in this school"))
		return
	}

	school, err := s.Services.SchoolSvc.Get(r.Context(), schoolIDUint)
	if err != nil {
		s.errorPage(w, r, "Error fetching school", err)
		return
	}

	students, err := s.Services.StudentSvc.ListForSchool(r.Context(), schoolID)
	if err != nil {
		s.errorPage(w, r, "Error fetching students", err)
		return
	}

	// Get health assessments for the current quarter
	quarterStart := getQuarterStart(time.Now())
	healthAssessments, err := s.Services.StudentSvc.GetHealthAssessments(r.Context(), schoolID, quarterStart, time.Time{})
	if err != nil {
		s.errorPage(w, r, "Error fetching health assessments", err)
		return
	}

	// Build a set of student IDs that have recent assessments
	assessedStudents := make(map[string]time.Time)
	for _, ha := range healthAssessments {
		if existing, ok := assessedStudents[ha.StudentID]; !ok || ha.AssessmentDate.After(existing) {
			assessedStudents[ha.StudentID] = ha.AssessmentDate
		}
	}

	studentsWithHealth := make([]nursetempl.StudentWithHealthStatus, 0, len(students))
	for _, s := range students {
		if !s.Active {
			continue
		}
		entry := nursetempl.StudentWithHealthStatus{
			Student: s,
		}
		if assessDate, ok := assessedStudents[fmt.Sprintf("%d", s.ID)]; ok {
			entry.HasRecentAssessment = true
			entry.LastAssessmentDate = assessDate.Format("Jan 2, 2006")
		}
		studentsWithHealth = append(studentsWithHealth, entry)
	}

	s.renderTempl(w, r, nursetempl.StudentList(schoolID, school, studentsWithHealth))
}

func (s *Server) nurseStudentAssessment(w http.ResponseWriter, r *http.Request) {
	schoolID := chi.URLParam(r, "schoolID")
	schoolIDUint, err := strconv.ParseUint(schoolID, 10, 64)
	if err != nil {
		s.errorPage(w, r, "Error parsing school ID", err)
		return
	}

	enrolled, err := s.isNurseEnrolledInSchool(r, schoolIDUint)
	if err != nil {
		s.errorPage(w, r, "Error checking enrollment", err)
		return
	}
	if !enrolled {
		s.errorPage(w, r, "Access denied", fmt.Errorf("you are not enrolled as a nurse in this school"))
		return
	}

	studentID := chi.URLParam(r, "studentID")
	studentIDUint, err := strconv.ParseUint(studentID, 10, 64)
	if err != nil {
		s.errorPage(w, r, "Error parsing student ID", err)
		return
	}

	studentAgg, err := s.Services.StudentSvc.GetStudent(r.Context(), studentIDUint)
	if err != nil {
		s.errorPage(w, r, "Error fetching student", err)
		return
	}

	if !studentAgg.IsActive() {
		s.errorPage(w, r, "Student is not active", fmt.Errorf("student %q is not active", studentAgg.GetID()))
		return
	}

	returnTo := r.URL.Query().Get("return_to")
	s.renderTempl(w, r, nursetempl.AssessmentForm(studentAgg, returnTo))
}

func (s *Server) nurseScan(w http.ResponseWriter, r *http.Request) {
	s.renderTempl(w, r, nursetempl.Scan())
}

func (s *Server) nurseConfirmCode(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	studentAgg, err := s.Services.StudentSvc.GetStudentByCode(r.Context(), []byte(code))
	if err != nil {
		s.errorPage(w, r, "Error getting student", fmt.Errorf("error getting student by code %q: %w", code, err))
		return
	}

	if !studentAgg.IsActive() {
		s.errorPage(w, r, "Student is not active", fmt.Errorf("student %q is not active", studentAgg.GetID()))
		return
	}

	s.renderTempl(w, r, nursetempl.AssessmentForm(studentAgg, ""))
}

func (s *Server) nurseConfirmStudentByLRN(w http.ResponseWriter, r *http.Request) {
	lrn := r.URL.Query().Get("student_school_id")
	studentAgg, err := s.Services.StudentSvc.GetStudentByStudentSchoolID(r.Context(), lrn)
	if err != nil {
		s.errorPage(w, r, "Error getting student", fmt.Errorf("error getting student by LRN %q: %w", lrn, err))
		return
	}

	if !studentAgg.IsActive() {
		s.errorPage(w, r, "Student is not active", fmt.Errorf("student %q is not active", studentAgg.GetID()))
		return
	}

	s.renderTempl(w, r, nursetempl.AssessmentForm(studentAgg, ""))
}

func (s *Server) nurseRecordAssessment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	studentIDStr := r.FormValue("student_id")
	heightStr := r.FormValue("height_cm")
	weightStr := r.FormValue("weight_kg")
	returnTo := r.FormValue("return_to")

	studentID, err := strconv.ParseUint(studentIDStr, 10, 64)
	if err != nil {
		s.errorPage(w, r, "Error parsing student ID", err)
		return
	}

	heightCm, err := strconv.ParseFloat(heightStr, 32)
	if err != nil {
		s.errorPage(w, r, "Error parsing height", err)
		return
	}
	if heightCm < 50 || heightCm > 250 {
		s.errorPage(w, r, "Invalid height", fmt.Errorf("height must be between 50 and 250 cm, got %.1f", heightCm))
		return
	}

	weightKg, err := strconv.ParseFloat(weightStr, 32)
	if err != nil {
		s.errorPage(w, r, "Error parsing weight", err)
		return
	}
	if weightKg < 5 || weightKg > 200 {
		s.errorPage(w, r, "Invalid weight", fmt.Errorf("weight must be between 5 and 200 kg, got %.1f", weightKg))
		return
	}

	bulkUploadID := "nurse-" + uuid.New().String()

	assessment := &eda.Student_HealthAssessment{
		HeightCm:                float32(heightCm),
		WeightKg:                float32(weightKg),
		AssessmentDate:          timestamppb.Now(),
		AssociatedBulkUploadId:  bulkUploadID,
	}

	err = s.Services.StudentSvc.AddHealthAssessment(r.Context(), studentID, assessment)
	if err != nil {
		s.errorPage(w, r, "Error recording health assessment", err)
		return
	}

	// Load student for the success page
	studentAgg, err := s.Services.StudentSvc.GetStudent(r.Context(), studentID)
	if err != nil {
		s.errorPage(w, r, "Error fetching student", err)
		return
	}

	s.renderTempl(w, r, nursetempl.Success(studentAgg, float32(heightCm), float32(weightKg), bulkUploadID, returnTo))
}

func (s *Server) nurseUndoAssessment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	studentIDStr := r.FormValue("student_id")
	bulkUploadID := r.FormValue("bulk_upload_id")

	studentID, err := strconv.ParseUint(studentIDStr, 10, 64)
	if err != nil {
		s.errorPage(w, r, "Error parsing student ID", err)
		return
	}

	err = s.Services.StudentSvc.RemoveHealthAssessment(r.Context(), studentID, bulkUploadID)
	if err != nil {
		s.errorPage(w, r, "Error undoing assessment", err)
		return
	}

	http.Redirect(w, r, "/nurse", http.StatusSeeOther)
}

// getQuarterStart returns the start of the current quarter
func getQuarterStart(t time.Time) time.Time {
	month := t.Month()
	var quarterStartMonth time.Month
	switch {
	case month <= 3:
		quarterStartMonth = time.January
	case month <= 6:
		quarterStartMonth = time.April
	case month <= 9:
		quarterStartMonth = time.July
	default:
		quarterStartMonth = time.October
	}
	return time.Date(t.Year(), quarterStartMonth, 1, 0, 0, 0, 0, t.Location())
}
