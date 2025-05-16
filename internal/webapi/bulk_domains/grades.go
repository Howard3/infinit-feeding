package bulk_domains

import (
	"fmt"
	"io"
	"net/http"

	"geevly/gen/go/eda"
)

// GradesDomain implements BulkUploadDomain for grades uploads
type GradesDomain struct{}

// ValidateFormData validates form data for grades upload
// Basic form validation - detailed business rule validation happens in the aggregate
func (d *GradesDomain) ValidateFormData(r *http.Request) (map[string]string, error) {
	schoolID := r.FormValue("school_id")
	schoolYear := r.FormValue("school_year")
	gradingPeriod := r.FormValue("grading_period")
	effectiveDate := r.FormValue("effective_date")

	// Check required fields
	if schoolID == "" || schoolYear == "" || gradingPeriod == "" || effectiveDate == "" {
		return nil, fmt.Errorf("Missing required fields")
	}

	// Return the metadata - deep validation happens in the aggregate
	return map[string]string{
		"school_id":      schoolID,
		"school_year":    schoolYear,
		"grading_period": gradingPeriod,
		"effective_date": effectiveDate,
	}, nil
}

// UploadFile handles file upload for grades
func (d *GradesDomain) UploadFile(r *http.Request, fileSvc FileService) (string, error) {
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
		Name:            "bulk_upload_grades",
		DomainReference: eda.File_BULK_UPLOAD,
	})
	if err != nil {
		return "", fmt.Errorf("storing file: %w", err)
	}

	return fileID, nil
}

// GetTargetDomain returns the EDA domain type for grades
func (d *GradesDomain) GetDomain() eda.BulkUpload_Domain {
	return eda.BulkUpload_GRADES
}

// GetFileName returns the name of the file field in the form
func (d *GradesDomain) GetFileName() string {
	return "grades_file"
}

// GetMaxFileSize returns the maximum file size in bytes (10MB)
func (d *GradesDomain) GetMaxFileSize() int64 {
	return 10 << 20 // 10MB
}
