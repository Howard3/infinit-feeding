package bulk_upload

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

// HandleBulkUploadEvent handles any event that requires updating the projection
func (eh *eventHandlers) HandleBulkUploadEvent(ctx context.Context, evt *gosignal.Event) {
	upload, err := eh.repo.loadBulkUpload(ctx, evt.AggregateID)
	if err != nil {
		slog.Error("failed to load bulk upload", "error", err)
		return
	}

	if err := eh.repo.upsertProjection(upload); err != nil {
		slog.Error("failed to upsert bulk upload", "error", err)
		return
	}
}

// routeEvent routes events to their appropriate handlers
func (eh *eventHandlers) routeEvent(ctx context.Context, evt *gosignal.Event) {
	switch evt.Type {
	case EventCreate,
		EventStartProcessing,
		EventUpdateProgress,
		EventComplete,
		EventStartInvalidation,
		EventCompleteInvalidation,
		EventFail:
		eh.HandleBulkUploadEvent(ctx, evt)
	default:
		slog.Error("unknown event type", "type", evt.Type)
	}
}
