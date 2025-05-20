package bulk_domains

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	if _, err := d.parseDate(effectiveDate); err != nil {
		return nil, fmt.Errorf("invalid effective date: %s", err.Error())
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

// GradeRow represents a single row in the grades CSV file
type GradeRow struct {
	LRN   string
	Grade string
}

func (row *GradeRow) Validate() error {
	if row.LRN == "" {
		return errors.New("LRN is required")
	}
	if row.Grade == "" {
		return errors.New("Grade is required")
	}
	grade, err := row.GradeInt()
	if err != nil {
		return errors.New("Grade must be a number")
	}
	if grade < 0 || grade > 100 {
		return errors.New("Grade must be between 0 and 100")
	}
	return nil
}

// GradeInt returns the grade as an integer
func (row *GradeRow) GradeInt() (int, error) {
	return strconv.Atoi(row.Grade)
}

// parseCSV parses the CSV file bytes and returns rows as GradeRow structs
func (d *GradesDomain) parseCSV(fileBytes []byte) (header []string, rows []GradeRow, err error) {
	// Parse the CSV data
	reader := csv.NewReader(strings.NewReader(string(fileBytes)))

	// Read header row
	header, err = reader.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Find column indexes
	lrnIndex := -1
	gradeIndex := -1
	for i, col := range header {
		if col == "LRN" {
			lrnIndex = i
		} else if col == "Grade" {
			gradeIndex = i
		}
	}

	if lrnIndex == -1 || gradeIndex == -1 {
		return header, nil, fmt.Errorf("required columns not found: LRN and/or Grade")
	}

	// Read all data rows
	dataRows, err := reader.ReadAll()
	if err != nil {
		return header, nil, fmt.Errorf("failed to read CSV data: %w", err)
	}

	// Convert rows to structs
	rows = make([]GradeRow, 0, len(dataRows))
	for _, row := range dataRows {
		if len(row) <= max(lrnIndex, gradeIndex) {
			continue // Skip rows that don't have enough columns
		}

		gradeRow := GradeRow{
			LRN:   row[lrnIndex],
			Grade: row[gradeIndex],
		}
		rows = append(rows, gradeRow)
	}

	return header, rows, nil
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
	header, rows, err := d.parseCSV(fileBytes)
	if err != nil {
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_CSV_DATA,
			Message: fmt.Sprintf("Failed to parse CSV: %s", err.Error()),
		})
	}

	// Validate header columns
	if header != nil {
		if err := d.validateHeaders(ctx, header); err != nil {
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context: eda.BulkUpload_ValidationError_CSV_HEADER,
				Message: fmt.Sprintf("Invalid header columns: %s", err.Error()),
			})
		}
	}

	// Track LRNs to check for duplicates
	lrnMap := make(map[string]int)

	// Validate each row
	for i, row := range rows {
		rowNum := i + 2 // +2 because row numbers are 1-based and we've already read the header

		// Check for missing data
		lrn := row.LRN
		grade := row.Grade

		if lrn == "" || grade == "" {
			result.IsValid = false
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context:   eda.BulkUpload_ValidationError_ROW_NUMBER,
				RowNumber: uint64(rowNum),
				Message:   "Row is missing required data (LRN or Grade)",
			})
			continue
		}

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
		if err := row.Validate(); err != nil {
			result.IsValid = false
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context:   eda.BulkUpload_ValidationError_ROW_NUMBER,
				RowNumber: uint64(rowNum),
				Field:     "Grade",
				Message:   fmt.Sprintf("Invalid LRN or grade value: %s (must be a number between 0 and 100)", grade),
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

// ProcessUpload processes the uploaded file for grades
func (d *GradesDomain) ProcessUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, svc *bulk_upload.Service, fileBytes []byte) error {
	if d.services == nil {
		return fmt.Errorf("services are not initialized")
	}

	if d.services.StudentService == nil {
		return fmt.Errorf("student service is not initialized")
	}

	// Parse the CSV data using the common function
	_, rows, err := d.parseCSV(fileBytes)
	if err != nil {
		return fmt.Errorf("failed to parse CSV: %w", err)
	}

	// Get metadata from aggregate
	metadata := aggregate.GetUploadMetadata()
	schoolID := metadata["school_id"]
	schoolYear := metadata["school_year"]
	gradingPeriod := metadata["grading_period"]
	effectiveDate := metadata["effective_date"]

	if schoolID == "" || schoolYear == "" || gradingPeriod == "" || effectiveDate == "" {
		return fmt.Errorf("missing required metadata for processing")
	}

	effectiveDateParsed, err := d.parseDate(effectiveDate)
	if err != nil {
		return fmt.Errorf("invalid effective date: %s", err.Error())
	}

	// Track processed records
	toProcessIDs := make([]string, 0)
	recentlyProcessed := make([]string, 0)

	defer func() {
		actions := bulk_upload.RecordActions{
			RecordIds:  recentlyProcessed,
			RecordType: eda.BulkUpload_STUDENT,
			Reason:     eda.BulkUpload_RecordAction_PROCESSING,
		}
		// Mark records as processed, regardless of how we exit
		svc.MarkRecordsAsUpdated(ctx, aggregate.GetID(), actions)
	}()

	// First pass: gather student IDs to process
	for _, row := range rows {
		// Find the student by LRN and school ID
		student, err := d.services.StudentService.GetStudentByStudentAndSchoolID(ctx, row.LRN, schoolID)
		if err != nil {
			return fmt.Errorf("failed to find student with LRN %s and school ID %s: %w", row.LRN, schoolID, err)
		}

		toProcessIDs = append(toProcessIDs, student.GetID())
	}

	// Mark records for processing
	recordActions := bulk_upload.RecordActions{
		RecordIds:  toProcessIDs,
		RecordType: eda.BulkUpload_STUDENT,
		Reason:     eda.BulkUpload_RecordAction_PROCESSING,
	}

	if err := svc.AddRecordsToProcess(ctx, aggregate.GetID(), recordActions); err != nil {
		return fmt.Errorf("error adding records to process: %w", err)
	}

	// Second pass: process each row and update student grades
	for _, row := range rows {
		// Parse the grade value
		gradeVal, err := row.GradeInt()
		if err != nil {
			return fmt.Errorf("failed to parse grade value: %w", err)
		}

		// Find the student by LRN and school ID
		student, err := d.services.StudentService.GetStudentByStudentAndSchoolID(ctx, row.LRN, schoolID)
		if err != nil {
			return fmt.Errorf("failed to find student with LRN %s and school ID %s: %w", row.LRN, schoolID, err)
		}

		// Create a grade update command based on your actual data model
		gradeCmd := &eda.Student_GradeReport{
			Grade: int32(gradeVal),
			TestDate: &eda.Date{
				Year:  int32(effectiveDateParsed.Year()),
				Month: int32(effectiveDateParsed.Month()),
				Day:   int32(effectiveDateParsed.Day()),
			},
			AssociatedBulkUploadId: aggregate.GetID(),
			SchoolYear:             schoolYear,
			GradingPeriod:          gradingPeriod,
		}

		err = d.services.StudentService.AddGradeReport(ctx, student.GetIDUint64(), gradeCmd)
		if err != nil {
			return fmt.Errorf("failed to add grade report for student %s: %w", student.GetID(), err)
		}

		recentlyProcessed = append(recentlyProcessed, student.GetID())
	}

	return nil
}

// date parser YYYY-MM-DD
func (d *GradesDomain) parseDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}
