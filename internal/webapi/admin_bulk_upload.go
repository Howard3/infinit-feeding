package webapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"geevly/gen/go/eda"
	"geevly/internal/webapi/static"
	"geevly/internal/webapi/templates/admin/bulk_upload"
	components "geevly/internal/webapi/templates/components"
	"geevly/internal/webapi/templates/layouts"

	"github.com/go-chi/chi/v5"
)

func (s *Server) bulkUploadAdminRoutes(r chi.Router) {
	r.Get("/", s.bulkUploadAdminList)
	r.Get("/create", s.bulkUploadAdminCreate)
	r.Get("/upload-form/{domain}", s.bulkUploadAdminUploadForm)
	r.Post("/create", s.bulkUploadAdminStoreUpload)
	r.Get("/template", s.bulkUploadAdminTemplate)
	r.Get("/instructions", s.bulkUploadAdminInstructions)
	r.Get("/{id}/view", s.bulkUploadAdminView)
	r.Post("/{id}/validate", s.bulkUploadAdminValidateUpload)
	r.Post("/{id}/start-processing", s.bulkUploadAdminProcessUpload)
	r.Get("/{id}/download", s.bulkUploadAdminDownload)
}

func (s *Server) bulkUploadAdminList(w http.ResponseWriter, r *http.Request) {
	limit := s.limitQuery(r)
	page := s.pageQuery(r)

	// Fetch bulk uploads from the service
	uploadList, err := s.Services.BulkUploadSvc.ListBulkUploads(r.Context(), limit, page-1)
	if err != nil {
		slog.Error("failed to list bulk uploads", "error", err)
		s.errorPage(w, r, "Failed to load bulk uploads", err)
		return
	}

	// Create pagination
	pagination := components.NewPagination(page, limit, uploadList.Count)
	s.renderTempl(w, r, bulk_upload.List(uploadList, pagination))
}

func (s *Server) bulkUploadAdminCreate(w http.ResponseWriter, r *http.Request) {
	s.renderTempl(w, r, bulk_upload.Create())
}

func (s *Server) bulkUploadAdminUploadForm(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")

	switch domain {
	case "new_students":
		s.newStudents(w, r)
		return
	case "grades":
		s.grades(w, r)
		return
	case "health_assessment":
		s.healthAssessment(w, r)
		return
	}

	// TODO: handle fallthrough
}

func (s *Server) grades(w http.ResponseWriter, r *http.Request) {
	schools, err := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.renderTempl(w, r, bulk_upload.GradesForm(schools))
}

func (s *Server) healthAssessment(w http.ResponseWriter, r *http.Request) {
	schools, err := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.renderTempl(w, r, bulk_upload.HealthAssessmentForm(schools))
}

func (s *Server) newStudents(w http.ResponseWriter, r *http.Request) {
	schools, err := s.Services.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.renderTempl(w, r, bulk_upload.NewStudents(schools))
}

// handleBulkUploadError sends an appropriate error response
func (s *Server) handleBulkUploadError(w http.ResponseWriter, message string, status int) {
	slog.Warn("bulk upload error", "error", message)
	w.WriteHeader(status)
	w.Write([]byte(message))
}

// handleBulkUploadSuccess redirects to the bulk upload view page
func (s *Server) handleBulkUploadSuccess(w http.ResponseWriter, r *http.Request, aggID string) {
	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/bulk-upload/%s/view", aggID), "File uploaded"))
}

func (s *Server) getDomain(domain string) eda.BulkUpload_Domain {
	switch domain {
	case "grades":
		return eda.BulkUpload_GRADES
	case "new_students":
		return eda.BulkUpload_NEW_STUDENTS
	case "health_assessment":
		return eda.BulkUpload_HEALTH_ASSESSMENT
	default:
		return eda.BulkUpload_UNKNOWN_DOMAIN
	}
}

