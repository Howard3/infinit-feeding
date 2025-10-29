package bulk_domains

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"geevly/gen/go/eda"
	"geevly/internal/bulk_upload"
	"geevly/internal/file"
	"geevly/internal/student"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type HealthAssessmentRow struct {
	LRN           string
	HeightCM      float64
	WeightKG      float64
	AssesmentDate time.Time
}

// HealthAssessmentDomain implements BulkUploadDomain for grades uploads
type HealthAssessmentDomain struct {
	services *ServiceRegistry
}

// NewHealthAssessmentDomain creates a new GradesDomain with the provided services
func NewHealthAsssementDomain(services *ServiceRegistry) *HealthAssessmentDomain {
	return &HealthAssessmentDomain{
		services: services,
	}
}

func (d *HealthAssessmentDomain) schoolIsValid(ctx context.Context, schoolID string) bool {
	schoolIDUint, err := strconv.ParseUint(schoolID, 10, 64)
	if err != nil {
		return false
	}

	err = d.services.SchoolService.ValidateSchoolID(ctx, schoolIDUint)
	return err == nil
}

// ValidateFormData validates domain-specific form data
func (d *HealthAssessmentDomain) ValidateFormData(r *http.Request) (map[string]string, error) {
	schoolID := r.FormValue("school_id")
	if schoolID == "" {
		return nil, fmt.Errorf("Missing required fields")
	}

	if !d.schoolIsValid(r.Context(), schoolID) {
		return nil, fmt.Errorf("Invalid school ID")
	}

	return map[string]string{
		"school_id": schoolID,
	}, nil
}

// UploadFile handles the file upload process
func (d *HealthAssessmentDomain) UploadFile(r *http.Request, fileSvc *file.Service) (string, error) {
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
		Name:            "bulk_upload_health_assessment",
		DomainReference: eda.File_BULK_UPLOAD,
	})
	if err != nil {
		return "", fmt.Errorf("storing file: %w", err)
	}

	return fileID, nil
}

// GetDomain returns the EDA domain type
func (d *HealthAssessmentDomain) GetDomain() eda.BulkUpload_Domain {
	return eda.BulkUpload_HEALTH_ASSESSMENT
}

// GetFileName returns the name of the file field in the form
func (d *HealthAssessmentDomain) GetFileName() string {
	return "health_file"
}

// GetMaxFileSize returns the maximum file size in bytes
func (d *HealthAssessmentDomain) GetMaxFileSize() int64 {
	return 10 * 1024 * 1024 // 10MB
}

// ValidateUpload validates the uploaded file against business rules
func (d *HealthAssessmentDomain) ValidateUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, fileBytes []byte) *ValidationResult {
	result := &ValidationResult{
		IsValid: true,
		Errors:  []*eda.BulkUpload_ValidationError{},
	}

	schoolID := aggregate.GetUploadMetadataField("school_id")
	if !d.schoolIsValid(ctx, schoolID) {
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
			Message: fmt.Sprintf("School with ID %s not found", schoolID),
		})
	}

	// parse the csv data
	_, rows, errors := d.parseCSV(fileBytes)
	if len(errors) > 0 {
		for _, err := range errors {
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context: eda.BulkUpload_ValidationError_CSV_DATA,
				Message: fmt.Sprintf("Error parsing CSV: %v", err),
			})
		}
	}

	for rowNum, row := range rows {
		_, err := d.services.StudentService.GetStudentByStudentAndSchoolID(ctx, row.LRN, schoolID)
		if err != nil {
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context:   eda.BulkUpload_ValidationError_CSV_DATA,
				Message:   fmt.Sprintf("Student with LRN %s not found for school ID %s", row.LRN, schoolID),
				RowNumber: uint64(rowNum),
				Field:     "LRN",
			})
		}
	}

	return result
}

