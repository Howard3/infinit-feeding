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
	bytes, err := s.Services.FileSvc.GetFileBytes(r.Context(), drString, id)
	if err != nil {
		s.errorPage(w, r, "Error", err)
		return
	}

	// TODO: don't assume the file is jpeg
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
	w.Write(bytes)
}

func (s *Server) studentFeedingPhoto(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ID")

	drString := eda.File_DomainReference_name[int32(eda.File_FEEDING_HISTORY)]
	bytes, err := s.Services.FileSvc.GetFileBytes(r.Context(), drString, id)
	if err != nil {
		s.errorPage(w, r, "Error", err)
		return
	}

	// TODO: don't assume the file is jpeg
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
	w.Write(bytes)
}
