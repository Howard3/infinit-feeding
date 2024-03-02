package student

import (
	"context"
	"fmt"
	"geevly/gen/go/eda"

	"github.com/Howard3/gosignal"
)

var ErrSchoolValidation = fmt.Errorf("error validating school")

type StudentService struct {
	repo          Repository
	eventHandlers *eventHandlers
	acl           AntiCorruptionLayer
}

type AntiCorruptionLayer interface {
	ValidateSchoolID(ctx context.Context, schoolID string) error
}

func NewStudentService(repo Repository, acl AntiCorruptionLayer) *StudentService {
	return &StudentService{
		repo:          repo,
		eventHandlers: NewEventHandlers(repo),
		acl:           acl,
	}
}

type ListStudentsResponse struct {
	Students []*ProjectedStudent
	Count    uint
}

func (s *StudentService) ListStudents(ctx context.Context, limit, page uint) (*ListStudentsResponse, error) {
	students, err := s.repo.ListStudents(ctx, limit, page)
	if err != nil {
		return nil, fmt.Errorf("failed to list students: %w", err)
	}

	count, err := s.repo.CountStudents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count students: %w", err)
	}

	return &ListStudentsResponse{
		Students: students,
		Count:    count,
	}, nil
}
func (s *StudentService) CreateStudent(ctx context.Context, req *eda.Student_Create) (*eda.Student_Create_Response, error) {
	studentAgg := &Aggregate{}
	newID, err := s.repo.GetNewID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get new ID: %w", err)
	}

	studentAgg.SetIDUint64(newID)

	evt, err := studentAgg.CreateStudent(req)
	if err != nil {
		return nil, err
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*evt}); err != nil {
		return nil, err
	}

	s.eventHandlers.HandleNewStudentEvent(ctx, evt)

	return &eda.Student_Create_Response{
		StudentId: studentAgg.GetID(),
		Version:   studentAgg.GetVersion(),
		Student:   studentAgg.GetStudent(),
	}, nil
}

func (s *StudentService) UpdateStudent(ctx context.Context, cmd *eda.Student_Update) (*eda.Student_Update_Response, error) {
	return withStudent(ctx, s, cmd.GetStudentId(), func(agg *Aggregate) (*eda.Student_Update_Response, []gosignal.Event, error) {
		evt, err := agg.UpdateStudent(cmd)
		res := eda.Student_Update_Response{
			StudentId: agg.GetID(),
			Version:   agg.GetVersion(),
			Student:   agg.GetStudent(),
		}

		return &res, []gosignal.Event{*evt}, err
	})
}

func (s *StudentService) SetStatus(ctx context.Context, cmd *eda.Student_SetStatus) (*eda.Student_SetStatus_Response, error) {
	return withStudent(ctx, s, cmd.GetStudentId(), func(agg *Aggregate) (*eda.Student_SetStatus_Response, []gosignal.Event, error) {
		evt, err := agg.SetStatus(cmd)
		res := eda.Student_SetStatus_Response{
			StudentId: agg.GetID(),
			Version:   agg.GetVersion(),
			Student:   agg.GetStudent(),
		}

		return &res, []gosignal.Event{*evt}, err
	})
}

// GetStudent returns a student aggregate by ID
func (s *StudentService) GetStudent(ctx context.Context, studentID string) (*Aggregate, error) {
	studentAgg, err := s.repo.loadStudent(ctx, studentID)
	if err != nil {
		return nil, err
	}

	return studentAgg, nil
}

// GetHistory returns the event history for a student aggregate
func (s *StudentService) GetHistory(ctx context.Context, studentID string) ([]gosignal.Event, error) {
	return s.repo.getEventHistory(ctx, studentID)
}

// EnrollStudent enrolls a student in a school
func (s *StudentService) EnrollStudent(ctx context.Context, cmd *eda.Student_Enroll) (*eda.Student_Enroll_Response, error) {
	if err := s.acl.ValidateSchoolID(ctx, cmd.GetSchoolId()); err != nil {
		return nil, fmt.Errorf("failed to validate school ID: %w", err)
	}

	return withStudent(ctx, s, cmd.GetStudentId(), func(agg *Aggregate) (*eda.Student_Enroll_Response, []gosignal.Event, error) {
		evt, err := agg.EnrollStudent(cmd)
		res := eda.Student_Enroll_Response{
			StudentId: agg.GetID(),
			Version:   agg.GetVersion(),
			Student:   agg.GetStudent(),
		}

		return &res, []gosignal.Event{*evt}, err
	})
}

// withStudent is a helper function that loads a student aggregate from the repository and executes a function on it
func withStudent[T any](ctx context.Context, s *StudentService, id string, fn func(*Aggregate) (*T, []gosignal.Event, error)) (*T, error) {
	studentAgg, err := s.repo.loadStudent(ctx, id)
	if err != nil {
		return nil, err
	}

	msg, evt, err := fn(studentAgg)
	if err != nil {
		return nil, err
	}

	if err := s.repo.saveEvents(ctx, evt); err != nil {
		return nil, err
	}

	for _, e := range evt {
		go s.eventHandlers.routeEvent(ctx, &e)
	}

	return msg, nil
}
