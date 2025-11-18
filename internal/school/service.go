package school

import (
	"context"
	"fmt"
	"geevly/gen/go/eda"
	"time"

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

// SetSchoolPeriod sets the school period for a school
func (s *Service) SetSchoolPeriod(ctx context.Context, cmd *eda.School_SetSchoolPeriod) (*eda.School_SetSchoolPeriod_Response, error) {
	agg, err := s.repo.loadSchool(ctx, cmd.Id)
	if err != nil {
		return nil, err
	}

	evt, err := agg.SetSchoolPeriod(cmd)
	if err != nil {
		return nil, err
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*evt}); err != nil {
		return nil, err
	}

	s.eventHandlers.HandleSetSchoolPeriodEvent(ctx, evt)

	return &eda.School_SetSchoolPeriod_Response{
		Id:     agg.GetIDUint64(),
		School: agg.data,
	}, nil
}

// List returns a list of schools from the projection
func (s *Service) List(ctx context.Context, limit, page uint) (*ListResponse, error) {
	schools, err := s.repo.listSchools(ctx, limit, page)
	if err != nil {
		return nil, fmt.Errorf("failed to list schools: %w", err)
	}

	count, err := s.repo.countSchools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count schools: %w", err)
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

// GetSchoolsByIDs returns a list of schools by their IDs
func (s *Service) GetSchoolsByIDs(ctx context.Context, schoolIDs []uint64) ([]*Aggregate, error) {
	out := make([]*Aggregate, 0)
	for _, schoolID := range schoolIDs {
		school, err := s.Get(ctx, schoolID)
		if err != nil {
			return nil, err
		}
		out = append(out, school)
	}
	return out, nil
}

// ListLocations returns a list of locations
func (s *Service) ListLocations(ctx context.Context) ([]Location, error) {
	return s.repo.listLocations(ctx)
}

// GetSchoolIDsByLocation returns a list of school IDs for a given location
func (s *Service) GetSchoolIDsByLocation(ctx context.Context, location Location) ([]uint64, error) {
	if location.Country == "" {
		return nil, fmt.Errorf("country is required")
	}

	return s.repo.getSchoolIDsByLocation(ctx, location)
}

// GetDomainEvents retrieves domain events with pagination and optional filtering
func (s *Service) GetDomainEvents(ctx context.Context, limit, offset uint, eventTypeFilter, aggregateIDFilter string, startDate, endDate *time.Time) ([]DomainEvent, uint, error) {
	return s.repo.GetDomainEvents(ctx, limit, offset, eventTypeFilter, aggregateIDFilter, startDate, endDate)
}

// GetEventTypes retrieves all distinct event types for the school domain
func (s *Service) GetEventTypes(ctx context.Context) ([]string, error) {
	return s.repo.GetEventTypes(ctx)
}

// GetEventStatistics retrieves aggregate statistics about school events
func (s *Service) GetEventStatistics(ctx context.Context) (*EventStatistics, error) {
	return s.repo.GetEventStatistics(ctx)
}
