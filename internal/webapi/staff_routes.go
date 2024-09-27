package webapi

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	stafftempl "geevly/internal/webapi/templates/staff"
)

func (s *Server) staffRoutes(r chi.Router) {
	r.Get("/", s.staffHome)
	r.Get("/school/{schoolID}", s.staffSchoolStudents)
}

func (s *Server) getFeederEnrollments(r *http.Request) ([]uint64, error) {
	user, err := s.getSessionUser(r)
	if err != nil {
		return nil, err
	}
	feederEnrollments, err := getMetadataValue[string](user.PrivateMetadata, "feeder_enrollments")
	if err != nil {
		return nil, err
	}

	if len(feederEnrollments) == 0 {
		return nil, nil
	}

	feederEnrollmentsStrSlice := strings.Split(feederEnrollments, ",")
	feederEnrollmentsSlice := make([]uint64, 0)
	for _, feederEnrollment := range feederEnrollmentsStrSlice {
		schoolID, err := strconv.ParseUint(feederEnrollment, 10, 64)
		if err != nil {
			return nil, err
		}
		feederEnrollmentsSlice = append(feederEnrollmentsSlice, schoolID)
	}

	return feederEnrollmentsSlice, nil
}

func (s *Server) staffHome(w http.ResponseWriter, r *http.Request) {
	feederEnrollments, err := s.getFeederEnrollments(r)
	if err != nil {
		s.errorPage(w, r, "Error fetching feeder enrollments", err)
		return
	}

	if len(feederEnrollments) == 0 {
		s.renderTempl(w, r, stafftempl.NoSchoolAssigned())
		return
	}

	// get schools by feeder enrollments
	schools, err := s.SchoolSvc.GetSchoolsByIDs(r.Context(), feederEnrollments)
	if err != nil {
		s.errorPage(w, r, "Error fetching schools", err)
		return
	}

	// if there is only one school, redirect to the school students page
	if len(schools) == 1 {
		http.Redirect(w, r, fmt.Sprintf("/staff/school/%d", schools[0].ID), http.StatusSeeOther)
		return
	}

	s.renderTempl(w, r, stafftempl.Home(schools))
}

func (s *Server) staffSchoolStudents(w http.ResponseWriter, r *http.Request) {
	schoolID := chi.URLParam(r, "schoolID")
	schoolIDUint, err := strconv.ParseUint(schoolID, 10, 64)
	if err != nil {
		s.errorPage(w, r, "Error parsing school ID", err)
		return
	}

	// get the school name
	school, err := s.SchoolSvc.Get(r.Context(), schoolIDUint)
	if err != nil {
		s.errorPage(w, r, "Error fetching school", err)
		return
	}

	students, err := s.StudentSvc.ListForSchool(r.Context(), schoolID)
	if err != nil {
		s.errorPage(w, r, "Error fetching students", err)
		return
	}

	// get feeding events for the school
	feedingEvents, err := s.StudentSvc.GetSchoolFeedingEvents(r.Context(), schoolID, time.Now().Add(time.Hour*-12), time.Now())
	if err != nil {
		s.errorPage(w, r, "Error fetching feeding events", err)
		return
	}

	studentsWithFeedingStatus := make([]stafftempl.StudentWithFeedingStatus, 0)
	for _, student := range students {
		fedToday := false
		for _, feedingEvent := range feedingEvents {
			if feedingEvent.Student.ID == student.ID {
				fedToday = true
				break
			}
		}

		studentsWithFeedingStatus = append(studentsWithFeedingStatus, stafftempl.StudentWithFeedingStatus{
			Student:  student,
			FedToday: fedToday,
		})
	}

	// Sort students by name (last name, then first name)
	sort.Slice(studentsWithFeedingStatus, func(i, j int) bool {
		if studentsWithFeedingStatus[i].Student.LastName == studentsWithFeedingStatus[j].Student.LastName {
			return studentsWithFeedingStatus[i].Student.FirstName < studentsWithFeedingStatus[j].Student.FirstName
		}
		return studentsWithFeedingStatus[i].Student.LastName < studentsWithFeedingStatus[j].Student.LastName
	})

	s.renderTempl(w, r, stafftempl.SchoolStudents(schoolID, school, studentsWithFeedingStatus))
}
