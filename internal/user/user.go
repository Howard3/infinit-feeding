package user

import (
	"fmt"
	"geevly/gen/go/eda"

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
	sourcing.DefaultAggregate
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
			First: data.First,
			Last:  data.Last,
		},
		Email: data.Email,
	}

	return nil
}

// onUserUpdated is a handler for the UserUpdated event
func (u *User) onUserUpdated(evt wrappedEvent) error {
	data := evt.data.(*eda.User_Update_Event)

	u.data.Name.First = data.First
	u.data.Name.Last = data.Last
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
