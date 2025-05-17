package bulk_domains

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"

	"geevly/gen/go/eda"
	"geevly/internal/bulk_upload"
	"geevly/internal/file"
)

// NewStudentsDomain implements BulkUploadDomain for new students uploads
type NewStudentsDomain struct {
	services *ServiceRegistry
}

// NewNewStudentsDomain creates a new NewStudentsDomain with the provided services
func NewNewStudentsDomain(services *ServiceRegistry) *NewStudentsDomain {
	return &NewStudentsDomain{
		services: services,
	}
}

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
func (d *NewStudentsDomain) UploadFile(r *http.Request, fileSvc *file.Service) (string, error) {
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
	fileID, err := fileSvc.CreateFile(r.Context(), fileBytes, &eda.File_Create{
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

// ValidateUpload validates the uploaded students file against business rules
func (d *NewStudentsDomain) ValidateUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, fileBytes []byte) *ValidationResult {
	// This is a placeholder implementation that will be enhanced later
	result := &ValidationResult{
		IsValid: true,
		Errors:  []*eda.BulkUpload_ValidationError{},
	}

	// Parse the CSV data
	reader := csv.NewReader(strings.NewReader(string(fileBytes)))

	// Read header row
	header, err := reader.Read()
	if err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
			Field:   "headers",
			Message: fmt.Sprintf("Failed to read CSV header: %v", err),
		})
	}

	// Validate header columns
	requiredColumns := []string{"First Name", "Last Name", "LRN", "Grade Level", "Date of Birth", "Gender", "Status"}
	missingColumns := validateCSVHeaders(header, requiredColumns)

	if len(missingColumns) > 0 {
		result.IsValid = false
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
			Field:   "headers",
			Message: fmt.Sprintf("Missing required columns: %s", strings.Join(missingColumns, ", ")),
		})
	}

	// Validate school ID exists
	schoolIDStr := aggregate.GetUploadMetadata()["school_id"]
	if schoolIDStr == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
			Field:   "school_id",
			Message: "School ID is required",
		})
	} else if d.services != nil && d.services.SchoolService != nil {
		// Convert schoolID to uint64 and validate it exists
		// Note: This validation already happens in the aggregate, so this is just a placeholder
	}

	// Read and validate each row
	// In a real implementation, we would:
	// 1. Validate required fields
	// 2. Validate data formats
	// 3. Check for duplicate LRNs
	// 4. Validate that students don't already exist in the system

	return result
}

// GetFileName returns the name of the file field in the form
func (d *NewStudentsDomain) GetFileName() string {
	return "students_file"
}

// GetMaxFileSize returns the maximum file size in bytes (50MB)
func (d *NewStudentsDomain) GetMaxFileSize() int64 {
	return 50 << 20 // 50MB
}
