package bulk_upload

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"geevly/gen/go/eda"
	"geevly/internal/bulk_upload/db"
	"geevly/internal/infrastructure"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/sourcing"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:embed db/schema/schema.sql
var schemaFiles embed.FS

type Repository interface {
	loadBulkUpload(ctx context.Context, id string) (*Aggregate, error)
	upsertProjection(upload *Aggregate) error
	saveEvents(ctx context.Context, evts []gosignal.Event) error
	listBulkUploads(ctx context.Context, limit, page uint) ([]*ProjectedBulkUpload, error)
	countBulkUploads(ctx context.Context) (uint, error)
	validateFileID(ctx context.Context, fileID string) error
}

type ProjectedBulkUpload struct {
	ID                      string
	Status                  string
	TargetDomain            string
	FileID                  string
	InitiatedAt             time.Time
	CompletedAt             *time.Time
	InvalidationStartedAt   *time.Time
	InvalidationCompletedAt *time.Time
	TotalRecords            int64
	ProcessedRecords        int64
	FailedRecords           int64
	UploadMetadata          map[string]string
	Version                 int
	UpdatedAt               time.Time
}

type sqlRepository struct {
	db            *sql.DB
	querier       db.Querier
	eventSourcing *sourcing.Repository
	queue         gosignal.Queue
}

func NewRepository(conn infrastructure.SQLConnection, queue gosignal.Queue) Repository {
	sqlDB, err := conn.Open()
	if err != nil {
		panic(fmt.Errorf("failed to open database: %w", err))
	}

	repo := &sqlRepository{
		db:      sqlDB,
		querier: db.New(sqlDB),
	}

	// Initialize the schema directly using our db package
	if err := db.InitSchema(sqlDB); err != nil {
		panic(fmt.Errorf("failed to initialize schema: %w", err))
	}

	repo.queue = queue
	repo.setupEventSourcing(conn)

	return repo
}

func (r *sqlRepository) setupEventSourcing(conn infrastructure.SQLConnection) {
	es := conn.GetSourcingConnection(r.db, "bulk_upload_events")
	r.eventSourcing = sourcing.NewRepository(sourcing.WithEventStore(es), sourcing.WithQueue(r.queue))

	// Ensure the SQLite database has the correct schema
	// This will be a no-op if the schema already exists
	r.ensureSchema()
}

// ensureSchema ensures that the SQLite database has the correct schema
func (r *sqlRepository) ensureSchema() {
	if err := db.InitSchema(r.db); err != nil {
		panic(fmt.Errorf("failed to initialize bulk upload schema: %w", err))
	}
}

func (r *sqlRepository) loadBulkUpload(ctx context.Context, id string) (*Aggregate, error) {
	agg := &Aggregate{}
	agg.SetID(id)

	if err := r.eventSourcing.Load(ctx, agg, nil); err != nil {
		return nil, fmt.Errorf("failed to load bulk upload events: %w", err)
	}

	return agg, nil
}

