package webapi

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"

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
	r.Get("/view/{id}", s.bulkUploadAdminView)
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
	}

	// TODO: handle fallthrough
}

func (s *Server) newStudents(w http.ResponseWriter, r *http.Request) {
	schools, err := s.SchoolSvc.MapSchoolsByID(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.renderTempl(w, r, bulk_upload.NewStudents(schools))
}

func (s *Server) bulkUploadAdminStoreUpload(w http.ResponseWriter, r *http.Request) {
	domain := r.FormValue("domain")
	switch domain {
	case "new_students":
		// upload to storage
		fileID, err := s.bulkUploadNewStudents(r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		// build the aggregate
		agg, err := s.BulkUploadSvc.Create(r.Context(), &eda.BulkUpload_Create{
			TargetDomain: eda.BulkUpload_NEW_STUDENTS,
			FileId:       fileID,
			UploadMetadata: map[string]string{
				"school_id": r.FormValue("school_id"),
			},
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		// redirect to agg id
		s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/bulk-upload/view/%s", agg.ID), "File uploaded"))
	}

	http.Error(w, "Invalid domain", http.StatusBadRequest)
	return
}
func (s *Server) bulkUploadNewStudents(r *http.Request) (string, error) {
	// TODO: allow larger file selection later. currently limited to 50MB
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		err = fmt.Errorf("parsing form %w", err)
		return "", err
	}

	file, _, err := r.FormFile("students_file")
	if err != nil {
		err = fmt.Errorf("getting file %w", err)
		return "", err
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		err = fmt.Errorf("reading file %w", err)
		return "", err
	}

	fileID, err := s.FileSvc.CreateFile(r.Context(), fileBytes, &eda.File_Create{
		Name:            "bulk_upload_new_students",
		DomainReference: eda.File_BULK_UPLOAD,
	})
	if err != nil {
		return "", err
	}

	return fileID, nil
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
