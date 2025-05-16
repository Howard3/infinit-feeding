package bulk_domains

import (
	"fmt"
	"io"
	"net/http"

	"geevly/gen/go/eda"
)

// NewStudentsDomain implements BulkUploadDomain for new students uploads
type NewStudentsDomain struct{}

// ValidateFormData validates form data for new students upload
func (d *NewStudentsDomain) ValidateFormData(r *http.Request) (map[string]string, error) {
	schoolID := r.FormValue("school_id")
	if schoolID == "" {
		return nil, fmt.Errorf("school ID is required")
	}

	// deeper validation occurs in the aggregate
	return map[string]string{
		"school_id": schoolID,
	}, nil
}

// UploadFile handles file upload for new students
func (d *NewStudentsDomain) UploadFile(r *http.Request, fileSvc FileService) (string, error) {
	// Parse the multipart form with the specified max file size
	if err := r.ParseMultipartForm(d.GetMaxFileSize()); err != nil {
		return "", fmt.Errorf("parsing form: %w", err)
	}

	// Get the file from the form
	file, _, err := r.FormFile(d.GetFileName())
	if err != nil {
		return "", fmt.Errorf("getting file: %w", err)
	}
	defer file.Close()

	// Read the file bytes
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	// Store the file
	fileID, err := fileSvc.CreateFile(*r, fileBytes, &eda.File_Create{
		Name:            "bulk_upload_new_students",
		DomainReference: eda.File_BULK_UPLOAD,
	})
	if err != nil {
		return "", fmt.Errorf("storing file: %w", err)
	}

	return fileID, nil
}

// GetTargetDomain returns the EDA domain type for new students
func (d *NewStudentsDomain) GetDomain() eda.BulkUpload_Domain {
	return eda.BulkUpload_NEW_STUDENTS
}

// GetFileName returns the name of the file field in the form
func (d *NewStudentsDomain) GetFileName() string {
	return "students_file"
}

// GetMaxFileSize returns the maximum file size in bytes (50MB)
func (d *NewStudentsDomain) GetMaxFileSize() int64 {
	return 50 << 20 // 50MB
}
