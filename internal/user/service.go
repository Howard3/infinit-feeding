package user

import (
	"context"
	"fmt"
	"geevly/gen/go/eda"

	"github.com/Howard3/gosignal"
	"google.golang.org/protobuf/proto"
)

type Service struct {
	repo          Repository
	eventHandlers *eventHandlers
}

func NewService(repo Repository) *Service {
	return &Service{
		repo:          repo,
		eventHandlers: NewEventHandlers(repo),
	}
}

// ListResponse is a struct that represents the response of the ListUsers method
type ListResponse struct {
	Users []*ProjectedUser
	Count uint
}

// RunCommand runs a command on a user aggregate
func (s *Service) RunCommand(ctx context.Context, aggID uint64, cmd proto.Message) (*User, error) {
	return s.withUser(ctx, aggID, func(agg *User) (*gosignal.Event, error) {
		switch cmd := cmd.(type) {
		case *eda.User_Update:
			return agg.UpdateUser(cmd)
		default:
			return nil, fmt.Errorf("unknown command type: %T", cmd)
		}
	})
}

// withUser is a helper function that loads an user aggregate from the repository and executes a function on it
func (s *Service) withUser(ctx context.Context, id uint64, fn func(*User) (*gosignal.Event, error)) (*User, error) {
	agg, err := s.repo.loadUser(ctx, id)
	if err != nil {
		return nil, err
	}

	evt, err := fn(agg)
	if err != nil {
		return nil, err
	}

	return agg, s.saveEvent(ctx, evt)
}

func (s *Service) saveEvent(ctx context.Context, evt *gosignal.Event) error {
	if evt != nil {
		if err := s.repo.saveEvents(ctx, []gosignal.Event{*evt}); err != nil {
			return err
		}

		go s.eventHandlers.routeEvent(context.Background(), evt)

	}

	return nil
}

// createUser creates a new user aggregate
func (s *Service) CreateUser(ctx context.Context, cmd *eda.User_Create) (*User, error) {
	agg := &User{}
	newID, err := s.repo.getNewID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get next ID: %w", err)
	}

	agg.ID = newID

	evt, err := agg.CreateUser(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	if err := s.saveEvent(ctx, evt); err != nil {
		return nil, fmt.Errorf("failed to save event: %w", err)
	}

	return agg, nil
}

// List returns a list of schools from the projection
func (s *Service) List(ctx context.Context, limit, page uint) (*ListResponse, error) {
	users, err := s.repo.listUsers(ctx, limit, page)
	if err != nil {
		return nil, fmt.Errorf("failed to list students: %w", err)
	}

	count, err := s.repo.countUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count students: %w", err)
	}

	return &ListResponse{
		Users: users,
		Count: count,
	}, nil
}

// Get returns a user aggregate by ID
func (s *Service) Get(ctx context.Context, id uint64) (*User, error) {
	agg, err := s.repo.loadUser(ctx, id)
	if err != nil {
		return nil, err
	}

	return agg, nil
}

// GetHistory returns the event history for a school aggregate
func (s *Service) GetHistory(ctx context.Context, id uint64) ([]gosignal.Event, error) {
	return s.repo.getEventHistory(ctx, id)
}

// ValidateUserID validates that a school exists
func (s *Service) ValidateUserID(ctx context.Context, userID uint64) error {
	return s.repo.validateUserID(ctx, userID)
}