func (d *HealthAssessmentDomain) parseCSV(data []byte) (header []string, rows []HealthAssessmentRow, errors []error) {
	reader := csv.NewReader(strings.NewReader(string(data)))

	header, err := reader.Read()
	if err != nil {
		return nil, nil, []error{fmt.Errorf("reading csv %w", err)}
	}

	lrnIndex := slices.Index(header, "lrn")
	heightIndex := slices.Index(header, "height_cm")
	weightIndex := slices.Index(header, "weight_kg")
	assessmentDateHeader := slices.Index(header, "assessment_date")

	if lrnIndex == -1 || heightIndex == -1 || weightIndex == -1 || assessmentDateHeader == -1 {
		return nil, nil, []error{fmt.Errorf("missing required columns in CSV")}
	}

	rowNum := 0
	errors = make([]error, 0)

	for {
		rowNum++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errors = append(errors, fmt.Errorf("reading csv at row %d: %w", rowNum, err))
			continue
		}

		// Try to parse height
		originalHeightValue := record[heightIndex]
		heightCM, err := strconv.ParseFloat(originalHeightValue, 64)
		if err != nil {
			errors = append(errors, fmt.Errorf("parsing height at row %d (value: '%s'): %w", rowNum, originalHeightValue, err))
			continue
		}

		// Try to parse weight
		originalWeightValue := record[weightIndex]
		weightKG, err := strconv.ParseFloat(originalWeightValue, 64)
		if err != nil {
			errors = append(errors, fmt.Errorf("parsing weight at row %d (value: '%s'): %w", rowNum, originalWeightValue, err))
			continue
		}

		// Try to parse assessment date
		originalDateValue := record[assessmentDateHeader]
		assessmentDate, err := time.Parse("2006-01-02", originalDateValue)
		if err != nil {
			errors = append(errors, fmt.Errorf("parsing assessment date at row %d (value: '%s'): %w", rowNum, originalDateValue, err))
			continue
		}

		rows = append(rows, HealthAssessmentRow{
			LRN:           record[lrnIndex],
			HeightCM:      heightCM,
			WeightKG:      weightKG,
			AssesmentDate: assessmentDate,
		})
	}

	// Return collected errors if any
	if len(errors) > 0 {
		errMsgs := make([]string, len(errors))
		for i, err := range errors {
			errMsgs[i] = err.Error()
		}
		return nil, nil, errors
	}

	return header, rows, nil
}

// ProcessUpload processes the validated upload
func (d *HealthAssessmentDomain) ProcessUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, svc *bulk_upload.Service, fileBytes []byte) error {
	_, rows, errors := d.parseCSV(fileBytes)
	if len(errors) > 0 {
		errMsgs := make([]string, len(errors))
		for i, err := range errors {
			errMsgs[i] = err.Error()
		}
		return fmt.Errorf("errors occurred while parsing CSV: %s", strings.Join(errMsgs, ", "))
	}

	// Track processed records
	toProcess := make(map[uint64]*eda.Student_HealthAssessment)
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

	for _, row := range rows {
		assessment := &eda.Student_HealthAssessment{
			HeightCm:               float32(row.HeightCM),
			WeightKg:               float32(row.WeightKG),
			AssociatedBulkUploadId: aggregate.GetID(),
			AssessmentDate:         timestamppb.New(row.AssesmentDate),
		}

		student, err := d.services.StudentService.GetStudentByStudentAndSchoolID(ctx, row.LRN, aggregate.GetUploadMetadataField("school_id"))
		if err != nil {
			return fmt.Errorf("error getting student by LRN %s: %w", row.LRN, err)
		}

		toProcessIDs = append(toProcessIDs, student.GetID())
		toProcess[student.GetIDUint64()] = assessment
	}

	recordActions := bulk_upload.RecordActions{
		RecordIds:  toProcessIDs,
		RecordType: eda.BulkUpload_STUDENT,
		Reason:     eda.BulkUpload_RecordAction_PROCESSING,
	}

	if err := svc.AddRecordsToProcess(ctx, aggregate.ID, recordActions); err != nil {
		return fmt.Errorf("error adding records to process: %w", err)
	}

	for id, assessment := range toProcess {
		err := d.services.StudentService.AddHealthAssessment(ctx, id, assessment)
		if err != nil {
			return fmt.Errorf("error creating health assessment for student ID %d: %w", id, err)
		}

		recentlyProcessed = append(recentlyProcessed, fmt.Sprintf("%d", id))
	}

	return nil
}

func (d *HealthAssessmentDomain) UndoUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, svc *bulk_upload.Service) (err error) {
	recordsUpdated := make([]string, 0)
	defer func() {
		action := bulk_upload.RecordActions{
			RecordIds:  recordsUpdated,
			RecordType: eda.BulkUpload_STUDENT,
			Reason:     eda.BulkUpload_RecordAction_INVALIDATED,
		}

		if err := svc.MarkRecordsAsUndone(context.Background(), aggregate.ID, action); err != nil {
			err = fmt.Errorf("error marking records as undone: %w", err)
		}
	}()

	for studentID, states := range aggregate.GetRecordStates() {
		finalState := states.RecordActions[len(states.RecordActions)-1]
		if finalState.Reason != eda.BulkUpload_RecordAction_PROCESSING {
			continue
		}

		studentIDUint, err := strconv.ParseUint(studentID, 10, 64)
		if err != nil {
			return fmt.Errorf("error parsing student ID %s: %w", studentID, err)
		}

		if err := d.services.StudentService.RemoveHealthAssessment(ctx, studentIDUint, aggregate.GetID()); err != nil {
			// permit undoing if the health assessment was not found. just log it.
			if errors.Is(err, student.ErrHealthAssessmentNotFound) {
				slog.Error("error removing health assessment for student ID %d: %w", studentID, err)
				continue
			}
			return fmt.Errorf("error deleting health assessment for student ID %d: %w", studentIDUint, err)
		}

		recordsUpdated = append(recordsUpdated, studentID)
	}

	return nil
}
