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

type wrappedEvent struct {
	event gosignal.Event
	data  proto.Message
}

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

func (sa *StudentAggregate) Apply(evt gosignal.Event) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("panic: %v", e)
		}

		if err != nil {
			err = fmt.Errorf("when processing event %q student aggregate %q: %w", evt.Type, evt.AggregateID, err)
		}
	}()

	if evt.Version != sa.version+1 {
		return fmt.Errorf("expected version %d, got %d: %w", sa.version+1, evt.Version, ErrVersionMismatch)
	}

	sa.version = evt.Version

	var eventData proto.Message
	var handler func(wrappedEvent) error

	switch evt.Type {
	case EVENT_ADD_STUDENT:
		eventData = &student.AddStudentEvent{}
		handler = sa.HandleCreateStudent
	case EVENT_SET_STUDENT_STATUS:
		eventData = &student.SetStudentStatusEvent{}
		handler = sa.HandleSetStudentStatus
	default:
		return ErrEventNotFound
	}

	if err := proto.Unmarshal(evt.Data, eventData); err != nil {
		return fmt.Errorf("error unmarshalling event data: %s", err)
	}

	wevt := wrappedEvent{event: evt, data: eventData}

	return handler(wevt)
}

func (sa *StudentAggregate) CreateStudent(student *student.AddStudentEvent) (*gosignal.Event, error) {
	sa.id = uuid.New().String()
	return sa.ApplyEvent(EVENT_ADD_STUDENT, student)
}

func (sa *StudentAggregate) SetStudentStatus(status *student.SetStudentStatusEvent) (*gosignal.Event, error) {
	return sa.ApplyEvent(EVENT_SET_STUDENT_STATUS, status)
}

// HandleSetStudentStatus handles the SetStudentStatus event
func (sa *StudentAggregate) HandleSetStudentStatus(evt wrappedEvent) error {
	data := evt.data.(*student.SetStudentStatusEvent)

	if sa.data == nil {
		return fmt.Errorf("student not found")
	}

	sa.data.Status = data.Status

	return nil
}

func (sa *StudentAggregate) HandleCreateStudent(evt wrappedEvent) error {
	data := evt.data.(*student.AddStudentEvent)

	if sa.data != nil {
		return fmt.Errorf("student already exists")
	}

	sa.data = &student.StudentAggregate{
		FirstName:        data.FirstName,
		LastName:         data.LastName,
		DateOfBirth:      data.DateOfBirth,
		SchoolId:         data.SchoolId,
		DateOfEnrollment: data.DateOfEnrollment,
	}

	sa.id = evt.event.AggregateID

	return nil
}

func (sa *StudentAggregate) ApplyEvent(evtType string, msg proto.Message) (*gosignal.Event, error) {
	sBytes, err := proto.Marshal(msg)
	if err != nil {
		return nil, errors.Join(ErrMarshallingEvent, err)
	}

	evt := gosignal.Event{
		Type:        evtType,
		Timestamp:   time.Now(),
		Data:        sBytes,
		Version:     sa.version + 1,
		AggregateID: sa.id,
	}

	return &evt, sa.Apply(evt)
}

func (StudentAggregate) ImportState([]byte) error {
	panic("not implemented") // TODO: Implement
}
func (StudentAggregate) ExportState() ([]byte, error) {
	panic("not implemented") // TODO: Implement
}
func (sa StudentAggregate) ID() string {
	return sa.id
}

func (sa StudentAggregate) String() string {
	return fmt.Sprintf("ID: %s, Version: %d, Data: %+v", sa.id, sa.version, sa.data.String())
}

func (StudentAggregate) Version() uint {
	panic("not implemented") // TODO: Implement
}
