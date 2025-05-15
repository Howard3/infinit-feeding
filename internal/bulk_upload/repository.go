package bulk_upload

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"geevly/gen/go/eda"
	"geevly/internal/bulk_upload/db/sqlc"
	"geevly/internal/infrastructure"
	"io/fs"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/sourcing"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:embed db/migrations/*.sql
var migrations embed.FS

type Repository interface {
	loadBulkUpload(ctx context.Context, id string) (*Aggregate, error)
	upsertProjection(upload *Aggregate) error
	saveEvents(ctx context.Context, evts []gosignal.Event) error
	listBulkUploads(ctx context.Context, limit, page uint) ([]sqlc.BulkUploadProjection, error)
	countBulkUploads(ctx context.Context) (uint, error)
}

type sqlRepository struct {
	db            *sql.DB
	eventSourcing *sourcing.Repository
	queue         gosignal.Queue
	queries       *sqlc.Queries
}

func NewRepository(conn infrastructure.SQLConnection, queue gosignal.Queue) Repository {
	sqlDB, err := conn.Open()
	if err != nil {
		panic(fmt.Errorf("failed to open database: %w", err))
	}

	repo := &sqlRepository{
		db:      sqlDB,
		queries: sqlc.New(sqlDB),
	}

	// sub the fs because we need to make it compatible with infrastructure.MigrateSQLDatabase
	subMigrationsFS, err := fs.Sub(migrations, "db")
	if err != nil {
		panic(fmt.Errorf("failed to load migrations: %w", err))
	}

	if err := infrastructure.MigrateSQLDatabase(`bulk_upload`, string(conn.Type), sqlDB, subMigrationsFS); err != nil {
		panic(fmt.Errorf("failed to migrate database for bulk_upload: %w", err))
	}

	repo.queue = queue
	repo.setupEventSourcing(conn)

	return repo
}

func (r *sqlRepository) setupEventSourcing(conn infrastructure.SQLConnection) {
	es := conn.GetSourcingConnection(r.db, "bulk_upload_events")
	r.eventSourcing = sourcing.NewRepository(sourcing.WithEventStore(es), sourcing.WithQueue(r.queue))
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

	params := sqlc.UpsertBulkUploadProjectionParams{
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

	err = r.queries.UpsertBulkUploadProjection(context.Background(), params)
	if err != nil {
		return fmt.Errorf("failed to upsert bulk upload: %w", err)
	}

	return nil
}

func (r *sqlRepository) saveEvents(ctx context.Context, evts []gosignal.Event) error {
	return r.eventSourcing.Store(ctx, evts)
}

// listBulkUploads returns a paginated list of bulk uploads
func (r *sqlRepository) listBulkUploads(ctx context.Context, limit, page uint) ([]sqlc.BulkUploadProjection, error) {
	offset := page * limit

	params := sqlc.ListBulkUploadsParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	}

	uploads, err := r.queries.ListBulkUploads(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to query bulk uploads: %w", err)
	}

	return uploads, nil
}

// countBulkUploads returns the total number of bulk uploads
func (r *sqlRepository) countBulkUploads(ctx context.Context) (uint, error) {
	count, err := r.queries.CountBulkUploads(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count bulk uploads: %w", err)
	}

	return uint(count), nil
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
