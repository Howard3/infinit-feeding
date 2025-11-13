package webapi

import (
	"encoding/csv"
	"fmt"
	"geevly/internal/student"
	reportstempl "geevly/internal/webapi/templates/admin/reports"
	components "geevly/internal/webapi/templates/components"
	"net/http"
	"sort"
	"strconv"
	"time"

	vex "github.com/Howard3/valueextractor"
	"github.com/go-chi/chi/v5"
)

func (s *Server) adminReports(r chi.Router) {
	r.Get("/", s.reportsHome)
	r.Get("/sponsored-students", s.adminSponsoredStudentsReport)
	r.Get("/recent-feedings", s.adminRecentFeedingsReport)
	r.Get("/health-csv", s.adminHealthCSV)
	r.Get("/grades-csv", s.adminGradesCSV)
	r.Post("/export", s.exportFeedingReport)
	r.Get("/student-qr", s.studentQRLeadIn)
	r.Get("/student-qr-bulk", s.exportStudentQRBulk)
	// HTMX endpoints for QR student selection
	r.Get("/student-search", s.adminReportStudentSearch)
	r.Get("/student-chip", s.adminReportStudentChip)
	// Data completeness reports
	r.Get("/completeness/grades", s.adminGradeCompletenessCSV)
	r.Get("/completeness/health", s.adminHealthCompletenessCSV)
	r.Get("/completeness/feedings", s.adminFeedingCompletenessCSV)
}

func (s *Server) reportsHome(w http.ResponseWriter, r *http.Request) {
	schoolMap, err := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		s.errorPage(w, r, "Error fetching schools", err)
		return
	}

	schoolStrMap := make(map[string]string)
	for k, v := range schoolMap {
		schoolStrMap[fmt.Sprintf("%d", k)] = v
	}

	s.renderTempl(w, r, reportstempl.ReportsHome(schoolStrMap))
}

func (s *Server) studentQRLeadIn(w http.ResponseWriter, r *http.Request) {
	schoolMap, err := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		s.errorPage(w, r, "Error fetching schools", err)
		return
	}

	schoolStrMap := make(map[string]string)
	for k, v := range schoolMap {
		schoolStrMap[fmt.Sprintf("%d", k)] = v
	}

	s.renderTempl(w, r, reportstempl.StudentQRLeadIn(schoolStrMap))
}

func AsDate(ref *time.Time) vex.Converter {
	return func(ec *vex.Extractor, value string) error {
		// parse and require YYYY-MM-DD
		t, err := time.Parse("2006-01-02", value)
		if err != nil {
			return fmt.Errorf("invalid date format (expected YYYY-MM-DD): %w", err)
		}
		*ref = t
		return nil
	}
}

func (s *Server) adminReportStudentSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	// Limit results to a reasonable number for the typeahead
	limit := uint(30)
	page := uint(1)

	listOptions := []student.ListOption{student.ActiveOnly()}
	if q != "" {
		listOptions = append(listOptions, student.WithNameSearch(q))
	}
	if schoolID := r.URL.Query().Get("school_id"); schoolID != "" {
		if id, err := strconv.ParseUint(schoolID, 10, 64); err == nil {
			listOptions = append(listOptions, student.InSchools(id))
		}
	}

	res, err := s.Services.StudentSvc.ListStudents(r.Context(), limit, page, listOptions...)
	if err != nil {
		s.errorPage(w, r, "Error searching students", err)
		return
	}

	// Build school map to show school names in results
	schoolMap, mapErr := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
	if mapErr != nil {
		// proceed with empty map on error
		schoolMap = map[uint64]string{}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	for _, st := range res.Students {
		// Resolve school name
		schoolName := ""
		if sid, err := strconv.ParseUint(st.SchoolID, 10, 64); err == nil {
			if name, ok := schoolMap[sid]; ok {
				schoolName = name
			}
		}
		if schoolName == "" {
			schoolName = "Unknown School"
		}

		// Each result is a button that appends a chip to the selected list
		fmt.Fprintf(w, `<button type="button" class="w-full text-left px-3 py-2 hover:bg-indigo-50/60 focus:bg-indigo-50 flex items-center justify-between"
			hx-get="/admin/reports/student-chip?student_id=%d" hx-target="#selected-students" hx-swap="beforeend" hx-push-url="false">
		  <div class="min-w-0">
		    <div class="font-medium text-gray-900 truncate">%s %s</div>
		    <div class="text-xs text-gray-500 truncate">LRN %s · %s</div>
		  </div>
		  <span class="ml-3 inline-flex items-center rounded-md bg-indigo-50 px-2 py-1 text-[10px] font-medium text-indigo-700 ring-1 ring-inset ring-indigo-600/20">Add</span>
		</button>`,
			st.ID, st.FirstName, st.LastName, st.StudentID, schoolName)
	}
}

