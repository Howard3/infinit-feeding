package student

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
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
const EVENT_SET_ELIGIBILITY = "SetEligibility"
const EVENT_UPDATE_SPONSORSHIP = "UpdateSponsorship"
const EVENT_ADD_GRADE_REPORT = "AddGradeReport"
const EVENT_ADD_HEALTH_ASSESSMENT = "AddHealthAssessment"

type wrappedEvent struct {
	event gosignal.Event
	data  proto.Message
}

type HealthReport struct {
	AssessmentDate         time.Time
	AssociatedBulkUploadId string
	HeightCm               float32
	WeightKg               float32
	sex                    *eda.Student_Sex
	dob                    *time.Time
}

func (h *HealthReport) BMI() float32 {
	heightMeters := h.HeightCm / 100
	return h.WeightKg / (heightMeters * heightMeters)
}

func (h *HealthReport) AgeYears() float64 {
	if h.dob == nil {
		return 0
	}

	return time.Since(*h.dob).Hours() / 24 / 365
}

func (h *HealthReport) NutritionalStatus() NutritionalStatus {
	var gender Gender
	switch *h.sex {
	case eda.Student_MALE:
		gender = Male
	case eda.Student_FEMALE:
		gender = Female
	default:
		return NutritionalStatusGenderError
	}

	age := int(math.Round(h.AgeYears()))

	status, err := CalculateNutritionalStatus(gender, age, h.BMI())
	if err != nil {
		slog.Error("error calculating nutritional status", "error", err)
		return NutritionalStatusError
	}
	return status
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
		eventData = &eda.Student_Feeding_Event{}
		handler = sd.handleFeedStudent
	case EVENT_SET_ELIGIBILITY:
		eventData = &eda.Student_SetEligibility_Event{}
		handler = sd.handleSetEligibility
	case EVENT_UPDATE_SPONSORSHIP:
		eventData = &eda.Student_UpdateSponsorship_Event{}
		handler = sd.handleUpdateSponsorship
	case EVENT_ADD_GRADE_REPORT:
		eventData = &eda.Student_GradeReport_Event{}
		handler = sd.handleAddGradeReport
	case EVENT_ADD_HEALTH_ASSESSMENT:
		eventData = &eda.Student_HealthAssessment_Event{}
		handler = sd.handleAddHealthAssessment

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

func (sd *Aggregate) GetHealthAssessments() []*HealthReport {
	hr := make([]*HealthReport, len(sd.data.HealthAssessments))
	dob := time.Date(
		int(sd.data.DateOfBirth.Year),
		time.Month(sd.data.DateOfBirth.Month),
		int(sd.data.DateOfBirth.Day), 0, 0, 0, 0, time.UTC)

	for i, h := range sd.data.HealthAssessments {
		hr[i] = &HealthReport{
			AssessmentDate:         h.AssessmentDate.AsTime(),
			AssociatedBulkUploadId: h.AssociatedBulkUploadId,
			HeightCm:               h.HeightCm,
			WeightKg:               h.WeightKg,
			sex:                    &sd.data.Sex,
			dob:                    &dob,
		}
	}
	return hr
}

func (sd *Aggregate) AddHealthAssessment(cmd *eda.Student_HealthAssessment) (*gosignal.Event, error) {
	if sd.data == nil {
		return nil, ErrStudentNotFound
	}

	// TODO: error on conflict

	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_ADD_HEALTH_ASSESSMENT,
		data: &eda.Student_HealthAssessment_Event{
			AssessmentDate:         cmd.GetAssessmentDate(),
			AssociatedBulkUploadId: cmd.GetAssociatedBulkUploadId(),
			HeightCm:               cmd.GetHeightCm(),
			WeightKg:               cmd.GetWeightKg(),
		},
		version: sd.Version,
	})
}

func (sd *Aggregate) handleAddHealthAssessment(evt wrappedEvent) error {
	event := evt.data.(*eda.Student_HealthAssessment_Event)
	sd.data.HealthAssessments = append(sd.data.HealthAssessments, &eda.Student_HealthAssessment{
		AssessmentDate:         event.AssessmentDate,
		AssociatedBulkUploadId: event.AssociatedBulkUploadId,
		HeightCm:               event.HeightCm,
		WeightKg:               event.WeightKg,
	})
	return nil
}

func (sd *Aggregate) AddGradeReport(cmd *eda.Student_GradeReport) (*gosignal.Event, error) {
	if sd.data == nil {
		return nil, ErrStudentNotFound
	}

	// TODO: error on conflict

	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_ADD_GRADE_REPORT,
		data: &eda.Student_GradeReport_Event{
			Grade:                  cmd.GetGrade(),
			TestDate:               cmd.GetTestDate(),
			AssociatedBulkUploadId: cmd.GetAssociatedBulkUploadId(),
			SchoolYear:             cmd.GetSchoolYear(),
			GradingPeriod:          cmd.GetGradingPeriod(),
		},
		version: sd.Version,
	})
}

