package file

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

func (eh *eventHandlers) updateFileProjection(ctx context.Context, aggregateID string) {
	file, err := eh.repo.loadFile(ctx, aggregateID)
	if err != nil {
		slog.Error("failed to load file", "error", err)
		return
	}

	if err := eh.repo.upsertFileProjection(ctx, file); err != nil {
		slog.Error("failed to upsert file", "error", err)
		return
	}
}

// routeEvent is a method that routes an event to the appropriate handler
func (eh *eventHandlers) routeEvent(ctx context.Context, evt *gosignal.Event) {
	switch evt.Type {
	case EventFileCreated, EventFileDeleted:
		eh.updateFileProjection(ctx, evt.AggregateID)
	default:
		slog.Error("unknown event type", "event", evt.Type)
	}
}
