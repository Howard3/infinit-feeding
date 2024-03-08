package webapi

import (
	feedingtempl "geevly/internal/webapi/templates/feeding"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) feedingRoutes(r chi.Router) {
	r.Get("/", s.feed)
}

func (s *Server) feed(w http.ResponseWriter, r *http.Request) {
	s.renderTempl(w, r, feedingtempl.Index())
}
