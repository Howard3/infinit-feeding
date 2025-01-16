package webapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "geevly/docs" // This is where the generated swagger docs are
	"geevly/internal/school"
	"geevly/internal/student"

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

// @securityDefinitions.apikey ApiKeyAuth
// @in                         header
// @name                       X-API-Key
// @description               API Key for authentication
// @example                   your-api-key-here

// @Security                  ApiKeyAuth

// @x-extension-openapi      {"example": "value on a json format"}

type ListStudentsResponse struct {
	Students []StudentResponse `json:"students"`
	Total    int64             `json:"total"`
}

type StudentResponse struct {
	ID              uint   `json:"id"`
	FirstName       string `json:"firstName"`
	LastName        string `json:"lastName"`
	SchoolID        string `json:"schoolId"`
	ProfilePhotoURL string `json:"profilePhotoUrl,omitempty"`
}

// Add new response types
type LocationResponse struct {
	Country string   `json:"country"`
	Cities  []string `json:"cities"`
}

type ListLocationsResponse struct {
	Locations []LocationResponse `json:"locations"`
}

// Add middleware for API key authentication
func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		expectedKey := os.Getenv("API_KEY")

		if expectedKey == "" {
			log.Printf("WARNING: API_KEY environment variable not set. API endpoints will be inaccessible.")
			s.respondWithError(w, http.StatusInternalServerError, "API key not configured")
			return
		}

		if apiKey == "" {
			s.respondWithError(w, http.StatusUnauthorized, "API key required")
			return
		}

		if apiKey != expectedKey {
			s.respondWithError(w, http.StatusUnauthorized, "Invalid API key")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) apiRoutes(r chi.Router) {
	// Swagger documentation endpoint - no auth required
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// API routes with authentication
	r.Route("/api", func(r chi.Router) {
		// Apply authentication middleware to all API routes
		r.Use(s.apiKeyAuth)

		r.Get("/students", s.apiListStudents)
		r.Get("/locations", s.apiListLocations)
		r.Get("/students/by-location", s.apiListStudentsByLocation)
	})
}

// @Summary     List students
// @Description Get a paginated list of students
// @Tags        students
// @Accept      json
// @Produce     json
// @Param       page                   query    int     false  "Page number"                   default(1)
// @Param       limit                  query    int     false  "Items per page"                default(10)
// @Param       active                 query    bool    false  "Filter active only"            default(false)
// @Param       eligible_for_sponsorship query    bool    false  "Filter eligible only"         default(false)
// @Success     200      {object}  ListStudentsResponse
// @Failure     500      {object}  ErrorResponse
// @Router      /students [get]
// @Security    ApiKeyAuth
func (s *Server) apiListStudents(w http.ResponseWriter, r *http.Request) {
	page := s.pageQuery(r)
	limit := s.limitQuery(r)

	var opts []student.ListOption

	if active := r.URL.Query().Get("active"); active == "true" {
		opts = append(opts, student.ActiveOnly())
	}

	if eligible := r.URL.Query().Get("eligible_for_sponsorship"); eligible == "true" {
		opts = append(opts, student.EligibleForSponsorshipOnly())
	}

	students, err := s.StudentSvc.ListStudents(r.Context(), limit, page, opts...)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Error fetching students")
		return
	}

	response := ListStudentsResponse{
		Students: make([]StudentResponse, 0, len(students.Students)),
		Total:    int64(students.Count),
	}

	for _, student := range students.Students {
		photoURL := ""
		if student.ProfilePhotoID != "" {
			photoURL = fmt.Sprintf("/student/profile/photo/%s", student.ProfilePhotoID)
		}

		response.Students = append(response.Students, StudentResponse{
			ID:              student.ID,
			FirstName:       student.FirstName,
			LastName:        student.LastName,
			SchoolID:        student.SchoolID,
			ProfilePhotoURL: photoURL,
		})
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// @Summary     List locations
// @Description Get a list of all active school locations
// @Tags        locations
// @Accept      json
// @Produce     json
// @Success     200  {object}  ListLocationsResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /locations [get]
// @Security    ApiKeyAuth
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

// @Summary     List students by location
// @Description Get a list of students filtered by country and optionally city
// @Tags        students
// @Accept      json
// @Produce     json
// @Param       country                query    string  true   "Country name"
// @Param       city                   query    string  false  "City name"
// @Param       active                 query    bool    false  "Filter active only"            default(false)
// @Param       eligible_for_sponsorship query    bool    false  "Filter eligible only"         default(false)
// @Success     200    {object}  ListStudentsResponse
// @Failure     400    {object}  ErrorResponse
// @Failure     500    {object}  ErrorResponse
// @Router      /students/by-location [get]
// @Security    ApiKeyAuth
func (s *Server) apiListStudentsByLocation(w http.ResponseWriter, r *http.Request) {
	country := r.URL.Query().Get("country")
	if country == "" {
		s.respondWithError(w, http.StatusBadRequest, "country parameter is required")
		return
	}

	city := r.URL.Query().Get("city")

	// Get school IDs for the location
	schoolIDs, err := s.SchoolSvc.GetSchoolIDsByLocation(r.Context(), school.Location{
		Country: country,
		City:    city,
	})
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Error fetching schools for location")
		return
	}

	if len(schoolIDs) == 0 {
		// Return empty response if no schools found
		s.respondWithJSON(w, http.StatusOK, ListStudentsResponse{
			Students: []StudentResponse{},
			Total:    0,
		})
		return
	}

	// Build filter options
	var opts []student.ListOption
	opts = append(opts, student.InSchools(schoolIDs...))

	if active := r.URL.Query().Get("active"); active == "true" {
		opts = append(opts, student.ActiveOnly())
	}

	if eligible := r.URL.Query().Get("eligible_for_sponsorship"); eligible == "true" {
		opts = append(opts, student.EligibleForSponsorshipOnly())
	}

	// Use the filter system
	result, err := s.StudentSvc.ListStudents(r.Context(), 0, 0, opts...)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Error fetching students")
		return
	}

	// Convert to response format
	response := ListStudentsResponse{
		Students: make([]StudentResponse, 0, len(result.Students)),
		Total:    int64(result.Count),
	}

	for _, student := range result.Students {
		photoURL := ""
		if student.ProfilePhotoID != "" {
			photoURL = fmt.Sprintf("/student/profile/photo/%s", student.ProfilePhotoID)
		}

		response.Students = append(response.Students, StudentResponse{
			ID:              student.ID,
			FirstName:       student.FirstName,
			LastName:        student.LastName,
			SchoolID:        student.SchoolID,
			ProfilePhotoURL: photoURL,
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
