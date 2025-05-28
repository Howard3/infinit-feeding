package file

import (
	"errors"
	"geevly/gen/go/eda"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/sourcing"
	"google.golang.org/protobuf/proto"
)

// Event types
const (
	EventFileCreated = "FileCreated"
	EventFileDeleted = "FileDeleted"
)

// ErrFileDeleted is returned when the file is already deleted
var ErrFileDeleted = errors.New("file already deleted")

// FileEvent is a struct that holds the event type and the data
type FileEvent struct {
	eventType string
	data      proto.Message
	version   uint64
}

// File aggregate root
type Aggregate struct {
	sourcing.DefaultAggregate
	data *eda.File
}

// Apply event to the file
func (a *Aggregate) Apply(evt gosignal.Event) error {
	return sourcing.SafeApply(evt, a, a.routeEvent)
}

// Route event to the appropriate handler
func (a *Aggregate) routeEvent(evt gosignal.Event) error {
	switch evt.Type {
	case EventFileCreated:
		return a.onFileCreated(evt)
	case EventFileDeleted:
		return a.onFileDeleted(evt)
	default:
		return errors.New("unknown event type")
	}
}

func (a *Aggregate) onFileCreated(evt gosignal.Event) error {
	var eventData eda.File_Create_Event
	if err := proto.Unmarshal(evt.Data, &eventData); err != nil {
		return err
	}
	a.data = &eda.File{
		Name:                   eventData.Name,
		DomainReference:        eventData.DomainReference,
		MimeType:               eventData.MimeType,
		Size:                   eventData.Size,
		Extension:              eventData.Extension,
		Metadata:               eventData.Metadata,
		Deleted:                false,
		AssociatedBulkUploadId: eventData.AssociatedBulkUploadId,
	}
	return nil
}

// DeleteFile deletes a file
func (a *Aggregate) DeleteFile(cmd *eda.File_Delete) (*gosignal.Event, error) {
	if a.data == nil {
		return nil, errors.New("file not found")
	}

	if a.data.Deleted {
		return nil, ErrFileDeleted
	}

	return a.ApplyEvent(FileEvent{
		eventType: EventFileDeleted,
		data: &eda.File_Delete_Event{
			Metadata: cmd.Metadata,
		},
		version: cmd.Version,
	})
}

// CreateFile creates a new file
func (a *Aggregate) CreateFile(cmd *eda.File_Create) (*gosignal.Event, error) {
	if cmd == nil {
		return nil, errors.New("file data not provided")
	}

	if a.data != nil {
		return nil, errors.New("file already created")

	}

	return a.ApplyEvent(FileEvent{
		eventType: EventFileCreated,
		data: &eda.File_Create_Event{
			Name:                   cmd.Name,
			DomainReference:        cmd.DomainReference,
			MimeType:               cmd.MimeType,
			Size:                   cmd.Size,
			Extension:              cmd.Extension,
			Metadata:               cmd.Metadata,
			AssociatedBulkUploadId: cmd.AssociatedBulkUploadId,
		},
		version: 0,
	})
}

// ApplyEvent is a function that applies an event to the aggregate
func (sd *Aggregate) ApplyEvent(fEvt FileEvent) (*gosignal.Event, error) {
	sBytes, marshalErr := proto.Marshal(fEvt.data)

	evt := gosignal.Event{
		Type:        fEvt.eventType,
		Timestamp:   time.Now(),
		Data:        sBytes,
		Version:     fEvt.version,
		AggregateID: sd.GetID(),
	}

	return &evt, errors.Join(sd.Apply(evt), marshalErr)
}

func (a *Aggregate) onFileDeleted(evt gosignal.Event) error {
	a.data.Deleted = true
	return nil
}

func (aggregate *Aggregate) ImportState(_ []byte) error {
	panic("not implemented") // TODO: Implement
}
func (aggregate *Aggregate) ExportState() ([]byte, error) {
	panic("not implemented") // TODO: Implement
}
