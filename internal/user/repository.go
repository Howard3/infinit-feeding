package user

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"geevly/internal/infrastructure"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/sourcing"
)

const MaxPageSize = 100

// ErrUserIDInvalid is an error that is returned when a user ID is invalid
var ErrUserIDInvalid = fmt.Errorf("user ID is invalid")

//go:embed migrations/*.sql
var migrations embed.FS

type Repository interface {
	loadUser(ctx context.Context, id uint64) (*User, error)
	upsertProjection(*User) error
	saveEvents(ctx context.Context, evts []gosignal.Event) error
	listUsers(ctx context.Context, limit, page uint) ([]*ProjectedUser, error)
	countUsers(ctx context.Context) (uint, error)
	getNewID(ctx context.Context) (uint64, error)
	getEventHistory(ctx context.Context, id uint64) ([]gosignal.Event, error)
	validateUserID(ctx context.Context, id uint64) error
}

// ProjectedUser is a struct that represents a user projection
type ProjectedUser struct {
	ID        uint
	Name      string
	Email     string
	Active    bool
	Version   int
	UpdatedAt time.Time
}

type sqlRepository struct {
	db            *sql.DB
	eventSourcing *sourcing.Repository
	queue         gosignal.Queue
}

func (r *sqlRepository) loadUser(ctx context.Context, id uint64) (*User, error) {
	agg := &User{}
	agg.SetIDUint64(id)

	if err := r.eventSourcing.Load(ctx, agg, nil); err != nil {
		return nil, fmt.Errorf("failed to load user events: %w", err)
	}

	return agg, nil
}

// upsertProjection - updates or inserts a projection
func (r *sqlRepository) upsertProjection(agg *User) error {
	if agg == nil || agg.data == nil {
		return fmt.Errorf("cannot upsert nil aggregate")
	}

	query := `INSERT INTO users 
		(id, name, email, active, version, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET 
			name = EXCLUDED.name,
			email = EXCLUDED.email,
			active = EXCLUDED.active,
			version = EXCLUDED.version,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id;
	`

	_, err := r.db.Exec(
		query,
		agg.ID,
		fmt.Sprintf("%s %s", agg.data.Name.First, agg.data.Name.Last),
		agg.data.Email,
		agg.data.Active,
		agg.Version,
	)

	if err != nil {
		return fmt.Errorf("failed to upsert user: %w", err)
	}

	return nil

}
func (r *sqlRepository) saveEvents(ctx context.Context, evts []gosignal.Event) error {
	return r.eventSourcing.Store(ctx, evts)
}
func (r *sqlRepository) listUsers(ctx context.Context, limit uint, page uint) ([]*ProjectedUser, error) {
	query := `SELECT id, name, email, active, version, updated_at FROM users LIMIT ? OFFSET ?;`

	if limit > MaxPageSize {
		limit = MaxPageSize
	}

	if page < 1 {
		page = 1
	}

	page--

	rows, err := r.db.Query(query, limit, limit*page)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	users := []*ProjectedUser{}
	for rows.Next() {
		user := &ProjectedUser{}
		if err := rows.Scan(
			&user.ID,
			&user.Name,
			&user.Email,
			&user.Active,
			&user.Version,
			&user.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		users = append(users, user)
	}

	return users, nil
}

func (r *sqlRepository) countUsers(ctx context.Context) (uint, error) {
	var count uint
	query := `SELECT COUNT(*) FROM users;`
	if err := r.db.QueryRow(query).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}

	return count, nil
}

func NewRepository(conn infrastructure.SQLConnection, queue gosignal.Queue) Repository {
	db, err := sql.Open(string(conn.Type), conn.URI)
	if err != nil {
		panic(fmt.Errorf("failed to open database: %w", err))
	}

	repo := &sqlRepository{db: db}

	if err := infrastructure.MigrateSQLDatabase(`user`, string(conn.Type), db, migrations); err != nil {
		panic(fmt.Errorf("failed to migrate database: %w", err))
	}

	repo.queue = queue
	repo.setupEventSourcing(conn)

	return repo
}

func (r *sqlRepository) setupEventSourcing(conn infrastructure.SQLConnection) {
	es := conn.GetSourcingConnection(r.db, "user_events")

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
	const typ = "user"
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

// getEventHistory - returns the event history for a user aggregate
func (r *sqlRepository) getEventHistory(ctx context.Context, id uint64) ([]gosignal.Event, error) {
	sID := fmt.Sprintf("%d", id)
	return r.eventSourcing.LoadEvents(ctx, sID, nil)
}

// validateUserID - checks if a user with the given ID exists
func (r *sqlRepository) validateUserID(ctx context.Context, id uint64) error {
	query := `SELECT id FROM users WHERE id = ?;`
	var exists uint64
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("id %d %w", id, ErrUserIDInvalid)
		}
		return fmt.Errorf("failed to validate user ID: %w", err)
	}

	return nil
}
