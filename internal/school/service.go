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

// Update updates a school on this Aggregate
func (s *Service) Update(ctx context.Context, cmd *eda.School_Update) (*eda.School_Update_Response, error) {
	agg, err := s.repo.loadSchool(ctx, cmd.Id)
	if err != nil {
		return nil, err
	}

	evt, err := agg.UpdateSchool(cmd)
	if err != nil {
		return nil, err
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*evt}); err != nil {
		return nil, err
	}

	s.eventHandlers.HandleUpdateSchoolEvent(ctx, evt)

	return &eda.School_Update_Response{
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

// Get returns a school aggregate by ID
func (s *Service) Get(ctx context.Context, id uint64) (*Aggregate, error) {
	agg, err := s.repo.loadSchool(ctx, id)
	if err != nil {
		return nil, err
	}

	return agg, nil
}

// GetHistory returns the event history for a school aggregate
func (s *Service) GetHistory(ctx context.Context, id uint64) ([]gosignal.Event, error) {
	return s.repo.getEventHistory(ctx, id)
}

// mapSchoolsByID - returns a map of school IDs to school names
func (s *Service) MapSchoolsByID(ctx context.Context) (map[uint64]string, error) {
	return s.repo.mapSchoolsByID(ctx)
}

// ValidateSchoolID validates that a school exists
func (s *Service) ValidateSchoolID(ctx context.Context, schoolID uint64) error {
	return s.repo.validateSchoolID(ctx, schoolID)
}
