package webapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// TODO: return error image when error
func (s *Server) studentProfilePhoto(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ID")
	// TODO: pipe reader/writer
	bytes, err := s.FileSvc.GetFileBytes(r.Context(), DomainReferenceStudents, id)
	if err != nil {
		s.errorPage(w, r, "Error", err)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
	w.Write(bytes)
}
