package webapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "geevly/docs" // This is where the generated swagger docs are
	"geevly/gen/go/eda"
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
	DateOfBirth     string `json:"dateOfBirth,omitempty"`
	Grade           string `json:"grade,omitempty"`
}

// Add new response types
type LocationResponse struct {
	Country string   `json:"country"`
	Cities  []string `json:"cities"`
}

type ListLocationsResponse struct {
	Locations []LocationResponse `json:"locations"`
}

// Add new response types after the existing response types
type SchoolResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
	City    string `json:"city"`
}

type ListSchoolsResponse struct {
	Schools []SchoolResponse `json:"schools"`
}

// Add this new response type after the other response types
type GetStudentResponse struct {
	ID              uint   `json:"id"`
	FirstName       string `json:"firstName"`
	LastName        string `json:"lastName"`
	SchoolID        string `json:"schoolId"`
	ProfilePhotoURL string `json:"profilePhotoUrl,omitempty"`
	DateOfBirth     string `json:"dateOfBirth,omitempty"`
	Grade           string `json:"grade,omitempty"`
	Active          bool   `json:"active"`
}

// Add this new request type after the other request types
type SponsorStudentRequest struct {
	SponsorID     string  `json:"sponsorId"`
	StartDate     string  `json:"startDate"`
	EndDate       string  `json:"endDate"`
	PaymentID     string  `json:"paymentId"`
	PaymentAmount float64 `json:"paymentAmount"`
}

// Add this new response type after the other response types
type SponsorStudentResponse struct {
	Success bool `json:"success"`
}

// Add this new response type
type SponsorshipResponse struct {
	StudentID     string  `json:"studentId"`
	StartDate     string  `json:"startDate"`
	EndDate       string  `json:"endDate"`
	PaymentID     string  `json:"paymentId"`
	PaymentAmount float64 `json:"paymentAmount"`
}

// Add this new response type after the other response types
type SponsorImpactResponse struct {
	TotalMealCount int64 `json:"totalMealCount"`
	// We can add more impact metrics here later
}

// Add this new response type
type SponsorFeedingEventResponse struct {
	StudentID   string `json:"studentId"`
	StudentName string `json:"studentName"`
	FeedingTime string `json:"feedingTime"`
	SchoolID    string `json:"schoolId"`
	EventType   string `json:"eventType"`
}

type ListSponsorFeedingEventsResponse struct {
	Events []SponsorFeedingEventResponse `json:"events"`
	Total  int64                         `json:"total"`
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
	// Swagger documentation endpoint - no auth required, only available in dev
	if os.Getenv("GO_ENV") == "development" {
		r.Get("/swagger/*", httpSwagger.Handler(
			httpSwagger.URL("/swagger/doc.json"),
		))
	}

	// API routes with authentication
	r.Route("/api", func(r chi.Router) {
		// Apply authentication middleware to all API routes
		r.Use(s.apiKeyAuth)

		r.Get("/students", s.apiListStudents)
		r.Get("/locations", s.apiListLocations)
		r.Get("/schools", s.apiListSchools)
		r.Get("/students/{id}", s.apiGetStudent)
		r.Post("/students/{id}/sponsor", s.apiSponsorStudent)
		r.Get("/sponsors/{id}/students", s.apiListSponsoredStudents)
		r.Get("/sponsors/{id}/impact", s.apiGetSponsorImpact)
		r.Get("/sponsors/{id}/events", s.apiListSponsorFeedingEvents)
	})
}