func (s *Server) bulkUploadAdminStoreUpload(w http.ResponseWriter, r *http.Request) {
	// Get domain handler for the requested domain
	domainName := r.FormValue("domain")

	domain, exists := s.bulkDomainRegistry.GetDomain(s.getDomain(domainName))
	if !exists {
		s.handleBulkUploadError(w, "could not find the specified domain", http.StatusInternalServerError)
		return
	}

	// Process the file upload using the domain handler
	fileID, err := domain.UploadFile(r, s.Services.FileSvc)
	if err != nil {
		s.handleBulkUploadError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Validate and get metadata
	metadata, err := domain.ValidateFormData(r)
	if err != nil {
		s.handleBulkUploadError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build the aggregate
	agg, err := s.Services.BulkUploadSvc.Create(r.Context(), &eda.BulkUpload_Create{
		TargetDomain:   domain.GetDomain(),
		FileId:         fileID,
		UploadMetadata: metadata,
	})
	if err != nil {
		s.handleBulkUploadError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// We'll implement validation during the processing phase for now
	slog.Info("file uploaded successfully, validation will occur during processing",
		"domain", domainName,
		"fileID", fileID)

	// Redirect to the view page
	s.handleBulkUploadSuccess(w, r, agg.ID)
}

func (s *Server) bulkUploadAdminValidateUpload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agg, err := s.Services.BulkUploadSvc.GetBulkUpload(r.Context(), id)
	if err != nil {
		slog.Error("error getting bulk upload for processing", "err", err)
		http.Error(w, "Error: "+err.Error(), http.StatusNotFound)
		return
	}

	s.Services.BulkUploadSvc.SetStatus(r.Context(), id, eda.BulkUpload_VALIDATING)

	// Get domain handler based on the domain name
	domain, exists := s.bulkDomainRegistry.GetDomain(agg.GetDomain())
	if !exists {
		http.Error(w, "Invalid domain in bulk upload", http.StatusBadRequest)
		return
	}

	data, err := s.getFile(r.Context(), agg.GetFileID())
	if err != nil {
		http.Error(w, "Error getting file", http.StatusInternalServerError)
		return
	}

	validationResult := domain.ValidateUpload(r.Context(), agg, data)
	if len(validationResult.Errors) > 0 {
		if err := s.Services.BulkUploadSvc.SaveValidationErrors(r.Context(), id, validationResult.Errors); err != nil {
			http.Error(w, "Error saving validation errors: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.Services.BulkUploadSvc.SetStatus(r.Context(), id, eda.BulkUpload_VALIDATION_FAILED)

		s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/bulk-upload/%s/view", id), "Validation error"))
		return
	} else {
		err := s.Services.BulkUploadSvc.SetStatus(r.Context(), id, eda.BulkUpload_VALIDATED)
		if err != nil {
			http.Error(w, "Error setting status: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/bulk-upload/%s/view", id), "Validation error"))
}

func (s *Server) bulkUploadAdminProcessUpload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agg, err := s.Services.BulkUploadSvc.GetBulkUpload(r.Context(), id)
	if err != nil {
		http.Error(w, "Error getting aggregate: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.Services.BulkUploadSvc.SetStatus(r.Context(), id, eda.BulkUpload_PROCESSING); err != nil {
		http.Error(w, "Error setting status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	domain, exists := s.bulkDomainRegistry.GetDomain(agg.GetDomain())
	if !exists {
		http.Error(w, "Domain not found", http.StatusNotFound)
		return
	}

	data, err := s.getFile(r.Context(), agg.GetFileID())
	if err != nil {
		http.Error(w, "Error getting file", http.StatusInternalServerError)
		return
	}

	if err := domain.ProcessUpload(r.Context(), agg, s.Services.BulkUploadSvc, data); err != nil {
		http.Error(w, "Error processing upload: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.Services.BulkUploadSvc.SetStatus(r.Context(), id, eda.BulkUpload_COMPLETED); err != nil {
		http.Error(w, "Error setting status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/bulk-upload/%s/view", id), "Validation error"))
}

func (s *Server) bulkUploadAdminTemplate(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("type")
	format := r.URL.Query().Get("format")

	// Default to CSV if no format specified
	templateType := static.CSV
	if format == "zip" {
		templateType = static.ZIP
	}

	// Get the template information
	templateInfo, err := static.GetBulkTemplateInfo(domain, templateType)
	if err != nil {
		templateInfo, err = static.GetBulkTemplateInfo(domain, static.CSV)
		if err != nil {
			slog.Error("error getting bulk template info", "err", err)
			http.Error(w, "Template not found (1)", http.StatusNotFound)
			return
		}
	}

	// Set the headers based on template info
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", templateInfo.Filename))
	w.Header().Set("Content-Type", templateInfo.ContentType)

	// Write the template data
	w.Write(templateInfo.Data)
}

func (s *Server) bulkUploadAdminInstructions(w http.ResponseWriter, r *http.Request) {
	// For now, just show a coming soon page
	s.renderTempl(w, r, bulk_upload.InstructionsComingSoon())
}

func (s *Server) bulkUploadAdminView(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agg, err := s.Services.BulkUploadSvc.GetBulkUpload(r.Context(), id)
	if err != nil {
		slog.Error("error getting bulk upload", "err", err)
		http.Error(w, "error: "+err.Error(), http.StatusNotFound)
		return
	}

	s.renderTempl(w, r, bulk_upload.ViewBulkUpload(agg))
}

func (s *Server) bulkUploadAdminDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agg, err := s.Services.BulkUploadSvc.GetBulkUpload(r.Context(), id)
	if err != nil {
		slog.Error("error getting bulk upload", "err", err)
		http.Error(w, "error: "+err.Error(), http.StatusNotFound)
		return
	}

	// TODO: implement download logic
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	// indicate not implemented
	data, err := s.getFile(r.Context(), agg.GetFileID())
	if err != nil {
		slog.Error("error getting file", "err", err)
		http.Error(w, "error: "+err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Add("Content-Type", "application/octet-stream")
	w.Header().Add("Content-Length", strconv.Itoa(len(data)))
	w.Header().Add("Content-Disposition", "attachment; filename=\"bulk_upload\"") // TODO: proper extension
	w.Write(data)
}

func (s *Server) getFile(ctx context.Context, fileID string) ([]byte, error) {
	return s.Services.FileSvc.GetFileBytes(ctx, eda.File_BULK_UPLOAD.String(), fileID)
}