func (s *Server) adminReportStudentChip(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("student_id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("missing student_id"))
		return
	}
	students, err := s.Services.StudentSvc.FetchManyStudentProjections(r.Context(), []string{id})
	if err != nil || len(students) == 0 {
		s.errorPage(w, r, "Error loading student", err)
		return
	}
	st := students[0]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Inline chip HTML with hidden input so the form submits student_ids[]
	fmt.Fprintf(w, `<span class="inline-flex items-center gap-2 bg-indigo-50 text-indigo-700 rounded-full px-2 py-1 text-xs mr-2 mb-2" data-chip>
  <input type="hidden" name="student_ids[]" value="%d"/>
  <span>%s %s · LRN %s</span>
  <button type="button" class="text-indigo-500 hover:text-red-600" onclick="this.closest('[data-chip]').remove()" aria-label="Remove">&times;</button>
</span>`, st.ID, st.FirstName, st.LastName, st.StudentID)
}

func (s *Server) exportStudentQRBulk(w http.ResponseWriter, r *http.Request) {
	// Allow selecting explicit students via student_ids[]; when present, ignore pagination and school filter
	studentIDs := r.URL.Query()["student_ids[]"]
	if len(studentIDs) == 0 {
		// support "student_ids" as well
		studentIDs = r.URL.Query()["student_ids"]
	}
	if len(studentIDs) > 0 {
		sts, err := s.Services.StudentSvc.FetchManyStudentProjections(r.Context(), studentIDs)
		if err != nil {
			s.errorPage(w, r, "Error fetching selected students", err)
			return
		}
		// Build a simple pagination showing total count
		count := uint(len(sts))
		pagination := components.NewPagination(1, count, count)
		pagination.URL = "/admin/reports/student-qr-bulk"
		s.renderTempl(w, r, reportstempl.StudentQRBulk(sts, "Selected Students", pagination))
		return
	}

	// Pagination: default to 30 per page for a 5x6 printable grid
	page := s.pageQuery(r)
	limit := uint(30)
	if l := s.limitQuery(r); l != 15 { // respect explicit limit when provided
		limit = l
	}

	// Filters: active students, optionally filter by school
	listOptions := []student.ListOption{student.ActiveOnly()}
	schoolID := r.URL.Query().Get("school_id")
	if schoolID != "" {
		if id, err := strconv.ParseUint(schoolID, 10, 64); err == nil {
			listOptions = append(listOptions, student.InSchools(id))
		}
	}

	res, err := s.Services.StudentSvc.ListStudents(r.Context(), limit, page, listOptions...)
	if err != nil {
		s.errorPage(w, r, "Error fetching students", err)
		return
	}

	// Determine school name for title
	schoolName := "All Schools"
	if schoolID != "" {
		schoolMap, err := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
		if err != nil {
			s.errorPage(w, r, "Error fetching schools", err)
			return
		}
		if id, err := strconv.ParseUint(schoolID, 10, 64); err == nil {
			if name, ok := schoolMap[id]; ok {
				schoolName = name
			}
		}
	}

	// Build pagination
	pagination := components.NewPagination(page, limit, uint(res.Count))
	baseURL := "/admin/reports/student-qr-bulk"
	if schoolID != "" {
		baseURL = fmt.Sprintf("%s?school_id=%s", baseURL, schoolID)
	}
	pagination.URL = baseURL

	// Render the bulk QR page with pagination and title
	s.renderTempl(w, r, reportstempl.StudentQRBulk(res.Students, schoolName, pagination))
}

