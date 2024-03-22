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
var ErrStudentNotFound = fmt.Errorf("student not found")

const EVENT_ADD_STUDENT = "AddStudent"
const EVENT_SET_STUDENT_STATUS = "SetStudentStatus"
const EVENT_UPDATE_STUDENT = "UpdateStudent"
const EVENT_ENROLL_STUDENT = "EnrollStudent"
const EVENT_UNENROLL_STUDENT = "UnenrollStudent"
const EVENT_SET_LOOKUP_CODE = "SetLookupCode"
const EVENT_SET_PROFILE_PHOTO = "SetProfilePhoto"
const EVENT_FEED_STUDENT = "FeedStudent"

type wrappedEvent struct {
	event gosignal.Event
	data  proto.Message
}

type Aggregate struct {
	sourcing.DefaultAggregateUint64
	data *eda.Student
}

// Apply is called when an event is applied to the aggregate, it should be called from the
// repository when applying new events or from commands as they're issued
func (sd *Aggregate) Apply(evt gosignal.Event) error {
	return sourcing.SafeApply(evt, sd, sd.routeEvent)
}

// Apply is called when an event is applied to the aggregate, it should be called from the
// root aggregate's Apply method, where checks for versioning are done
func (sd *Aggregate) routeEvent(evt gosignal.Event) (err error) {
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
	case EVENT_UNENROLL_STUDENT:
		eventData = &eda.Student_Unenroll_Event{}
		handler = sd.HandleUnenrollStudent
	case EVENT_SET_LOOKUP_CODE:
		eventData = &eda.Student_SetLookupCode_Event{}
		handler = sd.handleSetLookupCode
	case EVENT_SET_PROFILE_PHOTO:
		eventData = &eda.Student_SetProfilePhoto_Event{}
		handler = sd.handleSetProfilePhoto
	case EVENT_FEED_STUDENT:
		eventData = &eda.Student_Feeding{}
		handler = sd.handleFeedStudent
	default:
		return ErrEventNotFound
	}

	if err := proto.Unmarshal(evt.Data, eventData); err != nil {
		return fmt.Errorf("error unmarshalling event data: %s", err)
	}

	// if this is not a new student, we should expect there to be data.
	if evt.Type != EVENT_ADD_STUDENT && sd.data == nil {
		return fmt.Errorf("when processing event %q %w", evt.Type, ErrStudentNotFound)
	}

	wevt := wrappedEvent{event: evt, data: eventData}

	return handler(wevt)
}

// feed - handles the feeding of a student
func (sd *Aggregate) Feed(timestamp, version uint64) (*gosignal.Event, error) {
	if len(sd.data.FeedingReport) > 0 {
		lastFeeding := sd.data.FeedingReport[len(sd.data.FeedingReport)-1]
		lastFeedingTimestamp := int64(lastFeeding.UnixTimestamp)
		thisFeedingTimestamp := int64(timestamp)
		now := time.Now().Unix()

		newerThanLastFeeding := thisFeedingTimestamp > lastFeedingTimestamp

		isNotInFuture := thisFeedingTimestamp <= now

		lastFeedingDay := time.Unix(lastFeedingTimestamp, 0).Day()
		thisFeedingDay := time.Unix(thisFeedingTimestamp, 0).Day()
		isNotSameDayAsLastFeeding := lastFeedingDay != thisFeedingDay

		switch {
		case !newerThanLastFeeding:
			return nil, fmt.Errorf("feeding timestamp is not newer than the last feeding")
		case !isNotInFuture:
			return nil, fmt.Errorf("feeding timestamp is in the future")
		case !isNotSameDayAsLastFeeding:
			return nil, fmt.Errorf("feeding timestamp is on the same day as the last feeding")
		}
	}

	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_FEED_STUDENT,
		data: &eda.Student_Feeding_Event{
			UnixTimestamp: uint64(timestamp),
		},
		version: version,
	})
}

func (sd *Aggregate) handleFeedStudent(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_Feeding)

	sd.data.FeedingReport = append(sd.data.FeedingReport, data)

	return nil
}

func (sd *Aggregate) CreateStudent(cmd *eda.Student_Create) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_ADD_STUDENT,
		data: &eda.Student_Create_Event{
			FirstName:   cmd.FirstName,
			LastName:    cmd.LastName,
			DateOfBirth: cmd.DateOfBirth,
			Status:      eda.Student_INACTIVE,
			Sex:         cmd.Sex,
		},
		version: 0,
	})
}

// SetCode
func (sd *Aggregate) SetLookupCode(cmd *eda.Student_SetLookupCode) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_SET_LOOKUP_CODE,
		data: &eda.Student_SetLookupCode_Event{
			CodeUniqueId: cmd.CodeUniqueId,
		},
		version: cmd.GetVersion(),
	})
}

// SetStatus is a function that sets the status of a student, active or inactive
func (sd *Aggregate) SetStatus(cmd *eda.Student_SetStatus) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_SET_STUDENT_STATUS,
		data:      &eda.Student_SetStatus_Event{Status: cmd.GetStatus()},
		version:   cmd.GetVersion(),
	})
}

