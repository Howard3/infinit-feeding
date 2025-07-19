package school

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

// HandleNewSchoolEvent is a method that handles the NewSchoolEvent
// it loads the school aggregate from the repository and projects it to the database
func (eh *eventHandlers) HandleNewSchoolEvent(ctx context.Context, evt *gosignal.Event) {
	aggID, err := strconv.ParseUint(evt.AggregateID, 10, 64)
	if err != nil {
		slog.Error("failed to parse aggregate id", "error", err)
		return
	}

	school, err := eh.repo.loadSchool(ctx, aggID)
	if err != nil {
		slog.Error("failed to load school", "error", err)
		return
	}

	if err := eh.repo.upsertProjection(school); err != nil {
		slog.Error("failed to upsert school", "error", err)
		return
	}
}

// HandleUpdateSchoolEvent is a method that handles the UpdateSchoolEvent
// functionally the same as HandleNewSchoolEvent, thus it just aliases it
func (eh *eventHandlers) HandleUpdateSchoolEvent(ctx context.Context, evt *gosignal.Event) {
	eh.HandleNewSchoolEvent(ctx, evt)
}

// HandleSetStatusEvent is a method that handles the SetStatusEvent
// functionally the same as HandleNewSchoolEvent, thus it just aliases it
func (eh *eventHandlers) HandleSetStatusEvent(ctx context.Context, evt *gosignal.Event) {
	eh.HandleNewSchoolEvent(ctx, evt)
}

// HandleSetSchoolPeriodEvent is a method that handles the SetSchoolPeriodEvent
// functionally the same as HandleNewSchoolEvent, thus it just aliases it
func (eh *eventHandlers) HandleSetSchoolPeriodEvent(ctx context.Context, evt *gosignal.Event) {
	eh.HandleNewSchoolEvent(ctx, evt)
}
