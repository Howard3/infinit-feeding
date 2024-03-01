package student

import (
	"context"
	"log/slog"

	"github.com/Howard3/gosignal"
)

type eventHandlers struct {
	repo Repository
}

func NewEventHandlers(repo Repository) *eventHandlers {
	return &eventHandlers{
		repo: repo,
	}
}

// HandleNewStudentEvent is a method that handles the NewStudentEvent
// it loads the student aggregate from the repository and projects it to the database
func (eh *eventHandlers) HandleNewStudentEvent(ctx context.Context, evt *gosignal.Event) {
	student, err := eh.repo.loadStudent(ctx, evt.AggregateID)
	if err != nil {
		slog.Error("failed to load student", "error", err)
		return
	}

	if err := eh.repo.upsertStudent(student); err != nil {
		slog.Error("failed to upsert student", "error", err)
		return
	}
}

// HandleUpdateStudentEvent is a method that handles the UpdateStudentEvent
// functionally the same as HandleNewStudentEvent, thus it just aliases it
func (eh *eventHandlers) HandleUpdateStudentEvent(ctx context.Context, evt *gosignal.Event) {
	eh.HandleNewStudentEvent(ctx, evt)
}

// HandleSetStatusEvent is a method that handles the SetStatusEvent
// functionally the same as HandleNewStudentEvent, thus it just aliases it
func (eh *eventHandlers) HandleSetStatusEvent(ctx context.Context, evt *gosignal.Event) {
	eh.HandleNewStudentEvent(ctx, evt)
}

// routeEvent is a method that routes an event to the appropriate handler
func (eh *eventHandlers) routeEvent(ctx context.Context, evt *gosignal.Event) {
	switch evt.Type {
	case EVENT_ADD_STUDENT:
		eh.HandleNewStudentEvent(ctx, evt)
	case EVENT_UPDATE_STUDENT:
		eh.HandleUpdateStudentEvent(ctx, evt)
	case EVENT_SET_STUDENT_STATUS:
		eh.HandleSetStatusEvent(ctx, evt)
	}
}
