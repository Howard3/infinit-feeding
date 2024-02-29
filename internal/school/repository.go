package school

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"geevly/internal/infrastructure"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/drivers/eventstore"
	"github.com/Howard3/gosignal/sourcing"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Repository interface {
	loadSchool(ctx context.Context, id uint64) (*Aggregate, error)
	upsertProjection(school *Aggregate) error
	saveEvents(ctx context.Context, evts []gosignal.Event) error
	listSchools(ctx context.Context, limit, page uint)
	getNewID(ctx context.Context) (uint64, error)
	getAggregateEventHistory(ctx context.Context, id uint) ([]gosignal.Event, error)
}

// ProjectedSchool is a struct that represents a school projection
type ProjectedSchool struct {
	ID     uint
	Name   string // name of the school
	Active bool   // is this scool currently active
}

type sqlRepository struct {
	db            *sql.DB
	eventSourcing *sourcing.Repository
	queue         gosignal.Queue
}

func (r *sqlRepository) loadSchool(ctx context.Context, id uint64) (*Aggregate, error) {
	agg := &Aggregate{}
	agg.SetIDUint64(id)

	if err := r.eventSourcing.Load(ctx, agg, nil); err != nil {
		return nil, fmt.Errorf("failed to load school events: %w", err)
	}

	return agg, nil
}

// upsertProjection - updates or inserts a projection
// the source table is
// CREATE TABLE IF NOT EXISTS schools (
//
//	id TEXT PRIMARY KEY,
//	name TEXT NOT NULL,
//	active BOOLEAN NOT NULL,
//	version INT NOT NULL,
//	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
//
// );
func (r *sqlRepository) upsertProjection(agg *Aggregate) error {
	if agg == nil || agg.data == nil {
		return fmt.Errorf("cannot upsert nil aggregate")
	}

	query := `INSERT INTO schools 
		(id, name, active, version, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET 
			name = EXCLUDED.name,
			active = EXCLUDED.active,
			version = EXCLUDED.version,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id;
	`

	active := !agg.data.Disabled

	_, err := r.db.Exec(
		query,
		agg.ID,
		agg.data.Name,
		active,
		agg.Version,
	)

	if err != nil {
		return fmt.Errorf("failed to upsert student: %w", err)
	}

	return nil

}
func (r *sqlRepository) saveEvents(ctx context.Context, evts []gosignal.Event) (_ error) {
	return r.eventSourcing.Store(ctx, evts)
}
func (sqlRepository) listSchools(ctx context.Context, limit uint, page uint) {
	panic("not implemented") // TODO: Implement
}
func (sqlRepository) getAggregateEventHistory(ctx context.Context, id uint) (_ []gosignal.Event, _ error) {
	panic("not implemented") // TODO: Implement
}

func NewRepository(conn infrastructure.SQLConnection, queue gosignal.Queue) Repository {
	db, err := sql.Open(string(conn.Type), conn.URI)
	if err != nil {
		panic(fmt.Errorf("failed to open database: %w", err))
	}

	repo := &sqlRepository{db: db}

	if err := infrastructure.MigrateSQLDatabase(string(conn.Type), db, migrations); err != nil {
		panic(fmt.Errorf("failed to migrate database: %w", err))
	}

	repo.queue = queue
	repo.setupEventSourcing()

	return repo
}

func (r *sqlRepository) setupEventSourcing() {
	es := eventstore.SQLStore{DB: r.db, TableName: "school_events"}

	r.eventSourcing = sourcing.NewRepository(sourcing.WithEventStore(es), sourcing.WithQueue(r.queue))
}

// getNewID - returns a new unique ID
// given the table structure
//
//	CREATE TABLE IF NOT EXISTS aggregate_id_tracking (
//		type VARCHAR(255) NOT NULL,
//	 next_id INT NOT NULL,
//	);
//
// get the next ID for the given type
func (r *sqlRepository) getNewID(ctx context.Context) (uint64, error) {
	const typ = "student"
	query := `INSERT INTO aggregate_id_tracking (type, next_id)
		VALUES (?, 1)
		ON CONFLICT (type) DO UPDATE SET next_id = aggregate_id_tracking.next_id + 1
		RETURNING next_id;
	`

	var id uint64
	if err := r.db.QueryRowContext(ctx, query, typ).Scan(&id); err != nil {
		return 0, fmt.Errorf("failed to get new ID: %w", err)
	}

	return id, nil
}
