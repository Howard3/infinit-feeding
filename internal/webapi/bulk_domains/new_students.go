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
	"slices"
	"strconv"
	"strings"
	"time"

	"geevly/gen/go/eda"
	"geevly/internal/bulk_upload"
	"geevly/internal/file"
)

type newStudentReader struct {
	headerIndexes *newStudentHeaderIndexes
	zipReader     fs.FS
	csvReader     *csv.Reader
}

func (nsr *newStudentReader) getFSFromZip(data []byte) (fs.FS, error) {
	reader := bytes.NewReader(data)
	return zip.NewReader(reader, int64(len(data)))
}

func (nsr *newStudentReader) getCSVFromFS(zipFS fs.FS) (*csv.Reader, error) {
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

func (newStudentReader) getStudentPhotoPath(lrn string) string {
	return fmt.Sprintf("photos/%s.jpg", lrn)
}

func (nsr *newStudentReader) getStudentPhoto(lrn string) ([]byte, error) {
	data, err := fs.ReadFile(nsr.zipReader, nsr.getStudentPhotoPath(lrn))
	if err != nil {
		return nil, fmt.Errorf("reading student photo from zip: %w", err)
	}
	return data, nil
}

func createNewStudentReader(data []byte) (*newStudentReader, *ValidationResult) {
	nsr := &newStudentReader{}

	// This is a placeholder implementation that will be enhanced later
	result := &ValidationResult{
		IsValid: true,
		Errors:  []*eda.BulkUpload_ValidationError{},
	}

	// Get the file system from the zip archive
	zipReader, err := nsr.getFSFromZip(data)
	if err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
			Field:   "file",
			Message: fmt.Sprintf("Failed to extract file system from zip: %v", err),
		})
		return nil, result
	}

	// Get the CSV file from the file system
	csvReader, err := nsr.getCSVFromFS(zipReader)
	if err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
			Field:   "file",
			Message: fmt.Sprintf("Failed to open CSV file: %v", err),
		})
		return nil, result
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
	headerIndexes := newStudentHeaderIndexes{}
	missingColumns := headerIndexes.ValidateHeaders(header)
	if len(missingColumns) > 0 {
		result.IsValid = false
		result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
			Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
			Field:   "headers",
			Message: fmt.Sprintf("Missing required columns: %s", strings.Join(missingColumns, ", ")),
		})
	}

	nsr.csvReader = csvReader
	nsr.headerIndexes = &headerIndexes
	nsr.zipReader = zipReader

	return nsr, result
}

type newStudentHeaderIndexes struct {
	firstName   int
	lastName    int
	lrn         int
	gradeLevel  int
	dob         int
	gender      int
	status      int
	sponsorship int
}

func (h *newStudentHeaderIndexes) ValidateHeaders(headers []string) []string {
	missingHeaders := []string{}
	requiredHeaders := []string{
		"First Name",
		"Last Name",
		"LRN",
		"Grade Level",
		"Date of Birth",
		"Gender",
		"Status",
		"Sponsorship",
	}

	for _, validHeader := range requiredHeaders {
		headerIndex := slices.Index(headers, validHeader)
		if headerIndex == -1 {
			missingHeaders = append(missingHeaders, validHeader)
		}

		switch validHeader {
		case "First Name":
			h.firstName = headerIndex
		case "Last Name":
			h.lastName = headerIndex
		case "LRN":
			h.lrn = headerIndex
		case "Grade Level":
			h.gradeLevel = headerIndex
		case "Date of Birth":
			h.dob = headerIndex
		case "Gender":
			h.gender = headerIndex
		case "Status":
			h.status = headerIndex
		case "Sponsorship":
			h.sponsorship = headerIndex
		}
	}

	return missingHeaders
}

func (h *newStudentHeaderIndexes) getFirstName(row []string) string {
	return row[h.firstName]
}

func (h *newStudentHeaderIndexes) getLastName(row []string) string {
	return row[h.lastName]
}

func (h *newStudentHeaderIndexes) getLRN(row []string) string {
	return row[h.lrn]
}

func (h *newStudentHeaderIndexes) getGradeLevel(row []string) uint64 {
	gradeLevel := row[h.gradeLevel]
	gradeLevelUint, err := strconv.ParseUint(gradeLevel, 10, 64)
	if err != nil {
		return 0
	}

	return gradeLevelUint
}

func (h *newStudentHeaderIndexes) getDOB(row []string) *eda.Date {
	dob := row[h.dob]

	parsed, err := time.Parse("20060102", dob)
	if err != nil {
		return nil
	}

	return &eda.Date{
		Year:  int32(parsed.Year()),
		Month: int32(parsed.Month()),
		Day:   int32(parsed.Day()),
	}
}

func (h *newStudentHeaderIndexes) getGender(row []string) eda.Student_Sex {
	switch row[h.gender] {
	case "M":
		return eda.Student_MALE
	case "F":
		return eda.Student_FEMALE
	default:
		return eda.Student_UNKNOWN_SEX
	}
}

