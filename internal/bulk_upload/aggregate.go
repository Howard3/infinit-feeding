package bulk_upload

import (
	"errors"
	"fmt"
	"geevly/gen/go/eda"
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
)

type Aggregate struct {
	sourcing.DefaultAggregate
	data *eda.BulkUpload
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
	default:
		return fmt.Errorf("unknown event type: %s", evt.Type)
	}

	if err := proto.Unmarshal(evt.Data, eventData); err != nil {
		return fmt.Errorf("error unmarshalling event data: %w", err)
	}

	return handler(eventData)
}

func (a *Aggregate) handleStartProcessing(event proto.Message) error {
	panic("not implemented")
}

func (a *Aggregate) handleCompleteProcessing(event proto.Message) error {
	panic("not implemented")
}

func (a *Aggregate) handleFail(event proto.Message) error {
	panic("not implemented")
}

func (a *Aggregate) handleStartInvalidation(event proto.Message) error {
	panic("not implemented")
}

func (a *Aggregate) handleCompleteInvalidation(event proto.Message) error {
	panic("not implemented")
}

func (a *Aggregate) handleComplete(event proto.Message) error {
	panic("not implemented")
}

func (a *Aggregate) handleUpdateProgress(event proto.Message) error {
	panic("not implemented")
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
