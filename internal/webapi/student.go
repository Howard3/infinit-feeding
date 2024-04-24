package webapi

import (
	"geevly/gen/go/eda"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// TODO: return error image when error
func (s *Server) studentProfilePhoto(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ID")
	// TODO: pipe reader/writer
	drString := eda.File_DomainReference_name[int32(eda.File_STUDENT_PROFILE_PHOTO)]
	bytes, err := s.FileSvc.GetFileBytes(r.Context(), drString, id)
	if err != nil {
		s.errorPage(w, r, "Error", err)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
	w.Write(bytes)
}
