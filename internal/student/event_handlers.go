package student

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

// HandleNewStudentEvent is a method that handles the NewStudentEvent
// it loads the student aggregate from the repository and projects it to the database
func (eh *eventHandlers) HandleNewStudentEvent(ctx context.Context, aggregateID uint64) {
	student, err := eh.repo.loadStudent(ctx, aggregateID)
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
func (eh *eventHandlers) HandleUpdateStudentEvent(ctx context.Context, aggID uint64) {
	eh.HandleNewStudentEvent(ctx, aggID)
}

// HandleGenerateCodeEvent is a method that handles the GenerateCodeEvent
func (eh *eventHandlers) HandleGenerateCodeEvent(ctx context.Context, aggID uint64) {
	student, err := eh.repo.loadStudent(ctx, aggID)
	if err != nil {
		slog.Error("failed to load student", "error", err)
		return
	}

	code := student.data.CodeUniqueId
	if len(code) == 0 {
		slog.Error("code is empty")
		return
	}

	if err := eh.repo.insertStudentCode(ctx, aggID, code); err != nil {
		slog.Error("failed to insert student code", "error", err)
		return
	}
}

// handleSetProfilePhotoEvent is a method that handles the SetProfilePhotoEvent
func (eh *eventHandlers) handleSetProfilePhotoEvent(ctx context.Context, aggID uint64) {
	student, err := eh.repo.loadStudent(ctx, aggID)
	if err != nil {
		slog.Error("failed to load student", "error", err)
		return
	}

	if err := eh.repo.upsertStudentProfilePhoto(student); err != nil {
		slog.Error("failed to upsert student profile photo", "error", err)
		return
	}
}

// handleFeedStudentEvent is a method that handles the FeedStudentEvent
func (eh *eventHandlers) handleFeedStudentEvent(ctx context.Context, aggID uint64) {
	student, err := eh.repo.loadStudent(ctx, aggID)
	if err != nil {
		slog.Error("failed to load student", "error", err)
		return
	}

	// TODO: use the event version here because if somehow we end up with a later version of the event
	// we may miss a prior feeding event.
	if err := eh.repo.upsertFeedingEventProjection(student); err != nil {
		slog.Error("failed to upsert student feed", "error", err)
		return
	}
}

// handleUpdateSponsorshipEvent is a method that handles the UpdateSponsorshipEvent
func (eh *eventHandlers) handleUpdateSponsorshipEvent(ctx context.Context, aggID uint64) {
	student, err := eh.repo.loadStudent(ctx, aggID)
	if err != nil {
		slog.Error("failed to load student", "error", err)
		return
	}

	if err := eh.repo.upsertSponsorshipProjections(student); err != nil {
		slog.Error("failed to upsert sponsorship projections", "error", err)
		return
	}
}

func (eh *eventHandlers) handleHealthAssessmentEvent(ctx context.Context, aggID uint64) {
	student, err := eh.repo.loadStudent(ctx, aggID)
	if err != nil {
		slog.Error("failed to load student", "error", err)
	}

	if err = eh.repo.updateAllHealthProjectionsForStudent(student); err != nil {
		slog.Error("failed to update health projections", "error", err)
	}
}

func (eh *eventHandlers) handleGradeReportEvent(ctx context.Context, aggID uint64) {
	student, err := eh.repo.loadStudent(ctx, aggID)
	if err != nil {
		slog.Error("failed to load student", "error", err)
	}

	if err = eh.repo.updateAllGradeProjectionsForStudent(student); err != nil {
		slog.Error("failed to update grade projections", "error", err)
	}
}

// routeEvent is a method that routes an event to the appropriate handler
func (eh *eventHandlers) routeEvent(ctx context.Context, evt *gosignal.Event) {
	id, err := strconv.ParseUint(evt.AggregateID, 10, 64)
	if err != nil {
		slog.Error("failed to parse aggregate ID", "error", err)
		return
	}

	switch evt.Type {
	case EVENT_ADD_STUDENT:
		eh.HandleNewStudentEvent(ctx, id)
	case EVENT_UPDATE_STUDENT, EVENT_ENROLL_STUDENT, EVENT_UNENROLL_STUDENT, EVENT_SET_STUDENT_STATUS, EVENT_SET_ELIGIBILITY, EVENT_UNDO_CREATE_STUDENT:
		eh.HandleUpdateStudentEvent(ctx, id)
	case EVENT_SET_LOOKUP_CODE:
		eh.HandleGenerateCodeEvent(ctx, id)
	case EVENT_SET_PROFILE_PHOTO:
		eh.handleSetProfilePhotoEvent(ctx, id)
	case EVENT_FEED_STUDENT:
		eh.handleFeedStudentEvent(ctx, id)
	case EVENT_UPDATE_SPONSORSHIP:
		eh.handleUpdateSponsorshipEvent(ctx, id)
	case EVENT_ADD_HEALTH_ASSESSMENT, EVENT_REMOVE_HEALTH_ASSESSMENT:
		eh.handleHealthAssessmentEvent(ctx, id)
	case EVENT_ADD_GRADE_REPORT, EVENT_REMOVE_GRADE_REPORT:
		eh.handleGradeReportEvent(ctx, id)
	}
}
