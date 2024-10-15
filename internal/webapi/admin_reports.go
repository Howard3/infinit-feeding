package webapi

import (
	"encoding/csv"
	"fmt"
	"geevly/internal/student"
	reportstempl "geevly/internal/webapi/templates/admin/reports"
	"net/http"
	"time"

	vex "github.com/Howard3/valueextractor"
	"github.com/go-chi/chi/v5"
)

func (s *Server) adminReports(r chi.Router) {
	r.Get("/", s.reportsHome)
	r.Post("/export", s.exportReport)
}

func (s *Server) reportsHome(w http.ResponseWriter, r *http.Request) {
	schoolMap, err := s.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		s.errorPage(w, r, "Error fetching schools", err)
		return
	}

	schoolStrMap := make(map[string]string)
	for k, v := range schoolMap {
		schoolStrMap[fmt.Sprintf("%d", k)] = v
	}

	s.renderTempl(w, r, reportstempl.Home(schoolStrMap))
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

func (s *Server) exportReport(w http.ResponseWriter, r *http.Request) {
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

	studentList, err := s.StudentSvc.ListForSchool(r.Context(), schoolID)
	if err != nil {
		s.errorPage(w, r, "Error fetching students", err)
		return
	}

	students, err := s.StudentSvc.GetSchoolFeedingEvents(r.Context(), schoolID, startDate, endDate)
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
