package bulk_domains

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"geevly/gen/go/eda"
	"geevly/internal/bulk_upload"
	"geevly/internal/file"
)

// GradesDomain implements BulkUploadDomain for grades uploads
type GradesDomain struct {
	services *ServiceRegistry
}

// NewGradesDomain creates a new GradesDomain with the provided services
func NewGradesDomain(services *ServiceRegistry) *GradesDomain {
	return &GradesDomain{
		services: services,
	}
}

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

func (d *GradesDomain) validateSchoolID(ctx context.Context, schoolID string) error {
	if d.services != nil && d.services.SchoolService != nil {
		schoolIDInt, err := strconv.ParseUint(schoolID, 10, 64)
		if err == nil {
			err = d.services.SchoolService.ValidateSchoolID(ctx, schoolIDInt)
			if err != nil {
				return fmt.Errorf("invalid school ID: %s", err.Error())
			}
		}
	}
	return nil
}

func (d *GradesDomain) validateHeaders(_ context.Context, firstRow []string) error {
	requiredColumns := []string{"LRN", "Grade"}
	missingColumns := validateCSVHeaders(firstRow, requiredColumns)

	if len(missingColumns) > 0 {
		return fmt.Errorf("missing required columns: %v", missingColumns)
	}

	return nil
}

// ValidateUpload validates the uploaded grades file against business rules
func (d *GradesDomain) ValidateUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, fileBytes []byte) *ValidationResult {
	result := &ValidationResult{
		IsValid: true,
		Errors:  []*eda.BulkUpload_ValidationError{},
	}

	// Get metadata from aggregate
	metadata := aggregate.GetUploadMetadata()
	schoolID := metadata["school_id"]

	// Validate school exists (if school service is available)
	if err := d.validateSchoolID(ctx, schoolID); err != nil {
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
			Field:   "school_id",
			Message: fmt.Sprintf("Invalid school ID: %s", err.Error()),
		})
	}

	// Parse the CSV data
	reader := csv.NewReader(strings.NewReader(string(fileBytes)))

	// Read header row
	header, err := reader.Read()
	if err != nil {
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_CSV_HEADER,
			Message: fmt.Sprintf("Failed to read CSV header: %s", err.Error()),
		})
	}

	// Validate header columns
	if headerErr := d.validateHeaders(ctx, header); headerErr != nil {
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_CSV_HEADER,
			Message: fmt.Sprintf("Invalid header columns: %s", headerErr.Error()),
		})
	}

	// Read all rows for validation
	rows, err := reader.ReadAll()
	if err != nil {
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_CSV_DATA,
			Message: fmt.Sprintf("Failed to read CSV data: %s", err.Error()),
		})
	}

	// Track LRNs to check for duplicates
	lrnMap := make(map[string]int)

	// Validate each row
	for i, row := range rows {
		rowNum := i + 2 // +2 because row numbers are 1-based and we've already read the header

		// Skip empty rows
		if len(row) == 0 || (len(row) == 1 && row[0] == "") {
			continue
		}

		// Check for missing data
		if len(row) < 2 || row[0] == "" || row[1] == "" {
			result.IsValid = false
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context:   eda.BulkUpload_ValidationError_ROW_NUMBER,
				RowNumber: uint64(rowNum),
				Message:   "Row is missing required data (LRN or Grade)",
			})
			continue
		}

		lrn := row[0]
		grade := row[1]

		// Check for duplicate LRNs
		if prevRow, exists := lrnMap[lrn]; exists {
			result.IsValid = false
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context:   eda.BulkUpload_ValidationError_ROW_NUMBER,
				RowNumber: uint64(rowNum),
				Message:   fmt.Sprintf("Duplicate LRN %s (previously seen on row %d)", lrn, prevRow),
			})
		} else {
			lrnMap[lrn] = rowNum
		}

		// Validate grade format (numeric, 0-100)
		gradeVal, err := strconv.ParseFloat(grade, 64)
		if err != nil || gradeVal < 0 || gradeVal > 100 {
			result.IsValid = false
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context:   eda.BulkUpload_ValidationError_ROW_NUMBER,
				RowNumber: uint64(rowNum),
				Field:     "Grade",
				Message:   fmt.Sprintf("Invalid grade value: %s (must be a number between 0 and 100)", grade),
			})
		}

		// Validate the LRN's against the student service
		_, err = d.services.StudentService.GetStudentByStudentAndSchoolID(ctx, lrn, schoolID)
		if err != nil {
			result.IsValid = false
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context:   eda.BulkUpload_ValidationError_ROW_NUMBER,
				RowNumber: uint64(rowNum),
				Field:     "LRN",
				Message:   fmt.Sprintf("Invalid LRN: %s for school ID %s", lrn, schoolID),
			})
		}
	}

	return result
}

// UploadFile handles file upload for grades
func (d *GradesDomain) UploadFile(r *http.Request, fileSvc *file.Service) (string, error) {
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
