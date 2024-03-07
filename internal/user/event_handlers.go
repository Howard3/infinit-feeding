package user

import (
	"context"
	"log/slog"
	"strconv"

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

func (eh *eventHandlers) handleUpdatedUserState(ctx context.Context, evt *gosignal.Event) {
	aggID, err := strconv.ParseUint(evt.AggregateID, 10, 64)
	if err != nil {
		slog.Error("failed to parse aggregate id", "error", err)
		return
	}

	user, err := eh.repo.loadUser(ctx, aggID)
	if err != nil {
		slog.Error("failed to load user", "error", err)
		return
	}

	if err := eh.repo.upsertProjection(user); err != nil {
		slog.Error("failed to upsert user", "error", err)
		return
	}
}

// routeEvent is a method that routes an event to the appropriate handler
func (eh *eventHandlers) routeEvent(ctx context.Context, evt *gosignal.Event) {
	switch evt.Type {
	case EventCreated, EventUpdated:
		eh.handleUpdatedUserState(ctx, evt)
	}
}