func (h *newStudentHeaderIndexes) getStatus(row []string) eda.Student_Status {
	switch row[h.status] {
	case "Active":
		return eda.Student_ACTIVE
	case "Inactive":
		return eda.Student_INACTIVE
	default:
		return eda.Student_UNKNOWN_STATUS
	}
}

func (h *newStudentHeaderIndexes) isEligibleForSponsorship(row []string) bool {
	return row[h.sponsorship] == "Eligible"
}

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
	newStudentReader, result := createNewStudentReader(fileBytes)

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
		schoolIDUint, err := strconv.ParseUint(schoolIDStr, 10, 64)
		if err != nil {
			result.IsValid = false
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
				Field:   "school_id",
				Message: "Invalid school ID",
			})
		}
		if err := d.services.SchoolService.ValidateSchoolID(ctx, schoolIDUint); err != nil {
			result.IsValid = false
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context: eda.BulkUpload_ValidationError_METADATA_FIELD,
				Field:   "school_id",
				Message: "Invalid school ID",
			})
		}
	}

	if errs := d.validateRows(ctx, newStudentReader, schoolIDStr); len(errs) > 0 {
		result.IsValid = false
		for _, err := range errs {
			result.Errors = append(result.Errors, &eda.BulkUpload_ValidationError{
				Context: eda.BulkUpload_ValidationError_CSV_DATA,
				Field:   "rows",
				Message: err.Error(),
			})
		}
	}

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

func (d *NewStudentsDomain) processingMarkRecordsCreated(bulkUploadID string, students, files []string, svc *bulk_upload.Service, logger *slog.Logger) {
	studentActions := bulk_upload.RecordActions{
		RecordIds:  students,
		RecordType: eda.BulkUpload_STUDENT,
		Reason:     eda.BulkUpload_RecordAction_PROCESSING,
	}

	fileActions := bulk_upload.RecordActions{
		RecordIds:  files,
		RecordType: eda.BulkUpload_FILE,
		Reason:     eda.BulkUpload_RecordAction_PROCESSING,
	}

	logger.Info("marking records as created", slog.Int("student count", len(students)), slog.Int("file count", len(files)))

	svc.MarkRecordsAsUpdated(context.Background(), bulkUploadID, studentActions)
	svc.MarkRecordsAsUpdated(context.Background(), bulkUploadID, fileActions)
}

func (d *NewStudentsDomain) markStudentsInvalidated(bulkUploadID string, students []string, svc *bulk_upload.Service, logger *slog.Logger) {
	studentActions := bulk_upload.RecordActions{
		RecordIds:  students,
		RecordType: eda.BulkUpload_STUDENT,
		Reason:     eda.BulkUpload_RecordAction_INVALIDATED,
	}

	logger.Info("marking students as invalidated", slog.Int("student count", len(students)))

	svc.MarkRecordsAsUndone(context.Background(), bulkUploadID, studentActions)
}

func (d *NewStudentsDomain) ProcessUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, svc *bulk_upload.Service, fileBytes []byte) error {
	nsr, results := createNewStudentReader(fileBytes)
	if !results.IsValid {
		return fmt.Errorf("invalid file")
	}

	// track records created
	studentsCreated := []string{}
	filesCreated := []string{}

	schoolID := aggregate.GetUploadMetadataField("school_id")
	processLogger := slog.With(slog.String("domain", "bulk_upload"), slog.String("subdomain", "new_students:processUpload"))

	defer func() {
		d.processingMarkRecordsCreated(aggregate.GetID(), studentsCreated, filesCreated, svc, processLogger)
	}()

	for {
		record, err := nsr.csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return d.errorHandler(processLogger, err, "when reading csv file")
		}
		logger := processLogger.With(slog.String("student_lrn", nsr.headerIndexes.getLRN(record)))

		logger.Info("bulk upload: creating student")
		newStudent, err := d.services.StudentService.CreateStudent(ctx, &eda.Student_Create{
			FirstName:              nsr.headerIndexes.getFirstName(record),
			LastName:               nsr.headerIndexes.getLastName(record),
			DateOfBirth:            nsr.headerIndexes.getDOB(record),
			StudentSchoolId:        nsr.headerIndexes.getLRN(record),
			Sex:                    nsr.headerIndexes.getGender(record),
			GradeLevel:             nsr.headerIndexes.getGradeLevel(record),
			AssociatedBulkUploadId: aggregate.GetID(),
		})

		studentsCreated = append(studentsCreated, newStudent.GetID())

		if err != nil {
			return d.errorHandler(logger, err, "when creating a new student")
		}

		logger.Info("getting student's photo")
		photo, err := nsr.getStudentPhoto(nsr.headerIndexes.getLRN(record))
		if err != nil {
			return d.errorHandler(logger, err, "when getting student's photo")
		}

		logger.Info("saving photo")

		fileID, err := d.services.FileService.CreateFile(ctx, photo, &eda.File_Create{
			Name:                   "profile_photo",
			DomainReference:        eda.File_STUDENT_PROFILE_PHOTO,
			AssociatedBulkUploadId: aggregate.GetID(),
		})

		if err != nil {
			return d.errorHandler(logger, err, "when saving profile photo")
		}

		filesCreated = append(filesCreated, fileID)

		newStudent, err = d.services.StudentService.RunCommand(ctx, newStudent.GetIDUint64(), &eda.Student_SetProfilePhoto{
			FileId:  fileID,
			Version: newStudent.GetVersion(),
		})

		if err != nil {
			return d.errorHandler(logger, err, "when applying photo to student")
		}

		// enroll the student in the school w/ today's date
		now := time.Now()
		newStudent, err = d.services.StudentService.RunCommand(ctx, newStudent.GetIDUint64(), &eda.Student_Enroll{
			SchoolId: schoolID,
			DateOfEnrollment: &eda.Date{
				Year:  int32(now.Year()),
				Month: int32(now.Month()),
				Day:   int32(now.Day()),
			},
			Version: newStudent.GetVersion(),
		})

		if err != nil {
			return d.errorHandler(logger, err, "when enrolling student")
		}

		// set active status
		newStudent, err = d.services.StudentService.RunCommand(ctx, newStudent.GetIDUint64(), &eda.Student_SetStatus{
			Version: newStudent.GetVersion(),
			Status:  nsr.headerIndexes.getStatus(record),
		})

		if err != nil {
			return d.errorHandler(logger, err, "when setting active status")
		}

		// set sponsorship status
		if nsr.headerIndexes.isEligibleForSponsorship(record) {
			newStudent, err = d.services.StudentService.RunCommand(ctx, newStudent.GetIDUint64(), &eda.Student_SetEligibility{
				Version:  newStudent.GetVersion(),
				Eligible: nsr.headerIndexes.isEligibleForSponsorship(record),
			})
			if err != nil {
				return d.errorHandler(logger, err, "when setting sponsorship status")
			}
		}
	}

	return nil
}

