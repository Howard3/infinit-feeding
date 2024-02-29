package school

import (
	"errors"
	"fmt"
	"geevly/gen/go/eda"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/sourcing"
	"google.golang.org/protobuf/proto"
)

var ErrSchoolDoesNotExist = fmt.Errorf("school does not exist")
var ErrMustHaveName = fmt.Errorf("school must have a name")

const EventCreateSchool = "CreateSchool"
const EventUpdateSchool = "UpdateSchool"

var ErrEventNotFound = fmt.Errorf("event not found")

type Aggregate struct {
	sourcing.DefaultAggregateUint64
	data *eda.School
}

func (agg *Aggregate) Apply(evt gosignal.Event) error {
	return sourcing.SafeApply(evt, agg, agg.routeEvent)
}

type wrappedEvent struct {
	event gosignal.Event
	data  proto.Message
}

func (agg *Aggregate) routeEvent(evt gosignal.Event) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("panic: %v", e)
		}

		if err != nil {
			err = fmt.Errorf("when processing event %q school aggregate %q: %w", evt.Type, evt.AggregateID, err)
		}
	}()

	var eventData proto.Message
	var handler func(wrappedEvent) error

	switch evt.Type {
	case EventCreateSchool:
		eventData = &eda.School_Create_Event{}
		handler = agg.handleAddSchool
	case EventUpdateSchool:
		eventData = &eda.School_Update_Event{}
		handler = agg.handleUpdateSchool
	default:
		return ErrEventNotFound
	}

	if err := proto.Unmarshal(evt.Data, eventData); err != nil {
		return fmt.Errorf("error unmarshalling event data: %s", err)
	}

	if evt.Type != EventCreateSchool && agg.data == nil {
		return fmt.Errorf("when processing event %q: %w", evt.Type, ErrSchoolDoesNotExist)
	}

	return handler(wrappedEvent{event: evt, data: eventData})
}

// SchoolEvent is a struct that holds the event type and the data
type SchoolEvent struct {
	eventType string
	data      proto.Message
	version   uint64
}

// ApplyEvent is a function that applies an event to the aggregate
func (sd *Aggregate) ApplyEvent(sEvt SchoolEvent) (*gosignal.Event, error) {
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

func (agg *Aggregate) CreateSchool(cmd *eda.School_Create) (*gosignal.Event, error) {
	if cmd.Name == "" {
		return nil, ErrMustHaveName
	}

	return agg.ApplyEvent(SchoolEvent{
		eventType: EventCreateSchool,
		data: &eda.School_Create_Event{
			Name:      cmd.Name,
			Principal: cmd.Principal,
			Contact:   cmd.Contact,
		},
	})
}

func (agg *Aggregate) UpdateSchool(cmd *eda.School_Update) (*gosignal.Event, error) {
	return agg.ApplyEvent(SchoolEvent{
		eventType: EventUpdateSchool,
		data: &eda.School_Update_Event{
			Name:      cmd.Name,
			Principal: cmd.Principal,
			Contact:   cmd.Contact,
		},
		version: cmd.Version,
	})
}

func (agg *Aggregate) handleAddSchool(we wrappedEvent) error {
	data := we.data.(*eda.School_Create_Event)

	if agg.data != nil {
		return fmt.Errorf("school already exists")
	}

	agg.data = &eda.School{
		Name:      data.Name,
		Principal: data.Principal,
		Contact:   data.Contact,
	}

	return nil
}

func (agg *Aggregate) handleUpdateSchool(we wrappedEvent) error {
	data := we.data.(*eda.School_Update_Event)

	agg.data.Name = data.Name
	agg.data.Principal = data.Principal
	agg.data.Contact = data.Contact

	return nil
}

func NewAggregate() *Aggregate {
	return &Aggregate{
		data: &eda.School{},
	}
}

func (agg *Aggregate) ImportState(data []byte) error {
	school := eda.School{}

	if err := proto.Unmarshal(data, &school); err != nil {
		return fmt.Errorf("error unmarshalling snapshot data: %s", err)
	}

	agg.data = &school

	return nil
}
func (agg *Aggregate) ExportState() ([]byte, error) {
	return proto.Marshal(agg.data)
}

func (agg *Aggregate) GetData() *eda.School {
	return agg.data
}
