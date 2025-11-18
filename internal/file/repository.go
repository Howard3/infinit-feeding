package file

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"geevly/internal/infrastructure"
	"strings"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/sourcing"
	src "github.com/Howard3/gosignal/sourcing"
)

//go:embed migrations/*.sql
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
	loadFile(ctx context.Context, id string) (*Aggregate, error)
	saveEvent(ctx context.Context, evt *gosignal.Event) error
	upsertFileProjection(ctx context.Context, file *Aggregate) error
	validateFileID(ctx context.Context, fileID string) error
	GetDomainEvents(ctx context.Context, limit, offset uint, eventTypeFilter, aggregateIDFilter string, startDate, endDate *time.Time) ([]DomainEvent, uint, error)
	GetEventTypes(ctx context.Context) ([]string, error)
	GetEventStatistics(ctx context.Context) (*EventStatistics, error)
}

type sqlRepository struct {
	db            *sql.DB
	eventSourcing *src.Repository
	queue         gosignal.Queue
}

// use the following as the basis for the upsert
func (sr *sqlRepository) upsertFileProjection(ctx context.Context, file *Aggregate) error {
	query := `INSERT INTO files (id, domain, name, deleted, version, updated_at)
		VALUES (?,?,?,?,?, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
		domain = excluded.domain,
		name = excluded.name,
		deleted = excluded.deleted,
		version = excluded.version,
		updated_at = excluded.updated_at;`

	_, err := sr.db.ExecContext(
		ctx, query, file.ID, file.data.GetDomainReference(), file.data.GetName(),
		file.data.GetDeleted(), file.Version,
	)

	if err != nil {
		return fmt.Errorf("failed to upsert file: %w", err)
	}

	return nil
}

func (sr *sqlRepository) loadFile(ctx context.Context, id string) (*Aggregate, error) {
	agg := Aggregate{}
	agg.SetID(id)
	if err := sr.eventSourcing.Load(ctx, &agg, nil); err != nil {
		return nil, err
	}

	return &agg, nil
}

// validateFileID - searches the database for the file ID
func (sr *sqlRepository) validateFileID(ctx context.Context, fileID string) error {
	query := `SELECT id FROM files WHERE id = ?;`
	var id string
	if err := sr.db.QueryRowContext(ctx, query, fileID).Scan(&id); err != nil {
		return fmt.Errorf("failed to validate file ID: %w", err)
	}

	return nil
}

// SaveEvents - persists the generated events to the event store
func (sr *sqlRepository) saveEvents(ctx context.Context, evts []gosignal.Event) error {
	return sr.eventSourcing.Store(ctx, evts)
}
func (sr *sqlRepository) saveEvent(ctx context.Context, evt *gosignal.Event) error {
	return sr.saveEvents(ctx, []gosignal.Event{*evt})
}

func NewRepository(conn infrastructure.SQLConnection, queue gosignal.Queue) Repository {
	db, err := conn.Open()
	if err != nil {
		panic(fmt.Errorf("failed to open database: %w", err))
	}

	repo := &sqlRepository{db: db}

	if err := infrastructure.MigrateSQLDatabase(`file`, string(conn.Type), db, migrations); err != nil {
		panic(fmt.Errorf("failed to migrate database: %w", err))
	}

	repo.queue = queue
	repo.setupEventSourcing(conn)

	return repo
}

func (sr *sqlRepository) setupEventSourcing(conn infrastructure.SQLConnection) {
	es := conn.GetSourcingConnection(sr.db, "file_events")

	sr.eventSourcing = sourcing.NewRepository(sourcing.WithEventStore(es), sourcing.WithQueue(sr.queue))
}

// GetDomainEvents retrieves domain events with pagination and optional filtering
func (sr *sqlRepository) GetDomainEvents(ctx context.Context, limit, offset uint, eventTypeFilter, aggregateIDFilter string, startDate, endDate *time.Time) ([]DomainEvent, uint, error) {
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
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM file_events%s", whereClause)
	var total uint
	if err := sr.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count events: %w", err)
	}

	// Get paginated events with data column
	query := fmt.Sprintf(`
		SELECT type, aggregate_id, version, timestamp, data
		FROM file_events%s
		ORDER BY timestamp DESC, aggregate_id DESC, version DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	queryArgs = append(queryArgs, countArgs...)
	queryArgs = append(queryArgs, limit, offset)

	rows, err := sr.db.QueryContext(ctx, query, queryArgs...)
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
func (sr *sqlRepository) GetEventTypes(ctx context.Context) ([]string, error) {
	query := "SELECT DISTINCT type FROM file_events ORDER BY type"
	rows, err := sr.db.QueryContext(ctx, query)
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
func (sr *sqlRepository) GetEventStatistics(ctx context.Context) (*EventStatistics, error) {
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
		FROM file_events
	`
	var oldestTS, newestTS sql.NullInt64
	if err := sr.db.QueryRowContext(ctx, query).Scan(&stats.TotalEvents, &stats.UniqueAggregates, &oldestTS, &newestTS); err != nil {
		return nil, fmt.Errorf("failed to get event statistics: %w", err)
	}

	if oldestTS.Valid {
		stats.OldestEventTime = time.Unix(oldestTS.Int64, 0)
	}
	if newestTS.Valid {
		stats.NewestEventTime = time.Unix(newestTS.Int64, 0)
	}

	// Get events by type
	typeQuery := `SELECT type, COUNT(*) as count FROM file_events GROUP BY type ORDER BY count DESC`
	rows, err := sr.db.QueryContext(ctx, typeQuery)
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
