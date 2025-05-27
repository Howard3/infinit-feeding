package bulk_domains

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
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

	// Get the file system from the zip archive
	zipReader, err := d.getFSFromZip(fileBytes)
	if err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
			Field:   "file",
			Message: fmt.Sprintf("Failed to extract file system from zip: %v", err),
		})
		return result
	}

	// Get the CSV file from the file system
	csvReader, err := d.getCSVFromFS(zipReader)
	if err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
			Field:   "file",
			Message: fmt.Sprintf("Failed to open CSV file: %v", err),
		})
		return result
	}

	// Read header row
	header, err := csvReader.Read()
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

	if errs := d.validateRows(ctx, csvReader, zipReader, header, schoolIDStr); len(errs) > 0 {
		result.IsValid = false
		for _, err := range errs {
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context: eda.BulkUpload_ValidationError_CSV_DATA,
				Field:   "rows",
				Message: err.Error(),
			})
		}
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

func (d *NewStudentsDomain) ProcessUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, svc *bulk_upload.Service, fileBytes []byte) error {
	// Implement the logic to process the uploaded file
	// This could involve reading the CSV, validating data, and saving to the database

	return nil
}

func (d *NewStudentsDomain) UndoUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, svc *bulk_upload.Service) error {
	panic("not implemented")
}

func (d *NewStudentsDomain) getFSFromZip(data []byte) (fs.FS, error) {
	reader := bytes.NewReader(data)
	return zip.NewReader(reader, int64(len(data)))
}

func (d *NewStudentsDomain) getCSVFromFS(zipFS fs.FS) (*csv.Reader, error) {
	file, err := zipFS.Open("students.csv")
	if err != nil {
		if os.IsNotExist(err) {
			fileList, err := fs.ReadDir(zipFS, ".")
			if err != nil {
				return nil, fmt.Errorf("error when listing file, error when reading students.csv")
			}
			foundFiles := ""
			for _, file := range fileList {
				foundFiles += file.Name() + ", "
			}
			return nil, fmt.Errorf("file not found: %w. Found: %s", err, foundFiles)
		}
		return nil, err
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return csv.NewReader(bytes.NewReader(data)), nil
}

func (d *NewStudentsDomain) validateRows(ctx context.Context, csvReader *csv.Reader, zipReader fs.FS, headers []string, schoolID string) []error {
	var LRNIndex int
	errors := make([]error, 0)

	for i, header := range headers {
		switch header {
		case "LRN":
			LRNIndex = i
		}
	}

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errors = append(errors, fmt.Errorf("error reading csv file: %w", err))
			continue
		}

		slog.Info("validating student w/ lrn", "lrn", record[LRNIndex])
		// Check for duplicate LRNs
		if _, err := d.services.StudentService.GetStudentByStudentAndSchoolID(ctx, record[LRNIndex], schoolID); err == nil {
			errors = append(errors, fmt.Errorf("duplicate LRN: %s", record[LRNIndex]))
			continue
		}

		// TODO: right now we validate a student by the school+student ID, are student id's universally unique?

		photoFileName := fmt.Sprintf("photos/%s.jpg", record[LRNIndex])
		photoStat, err := fs.Stat(zipReader, photoFileName)
		if err != nil {
			errors = append(errors, fmt.Errorf("error checking photo file: %w", err))
			continue
		}

		slog.Info("bulk upload: found photo for new student", "lrn", record[LRNIndex], "photo", photoStat.Name(), "size", photoStat.Size())
	}

	return errors
}
