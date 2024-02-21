package student

import (
	"context"
	student "geevly/events/gen/proto/go"

	"github.com/Howard3/gosignal"
	"github.com/google/uuid"
)

type studentService struct {
	repo          Repository
	eventHandlers *eventHandlers
}

func NewStudentService(repo Repository) *studentService {
	return &studentService{
		repo:          repo,
		eventHandlers: NewEventHandlers(repo),
	}
}

func (s *studentService) CreateStudent(ctx context.Context, req *student.AddStudentEvent) (*student.AddStudentResponse, error) {
	studentAgg := &Student{}
	studentAgg.SetID(uuid.New().String())

	evt, err := studentAgg.AddStudent(req)
	if err != nil {
		return nil, err
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*evt}); err != nil {
		return nil, err
	}

	s.eventHandlers.HandleNewStudentEvent(ctx, evt)

	return &student.AddStudentResponse{
		StudentId: studentAgg.GetID(),
	}, nil
}
