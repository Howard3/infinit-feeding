package webapi

import (
	"encoding/json"
	"net/http"

	_ "geevly/docs" // This is where the generated swagger docs are

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger"
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
// @schemes   http

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

// Add new response types
type LocationResponse struct {
	Country string   `json:"country"`
	Cities  []string `json:"cities"`
}

type ListLocationsResponse struct {
	Locations []LocationResponse `json:"locations"`
}

func (s *Server) apiRoutes(r chi.Router) {
	// Swagger documentation endpoint
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"), // Changed from /api/swagger/doc.json
	))

	r.Route("/api", func(r chi.Router) {
		r.Get("/students", s.apiListStudents)
		r.Get("/locations", s.apiListLocations)
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
	}
}

// @Summary     List locations
// @Description Get a list of all active school locations
// @Tags        locations
// @Accept      json
// @Produce     json
// @Success     200  {object}  ListLocationsResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /locations [get]
func (s *Server) apiListLocations(w http.ResponseWriter, r *http.Request) {
	locations, err := s.SchoolSvc.ListLocations(r.Context())
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Error fetching locations")
		return
	}

	// Group locations by country
	locationMap := make(map[string][]string)
	for _, loc := range locations {
		locationMap[loc.Country] = append(locationMap[loc.Country], loc.City)
	}

	// Convert to response format
	response := ListLocationsResponse{
		Locations: make([]LocationResponse, 0, len(locationMap)),
	}

	for country, cities := range locationMap {
		response.Locations = append(response.Locations, LocationResponse{
			Country: country,
			Cities:  cities,
		})
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// Helper method for JSON responses
func (s *Server) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Error encoding response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

// Helper method for error responses
func (s *Server) respondWithError(w http.ResponseWriter, code int, message string) {
	s.respondWithJSON(w, code, ErrorResponse{Error: message})
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}
