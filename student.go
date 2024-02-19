package main

import (
	"errors"
	"fmt"
	student "geevly/events/gen/proto/go"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

var ErrEventNotFound = fmt.Errorf("event not found")
var ErrApplyingEvent = fmt.Errorf("error applying event")
var ErrMarshallingEvent = fmt.Errorf("error marshalling event")
var ErrVersionMismatch = fmt.Errorf("version mismatch")

const EVENT_ADD_STUDENT = "AddStudent"
const EVENT_SET_STUDENT_STATUS = "SetStudentStatus"

type StudentAggregate struct {
	data    *student.StudentAggregate
	version uint
	id      string
}

func (sa *StudentAggregate) GetID() string {
	return sa.id
}

func (sa *StudentAggregate) GetVersion() uint {
	return sa.version
}

func (sa *StudentAggregate) Apply(evt gosignal.Event) error {
	if evt.Version != sa.version+1 {
		return fmt.Errorf("expected version %d, got %d: %w", sa.version+1, evt.Version, ErrVersionMismatch)
	}

	sa.version = evt.Version

	switch evt.Type {
	case EVENT_ADD_STUDENT:
		return sa.HandleCreateStudent(evt)
	case EVENT_SET_STUDENT_STATUS:
		return sa.HandleSetStudentStatus(evt)
	default:
		return ErrEventNotFound
	}
}

func (sa *StudentAggregate) HandleCreateStudent(evt gosignal.Event) error {
	// unmarshal the event data
	var data student.AddStudentEvent
	err := proto.Unmarshal(evt.Data, &data)
	if err != nil {
		return fmt.Errorf("error unmarshalling event data: %s", err)
	}

	if sa.data != nil {
		return fmt.Errorf("student already exists: %s", evt.AggregateID)
	}

	sa.data = &student.StudentAggregate{
		FirstName:        data.FirstName,
		LastName:         data.LastName,
		DateOfBirth:      data.DateOfBirth,
		SchoolId:         data.SchoolId,
		DateOfEnrollment: data.DateOfEnrollment,
	}

	return nil
}

func (sa *StudentAggregate) CreateStudent(student *student.AddStudentEvent) (*gosignal.Event, error) {
	newID := uuid.New().String()

	sBytes, err := proto.Marshal(student)
	if err != nil {
		return nil, errors.Join(ErrMarshallingEvent, err)
	}
	// create the event
	evt := gosignal.Event{
		Type:        EVENT_ADD_STUDENT,
		Timestamp:   time.Now(),
		Data:        sBytes,
		Version:     1,
		AggregateID: newID,
	}

	// apply the event to the aggregate
	if err = sa.Apply(evt); err != nil {
		return nil, errors.Join(ErrApplyingEvent, err)
	}

	return &evt, nil
}

func (sa *StudentAggregate) SetStudentStatus(status *student.SetStudentStatusEvent) (*gosignal.Event, error) {
	sBytes, err := proto.Marshal(status)
	if err != nil {
		return nil, errors.Join(ErrMarshallingEvent, err)
	}

	// create the event
	evt := gosignal.Event{
		Type:        EVENT_SET_STUDENT_STATUS,
		Timestamp:   time.Now(),
		Data:        sBytes,
		Version:     sa.version + 1,
		AggregateID: sa.id,
	}

	// apply the event to the aggregate
	if err = sa.Apply(evt); err != nil {
		return nil, errors.Join(ErrApplyingEvent, err)
	}

	return &evt, nil
}

// HandleSetStudentStatus handles the SetStudentStatus event
func (sa *StudentAggregate) HandleSetStudentStatus(evt gosignal.Event) error {
	// unmarshal the event data
	var data student.SetStudentStatusEvent
	err := proto.Unmarshal(evt.Data, &data)
	if err != nil {
		return fmt.Errorf("error unmarshalling event data: %s", err)
	}

	if sa.data == nil {
		return fmt.Errorf("student not found: %s", evt.AggregateID)
	}

	sa.data.Status = data.Status

	return nil
}

func (StudentAggregate) ImportState([]byte) error {
	panic("not implemented") // TODO: Implement
}
func (StudentAggregate) ExportState() ([]byte, error) {
	panic("not implemented") // TODO: Implement
}
func (StudentAggregate) ID() string {
	panic("not implemented") // TODO: Implement
}
func (StudentAggregate) Version() uint {
	panic("not implemented") // TODO: Implement
}
