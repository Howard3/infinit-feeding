package bulk_upload

import (
	"errors"
	"fmt"
	"geevly/gen/go/eda"
	"strconv"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/sourcing"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	EventCreate               = "BulkUploadCreate"
	EventStartProcessing      = "BulkUploadStartProcessing"
	EventUpdateProgress       = "BulkUploadUpdateProgress"
	EventComplete             = "BulkUploadComplete"
	EventStartInvalidation    = "BulkUploadStartInvalidation"
	EventCompleteInvalidation = "BulkUploadCompleteInvalidation"
	EventFail                 = "BulkUploadFail"
	EventAddValidationErrors  = "BulkUploadAddValidationErrors"
)

type Aggregate struct {
	sourcing.DefaultAggregate
	data *eda.BulkUpload
}

func (a *Aggregate) GetValidationErrors() []*eda.BulkUpload_ValidationError {
	if a.data == nil {
		return nil
	}
	return a.data.ValidationErrors
}

func (a *Aggregate) GetStatus() eda.BulkUpload_Status {
	if a.data == nil {
		return eda.BulkUpload_UNKNOWN
	}
	return a.data.Status
}

func (a *Aggregate) GetDomain() eda.BulkUpload_Domain {
	return a.data.GetTargetDomain()
}

// AddValidationErrors adds validation errors to the aggregate
func (a *Aggregate) AddValidationErrors(errors []*eda.BulkUpload_ValidationError) (*gosignal.Event, error) {
	if a.data == nil {
		return nil, fmt.Errorf("bulk upload aggregate is not initialized")
	}

	return a.ApplyEvent(BulkUploadEvent{
		eventType: EventAddValidationErrors,
		data: &eda.BulkUpload_ValidationError_Event{
			Errors: errors,
		},
		version: a.Version,
	})
}

// StartProcessing initiates the processing of a bulk upload
func (a *Aggregate) StartProcessing(cmd *eda.BulkUpload_StartProcessing) (*gosignal.Event, error) {
	if a.data == nil {
		return nil, errors.New("bulk upload does not exist")
	}

	if a.data.Status != eda.BulkUpload_PENDING {
		return nil, fmt.Errorf("cannot start processing, current status: %s", a.data.Status.String())
	}

	return a.ApplyEvent(BulkUploadEvent{
		eventType: EventStartProcessing,
		data: &eda.BulkUpload_StartProcessing_Event{
			Status:       eda.BulkUpload_VALIDATING,
			TotalRecords: cmd.TotalRecords,
			StartedAt:    timestamppb.Now(),
		},
		version: cmd.Version,
	})
}

// UpdateProgress updates the progress of a bulk upload
func (a *Aggregate) UpdateProgress(cmd *eda.BulkUpload_UpdateProgress) (*gosignal.Event, error) {
	if a.data == nil {
		return nil, errors.New("bulk upload does not exist")
	}

	if a.data.Status != eda.BulkUpload_VALIDATING && a.data.Status != eda.BulkUpload_PROCESSING {
		return nil, fmt.Errorf("cannot update progress, current status: %s", a.data.Status.String())
	}

	return a.ApplyEvent(BulkUploadEvent{
		eventType: EventUpdateProgress,
		data: &eda.BulkUpload_UpdateProgress_Event{
			ProcessedRecords: cmd.ProcessedRecords,
			ValidationErrors: cmd.ValidationErrors,
		},
		version: cmd.Version,
	})
}

func (a *Aggregate) GetFileID() string {
	return a.data.FileId
}

func (a *Aggregate) CountTotalRecords() uint64 {
	return a.data.TotalRecords
}

func (a *Aggregate) CountProcessedRecords() uint64 {
	return a.data.ProcessedRecords
}

func (a *Aggregate) GetUploadMetadata() map[string]string {
	return a.data.UploadMetadata
}

// Apply event to the bulk upload
func (a *Aggregate) Apply(evt gosignal.Event) error {
	return sourcing.SafeApply(evt, a, a.routeEvent)
}

func (a *Aggregate) ExportState() ([]byte, error) {
	return proto.Marshal(a.data)
}

func (a *Aggregate) ImportState(data []byte) error {
	return proto.Unmarshal(data, a.data)
}