func (s *Server) exportFeedingReport(w http.ResponseWriter, r *http.Request) {
	ex := vex.Using(&vex.FormExtractor{Request: r})
	schoolID := vex.Result(ex, "school_id", vex.AsString)
	startDate := vex.Result(ex, "start_date", AsDate)
	endDate := vex.Result(ex, "end_date", AsDate)
	output := vex.Result(ex, "output", vex.AsString)

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	if output != "html" && output != "csv" {
		s.errorPage(w, r, "Invalid output format", fmt.Errorf("invalid output format: %s", output))
		return
	}

	studentList, err := s.Services.StudentSvc.ListForSchool(r.Context(), schoolID)
	if err != nil {
		s.errorPage(w, r, "Error fetching students", err)
		return
	}

	students, err := s.Services.StudentSvc.GetSchoolFeedingEvents(r.Context(), schoolID, startDate, endDate)
	if err != nil {
		s.errorPage(w, r, "Error fetching feeding events", err)
		return
	}

	dateColumns := make([]time.Time, 0)
	for d := startDate; d.Before(endDate); d = d.AddDate(0, 0, 1) {
		dateColumns = append(dateColumns, d)
	}

	students = s.addMissingStudentsToReport(studentList, students)

	switch output {
	case "html":
		s.renderTempl(w, r, reportstempl.FeedingReport(students, dateColumns))
	case "csv":
		s.feedingReportCSV(w, students, dateColumns)
	default:
		s.errorPage(w, r, "Invalid output format", fmt.Errorf("invalid output format: %s", output))
	}
}

// addMissingStudentsToReport ensures that all students in the studentList are included in the report,
// even if they don't have any feeding events. This function adds students from the studentList
// who are not present in the groupedByFeedingEvents to the report with empty feeding event data.
// It returns an updated slice of GroupedByStudentReturn that includes all students.
func (s *Server) addMissingStudentsToReport(studentList []*student.ProjectedStudent, groupedByFeedingEvents []*student.GroupedByStudentReturn) []*student.GroupedByStudentReturn {
	// Create a map of existing students in groupedByFeedingEvents for quick lookup
	existingStudents := make(map[string]bool)
	for _, event := range groupedByFeedingEvents {
		existingStudents[event.Student.StudentID] = true
	}

	// Add missing students to groupedByFeedingEvents
	for _, projectedStudent := range studentList {
		if _, exists := existingStudents[projectedStudent.StudentID]; !exists {
			groupedByFeedingEvents = append(groupedByFeedingEvents, &student.GroupedByStudentReturn{
				Student: *projectedStudent,
			})
		}
	}

	return groupedByFeedingEvents
}

func (s *Server) feedingReportCSV(w http.ResponseWriter, students []*student.GroupedByStudentReturn, dateColumns []time.Time) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=feeding_report_%s.csv", time.Now().Format("2006-01-02")))

	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	rows := make([][]string, 0, len(students)+2)
	// write Header
	header := []string{"Student ID", "Student Last Name"}
	for _, d := range dateColumns {
		header = append(header, d.Format("2006-01-02"))
	}
	header = append(header, "Total")
	rows = append(rows, header)
	totalFed := 0
	totalFedByDay := make(map[time.Time]int)

	for _, student := range students {
		row := make([]string, 0, len(dateColumns)+3)
		row = append(row, student.Student.StudentID, student.Student.LastName)
		daysFed := 0
		for _, d := range dateColumns {
			fedOnThisDay := student.WasFedOnDay(d)
			if fedOnThisDay {
				row = append(row, "1")
				daysFed++
				totalFedByDay[d]++
			} else {
				row = append(row, "")
			}
		}
		row = append(row, fmt.Sprintf("%d", daysFed))
		rows = append(rows, row)
		totalFed += daysFed
	}

	// write total row
	totalRow := make([]string, 0, len(header))
	totalRow = append(totalRow, "", "Total")
	for _, d := range dateColumns {
		totalRow = append(totalRow, fmt.Sprintf("%d", totalFedByDay[d]))
	}
	totalRow = append(totalRow, fmt.Sprintf("%d", totalFed))
	rows = append(rows, totalRow)

	csvWriter.WriteAll(rows)
}

