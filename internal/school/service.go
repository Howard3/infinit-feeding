package school

import (
	"context"
	"fmt"
	"geevly/gen/go/eda"

	"github.com/Howard3/gosignal"
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

// ListResponse is a struct that represents the response of the ListSchools method
type ListResponse struct {
	Schools []*ProjectedSchool
	Count   uint
}

// Create creates a new school on this aggregate, only works if the school is not already created
func (s *Service) Create(ctx context.Context, cmd *eda.School_Create) (*eda.School_Create_Response, error) {
	agg := &Aggregate{}
	newID, err := s.repo.getNewID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get new ID: %w", err)
	}

	agg.SetIDUint64(uint64(newID))

	evt, err := agg.CreateSchool(cmd)
	if err != nil {
		return nil, err
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*evt}); err != nil {
		return nil, err
	}

	s.eventHandlers.HandleNewSchoolEvent(ctx, evt)

	return &eda.School_Create_Response{
		Id:     agg.GetIDUint64(),
		School: agg.data,
	}, nil
}

// List returns a list of schools from the projection
func (s *Service) List(ctx context.Context, limit, page uint) (*ListResponse, error) {
	schools, err := s.repo.listSchools(ctx, limit, page)
	if err != nil {
		return nil, fmt.Errorf("failed to list students: %w", err)
	}

	count, err := s.repo.countSchools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count students: %w", err)
	}

	return &ListResponse{
		Schools: schools,
		Count:   count,
	}, nil
}
