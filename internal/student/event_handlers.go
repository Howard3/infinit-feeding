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