func (s *Server) adminSponsoredStudentsReport(w http.ResponseWriter, r *http.Request) {
	// Get all current sponsorships
	sponsorships, err := s.Services.StudentSvc.GetAllCurrentSponsorships(r.Context())
	if err != nil {
		s.errorPage(w, r, "Error fetching sponsorships", err)
		return
	}

	// Group sponsorships by sponsor ID
	sponsorMap := make(map[string][]reportstempl.SponsoredStudent)
	for _, sp := range sponsorships {
		// Convert string ID to uint64
		studentID, err := strconv.ParseUint(sp.StudentID, 10, 64)
		if err != nil {
			s.errorPage(w, r, "Error parsing student ID", err)
			return
		}

		// Get student details
		student, err := s.Services.StudentSvc.GetStudent(r.Context(), studentID)
		if err != nil {
			s.errorPage(w, r, "Error fetching student details", err)
			return
		}

		sponsoredStudent := reportstempl.SponsoredStudent{
			StudentID:   sp.StudentID,
			StudentName: fmt.Sprintf("%s %s", student.GetStudent().FirstName, student.GetStudent().LastName),
			SponsorID:   sp.SponsorID,
			StartDate:   sp.StartDate,
			EndDate:     sp.EndDate,
		}
		sponsorMap[sp.SponsorID] = append(sponsorMap[sp.SponsorID], sponsoredStudent)
	}

	// Convert map to slice of groups
	groups := make([]reportstempl.SponsorGroup, 0, len(sponsorMap))
	for sponsorID, students := range sponsorMap {
		groups = append(groups, reportstempl.SponsorGroup{
			SponsorID: sponsorID,
			Students:  students,
		})
	}

	// Sort groups by sponsor ID for consistent display
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].SponsorID < groups[j].SponsorID
	})

	s.renderTempl(w, r, reportstempl.SponsoredStudentsReport(groups))
}

func (s *Server) adminRecentFeedingsReport(w http.ResponseWriter, r *http.Request) {
	page := s.pageQuery(r)
	limit := 20
	feedingEvents, total, err := s.Services.StudentSvc.GetRecentFeedingEvents(r.Context(), int(page), limit)
	if err != nil {
		s.errorPage(w, r, "Error fetching recent feedings", err)
		return
	}

	// Get school names
	schoolMap, err := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		s.errorPage(w, r, "Error fetching schools", err)
		return
	}

	// Convert to template format
	recentFeedings := make([]reportstempl.RecentFeeding, 0, len(feedingEvents))
	for _, event := range feedingEvents {
		schoolID, _ := strconv.ParseUint(event.SchoolID, 10, 64)
		schoolName := schoolMap[schoolID]
		recentFeedings = append(recentFeedings, reportstempl.RecentFeeding{
			StudentID:      event.StudentID,
			StudentName:    event.StudentName,
			SchoolID:       event.SchoolID,
			SchoolName:     schoolName,
			FeedingTime:    event.FeedingTime,
			FeedingImageID: event.FeedingImageID,
		})
	}

	pagination := components.NewPagination(page, uint(limit), uint(total))
	pagination.URL = "/admin/reports/recent-feedings"
	s.renderTempl(w, r, reportstempl.RecentFeedingsReport(recentFeedings, pagination))
}

