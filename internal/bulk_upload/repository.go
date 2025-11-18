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
	"strings"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/sourcing"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:embed db/migrations/*.sql
var migrations embed.FS

// DomainEvent represents a raw event from the event store
type DomainEvent struct {
	Type        string
	AggregateID string
	Version     int
	Timestamp   time.Time
	Data        []byte
}

// EventStatistics represents aggregate statistics about events
type EventStatistics struct {
	TotalEvents      uint
	EventsByType     map[string]uint
	OldestEventTime  time.Time
	NewestEventTime  time.Time
	UniqueAggregates uint
}

type Repository interface {
	loadBulkUpload(ctx context.Context, id string) (*Aggregate, error)
	upsertProjection(upload *Aggregate) error
	saveEvents(ctx context.Context, evts []gosignal.Event) error
	listBulkUploads(ctx context.Context, limit, page uint) ([]sqlc.BulkUploadProjection, error)
	countBulkUploads(ctx context.Context) (uint, error)
	GetDomainEvents(ctx context.Context, limit, offset uint, eventTypeFilter, aggregateIDFilter string, startDate, endDate *time.Time) ([]DomainEvent, uint, error)
	GetEventTypes(ctx context.Context) ([]string, error)
	GetEventStatistics(ctx context.Context) (*EventStatistics, error)
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

// GetDomainEvents retrieves domain events with pagination and optional filtering
func (r *sqlRepository) GetDomainEvents(ctx context.Context, limit, offset uint, eventTypeFilter, aggregateIDFilter string, startDate, endDate *time.Time) ([]DomainEvent, uint, error) {
	var whereClauses []string
	var countArgs []interface{}
	var queryArgs []interface{}

	if eventTypeFilter != "" {
		whereClauses = append(whereClauses, "type = ?")
		countArgs = append(countArgs, eventTypeFilter)
	}

	if aggregateIDFilter != "" {
		whereClauses = append(whereClauses, "aggregate_id = ?")
		countArgs = append(countArgs, aggregateIDFilter)
	}

	if startDate != nil {
		whereClauses = append(whereClauses, "timestamp >= ?")
		countArgs = append(countArgs, startDate.Unix())
	}

	if endDate != nil {
		whereClauses = append(whereClauses, "timestamp <= ?")
		countArgs = append(countArgs, endDate.Unix())
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Get total count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM bulk_upload_events%s", whereClause)
	var total uint
	if err := r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count events: %w", err)
	}

	// Get paginated events with data column
	query := fmt.Sprintf(`
		SELECT type, aggregate_id, version, timestamp, data
		FROM bulk_upload_events%s
		ORDER BY timestamp DESC, aggregate_id DESC, version DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	queryArgs = append(queryArgs, countArgs...)
	queryArgs = append(queryArgs, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []DomainEvent
	for rows.Next() {
		var evt DomainEvent
		var timestamp int64
		var aggregateID string

		if err := rows.Scan(&evt.Type, &aggregateID, &evt.Version, &timestamp, &evt.Data); err != nil {
			return nil, 0, fmt.Errorf("failed to scan event: %w", err)
		}

		evt.Timestamp = time.Unix(timestamp, 0)
		evt.AggregateID = aggregateID

		events = append(events, evt)
	}

	return events, total, nil
}

// GetEventTypes retrieves all distinct event types for this domain
func (r *sqlRepository) GetEventTypes(ctx context.Context) ([]string, error) {
	query := "SELECT DISTINCT type FROM bulk_upload_events ORDER BY type"
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query event types: %w", err)
	}
	defer rows.Close()

	var eventTypes []string
	for rows.Next() {
		var eventType string
		if err := rows.Scan(&eventType); err != nil {
			return nil, fmt.Errorf("failed to scan event type: %w", err)
		}
		eventTypes = append(eventTypes, eventType)
	}

	return eventTypes, nil
}

// GetEventStatistics retrieves aggregate statistics about events
func (r *sqlRepository) GetEventStatistics(ctx context.Context) (*EventStatistics, error) {
	stats := &EventStatistics{
		EventsByType: make(map[string]uint),
	}

	// Get total count and unique aggregates
	query := `
		SELECT 
			COUNT(*) as total_events,
			COUNT(DISTINCT aggregate_id) as unique_aggregates,
			MIN(timestamp) as oldest_timestamp,
			MAX(timestamp) as newest_timestamp
		FROM bulk_upload_events
	`
	var oldestTS, newestTS sql.NullInt64
	if err := r.db.QueryRowContext(ctx, query).Scan(&stats.TotalEvents, &stats.UniqueAggregates, &oldestTS, &newestTS); err != nil {
		return nil, fmt.Errorf("failed to get event statistics: %w", err)
	}

	if oldestTS.Valid {
		stats.OldestEventTime = time.Unix(oldestTS.Int64, 0)
	}
	if newestTS.Valid {
		stats.NewestEventTime = time.Unix(newestTS.Int64, 0)
	}

	// Get events by type
	typeQuery := `SELECT type, COUNT(*) as count FROM bulk_upload_events GROUP BY type ORDER BY count DESC`
	rows, err := r.db.QueryContext(ctx, typeQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query events by type: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var eventType string
		var count uint
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan event type count: %w", err)
		}
		stats.EventsByType[eventType] = count
	}

	return stats, nil
}