// Route event to the appropriate handler
func (a *Aggregate) routeEvent(evt gosignal.Event) error {
	var eventData proto.Message
	var handler func(proto.Message) error

	switch evt.Type {
	case EventCreate:
		eventData = &eda.BulkUpload_Create_Event{}
		handler = a.handleCreate
	case EventStartProcessing:
		eventData = &eda.BulkUpload_StartProcessing_Event{}
		handler = a.handleStartProcessing
	case EventUpdateProgress:
		eventData = &eda.BulkUpload_UpdateProgress_Event{}
		handler = a.handleUpdateProgress
	case EventComplete:
		eventData = &eda.BulkUpload_Complete_Event{}
		handler = a.handleComplete
	case EventStartInvalidation:
		eventData = &eda.BulkUpload_StartInvalidation_Event{}
		handler = a.handleStartInvalidation
	case EventCompleteInvalidation:
		eventData = &eda.BulkUpload_CompleteInvalidation_Event{}
		handler = a.handleCompleteInvalidation
	case EventFail:
		eventData = &eda.BulkUpload_Fail_Event{}
		handler = a.handleFail
	case EventAddValidationErrors:
		eventData = &eda.BulkUpload_ValidationError_Event{}
		handler = a.handleAddValidationErrors
	default:
		return fmt.Errorf("unknown event type: %s", evt.Type)
	}

	if err := proto.Unmarshal(evt.Data, eventData); err != nil {
		return fmt.Errorf("error unmarshalling event data: %w", err)
	}

	return handler(eventData)
}

func (a *Aggregate) handleAddValidationErrors(event proto.Message) error {
	evt := event.(*eda.BulkUpload_ValidationError_Event)

	// Update status to VALIDATION_FAILED
	a.data.Status = eda.BulkUpload_VALIDATION_FAILED

	// Add timestamp for status change
	a.data.StatusTimestamps = append(a.data.StatusTimestamps, &eda.BulkUpload_StatusTimestamp{
		Status:    eda.BulkUpload_VALIDATION_FAILED,
		Timestamp: timestamppb.Now(),
	})

	for _, error := range evt.Errors {
		a.data.ValidationErrors = append(a.data.ValidationErrors, error)
	}

	return nil
}

func (a *Aggregate) handleStartProcessing(event proto.Message) error {
	evt := event.(*eda.BulkUpload_StartProcessing_Event)

	// Update status to VALIDATING
	a.data.Status = eda.BulkUpload_VALIDATING

	// Add timestamp for status change
	a.data.StatusTimestamps = append(a.data.StatusTimestamps, &eda.BulkUpload_StatusTimestamp{
		Status:    eda.BulkUpload_VALIDATING,
		Timestamp: timestamppb.Now(),
	})

	// Set total records
	a.data.TotalRecords = evt.TotalRecords

	return nil
}

func (a *Aggregate) handleCompleteProcessing(event proto.Message) error {
	panic("not implemented")
}

func (a *Aggregate) handleFail(event proto.Message) error {
	// Update status to VALIDATION_FAILED
	a.data.Status = eda.BulkUpload_VALIDATION_FAILED

	// Add timestamp for status change
	a.data.StatusTimestamps = append(a.data.StatusTimestamps, &eda.BulkUpload_StatusTimestamp{
		Status:    eda.BulkUpload_VALIDATION_FAILED,
		Timestamp: timestamppb.Now(),
	})

	return nil
}

func (a *Aggregate) handleStartInvalidation(event proto.Message) error {
	panic("not implemented")
}

func (a *Aggregate) handleCompleteInvalidation(event proto.Message) error {
	panic("not implemented")
}

func (a *Aggregate) handleComplete(event proto.Message) error {
	evt := event.(*eda.BulkUpload_Complete_Event)

	// Update status to COMPLETED
	a.data.Status = eda.BulkUpload_COMPLETED

	// Add timestamp for status change
	a.data.StatusTimestamps = append(a.data.StatusTimestamps, &eda.BulkUpload_StatusTimestamp{
		Status:    eda.BulkUpload_COMPLETED,
		Timestamp: evt.CompletedAt,
	})

	return nil
}

func (a *Aggregate) handleUpdateProgress(event proto.Message) error {
	evt := event.(*eda.BulkUpload_UpdateProgress_Event)

	// Update processed records count
	a.data.ProcessedRecords = evt.ProcessedRecords

	// Add any validation errors
	if len(evt.ValidationErrors) > 0 {
		a.data.ValidationErrors = append(a.data.ValidationErrors, evt.ValidationErrors...)
	}

	return nil
}

