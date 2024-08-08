package webapi

import (
	"fmt"
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
	schoolID := vex.Result(ex, "school_id", vex.AsUint64)
	startDate := vex.Result(ex, "start_date", AsDate)
	endDate := vex.Result(ex, "end_date", AsDate)

	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	//	w.Write([]byte(fmt.Sprintf("Exporting report for school %d from %s to %s", schoolID, startDate.String(), endDate.String())))
	_ = schoolID
	_ = startDate
	_ = endDate

	s.renderTempl(w, r, reportstempl.FeedingReport())
}
