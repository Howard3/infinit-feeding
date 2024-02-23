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

func (s *studentService) CreateStudent(ctx context.Context, req *student.Student_Create) (*student.Student_Create_Response, error) {
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

	return &student.Student_Create_Response{
		StudentId: studentAgg.GetID(),
		Version:   studentAgg.GetVersion(),
		Student:   studentAgg.GetStudent(),
	}, nil
}

func (s *studentService) UpdateStudent(ctx context.Context, cmd *student.Student_Update) (*student.Student_Update_Response, error) {
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

	return &student.Student_Update_Response{
		StudentId: studentAgg.GetID(),
		Version:   studentAgg.GetVersion(),
		Student:   studentAgg.GetStudent(),
	}, nil
}

func (s *studentService) SetStatus(ctx context.Context, cmd *student.Student_SetStatus) (*student.Student_SetStatus_Response, error) {
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

	return &student.Student_SetStatus_Response{
		StudentId: studentAgg.GetID(),
		Version:   studentAgg.GetVersion(),
		Student:   studentAgg.GetStudent(),
	}, nil
}