// @Summary     List students
// @Description Get a paginated list of students with optional filters
// @Tags        students
// @Accept      json
// @Produce     json
// @Param       page                   query    int     false  "Page number"                   default(1)
// @Param       limit                  query    int     false  "Items per page"                default(10)
// @Param       active                 query    bool    false  "Filter active only"            default(false)
// @Param       eligible_for_sponsorship query    bool    false  "Filter eligible only"         default(false)
// @Param       min_age                query    int     false  "Minimum age filter"
// @Param       max_age                query    int     false  "Maximum age filter"
// @Param       country                query    string  false  "Filter by country"
// @Param       city                   query    string  false  "Filter by city"
// @Success     200      {object}  ListStudentsResponse
// @Failure     400      {object}  ErrorResponse
// @Failure     500      {object}  ErrorResponse
// @Router      /students [get]
// @Security    ApiKeyAuth
func (s *Server) apiListStudents(w http.ResponseWriter, r *http.Request) {
	page := s.pageQuery(r)
	limit := s.limitQuery(r)

	var opts []student.ListOption

	// Location filtering
	country := r.URL.Query().Get("country")
	city := r.URL.Query().Get("city")
	if country != "" {
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
		opts = append(opts, student.InSchools(schoolIDs...))
	}

	// Other filters
	if active := r.URL.Query().Get("active"); active == "true" {
		opts = append(opts, student.ActiveOnly())
	}

	if eligible := r.URL.Query().Get("eligible_for_sponsorship"); eligible == "true" {
		opts = append(opts, student.EligibleForSponsorshipOnly())
	}

	if minAgeStr := r.URL.Query().Get("min_age"); minAgeStr != "" {
		minAge, err := strconv.Atoi(minAgeStr)
		if err != nil {
			s.respondWithError(w, http.StatusBadRequest, "Invalid min_age parameter")
			return
		}
		opts = append(opts, student.MinAge(minAge))
	}

	if maxAgeStr := r.URL.Query().Get("max_age"); maxAgeStr != "" {
		maxAge, err := strconv.Atoi(maxAgeStr)
		if err != nil {
			s.respondWithError(w, http.StatusBadRequest, "Invalid max_age parameter")
			return
		}
		opts = append(opts, student.MaxAge(maxAge))
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
			DateOfBirth:     student.DateOfBirth.Format("2006-01-02"),
			Grade:           fmt.Sprintf("%d", student.Grade),
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

// @Summary     List schools
// @Description Get a list of schools by their IDs
// @Tags        schools
// @Accept      json
// @Produce     json
// @Param       ids query string true "Comma-separated list of school IDs"
// @Success     200 {object} ListSchoolsResponse
// @Failure     400 {object} ErrorResponse
// @Failure     500 {object} ErrorResponse
// @Router      /schools [get]
// @Security    ApiKeyAuth
func (s *Server) apiListSchools(w http.ResponseWriter, r *http.Request) {
	// Get the ids from query parameter
	idsParam := r.URL.Query().Get("ids")
	if idsParam == "" {
		s.respondWithError(w, http.StatusBadRequest, "ids parameter is required")
		return
	}

	// Split the comma-separated IDs
	schoolIDs := strings.Split(idsParam, ",")
	if len(schoolIDs) == 0 {
		s.respondWithError(w, http.StatusBadRequest, "at least one school ID is required")
		return
	}

	schoolIDsUint64 := make([]uint64, 0, len(schoolIDs))
	for _, id := range schoolIDs {
		schoolIDUint64, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			s.respondWithError(w, http.StatusBadRequest, "invalid school ID")
			return
		}
		schoolIDsUint64 = append(schoolIDsUint64, schoolIDUint64)
	}

	// Get the schools from the service
	schools, err := s.SchoolSvc.GetSchoolsByIDs(r.Context(), schoolIDsUint64)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Error fetching schools")
		return
	}

	// Convert to response format
	response := ListSchoolsResponse{
		Schools: make([]SchoolResponse, 0, len(schools)),
	}

	for _, school := range schools {
		response.Schools = append(response.Schools, SchoolResponse{
			ID:      fmt.Sprintf("%d", school.ID),
			Name:    school.GetData().Name,
			Country: school.GetData().Country,
			City:    school.GetData().City,
		})
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// @Summary     Get student by ID
// @Description Get detailed information about a specific student
// @Tags        students
// @Accept      json
// @Produce     json
// @Param       id   path      int  true  "Student ID"
// @Success     200  {object}  GetStudentResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /students/{id} [get]
// @Security    ApiKeyAuth
func (s *Server) apiGetStudent(w http.ResponseWriter, r *http.Request) {
	// Get student ID from URL parameter
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid student ID")
		return
	}

	// Get student from service
	student, err := s.StudentSvc.GetStudent(r.Context(), id)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Error fetching student")
		return
	}

	if student == nil {
		s.respondWithError(w, http.StatusNotFound, "Student not found")
		return
	}

	// Build photo URL if photo exists
	photoURL := ""
	if student.GetStudent().GetProfilePhotoId() != "" {
		photoURL = fmt.Sprintf("/student/profile/photo/%s", student.GetStudent().GetProfilePhotoId())
	}

	// Convert to response format
	response := GetStudentResponse{
		ID:              uint(student.ID),
		FirstName:       student.GetStudent().FirstName,
		LastName:        student.GetStudent().LastName,
		SchoolID:        student.GetStudent().SchoolId,
		ProfilePhotoURL: photoURL,
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// @Summary     Sponsor a student
// @Description Add or update sponsorship for a student
// @Tags        students
// @Accept      json
// @Produce     json
// @Param       id      path      int                 true  "Student ID"
// @Param       request body      SponsorStudentRequest true "Sponsorship details"
// @Success     200     {object}  SponsorStudentResponse
// @Failure     400     {object}  ErrorResponse
// @Failure     404     {object}  ErrorResponse
// @Failure     500     {object}  ErrorResponse
// @Router      /students/{id}/sponsor [post]
// @Security    ApiKeyAuth
// TODO: prevent duplicate sponsorships
func (s *Server) apiSponsorStudent(w http.ResponseWriter, r *http.Request) {
	// Get student ID from URL parameter
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid student ID")
		return
	}

	// Get student from service
	student, err := s.StudentSvc.GetStudent(r.Context(), id)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Error fetching student")
		return
	}

	if student == nil {
		s.respondWithError(w, http.StatusNotFound, "Student not found")
		return
	}

	// Verify student is eligible for sponsorship
	if !student.GetStudent().GetEligibleForSponsorship() {
		s.respondWithError(w, http.StatusBadRequest, "Student is not eligible for sponsorship")
		return
	}

	// Verify student is active
	if !student.IsActive() {
		s.respondWithError(w, http.StatusBadRequest, "Cannot sponsor inactive student")
		return
	}

	// Parse request body
	var req SponsorStudentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if req.SponsorID == "" {
		s.respondWithError(w, http.StatusBadRequest, "Sponsor ID is required")
		return
	}

	// Parse dates
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid start date format (use YYYY-MM-DD)")
		return
	}

	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid end date format (use YYYY-MM-DD)")
		return
	}

	// Validate dates
	if startDate.After(endDate) {
		s.respondWithError(w, http.StatusBadRequest, "Start date must be before end date")
		return
	}

	// Create the command
	cmd := &eda.Student_UpdateSponsorship{
		SponsorId: req.SponsorID,
		StartDate: &eda.Date{
			Year:  int32(startDate.Year()),
			Month: int32(startDate.Month()),
			Day:   int32(startDate.Day()),
		},
		EndDate: &eda.Date{
			Year:  int32(endDate.Year()),
			Month: int32(endDate.Month()),
			Day:   int32(endDate.Day()),
		},
		Version:       student.GetVersion(),
		PaymentId:     req.PaymentID,
		PaymentAmount: req.PaymentAmount,
	}

	// Run the command
	_, err = s.StudentSvc.RunCommand(r.Context(), id, cmd)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to sponsor student: %v", err))
		return
	}

	// Return success response
	s.respondWithJSON(w, http.StatusOK, SponsorStudentResponse{Success: true})
}

