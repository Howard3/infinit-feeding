package bulk_upload

import (
	"errors"
	"fmt"
	"geevly/gen/go/eda"
	"slices"
	"strconv"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/sourcing"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	EventCreate                   = "BulkUploadCreate"
	EventAddValidationErrors      = "BulkUploadAddValidationErrors"
	EventSetStatus                = "BulkUploadSetStatus"
	EventMarkRecordsAsProcessed   = "BulkUploadMarkRecordsAsProcessed"
	EventAddRecordsToProcessEvent = "BulkUploadAddRecordsToProcess"
)

var permittedStatusChanges = map[eda.BulkUpload_Status][]eda.BulkUpload_Status{
	eda.BulkUpload_UNKNOWN:      {eda.BulkUpload_PENDING},
	eda.BulkUpload_PENDING:      {eda.BulkUpload_VALIDATING},
	eda.BulkUpload_VALIDATING:   {eda.BulkUpload_VALIDATED, eda.BulkUpload_VALIDATION_FAILED},
	eda.BulkUpload_VALIDATED:    {eda.BulkUpload_PROCESSING},
	eda.BulkUpload_PROCESSING:   {eda.BulkUpload_COMPLETED, eda.BulkUpload_ERROR},
	eda.BulkUpload_COMPLETED:    {eda.BulkUpload_INVALIDATING},
	eda.BulkUpload_INVALIDATING: {eda.BulkUpload_INVALIDATED, eda.BulkUpload_INVALIDATION_FAILED},
}

// BulkUploadEvent is a struct that holds the event type and the data
type BulkUploadEvent struct {
	eventType string
	data      proto.Message
	version   uint64
}

type Aggregate struct {
	sourcing.DefaultAggregate
	data *eda.BulkUpload
}

func GetStatusTimestamps(a *Aggregate) []*eda.BulkUpload_StatusTimestamp {
	if a.data == nil {
		return nil
	}
	return a.data.StatusTimestamps
}

func (a *Aggregate) GetRecordProcessedStatuses() map[string]bool {
	return a.data.RecordsProcessed
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

func (a *Aggregate) setStatus(status eda.BulkUpload_Status) (*gosignal.Event, error) {
	if a.data == nil {
		return nil, fmt.Errorf("bulk upload aggregate is not initialized")
	}

	// Check if the status transition is allowed
	currentStatus := a.data.Status
	allowedStatuses, exists := permittedStatusChanges[currentStatus]
	if !exists {
		return nil, fmt.Errorf("no permitted status transitions defined for current status: %s", currentStatus.String())
	}

	isAllowed := slices.Contains(allowedStatuses, status)
	if !isAllowed {
		return nil, fmt.Errorf("transition from %s to %s is not allowed", currentStatus.String(), status.String())
	}

	return a.ApplyEvent(BulkUploadEvent{
		eventType: EventSetStatus,
		data: &eda.BulkUpload_SetStatusEvent{
			Status:          status,
			StatusTimestamp: timestamppb.Now(),
		},
		version: a.Version,
	})
}

func (a *Aggregate) addRecordsToProcess(recordIds []string) (*gosignal.Event, error) {
	if a.data == nil {
		return nil, fmt.Errorf("bulk upload aggregate is not initialized")
	}

	return a.ApplyEvent(BulkUploadEvent{
		eventType: EventAddRecordsToProcessEvent,
		data: &eda.BulkUpload_AddRecordsToProcessEvent{
			RecordIds: recordIds,
		},
		version: a.Version,
	})
}

func (a *Aggregate) markRecordsAsProcessed(recordIds []string) (*gosignal.Event, error) {
	if a.data == nil {
		return nil, fmt.Errorf("bulk upload aggregate is not initialized")
	}

	return a.ApplyEvent(BulkUploadEvent{
		eventType: EventMarkRecordsAsProcessed,
		data: &eda.BulkUpload_MarkRecordsAsProcessed{
			RecordIds: recordIds,
		},
		version: a.Version,
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

func (a *Aggregate) GetUploadMetadataField(field string) string {
	return a.data.UploadMetadata[field]
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
	case EventAddValidationErrors:
		eventData = &eda.BulkUpload_ValidationError_Event{}
		handler = a.handleAddValidationErrors
	case EventSetStatus:
		eventData = &eda.BulkUpload_SetStatusEvent{}
		handler = a.handleSetStatus
	case EventMarkRecordsAsProcessed:
		eventData = &eda.BulkUpload_MarkRecordsAsProcessed{}
		handler = a.handleMarkRecordsAsProcessed
	case EventAddRecordsToProcessEvent:
		eventData = &eda.BulkUpload_AddRecordsToProcessEvent{}
		handler = a.handleAddRecordsToProcess
	default:
		return fmt.Errorf("unknown event type: %s", evt.Type)
	}

	if err := proto.Unmarshal(evt.Data, eventData); err != nil {
		return fmt.Errorf("error unmarshalling event data: %w", err)
	}

	return handler(eventData)
}

func (a *Aggregate) handleAddRecordsToProcess(event proto.Message) error {
	evt := event.(*eda.BulkUpload_AddRecordsToProcessEvent)

	for _, record := range evt.RecordIds {
		if _, exists := a.data.RecordsProcessed[record]; exists {
			continue
		}
		a.data.RecordsProcessed[record] = false
	}

	return nil
}

func (a *Aggregate) handleMarkRecordsAsProcessed(event proto.Message) error {
	evt := event.(*eda.BulkUpload_MarkRecordsAsProcessed)

	for _, record := range evt.RecordIds {
		if _, exists := a.data.RecordsProcessed[record]; !exists {
			continue
		}
		a.data.RecordsProcessed[record] = true
	}

	return nil
}

func (a *Aggregate) handleSetStatus(event proto.Message) error {
	evt := event.(*eda.BulkUpload_SetStatusEvent)

	a.data.Status = evt.Status

	// Add timestamp for status change
	a.data.StatusTimestamps = append(a.data.StatusTimestamps, &eda.BulkUpload_StatusTimestamp{
		Status:    evt.Status,
		Timestamp: evt.GetStatusTimestamp(),
	})

	return nil
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
	case eda.BulkUpload_HEALTH_ASSESSMENT:
		if err := a.validateHealthAssessment(cmd); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("agg:create - invalid target domain")
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

func (a *Aggregate) validateHealthAssessment(cmd *eda.BulkUpload_Create) error {
	switch {
	case cmd.UploadMetadata == nil:
		return errors.New("upload metadata is required")
	case cmd.UploadMetadata["school_id"] == "":
		return errors.New("school id is required")
	default:
		return nil
	}
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
		Id:               a.ID,
		Status:           eda.BulkUpload_PENDING,
		TargetDomain:     evt.TargetDomain,
		Metadata:         evt.Metadata,
		FileId:           evt.FileId,
		UploadMetadata:   evt.UploadMetadata,
		RecordsProcessed: make(map[string]bool),
		StatusTimestamps: []*eda.BulkUpload_StatusTimestamp{
			{
				Status:    eda.BulkUpload_PENDING,
				Timestamp: evt.GetInitiatedAt(),
			},
		},
	}

	return nil
}
