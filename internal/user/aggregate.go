package user

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

const EventCreated = "UserAdded"
const EventUpdated = "UserUpdated"
const EventPasswordChanged = "UserPasswordChanged"
const EventSetActiveState = "UserSetActiveState"
const EventAddRole = "UserAddRole"
const EventRemoveRole = "UserRemoveRole"

type wrappedEvent struct {
	event gosignal.Event
	data  proto.Message
}

type User struct {
	sourcing.DefaultAggregateUint64
	data *eda.User
}

func (User) ImportState(_ []byte) (_ error) {
	panic("not implemented") // TODO: Implement
}
func (User) ExportState() (_ []byte, _ error) {
	panic("not implemented") // TODO: Implement
}

func (u *User) Apply(evt gosignal.Event) error {
	return sourcing.SafeApply(evt, u, u.routeEvent)
}

// GetData returns the user data
func (u *User) GetData() *eda.User {
	return u.data
}

func (u *User) routeEvent(evt gosignal.Event) error {
	var eventData proto.Message
	var handler func(wrappedEvent) error
	switch evt.Type {
	case EventCreated:
		eventData = &eda.User_Create_Event{}
		handler = u.onUserCreated
	case EventUpdated:
		eventData = &eda.User_Update_Event{}
		handler = u.onUserUpdated
	case EventPasswordChanged:
		eventData = &eda.User_PasswordChange_Event{}
		handler = u.onUserPasswordChanged
	case EventSetActiveState:
		eventData = &eda.User_SetActiveState_Event{}
		handler = u.onUserSetActiveState
	case EventAddRole:
		eventData = &eda.User_AddRole_Event{}
		handler = u.onUserAddRole
	case EventRemoveRole:
		eventData = &eda.User_RemoveRole_Event{}
		handler = u.onUserRemoveRole
	}

	if err := proto.Unmarshal(evt.Data, eventData); err != nil {
		return fmt.Errorf("error unmarshalling event data: %s", err)
	}

	wevt := wrappedEvent{event: evt, data: eventData}

	return handler(wevt)
}

// onUserCreated is a handler for the UserCreated event
func (u *User) onUserCreated(evt wrappedEvent) error {
	data := evt.data.(*eda.User_Create_Event)

	u.data = &eda.User{
		Name: &eda.User_Name{
			First: data.FirstName,
			Last:  data.LastName,
		},
		Email: data.Email,
	}

	return nil
}

// onUserUpdated is a handler for the UserUpdated event
func (u *User) onUserUpdated(evt wrappedEvent) error {
	data := evt.data.(*eda.User_Update_Event)

	u.data.Name.First = data.FirstName
	u.data.Name.Last = data.LastName
	u.data.Email = data.Email

	return nil
}

// onUserPasswordChanged is a handler for the UserPasswordChanged event
// password change event is a special case, as it is not a part of the user's state
func (u *User) onUserPasswordChanged(evt wrappedEvent) error {
	u.data.LastPasswordChange = timestamppb.New(evt.event.Timestamp)

	return nil
}

// onUserSetActiveState is a handler for the UserSetActiveState eventData
func (u *User) onUserSetActiveState(evt wrappedEvent) error {
	data := evt.data.(*eda.User_SetActiveState_Event)

	u.data.Active = data.Active

	return nil
}

// onUserAddRole is a handler for the UserAddRole eventData
func (u *User) onUserAddRole(evt wrappedEvent) error {
	data := evt.data.(*eda.User_AddRole_Event)

	u.data.NextRoleId++
	data.Role.Id = u.data.NextRoleId
	u.data.Roles = append(u.data.Roles, data.Role)

	return nil
}

// onUserRemoveRole is a handler for the UserRemoveRole eventData
func (u *User) onUserRemoveRole(evt wrappedEvent) error {
	data := evt.data.(*eda.User_RemoveRole_Event)

	for i, r := range u.data.Roles {
		if r.Id == data.RoleId {
			u.data.Roles = append(u.data.Roles[:i], u.data.Roles[i+1:]...)
			break
		}
	}

	return nil
}

// UserEvent is a struct that holds the event type and the data
type UserEvent struct {
	eventType string
	data      proto.Message
	version   uint64
}

// ApplyEvent is a function that applies an event to the aggregate
func (u *User) ApplyEvent(uEvt UserEvent) (*gosignal.Event, error) {
	sBytes, marshalErr := proto.Marshal(uEvt.data)

	evt := gosignal.Event{
		Type:        uEvt.eventType,
		Timestamp:   time.Now(),
		Data:        sBytes,
		Version:     uEvt.version,
		AggregateID: u.GetID(),
	}

	return &evt, errors.Join(u.Apply(evt), marshalErr)
}

// CreateUser creates a new user on this aggregate
func (u *User) CreateUser(cmd *eda.User_Create) (*gosignal.Event, error) {
	if u.data != nil {
		return nil, errors.New("user already exists")
	}

	evt := &eda.User_Create_Event{
		FirstName: cmd.FirstName,
		LastName:  cmd.LastName,
		Email:     cmd.Email,
	}

	return u.ApplyEvent(UserEvent{eventType: EventCreated, data: evt})
}

// UpdateUser updates a user on this Aggregate
func (u *User) UpdateUser(cmd *eda.User_Update) (*gosignal.Event, error) {
	evt := &eda.User_Update_Event{
		FirstName: cmd.FirstName,
		LastName:  cmd.LastName,
		Email:     cmd.Email,
	}

	return u.ApplyEvent(UserEvent{eventType: EventUpdated, data: evt, version: cmd.Version})
}

// SetActiveState sets the active state of the user
func (u *User) SetActiveState(cmd *eda.User_SetActiveState) (*gosignal.Event, error) {
	evt := &eda.User_SetActiveState_Event{
		Active: cmd.Active,
	}

	return u.ApplyEvent(UserEvent{eventType: EventSetActiveState, data: evt, version: cmd.Version})
}

// IsActive returns the active state of the user
func (u *User) IsActive() bool {
	return u.data.Active
}
