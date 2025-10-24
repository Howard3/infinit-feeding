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
	r.Post("/export", s.exportFeedingReport)
	r.Get("/student-qr", s.studentQRLeadIn)
	r.Get("/student-qr-bulk", s.exportStudentQRBulk)
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

func (s *Server) exportStudentQRBulk(w http.ResponseWriter, r *http.Request) {
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
