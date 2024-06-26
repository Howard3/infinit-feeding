package file

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"geevly/internal/infrastructure"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/sourcing"
	src "github.com/Howard3/gosignal/sourcing"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Repository interface {
	loadFile(ctx context.Context, id string) (*Aggregate, error)
	saveEvent(ctx context.Context, evt *gosignal.Event) error
	upsertFileProjection(ctx context.Context, file *Aggregate) error
	validateFileID(ctx context.Context, fileID string) error
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
