package webapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"geevly/gen/go/eda"
	"geevly/internal/webapi/bulk_domains"
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
	r.Post("/{id}/start-processing", s.bulkUploadAdminProcessUpload)
	r.Get("/{id}/download", s.bulkUploadAdminDownload)
}

// fileServiceWrapper adapts the Server's FileSvc to the bulk_domains.FileService interface
type fileServiceWrapper struct {
	svc *Server
}

func (f *fileServiceWrapper) CreateFile(r http.Request, fileBytes []byte, fileCreate *eda.File_Create) (string, error) {
	return f.svc.FileSvc.CreateFile(r.Context(), fileBytes, fileCreate)
}

func (f *fileServiceWrapper) GetFileBytes(r http.Request, domainRef string, fileID string) ([]byte, error) {
	return f.svc.FileSvc.GetFileBytes(r.Context(), domainRef, fileID)
}

func (s *Server) bulkUploadAdminList(w http.ResponseWriter, r *http.Request) {
	limit := s.limitQuery(r)
	page := s.pageQuery(r)

	// Fetch bulk uploads from the service
	uploadList, err := s.BulkUploadSvc.ListBulkUploads(r.Context(), limit, page-1)
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
	}

	// TODO: handle fallthrough
}

func (s *Server) grades(w http.ResponseWriter, r *http.Request) {
	schools, err := s.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.renderTempl(w, r, bulk_upload.GradesForm(schools))
}

func (s *Server) newStudents(w http.ResponseWriter, r *http.Request) {
	schools, err := s.SchoolSvc.MapSchoolsByID(r.Context())
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

func (s *Server) bulkUploadAdminStoreUpload(w http.ResponseWriter, r *http.Request) {
	// Create a domain registry
	registry := bulk_domains.NewDomainRegistry()

	// Get domain handler for the requested domain
	domainName := r.FormValue("domain")
	domain, exists := registry.GetDomain(domainName)

	if !exists {
		http.Error(w, "Invalid domain", http.StatusBadRequest)
		return
	}

	// Create a file service wrapper to adapt our service to the domain interface
	fileService := &fileServiceWrapper{svc: s}

	// Process the file upload using the domain handler
	fileID, err := domain.UploadFile(r, fileService)
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
	agg, err := s.BulkUploadSvc.Create(r.Context(), &eda.BulkUpload_Create{
		TargetDomain:   domain.GetDomain(),
		FileId:         fileID,
		UploadMetadata: metadata,
	})
	if err != nil {
		s.handleBulkUploadError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect to the view page
	s.handleBulkUploadSuccess(w, r, agg.ID)
}

func (s *Server) bulkUploadAdminProcessUpload(w http.ResponseWriter, r *http.Request) {
	// TODO: implement upload processing logic
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	// indicate not implemented
	w.Write([]byte("Not implemented"))
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
	agg, err := s.BulkUploadSvc.GetBulkUpload(r.Context(), id)
	if err != nil {
		slog.Error("error getting bulk upload", "err", err)
		http.Error(w, "error: "+err.Error(), http.StatusNotFound)
		return
	}

	s.renderTempl(w, r, bulk_upload.ViewBulkUpload(agg))
}

func (s *Server) bulkUploadAdminDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agg, err := s.BulkUploadSvc.GetBulkUpload(r.Context(), id)
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
	return s.FileSvc.GetFileBytes(ctx, eda.File_BULK_UPLOAD.String(), fileID)
}