func (r *sqlRepository) upsertProjection(agg *Aggregate) error {
	metadata, err := json.Marshal(agg.data.UploadMetadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Extract timestamps from status timestamps
	var initiatedAt time.Time
	var completedAt, invalidationStartedAt, invalidationCompletedAt sql.NullTime

	// Find the relevant status timestamps
	for _, st := range agg.data.StatusTimestamps {
		switch st.Status {
		case eda.BulkUpload_PENDING:
			initiatedAt = st.Timestamp.AsTime()
		case eda.BulkUpload_COMPLETED:
			completedAt = sql.NullTime{Time: st.Timestamp.AsTime(), Valid: true}
		case eda.BulkUpload_INVALIDATING:
			invalidationStartedAt = sql.NullTime{Time: st.Timestamp.AsTime(), Valid: true}
		case eda.BulkUpload_INVALIDATED:
			invalidationCompletedAt = sql.NullTime{Time: st.Timestamp.AsTime(), Valid: true}
		}
	}

	// If no initiated timestamp found, use current time
	if initiatedAt.IsZero() {
		initiatedAt = time.Now()
	}

	params := db.UpsertBulkUploadProjectionParams{
		ID:                      agg.ID,
		Status:                  agg.data.Status.String(),
		TargetDomain:            agg.data.TargetDomain.String(),
		FileID:                  agg.data.FileId,
		InitiatedAt:             initiatedAt,
		CompletedAt:             completedAt,
		InvalidationStartedAt:   invalidationStartedAt,
		InvalidationCompletedAt: invalidationCompletedAt,
		TotalRecords:            int64(agg.data.TotalRecords),
		ProcessedRecords:        int64(agg.data.ProcessedRecords),
		FailedRecords:           int64(agg.data.FailedRecords),
		UploadMetadata:          string(metadata),
		Version:                 int64(agg.Version),
	}

	err = r.querier.UpsertBulkUploadProjection(context.Background(), params)
	if err != nil {
		return fmt.Errorf("failed to upsert bulk upload: %w", err)
	}

	return nil
}

func (r *sqlRepository) saveEvents(ctx context.Context, evts []gosignal.Event) error {
	return r.eventSourcing.Store(ctx, evts)
}

// listBulkUploads returns a paginated list of bulk uploads
func (r *sqlRepository) listBulkUploads(ctx context.Context, limit, page uint) ([]*ProjectedBulkUpload, error) {
	offset := page * limit

	params := db.ListBulkUploadsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	}

	dbUploads, err := r.querier.ListBulkUploads(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to query bulk uploads: %w", err)
	}

	uploads := make([]*ProjectedBulkUpload, 0, len(dbUploads))
	for _, dbUpload := range dbUploads {
		upload := &ProjectedBulkUpload{
			ID:               dbUpload.ID,
			Status:           dbUpload.Status,
			TargetDomain:     dbUpload.TargetDomain,
			FileID:           dbUpload.FileID,
			InitiatedAt:      dbUpload.InitiatedAt,
			TotalRecords:     dbUpload.TotalRecords,
			ProcessedRecords: dbUpload.ProcessedRecords,
			FailedRecords:    dbUpload.FailedRecords,
			UpdatedAt:        dbUpload.UpdatedAt,
			Version:          int(dbUpload.Version),
		}

		if dbUpload.CompletedAt.Valid {
			upload.CompletedAt = &dbUpload.CompletedAt.Time
		}
		if dbUpload.InvalidationStartedAt.Valid {
			upload.InvalidationStartedAt = &dbUpload.InvalidationStartedAt.Time
		}
		if dbUpload.InvalidationCompletedAt.Valid {
			upload.InvalidationCompletedAt = &dbUpload.InvalidationCompletedAt.Time
		}

		// Parse metadata JSON
		upload.UploadMetadata = make(map[string]string)
		if dbUpload.UploadMetadata != "" {
			if err := json.Unmarshal([]byte(dbUpload.UploadMetadata), &upload.UploadMetadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		uploads = append(uploads, upload)
	}

	return uploads, nil
}

// countBulkUploads returns the total number of bulk uploads
func (r *sqlRepository) countBulkUploads(ctx context.Context) (uint, error) {
	count, err := r.querier.CountBulkUploads(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count bulk uploads: %w", err)
	}

	return uint(count), nil
}

// validateFileID checks if a file ID exists in the system
func (r *sqlRepository) validateFileID(ctx context.Context, fileID string) error {
	exists, err := r.querier.ValidateFileID(ctx, fileID)
	if err != nil {
		return fmt.Errorf("failed to validate file ID: %w", err)
	}

	if !exists {
		return fmt.Errorf("file ID %s does not exist", fileID)
	}

	return nil
}

// Helper function to convert protobuf timestamp to sql.NullTime
func timeFromProto(ts *timestamppb.Timestamp) sql.NullTime {
	if ts == nil {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{
		Time:  ts.AsTime(),
		Valid: true,
	}
}
