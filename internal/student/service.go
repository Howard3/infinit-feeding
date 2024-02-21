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
		Version:   studentAgg.GetVersion(),
		Student:   studentAgg.GetStudent(),
	}, nil
}

func (s *studentService) UpdateStudent(ctx context.Context, req *student.UpdateStudentRequest) (*student.UpdateStudentResponse, error) {
	studentAgg, err := s.repo.loadStudent(ctx, req.GetData().GetStudentId())
	if err != nil {
		return nil, err
	}

	evt, err := studentAgg.UpdateStudent(req.GetData(), req.GetVersion())
	if err != nil {
		return nil, err
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*evt}); err != nil {
		return nil, err
	}

	s.eventHandlers.HandleUpdateStudentEvent(ctx, evt)

	return &student.UpdateStudentResponse{
		StudentId: studentAgg.GetID(),
		Version:   studentAgg.GetVersion(),
		Student:   studentAgg.GetStudent(),
	}, nil
}
