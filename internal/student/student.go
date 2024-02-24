package student

import (
	"errors"
	"fmt"
	"time"

	"geevly/gen/go/eda"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/sourcing"
	"google.golang.org/protobuf/proto"
)

var ErrEventNotFound = fmt.Errorf("event not found")
var ErrApplyingEvent = fmt.Errorf("error applying event")
var ErrMarshallingEvent = fmt.Errorf("error marshalling event")
var ErrVersionMismatch = fmt.Errorf("version mismatch")

const EVENT_ADD_STUDENT = "AddStudent"
const EVENT_SET_STUDENT_STATUS = "SetStudentStatus"
const EVENT_UPDATE_STUDENT = "UpdateStudent"
const EVENT_ENROLL_STUDENT = "EnrollStudent"

type wrappedEvent struct {
	event gosignal.Event
	data  proto.Message
}

type Student struct {
	sourcing.DefaultAggregate
	data *eda.Student
}

// Apply is called when an event is applied to the aggregate, it should be called from the
// repository when applying new events or from commands as they're issued
func (sd *Student) Apply(evt gosignal.Event) error {
	return sourcing.SafeApply(evt, sd, sd.routeEvent)
}

// Apply is called when an event is applied to the aggregate, it should be called from the
// root aggregate's Apply method, where checks for versioning are done
func (sd *Student) routeEvent(evt gosignal.Event) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("panic: %v", e)
		}

		if err != nil {
			err = fmt.Errorf("when processing event %q student aggregate %q: %w", evt.Type, evt.AggregateID, err)
		}
	}()

	var eventData proto.Message
	var handler func(wrappedEvent) error

	switch evt.Type {
	case EVENT_ADD_STUDENT:
		eventData = &eda.Student_Create_Event{}
		handler = sd.HandleCreateStudent
	case EVENT_SET_STUDENT_STATUS:
		eventData = &eda.Student_SetStatus_Event{}
		handler = sd.HandleSetStudentStatus
	case EVENT_UPDATE_STUDENT:
		eventData = &eda.Student_Update_Event{}
		handler = sd.HandleUpdateStudent
	case EVENT_ENROLL_STUDENT:
		eventData = &eda.Student_Enroll_Event{}
		handler = sd.HandleEnrollStudent
	default:
		return ErrEventNotFound
	}

	if err := proto.Unmarshal(evt.Data, eventData); err != nil {
		return fmt.Errorf("error unmarshalling event data: %s", err)
	}

	wevt := wrappedEvent{event: evt, data: eventData}

	return handler(wevt)
}

func (sd *Student) CreateStudent(cmd *eda.Student_Create) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_ADD_STUDENT,
		data: &eda.Student_Create_Event{
			FirstName:   cmd.FirstName,
			LastName:    cmd.LastName,
			DateOfBirth: cmd.DateOfBirth,
			Status:      eda.Student_INACTIVE,
		},
		version: 0,
	})
}

// SetStatus is a function that sets the status of a student, active or inactive
func (sd *Student) SetStatus(cmd *eda.Student_SetStatus) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_SET_STUDENT_STATUS,
		data:      &eda.Student_SetStatus_Event{Status: cmd.GetStatus()},
		version:   cmd.GetVersion(),
	})
}

func (sd *Student) UpdateStudent(cmd *eda.Student_Update) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_UPDATE_STUDENT,
		data: &eda.Student_Update_Event{
			FirstName:   cmd.FirstName,
			LastName:    cmd.LastName,
			DateOfBirth: cmd.DateOfBirth,
		},
		version: cmd.GetVersion(),
	})
}

func (sd *Student) EnrollStudent(cmd *eda.Student_Enroll) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_ENROLL_STUDENT,
		data: &eda.Student_Enroll_Event{
			SchoolId:         cmd.SchoolId,
			DateOfEnrollment: cmd.DateOfEnrollment,
		},
		version: cmd.GetVersion(),
	})
}

// HandleSetStudentStatus handles the SetStudentStatus event
func (sd *Student) HandleSetStudentStatus(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_SetStatus_Event)

	if sd.data == nil {
		return fmt.Errorf("student not found")
	}

	sd.data.Status = data.Status

	return nil
}

func (sd *Student) HandleCreateStudent(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_Create_Event)

	if sd.data != nil {
		return fmt.Errorf("student already exists")
	}

	sd.data = &eda.Student{
		FirstName:   data.FirstName,
		LastName:    data.LastName,
		DateOfBirth: data.DateOfBirth,
	}

	return nil
}

func (sd *Student) HandleUpdateStudent(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_Update_Event)

	if sd.data == nil {
		return fmt.Errorf("student not found")
	}

	sd.data.FirstName = data.FirstName
	sd.data.LastName = data.LastName
	sd.data.DateOfBirth = data.DateOfBirth

	return nil
}

func (sd *Student) HandleEnrollStudent(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_Enroll_Event)

	if sd.data == nil {
		return fmt.Errorf("student not found")
	}

	sd.data.SchoolId = data.SchoolId
	sd.data.DateOfEnrollment = data.DateOfEnrollment

	return nil
}

// StudentEvent is a struct that holds the event type and the data
type StudentEvent struct {
	eventType string
	data      proto.Message
	version   uint64
}

// ApplyEvent is a function that applies an event to the aggregate
func (sd *Student) ApplyEvent(sEvt StudentEvent) (*gosignal.Event, error) {
	sBytes, marshalErr := proto.Marshal(sEvt.data)

	evt := gosignal.Event{
		Type:        sEvt.eventType,
		Timestamp:   time.Now(),
		Data:        sBytes,
		Version:     sEvt.version,
		AggregateID: sd.GetID(),
	}

	return &evt, errors.Join(sd.Apply(evt), marshalErr)
}

func (sd *Student) ImportState(data []byte) error {
	student := eda.Student{}

	if err := proto.Unmarshal(data, &student); err != nil {
		return fmt.Errorf("error unmarshalling snapshot data: %s", err)
	}

	sd.data = &student

	return nil
}
func (sd *Student) ExportState() ([]byte, error) {
	return proto.Marshal(sd.data)
}

func (sd Student) String() string {
	id := sd.GetID()
	ver := sd.GetVersion()

	return fmt.Sprintf("ID: %s, Version: %d, Data: %+v", id, ver, sd.data.String())
}

func (sd Student) GetStudent() *eda.Student {
	return sd.data
}