func (sd *Aggregate) UpdateStudent(cmd *eda.Student_Update) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_UPDATE_STUDENT,
		data: &eda.Student_Update_Event{
			FirstName:       cmd.FirstName,
			LastName:        cmd.LastName,
			DateOfBirth:     cmd.DateOfBirth,
			StudentSchoolId: cmd.StudentSchoolId,
			Sex:             cmd.Sex,
		},
		version: cmd.GetVersion(),
	})
}

func (sd *Aggregate) EnrollStudent(cmd *eda.Student_Enroll) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_ENROLL_STUDENT,
		data: &eda.Student_Enroll_Event{
			SchoolId:         cmd.SchoolId,
			DateOfEnrollment: cmd.DateOfEnrollment,
		},
		version: cmd.GetVersion(),
	})
}

func (sd *Aggregate) UnenrollStudent(cmd *eda.Student_Unenroll) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_UNENROLL_STUDENT,
		data:      &eda.Student_Unenroll_Event{},
		version:   cmd.GetVersion(),
	})
}

// HandleSetStudentStatus handles the SetStudentStatus event
func (sd *Aggregate) HandleSetStudentStatus(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_SetStatus_Event)

	sd.data.Status = data.Status

	return nil
}

func (sd *Aggregate) HandleCreateStudent(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_Create_Event)

	if sd.data != nil {
		return fmt.Errorf("student already exists")
	}

	sd.data = &eda.Student{
		FirstName:     data.FirstName,
		LastName:      data.LastName,
		DateOfBirth:   data.DateOfBirth,
		Sex:           data.Sex,
		Status:        eda.Student_INACTIVE,
		FeedingReport: make([]*eda.Student_Feeding, 0),
	}

	return nil
}

func (sd *Aggregate) HandleUpdateStudent(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_Update_Event)

	sd.data.FirstName = data.FirstName
	sd.data.LastName = data.LastName
	sd.data.DateOfBirth = data.DateOfBirth
	sd.data.StudentSchoolId = data.StudentSchoolId
	sd.data.Sex = data.Sex

	return nil
}

func (sd *Aggregate) HandleEnrollStudent(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_Enroll_Event)

	sd.data.SchoolId = data.SchoolId
	sd.data.DateOfEnrollment = data.DateOfEnrollment

	return nil
}

// HandleUnenrollStudent handles the UnenrollStudent event
func (sd *Aggregate) HandleUnenrollStudent(evt wrappedEvent) error {
	sd.data.SchoolId = ""
	sd.data.DateOfEnrollment = nil

	return nil
}

func (sd *Aggregate) handleSetLookupCode(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_SetLookupCode_Event)

	sd.data.CodeUniqueId = data.CodeUniqueId

	return nil
}

func (sd *Aggregate) handleSetProfilePhoto(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_SetProfilePhoto_Event)
	sd.data.ProfilePhotoId = data.FileId
	return nil
}

// StudentEvent is a struct that holds the event type and the data
type StudentEvent struct {
	eventType string
	data      proto.Message
	version   uint64
}

// ApplyEvent is a function that applies an event to the aggregate
func (sd *Aggregate) ApplyEvent(sEvt StudentEvent) (*gosignal.Event, error) {
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

func (sd *Aggregate) ImportState(data []byte) error {
	student := eda.Student{}

	if err := proto.Unmarshal(data, &student); err != nil {
		return fmt.Errorf("error unmarshalling snapshot data: %s", err)
	}

	sd.data = &student

	return nil
}
func (sd *Aggregate) ExportState() ([]byte, error) {
	return proto.Marshal(sd.data)
}

func (sd Aggregate) String() string {
	id := sd.GetID()
	ver := sd.GetVersion()

	return fmt.Sprintf("ID: %s, Version: %d, Data: %+v", id, ver, sd.data.String())
}

func (sd Aggregate) GetStudent() *eda.Student {
	return sd.data
}

// GetFullName returns the student's full nam
func (sd Aggregate) GetFullName() string {
	return fmt.Sprintf("%s %s", sd.data.FirstName, sd.data.LastName)
}

// IsActive returns the active state of the student
func (sd Aggregate) IsActive() bool {
	return sd.data.Status == eda.Student_ACTIVE
}

// GetLastFeeding returns the last feeding event
func (sd Aggregate) GetLastFeeding() *eda.Student_Feeding {
	if sd.data.FeedingReport == nil || len(sd.data.FeedingReport) == 0 {
		return nil
	}

	return sd.data.FeedingReport[len(sd.data.FeedingReport)-1]
}

// SetProfilePhoto sets the student's profile photo
func (sd *Aggregate) SetProfilePhoto(cmd *eda.Student_SetProfilePhoto) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_SET_PROFILE_PHOTO,
		data: &eda.Student_SetProfilePhoto{
			FileId: cmd.FileId,
		},
		version: cmd.GetVersion(),
	})
}