// adminHealthCSV streams a CSV of height and weight assessments
func (s *Server) adminHealthCSV(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	schoolID := q.Get("school_id")
	startStr := q.Get("start_date")
	endStr := q.Get("end_date")

	var startDate, endDate time.Time
	var err error
	if startStr != "" {
		startDate, err = time.Parse("2006-01-02", startStr)
		if err != nil {
			s.errorPage(w, r, "Invalid start date", err)
			return
		}
	}
	if endStr != "" {
		endDate, err = time.Parse("2006-01-02", endStr)
		if err != nil {
			s.errorPage(w, r, "Invalid end date", err)
			return
		}
	}

	recs, err := s.Services.StudentSvc.GetHealthAssessments(r.Context(), schoolID, startDate, endDate)
	if err != nil {
		s.errorPage(w, r, "Error fetching health assessments", err)
		return
	}

	// Only include students that are actually referenced in the projections
	studentIDs := student.CollectDistinctStudentAggIDs(recs)
	sts, err := s.Services.StudentSvc.FetchManyStudentProjections(r.Context(), studentIDs)
	if err != nil {
		s.errorPage(w, r, "Error fetching students", err)
		return
	}

	studentsByAggID := make(map[string]*student.ProjectedStudent)
	for _, st := range sts {
		studentsByAggID[fmt.Sprintf("%d", st.ID)] = st
	}

	schoolMap, err := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		s.errorPage(w, r, "Error fetching schools", err)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=health_report_%s.csv", time.Now().Format("2006-01-02")))
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// header (combined Health + BMI + Nutrition)
	_ = cw.Write([]string{"Student ID", "Student LRN", "First Name", "Last Name", "School", "Assessment Date", "Height (cm)", "Weight (kg)", "BMI", "Nutritional Status"})
	for _, rec := range recs {
		sid, _ := strconv.ParseUint(rec.SchoolID, 10, 64)
		schoolName := schoolMap[sid]
		bmiStr := ""
		if rec.BMI.Valid {
			bmiStr = fmt.Sprintf("%.1f", rec.BMI.Float64)
		}
		st := studentsByAggID[rec.GetStudentAggID()]
		lrn := ""
		firstName := ""
		lastName := ""
		if st != nil {
			lrn = st.StudentID
			firstName = st.FirstName
			lastName = st.LastName
		}
		row := []string{
			rec.StudentID,
			lrn,
			firstName,
			lastName,
			schoolName,
			rec.AssessmentDate.Format("2006-01-02"),
			fmt.Sprintf("%.1f", rec.HeightCM),
			fmt.Sprintf("%.1f", rec.WeightKG),
			bmiStr,
			rec.NutritionalStatus.String,
		}
		_ = cw.Write(row)
	}
}

// adminGradesCSV streams a CSV of student grades
func (s *Server) adminGradesCSV(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	schoolID := q.Get("school_id")
	startStr := q.Get("start_date")
	endStr := q.Get("end_date")

	var startDate, endDate time.Time
	var err error
	if startStr != "" {
		startDate, err = time.Parse("2006-01-02", startStr)
		if err != nil {
			s.errorPage(w, r, "Invalid start date", err)
			return
		}
	}
	if endStr != "" {
		endDate, err = time.Parse("2006-01-02", endStr)
		if err != nil {
			s.errorPage(w, r, "Invalid end date", err)
			return
		}
	}

	recs, err := s.Services.StudentSvc.GetGrades(r.Context(), schoolID, startDate, endDate)
	if err != nil {
		s.errorPage(w, r, "Error fetching grades", err)
		return
	}

	// Build student map for enriching rows with LRN and names
	studentIDs := student.CollectDistinctStudentAggIDs(recs)
	sts, err := s.Services.StudentSvc.FetchManyStudentProjections(r.Context(), studentIDs)
	if err != nil {
		s.errorPage(w, r, "Error fetching students", err)
		return
	}

	var studentsByAggID = make(map[string]*student.ProjectedStudent, 0)
	for _, st := range sts {
		studentsByAggID[fmt.Sprintf("%d", st.ID)] = st
	}

	schoolMap, err := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		s.errorPage(w, r, "Error fetching schools", err)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=grades_report_%s.csv", time.Now().Format("2006-01-02")))
	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{"Student ID", "Student LRN", "First Name", "Last Name", "School", "Test Date", "Grade", "School Year", "Grading Period"})
	for _, rec := range recs {
		sid, _ := strconv.ParseUint(rec.SchoolID, 10, 64)
		schoolName := schoolMap[sid]
		st := studentsByAggID[rec.GetStudentAggID()]
		lrn := ""
		firstName := ""
		lastName := ""
		if st != nil {
			lrn = st.StudentID
			firstName = st.FirstName
			lastName = st.LastName
		}
		row := []string{
			rec.StudentID,
			lrn,
			firstName,
			lastName,
			schoolName,
			rec.TestDate.Format("2006-01-02"),
			fmt.Sprintf("%d", rec.Grade),
			rec.SchoolYear.String,
			rec.GradingPeriod.String,
		}
		_ = cw.Write(row)
	}
}