// @Summary     List sponsored students
// @Description Get a list of students currently sponsored by the given sponsor
// @Tags        sponsors
// @Accept      json
// @Produce     json
// @Param       id   path      string  true  "Sponsor ID"
// @Success     200  {array}   SponsorshipResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /sponsors/{id}/students [get]
// @Security    ApiKeyAuth
func (s *Server) apiListSponsoredStudents(w http.ResponseWriter, r *http.Request) {
	sponsorID := chi.URLParam(r, "id")
	if sponsorID == "" {
		s.respondWithError(w, http.StatusBadRequest, "Sponsor ID is required")
		return
	}

	sponsorships, err := s.StudentSvc.GetCurrentSponsorships(r.Context(), sponsorID)
	if err != nil {
		log.Printf("Error fetching sponsorships: %v", err)
		s.respondWithError(w, http.StatusInternalServerError, "Failed to fetch sponsorships")
		return
	}

	response := make([]SponsorshipResponse, len(sponsorships))
	for i, sp := range sponsorships {
		response[i] = SponsorshipResponse{
			StudentID: sp.StudentID,
			StartDate: sp.StartDate.Format("2006-01-02"),
			EndDate:   sp.EndDate.Format("2006-01-02"),
		}
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// @Summary     Get sponsor impact metrics
// @Description Get impact metrics for a sponsor's contributions
// @Tags        sponsors
// @Accept      json
// @Produce     json
// @Param       id   path      string  true  "Sponsor ID"
// @Success     200  {object}  SponsorImpactResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /sponsors/{id}/impact [get]
// @Security    ApiKeyAuth
func (s *Server) apiGetSponsorImpact(w http.ResponseWriter, r *http.Request) {
	sponsorID := chi.URLParam(r, "id")
	if sponsorID == "" {
		s.respondWithError(w, http.StatusBadRequest, "Sponsor ID is required")
		return
	}

	mealCount, err := s.StudentSvc.GetSponsorImpactMetrics(r.Context(), sponsorID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get impact metrics: %v", err))
		return
	}

	response := SponsorImpactResponse{
		TotalMealCount: mealCount,
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// @Summary     List sponsor-related feeding events
// @Description Get a paginated list of feeding events that occurred during sponsorship periods
// @Tags        sponsors
// @Accept      json
// @Produce     json
// @Param       id    path      string  true   "Sponsor ID"
// @Param       page  query     int     false  "Page number"      default(1)
// @Param       limit query     int     false  "Items per page"   default(10)
// @Success     200   {object}  ListSponsorFeedingEventsResponse
// @Failure     400   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /sponsors/{id}/events [get]
// @Security    ApiKeyAuth
func (s *Server) apiListSponsorFeedingEvents(w http.ResponseWriter, r *http.Request) {
	sponsorID := chi.URLParam(r, "id")
	if sponsorID == "" {
		s.respondWithError(w, http.StatusBadRequest, "Sponsor ID is required")
		return
	}

	page := s.pageQuery(r)
	limit := s.limitQuery(r)

	events, total, err := s.StudentSvc.ListSponsorFeedingEvents(r.Context(), sponsorID, limit, page)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list feeding events: %v", err))
		return
	}

	response := ListSponsorFeedingEventsResponse{
		Events: make([]SponsorFeedingEventResponse, len(events)),
		Total:  total,
	}

	for i, event := range events {
		response.Events[i] = SponsorFeedingEventResponse{
			StudentID:   event.StudentID,
			StudentName: event.StudentName,
			FeedingTime: event.FeedingTime.Format(time.RFC3339),
			SchoolID:    event.SchoolID,
			EventType:   "feeding",
		}
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