func (sd *Aggregate) handleAddGradeReport(evt wrappedEvent) error {
	event := evt.data.(*eda.Student_GradeReport_Event)
	sd.data.GradeHistory = append(sd.data.GradeHistory, &eda.Student_GradeReport{
		Grade:                  event.Grade,
		TestDate:               event.TestDate,
		AssociatedBulkUploadId: event.AssociatedBulkUploadId,
		SchoolYear:             event.SchoolYear,
		GradingPeriod:          event.GradingPeriod,
	})
	return nil
}

// feed - handles the feeding of a student
func (sd *Aggregate) Feed(cmd *eda.Student_Feeding) (*gosignal.Event, error) {
	timestamp := cmd.GetUnixTimestamp()
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
			FileId:        cmd.GetFileId(),
		},
		version: cmd.GetVersion(),
	})
}

func (sd *Aggregate) handleFeedStudent(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_Feeding_Event)

	sd.data.FeedingNextId++ // handle incrementing the next id

	sd.data.FeedingReport = append(sd.data.FeedingReport, data)

	return nil
}

func (sd *Aggregate) CreateStudent(cmd *eda.Student_Create) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_ADD_STUDENT,
		data: &eda.Student_Create_Event{
			FirstName:       cmd.FirstName,
			LastName:        cmd.LastName,
			DateOfBirth:     cmd.DateOfBirth,
			Status:          eda.Student_INACTIVE,
			Sex:             cmd.Sex,
			GradeLevel:      cmd.GradeLevel,
			StudentSchoolId: cmd.StudentSchoolId,
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
			GradeLevel:      cmd.GradeLevel,
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
		FirstName:       data.FirstName,
		LastName:        data.LastName,
		DateOfBirth:     data.DateOfBirth,
		Sex:             data.Sex,
		Status:          eda.Student_INACTIVE,
		StudentSchoolId: data.StudentSchoolId,
		GradeLevel:      data.GradeLevel,
		FeedingReport:   make([]*eda.Student_Feeding_Event, 0),
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
	sd.data.GradeLevel = data.GradeLevel

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

func (sd *Aggregate) handleSetEligibility(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_SetEligibility_Event)
	sd.data.EligibleForSponsorship = data.Eligible
	return nil
}

func (sd *Aggregate) handleUpdateSponsorship(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_UpdateSponsorship_Event)

	// Create new sponsorship record
	newRecord := &eda.Student_SponsorshipRecord{
		SponsorId: data.SponsorId,
		StartDate: data.StartDate,
		EndDate:   data.EndDate,
	}

	// Add to sponsorship history
	if sd.data.SponsorshipHistory == nil {
		sd.data.SponsorshipHistory = make([]*eda.Student_SponsorshipRecord, 0)
	}
	sd.data.SponsorshipHistory = append(sd.data.SponsorshipHistory, newRecord)

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

// GetAge returns the student's agg
func (sd Aggregate) GetAge() int {
	dobYear := sd.data.DateOfBirth.GetYear()
	dobMonth := sd.data.DateOfBirth.GetMonth()
	dobDay := sd.data.DateOfBirth.GetDay()
	dobTime := time.Date(int(dobYear), time.Month(dobMonth), int(dobDay), 0, 0, 0, 0, time.UTC)
	since := time.Since(dobTime)
	years := since.Seconds() / 60 / 60 / 24 / 365
	return int(years)
}

// IsActive returns the active state of the student
func (sd Aggregate) IsActive() bool {
	return sd.data.Status == eda.Student_ACTIVE
}

// MaxSponsorshipDate returns the maximum sponsorship date,
// if there is no sponsorship history, it returns nil
func (sd Aggregate) MaxSponsorshipDate() *time.Time {
	var max *time.Time
	if sd.data.SponsorshipHistory != nil {
		for _, sponsorship := range sd.data.SponsorshipHistory {
			endDate := time.Date(
				int(sponsorship.EndDate.Year),
				time.Month(sponsorship.EndDate.Month),
				int(sponsorship.EndDate.Day),
				0, 0, 0, 0,
				time.UTC,
			)

			if max == nil || endDate.After(*max) {
				max = &endDate
			}
		}
	}
	return max
}

// GetLastFeeding returns the last feeding event
func (sd Aggregate) GetLastFeeding() *eda.Student_Feeding_Event {
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

func (sd *Aggregate) SetEligibility(cmd *eda.Student_SetEligibility) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_SET_ELIGIBILITY,
		data: &eda.Student_SetEligibility_Event{
			Eligible: cmd.Eligible,
		},
		version: cmd.GetVersion(),
	})
}

func (sd *Aggregate) UpdateSponsorship(cmd *eda.Student_UpdateSponsorship) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_UPDATE_SPONSORSHIP,
		data: &eda.Student_UpdateSponsorship_Event{
			SponsorId: cmd.SponsorId,
			StartDate: cmd.StartDate,
			EndDate:   cmd.EndDate,
		},
		version: cmd.GetVersion(),
	})
}