// adminGradeCompletenessCSV streams a CSV of grade report completeness by student and school year
func (s *Server) adminGradeCompletenessCSV(w http.ResponseWriter, r *http.Request) {
	schoolID := r.URL.Query().Get("school_id")

	data, schoolYears, err := s.Services.StudentSvc.GetGradeCompletenessReport(r.Context(), schoolID)
	if err != nil {
		s.errorPage(w, r, "Error fetching grade completeness report", err)
		return
	}

	// Get ALL students (not just those with grade reports)
	var students []*student.ProjectedStudent
	if schoolID != "" {
		students, err = s.Services.StudentSvc.ListForSchool(r.Context(), schoolID)
	} else {
		// Get all students across all schools
		res, err := s.Services.StudentSvc.ListStudents(r.Context(), 0, 1)
		if err != nil {
			s.errorPage(w, r, "Error fetching students", err)
			return
		}
		students = res.Students
	}
	if err != nil {
		s.errorPage(w, r, "Error fetching students", err)
		return
	}

	studentsByID := make(map[string]*student.ProjectedStudent)
	for _, st := range students {
		studentsByID[fmt.Sprintf("%d", st.ID)] = st
	}

	// Get school names
	schoolMap, err := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		s.errorPage(w, r, "Error fetching schools", err)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=grade_completeness_%s.csv", time.Now().Format("2006-01-02")))
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Build header
	header := []string{"Student ID", "Student LRN", "First Name", "Last Name", "School", "Grade Level", "Created Date"}
	header = append(header, schoolYears...)
	header = append(header, "Total")
	_ = cw.Write(header)

	// Write rows for ALL students
	for _, st := range students {
		studentID := fmt.Sprintf("%d", st.ID)
		yearCounts := data[studentID] // Will be nil/empty if student has no records

		lrn := st.StudentID
		firstName := st.FirstName
		lastName := st.LastName
		schoolName := ""
		gradeLevel := fmt.Sprintf("%d", st.Grade)
		createdDate := ""

		if sid, err := strconv.ParseUint(st.SchoolID, 10, 64); err == nil {
			schoolName = schoolMap[sid]
		}
		if !st.CreatedAt.IsZero() {
			createdDate = st.CreatedAt.Format("2006-01-02")
		}

		row := []string{studentID, lrn, firstName, lastName, schoolName, gradeLevel, createdDate}
		total := 0
		for _, schoolYear := range schoolYears {
			count := yearCounts[schoolYear]
			total += count
			if count > 0 {
				row = append(row, fmt.Sprintf("%d", count))
			} else {
				row = append(row, "")
			}
		}
		row = append(row, fmt.Sprintf("%d", total))
		_ = cw.Write(row)
	}
}

