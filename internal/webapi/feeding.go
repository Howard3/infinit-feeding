package webapi

import (
	"fmt"
	"geevly/internal/webapi/feeding"
	feedingtempl "geevly/internal/webapi/templates/feeding"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) feedingRoutes(r chi.Router) {
	r.Get("/", s.feed)
	r.Post(`/upload`, s.feedingUpload)
}

func (s *Server) feed(w http.ResponseWriter, r *http.Request) {
	s.renderTempl(w, r, feedingtempl.Index())
}

func (s *Server) feedingUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10 MB
	file, _, err := r.FormFile("file")
	if err != nil {
		s.errorPage(w, r, "Error parsing file", err)
		return
	}
	defer file.Close()

	// read all the file content
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		s.errorPage(w, r, "Error reading file", err)
		return
	}

	code, err := feeding.GetQRCode(fileBytes)
	if err != nil {
		s.errorPage(w, r, "Error reading QR code", err)
		return
	}

	student, err := s.StudentSvc.GetStudentByCode(r.Context(), code)
	if err != nil {
		s.errorPage(w, r, "Error getting student", fmt.Errorf("error getting student by code %q: %w", code, err))
		return
	}

	if !student.IsActive() {
		s.errorPage(w, r, "Student is not active", fmt.Errorf("student %q is not active", student.ID))
		return
	}

	s.renderTempl(w, r, feedingtempl.Received(student))

}
