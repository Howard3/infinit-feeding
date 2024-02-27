package student

import (
	"context"
	"fmt"
	"geevly/gen/go/eda"

	"github.com/Howard3/gosignal"
	"github.com/google/uuid"
)

type StudentService struct {
	repo          Repository
	eventHandlers *eventHandlers
}

func NewStudentService(repo Repository) *StudentService {
	return &StudentService{
		repo:          repo,
		eventHandlers: NewEventHandlers(repo),
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
	studentAgg := &Student{}
	studentAgg.SetID(uuid.New().String())

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
	studentAgg, err := s.repo.loadStudent(ctx, cmd.GetStudentId())
	if err != nil {
		return nil, err
	}

	evt, err := studentAgg.UpdateStudent(cmd)
	if err != nil {
		return nil, err
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*evt}); err != nil {
		return nil, err
	}

	s.eventHandlers.HandleUpdateStudentEvent(ctx, evt)

	return &eda.Student_Update_Response{
		StudentId: studentAgg.GetID(),
		Version:   studentAgg.GetVersion(),
		Student:   studentAgg.GetStudent(),
	}, nil
}

func (s *StudentService) SetStatus(ctx context.Context, cmd *eda.Student_SetStatus) (*eda.Student_SetStatus_Response, error) {
	studentAgg, err := s.repo.loadStudent(ctx, cmd.GetStudentId())
	if err != nil {
		return nil, err
	}

	evt, err := studentAgg.SetStatus(cmd)
	if err != nil {
		return nil, err
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*evt}); err != nil {
		return nil, err
	}

	s.eventHandlers.HandleSetStatusEvent(ctx, evt)

	return &eda.Student_SetStatus_Response{
		StudentId: studentAgg.GetID(),
		Version:   studentAgg.GetVersion(),
		Student:   studentAgg.GetStudent(),
	}, nil
}

// GetStudent returns a student aggregate by ID
func (s *StudentService) GetStudent(ctx context.Context, studentID string) (*Student, error) {
	studentAgg, err := s.repo.loadStudent(ctx, studentID)
	if err != nil {
		return nil, err
	}

	return studentAgg, nil
}
