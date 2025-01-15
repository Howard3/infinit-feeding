package webapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	_ "github.com/swaggo/swag"
)

// @title           Infinit Feeding API
// @version         1.0
// @description     API Server for Infinit Feeding application
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.infinitfeeding.com/support
// @contact.email  support@infinitfeeding.com

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:3000
// @BasePath  /api

type ListStudentsResponse struct {
	Students []StudentResponse `json:"students"`
	Total    int64             `json:"total"`
}

type StudentResponse struct {
	ID        uint   `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	SchoolID  string `json:"schoolId"`
}

func (s *Server) apiRoutes(r chi.Router) {
	r.Route("/api", func(r chi.Router) {
		r.Get("/students", s.apiListStudents)
	})
}

// @Summary     List students
// @Description Get a paginated list of students
// @Tags        students
// @Accept      json
// @Produce     json
// @Param       page  query    int  false  "Page number"     default(1)
// @Param       limit query    int  false  "Items per page"  default(10)
// @Success     200  {object}  ListStudentsResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /students [get]
func (s *Server) apiListStudents(w http.ResponseWriter, r *http.Request) {
	page := s.pageQuery(r)
	limit := s.limitQuery(r)

	students, err := s.StudentSvc.ListStudents(r.Context(), limit, page)
	if err != nil {
		http.Error(w, "Error fetching students", http.StatusInternalServerError)
		return
	}

	response := ListStudentsResponse{
		Students: make([]StudentResponse, 0, len(students.Students)),
		Total:    int64(students.Count),
	}

	for _, student := range students.Students {
		response.Students = append(response.Students, StudentResponse{
			ID:        student.ID,
			FirstName: student.FirstName,
			LastName:  student.LastName,
			SchoolID:  student.SchoolID,
		})
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		return
	}
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}