// BulkUploadEvent is a struct that holds the event type and the data
type BulkUploadEvent struct {
	eventType string
	data      proto.Message
	version   uint64
}

// ApplyEvent is a function that applies an event to the aggregate
func (a *Aggregate) ApplyEvent(evt BulkUploadEvent) (*gosignal.Event, error) {
	bytes, err := proto.Marshal(evt.data)
	if err != nil {
		return nil, fmt.Errorf("error marshalling event data: %w", err)
	}

	event := gosignal.Event{
		Type:        evt.eventType,
		Data:        bytes,
		Timestamp:   time.Now(),
		Version:     evt.version,
		AggregateID: a.ID,
	}

	return &event, a.Apply(event)
}

func (a *Aggregate) Create(cmd *eda.BulkUpload_Create) (*gosignal.Event, error) {
	if a.data != nil {
		return nil, errors.New("bulk upload already exists")
	}

	switch cmd.TargetDomain {
	case eda.BulkUpload_NEW_STUDENTS:
		if err := a.validateNewStudents(cmd); err != nil {
			return nil, err
		}
	case eda.BulkUpload_GRADES:
		if err := a.validateGrades(cmd); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("invalid target domain")
	}

	return a.ApplyEvent(BulkUploadEvent{
		eventType: EventCreate,
		data: &eda.BulkUpload_Create_Event{
			TargetDomain:   cmd.TargetDomain,
			FileId:         cmd.FileId,
			Metadata:       cmd.Metadata,
			InitiatedAt:    timestamppb.Now(),
			UploadMetadata: cmd.UploadMetadata,
		},
	})
}

func (a *Aggregate) validateNewStudents(cmd *eda.BulkUpload_Create) error {
	switch {
	case cmd.UploadMetadata == nil:
		return errors.New("upload metadata is required")
	case cmd.UploadMetadata["school_id"] == "":
		return errors.New("school id is required")
	default:
		return nil
	}
}

func (a *Aggregate) validateGrades(cmd *eda.BulkUpload_Create) error {
	// First check if metadata exists
	if cmd.UploadMetadata == nil {
		return errors.New("upload metadata is required")
	}

	// Extract values from metadata
	schoolID := cmd.UploadMetadata["school_id"]
	schoolYear := cmd.UploadMetadata["school_year"]
	gradingPeriod := cmd.UploadMetadata["grading_period"]
	effectiveDate := cmd.UploadMetadata["effective_date"]

	// Check required fields
	if schoolID == "" {
		return errors.New("school id is required")
	}
	if schoolYear == "" {
		return errors.New("school year is required")
	}
	if gradingPeriod == "" {
		return errors.New("grading period is required")
	}
	if effectiveDate == "" {
		return errors.New("effective date is required")
	}

	// Validate numeric fields
	if _, err := strconv.Atoi(schoolID); err != nil {
		return errors.New("invalid school ID format")
	}
	if _, err := strconv.Atoi(gradingPeriod); err != nil {
		return errors.New("invalid grading period format")
	}

	// Validate date format
	if _, err := time.Parse("2006-01-02", effectiveDate); err != nil {
		return errors.New("invalid date format")
	}

	// Validate school year format (YYYY-YYYY)
	if len(schoolYear) != 9 || schoolYear[4] != '-' {
		return errors.New("invalid school year format")
	}

	startYear := schoolYear[:4]
	endYear := schoolYear[5:]

	// Check if the years are valid
	if _, err := strconv.Atoi(startYear); err != nil {
		return errors.New("invalid school year start")
	}
	if _, err := strconv.Atoi(endYear); err != nil {
		return errors.New("invalid school year end")
	}

	// Validate date is not in the future
	if effectiveDate > time.Now().Format("2006-01-02") {
		return errors.New("effective date cannot be in the future")
	}

	return nil
}

// Event handlers
func (a *Aggregate) handleCreate(msg proto.Message) error {
	evt := msg.(*eda.BulkUpload_Create_Event)

	a.data = &eda.BulkUpload{
		Id:             a.ID,
		Status:         eda.BulkUpload_PENDING,
		TargetDomain:   evt.TargetDomain,
		Metadata:       evt.Metadata,
		FileId:         evt.FileId,
		UploadMetadata: evt.UploadMetadata,
		StatusTimestamps: []*eda.BulkUpload_StatusTimestamp{
			{
				Status:    eda.BulkUpload_PENDING,
				Timestamp: timestamppb.Now(),
			},
		},
	}

	return nil
}
