package bulk_domains

import (
	"context"
	"errors"
	"fmt"
	"geevly/internal/student"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"geevly/gen/go/eda"
	"geevly/internal/bulk_upload"
	"geevly/internal/file"
	"golang.org/x/sync/errgroup"
)

const (
	gradesWorkerPoolSize = 20
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
	if grade < 0 || grade >= 100 {
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
	reader := newCSVReader(fileBytes)

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

	// Pre-load the set of existing LRNs for this school in a single
	// query instead of hitting the DB once per CSV row. Per-row lookups
	// over Turso/Hrana were pushing validate past the 2 minute mark and
	// masking real failures as hangs.
	existingLRNs := make(map[string]struct{})
	if d.services != nil && d.services.StudentService != nil && schoolID != "" {
		existing, lerr := d.services.StudentService.ListForSchool(ctx, schoolID)
		if lerr != nil {
			result.IsValid = false
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
				Field:   "school_id",
				Message: fmt.Sprintf("Failed to load students for school %s: %s", schoolID, lerr.Error()),
			})
			// Bail out early — without this set every row's LRN check
			// below would otherwise fall back to false-positive "invalid
			// LRN" errors.
			return result
		}
		for _, s := range existing {
			if s == nil || s.StudentID == "" {
				continue
			}
			existingLRNs[s.StudentID] = struct{}{}
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

		// Validate the LRN against the pre-loaded set.
		if _, ok := existingLRNs[lrn]; !ok {
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
	type studentGrade struct {
		studentID    uint64
		studentIDStr string
		gradeVal     int
	}
	toProcess := make([]studentGrade, 0)
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

	// Phase 1: Look up every row's student via a single ListForSchool
	// call instead of N concurrent GetStudentByStudentAndSchoolID calls.
	// Each of those did two Turso round-trips (ID lookup + aggregate
	// load); over Hrana that's the main contributor to slow processing.
	existing, err := d.services.StudentService.ListForSchool(ctx, schoolID)
	if err != nil {
		return fmt.Errorf("failed to list students for school %s: %w", schoolID, err)
	}
	lrnToStudent := make(map[string]*student.ProjectedStudent, len(existing))
	for _, s := range existing {
		if s == nil || s.StudentID == "" {
			continue
		}
		lrnToStudent[s.StudentID] = s
	}

	for _, row := range rows {
		gradeVal, err := row.GradeInt()
		if err != nil {
			return fmt.Errorf("failed to parse grade value: %w", err)
		}

		s, ok := lrnToStudent[row.LRN]
		if !ok {
			return fmt.Errorf("failed to find student with LRN %s and school ID %s", row.LRN, schoolID)
		}

		idStr := strconv.FormatUint(uint64(s.ID), 10)
		toProcessIDs = append(toProcessIDs, idStr)
		toProcess = append(toProcess, studentGrade{
			studentID:    uint64(s.ID),
			studentIDStr: idStr,
			gradeVal:     gradeVal,
		})
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

	// Phase 2: Process grade reports concurrently
	const progressFlushSize = 10
	var mu sync.Mutex
	g2, gctx2 := errgroup.WithContext(ctx)
	semaphore2 := make(chan struct{}, gradesWorkerPoolSize)

	// flushProgress sends the current batch of processed IDs to the bulk
	// upload aggregate so the UI reflects progress in near-real-time.
	// Caller must hold mu.
	flushProgress := func() {
		if len(recentlyProcessed) == 0 {
			return
		}
		batch := make([]string, len(recentlyProcessed))
		copy(batch, recentlyProcessed)
		recentlyProcessed = recentlyProcessed[:0]

		actions := bulk_upload.RecordActions{
			RecordIds:  batch,
			RecordType: eda.BulkUpload_STUDENT,
			Reason:     eda.BulkUpload_RecordAction_PROCESSING,
		}
		svc.MarkRecordsAsUpdated(ctx, aggregate.GetID(), actions)
	}

	for _, sg := range toProcess {
		sg := sg // capture loop variable

		semaphore2 <- struct{}{} // acquire
		g2.Go(func() error {
			defer func() { <-semaphore2 }() // release

			// Create a grade update command based on your actual data model
			gradeCmd := &eda.Student_GradeReport{
				Grade: int32(sg.gradeVal),
				TestDate: &eda.Date{
					Year:  int32(effectiveDateParsed.Year()),
					Month: int32(effectiveDateParsed.Month()),
					Day:   int32(effectiveDateParsed.Day()),
				},
				AssociatedBulkUploadId: aggregate.GetID(),
				SchoolYear:             schoolYear,
				GradingPeriod:          gradingPeriod,
			}

			err := d.services.StudentService.AddGradeReport(gctx2, sg.studentID, gradeCmd)
			if err != nil {
				return fmt.Errorf("failed to add grade report for student %s: %w", sg.studentIDStr, err)
			}

			mu.Lock()
			recentlyProcessed = append(recentlyProcessed, sg.studentIDStr)
			if len(recentlyProcessed) >= progressFlushSize {
				flushProgress()
			}
			mu.Unlock()

			return nil
		})
	}

	if err := g2.Wait(); err != nil {
		return err
	}

	// Flush any remaining
	mu.Lock()
	flushProgress()
	mu.Unlock()

	return nil
}

func (d *GradesDomain) UndoUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, svc *bulk_upload.Service) error {
	recordsUpdated := make([]string, 0)
	defer func() {
		actions := bulk_upload.RecordActions{
			RecordIds:  recordsUpdated,
			RecordType: eda.BulkUpload_STUDENT,
			Reason:     eda.BulkUpload_RecordAction_INVALIDATED,
		}

		svc.MarkRecordsAsUndone(context.Background(), aggregate.GetID(), actions)
	}()

	// Collect students to process
	type studentToUndo struct {
		id     string
		idUint uint64
	}
	studentsToUndo := make([]studentToUndo, 0)

	for studentID, states := range aggregate.GetRecordStates() {
		finalState := states.RecordActions[len(states.RecordActions)-1]
		if finalState.Reason != eda.BulkUpload_RecordAction_PROCESSING {
			slog.Info("skipping student id", "student_id", studentID, "final_state", finalState)
			continue
		}

		studentIDUint, err := strconv.ParseUint(studentID, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse student ID %s: %w", studentID, err)
		}

		studentsToUndo = append(studentsToUndo, studentToUndo{
			id:     studentID,
			idUint: studentIDUint,
		})
	}

	// Process removals concurrently
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	semaphore := make(chan struct{}, gradesWorkerPoolSize)

	for _, stud := range studentsToUndo {
		stud := stud // capture loop variable

		semaphore <- struct{}{} // acquire
		g.Go(func() error {
			defer func() { <-semaphore }() // release

			if err := d.services.StudentService.RemoveGradeReport(gctx, stud.idUint, aggregate.GetID()); err != nil {
				if errors.Is(err, student.ErrGradeReportNotFound) {
					slog.Warn("error removing grade report for student ID %s: %v", stud.id, err)
					return nil
				}
				return fmt.Errorf("failed to remove grade report for student %s: %w", stud.id, err)
			}

			mu.Lock()
			recordsUpdated = append(recordsUpdated, stud.id)
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

// date parser YYYY-MM-DD
func (d *GradesDomain) parseDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}
