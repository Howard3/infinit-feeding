package bulk_upload

import (
	"context"
	"fmt"
	"geevly/gen/go/eda"
	"geevly/internal/bulk_upload/db/sqlc"

	"github.com/Howard3/gosignal"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
	repo          Repository
	eventHandlers *eventHandlers
	acl           AntiCorruptionLayer
}

// Helper function to get timestamp for a specific status
func getStatusTimestamp(statuses []*eda.BulkUpload_StatusTimestamp, status eda.BulkUpload_Status) *timestamppb.Timestamp {
	for _, st := range statuses {
		if st.Status == status {
			return st.Timestamp
		}
	}
	return nil
}

type AntiCorruptionLayer interface {
	ValidateFileID(ctx context.Context, fileID string) error
}

func NewService(repo Repository, acl AntiCorruptionLayer) *Service {
	return &Service{
		repo:          repo,
		eventHandlers: NewEventHandlers(repo),
		acl:           acl,
	}
}

type ListResponse struct {
	BulkUploads []sqlc.BulkUploadProjection
	Count       uint
}

// GetBulkUpload retrieves a single bulk upload by ID
func (s *Service) GetBulkUpload(ctx context.Context, id string) (*Aggregate, error) {
	agg, err := s.repo.loadBulkUpload(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load bulk upload: %w", err)
	}

	return agg, nil
}

func (s *Service) Create(ctx context.Context, cmd *eda.BulkUpload_Create) (*Aggregate, error) {
	if err := s.acl.ValidateFileID(ctx, cmd.FileId); err != nil {
		return nil, fmt.Errorf("invalid file ID: %w", err)
	}

	agg := &Aggregate{}
	id := uuid.New().String()
	agg.SetID(id)

	evt, err := agg.Create(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to create bulk upload: %w", err)
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*evt}); err != nil {
		return nil, fmt.Errorf("failed to save bulk upload event: %w", err)
	}

	s.eventHandlers.HandleBulkUploadEvent(ctx, evt)

	return agg, nil
}

func (s *Service) SaveValidationErrors(ctx context.Context, id string, res []*eda.BulkUpload_ValidationError) error {
	agg, err := s.repo.loadBulkUpload(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load bulk upload: %w", err)
	}

	event, err := agg.AddValidationErrors(res)
	if err != nil {
		return fmt.Errorf("failed to add validation errors: %w", err)
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*event}); err != nil {
		return fmt.Errorf("failed to save bulk upload event: %w", err)
	}

	s.eventHandlers.HandleBulkUploadEvent(ctx, event)

	return nil
}

// ListBulkUploads retrieves a paginated list of bulk uploads
func (s *Service) ListBulkUploads(ctx context.Context, limit, page uint) (*ListResponse, error) {
	// Get bulk uploads
	uploads, err := s.repo.listBulkUploads(ctx, limit, page)
	if err != nil {
		return nil, fmt.Errorf("failed to list bulk uploads: %w", err)
	}

	// Get total count
	count, err := s.repo.countBulkUploads(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count bulk uploads: %w", err)
	}

	return &ListResponse{
		BulkUploads: uploads,
		Count:       count,
	}, nil
}

func (s *Service) SetStatus(ctx context.Context, id string, status eda.BulkUpload_Status) error {
	agg, err := s.repo.loadBulkUpload(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load bulk upload: %w", err)
	}

	event, err := agg.setStatus(status)
	if err != nil {
		return fmt.Errorf("failed to set status: %w", err)
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*event}); err != nil {
		return fmt.Errorf("failed to save bulk upload event: %w", err)
	}

	s.eventHandlers.HandleBulkUploadEvent(ctx, event)

	return nil
}

type RecordActions struct {
	RecordIds  []string
	RecordType eda.BulkUpload_RecordType
	Reason     eda.BulkUpload_RecordAction_Reason
}

func (s *Service) MarkRecordsAsUpdated(ctx context.Context, id string, actions RecordActions) error {
	agg, err := s.repo.loadBulkUpload(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load bulk upload: %w", err)
	}

	event, err := agg.markRecordsAsUpdated(actions)
	if err != nil {
		return fmt.Errorf("failed to mark records as processed: %w", err)
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*event}); err != nil {
		return fmt.Errorf("failed to save bulk upload event: %w", err)
	}

	s.eventHandlers.HandleBulkUploadEvent(ctx, event)

	return nil
}

func (s *Service) AddRecordsToProcess(ctx context.Context, id string, actions RecordActions) error {
	agg, err := s.repo.loadBulkUpload(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load bulk upload: %w", err)
	}

	event, err := agg.addRecordsToProcess(actions)
	if err != nil {
		return fmt.Errorf("failed to add records to process: %w", err)
	}

	if err := s.repo.saveEvents(ctx, []gosignal.Event{*event}); err != nil {
		return fmt.Errorf("failed to save bulk upload event: %w", err)
	}

	s.eventHandlers.HandleBulkUploadEvent(ctx, event)

	return nil
}