// adminHealthCompletenessCSV streams a CSV of health assessment completeness by student and year
func (s *Server) adminHealthCompletenessCSV(w http.ResponseWriter, r *http.Request) {
	schoolID := r.URL.Query().Get("school_id")

	data, years, err := s.Services.StudentSvc.GetHealthAssessmentCompletenessReport(r.Context(), schoolID)
	if err != nil {
		s.errorPage(w, r, "Error fetching health assessment completeness report", err)
		return
	}

	// Get ALL students (not just those with health assessments)
	var students []*student.ProjectedStudent
	if schoolID != "" {
		students, err = s.Services.StudentSvc.ListForSchool(r.Context(), schoolID)
	} else {
		// Get all students across all schools
		res, err := s.Services.StudentSvc.ListStudents(r.Context(), 0, 1)
		if err != nil {
			s.errorPage(w, r, "Error fetching students", err)
			return
		}
		students = res.Students
	}
	if err != nil {
		s.errorPage(w, r, "Error fetching students", err)
		return
	}

	studentsByID := make(map[string]*student.ProjectedStudent)
	for _, st := range students {
		studentsByID[fmt.Sprintf("%d", st.ID)] = st
	}

	// Get school names
	schoolMap, err := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		s.errorPage(w, r, "Error fetching schools", err)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=health_completeness_%s.csv", time.Now().Format("2006-01-02")))
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Build header
	header := []string{"Student ID", "Student LRN", "First Name", "Last Name", "School", "Grade Level", "Created Date"}
	for _, year := range years {
		header = append(header, fmt.Sprintf("%d", year))
	}
	header = append(header, "Total")
	_ = cw.Write(header)

	// Write rows for ALL students
	for _, st := range students {
		studentID := fmt.Sprintf("%d", st.ID)
		yearCounts := data[studentID] // Will be nil/empty if student has no records

		lrn := st.StudentID
		firstName := st.FirstName
		lastName := st.LastName
		schoolName := ""
		gradeLevel := fmt.Sprintf("%d", st.Grade)
		createdDate := ""

		if sid, err := strconv.ParseUint(st.SchoolID, 10, 64); err == nil {
			schoolName = schoolMap[sid]
		}
		if !st.CreatedAt.IsZero() {
			createdDate = st.CreatedAt.Format("2006-01-02")
		}

		row := []string{studentID, lrn, firstName, lastName, schoolName, gradeLevel, createdDate}
		total := 0
		for _, year := range years {
			count := yearCounts[year]
			total += count
			if count > 0 {
				row = append(row, fmt.Sprintf("%d", count))
			} else {
				row = append(row, "")
			}
		}
		row = append(row, fmt.Sprintf("%d", total))
		_ = cw.Write(row)
	}
}

// adminFeedingCompletenessCSV streams a CSV of feeding completeness by student and year
func (s *Server) adminFeedingCompletenessCSV(w http.ResponseWriter, r *http.Request) {
	schoolID := r.URL.Query().Get("school_id")

	data, years, err := s.Services.StudentSvc.GetFeedingCompletenessReport(r.Context(), schoolID)
	if err != nil {
		s.errorPage(w, r, "Error fetching feeding completeness report", err)
		return
	}

	// Get ALL students (not just those with feedings)
	var students []*student.ProjectedStudent
	if schoolID != "" {
		students, err = s.Services.StudentSvc.ListForSchool(r.Context(), schoolID)
	} else {
		// Get all students across all schools
		res, err := s.Services.StudentSvc.ListStudents(r.Context(), 0, 1)
		if err != nil {
			s.errorPage(w, r, "Error fetching students", err)
			return
		}
		students = res.Students
	}
	if err != nil {
		s.errorPage(w, r, "Error fetching students", err)
		return
	}

	studentsByID := make(map[string]*student.ProjectedStudent)
	for _, st := range students {
		studentsByID[fmt.Sprintf("%d", st.ID)] = st
	}

	// Get school names
	schoolMap, err := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		s.errorPage(w, r, "Error fetching schools", err)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=feeding_completeness_%s.csv", time.Now().Format("2006-01-02")))
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Build header
	header := []string{"Student ID", "Student LRN", "First Name", "Last Name", "School", "Grade Level", "Created Date"}
	for _, year := range years {
		header = append(header, fmt.Sprintf("%d", year))
	}
	header = append(header, "Total")
	_ = cw.Write(header)

	// Write rows for ALL students
	for _, st := range students {
		studentID := fmt.Sprintf("%d", st.ID)
		yearCounts := data[studentID] // Will be nil/empty if student has no records

		lrn := st.StudentID
		firstName := st.FirstName
		lastName := st.LastName
		schoolName := ""
		gradeLevel := fmt.Sprintf("%d", st.Grade)
		createdDate := ""

		if sid, err := strconv.ParseUint(st.SchoolID, 10, 64); err == nil {
			schoolName = schoolMap[sid]
		}
		if !st.CreatedAt.IsZero() {
			createdDate = st.CreatedAt.Format("2006-01-02")
		}

		row := []string{studentID, lrn, firstName, lastName, schoolName, gradeLevel, createdDate}
		total := 0
		for _, year := range years {
			count := yearCounts[year]
			total += count
			if count > 0 {
				row = append(row, fmt.Sprintf("%d", count))
			} else {
				row = append(row, "")
			}
		}
		row = append(row, fmt.Sprintf("%d", total))
		_ = cw.Write(row)
	}
}