func (d *NewStudentsDomain) errorHandler(logger *slog.Logger, err error, contextMessage string) error {
	err = fmt.Errorf("%s:%w", contextMessage, err)
	logger.Error(err.Error())
	return err
}

// TODO: handle errors and log in the aggregate
// TODO: delete uploaded photos as well
func (d *NewStudentsDomain) UndoUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, svc *bulk_upload.Service) error {
	recordsUpdated := make([]string, 0)
	processLogger := slog.With(slog.String("domain", "bulk_upload"), slog.String("subdomain", "new_students:undoUpload"))

	defer func() { d.markStudentsInvalidated(aggregate.GetID(), recordsUpdated, svc, processLogger) }()

	for studentID, states := range aggregate.GetRecordStates() {
		logger := processLogger.With(slog.String("student_id", studentID))
		finalState := states.RecordActions[len(states.RecordActions)-1]

		if finalState.RecordType != eda.BulkUpload_STUDENT {
			continue
		}

		if finalState.Reason != eda.BulkUpload_RecordAction_PROCESSING {
			logger.Error("invalid state", "state", finalState)
			continue
		}

		studentIDUint, err := strconv.ParseUint(studentID, 10, 64)
		if err != nil {
			d.errorHandler(logger, err, "failed to parse student ID")
			continue
		}

		if err := d.services.StudentService.DeleteStudent(ctx, studentIDUint, aggregate.GetID()); err != nil {
			d.errorHandler(logger, err, "failed to delete student")
			continue
		}

		recordsUpdated = append(recordsUpdated, studentID)
	}

	return nil
}

func (d *NewStudentsDomain) validateRows(ctx context.Context, newStudentReader *newStudentReader, schoolID string) []error {
	errors := make([]error, 0)
	for {
		record, err := newStudentReader.csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errors = append(errors, fmt.Errorf("error reading csv file: %w", err))
			continue
		}

		lrn := newStudentReader.headerIndexes.getLRN(record)

		slog.Info("validating student w/ lrn", "lrn", lrn)
		// Check for duplicate LRNs
		if _, err := d.services.StudentService.GetStudentByStudentAndSchoolID(ctx, lrn, schoolID); err == nil {
			errors = append(errors, fmt.Errorf("duplicate LRN: %s for school %s", lrn, schoolID))
			continue
		}

		if newStudentReader.headerIndexes.getDOB(record) == nil {
			errors = append(errors, fmt.Errorf("missing or invalid date of birth for student with LRN %s", lrn))
			continue
		}

		sex := newStudentReader.headerIndexes.getGender(record)
		if sex == eda.Student_UNKNOWN_SEX {
			errors = append(errors, fmt.Errorf("missing or invalid sex for student with LRN %s", lrn))
			continue
		}

		if newStudentReader.headerIndexes.getGradeLevel(record) == 0 {
			errors = append(errors, fmt.Errorf("missing or invalid grade level for student with LRN %s", lrn))
			continue
		}

		// TODO: right now we validate a student by the school+student ID, are student id's universally unique?

		photoFileName := newStudentReader.getStudentPhotoPath(lrn)
		photoStat, err := fs.Stat(newStudentReader.zipReader, photoFileName)
		if err != nil {
			errors = append(errors, fmt.Errorf("error checking photo file: %w", err))
			continue
		}

		slog.Info("bulk upload: found photo for new student", "lrn", lrn, "photo", photoStat.Name(), "size", photoStat.Size())
	}

	return errors
}
