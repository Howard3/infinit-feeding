package student

import (
	"context"
	"database/sql"
	"embed"
	_ "embed"
	"fmt"
	student "geevly/events/gen/proto/go"
	"geevly/internal/infrastructure"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/drivers/eventstore"

	src "github.com/Howard3/gosignal/sourcing"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repository incorporates the methods for persisting and loading student aggregates and projections
type Repository interface {
	upsertStudent(student *Student) error
	saveEvents(ctx context.Context, evts []gosignal.Event) error
	loadStudent(ctx context.Context, id string) (*Student, error)
}

// sqlRepository is the implementation of the Repository interface using SQL
type sqlRepository struct {
	db            *sql.DB
	eventSourcing *src.Repository
	queue         gosignal.Queue
}

// NewRepository creates a new instance of the sqlRepository
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
	es := eventstore.SQLStore{DB: r.db, TableName: "student_events"}

	r.eventSourcing = src.NewRepository(src.WithEventStore(es), src.WithQueue(r.queue))
}

// SaveEvents - persists the generated events to the event store
func (r *sqlRepository) saveEvents(ctx context.Context, evts []gosignal.Event) error {
	return r.eventSourcing.Store(ctx, evts)
}

// upsertStudent - persists the student projection to the database
func (r *sqlRepository) upsertStudent(agg *Student) error {
	query := `INSERT INTO student_projections
		(id, first_name, last_name, school_id, date_of_birth, date_of_enrollment, version, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET 
			first_name = excluded.first_name,
			last_name = excluded.last_name,
			school_id = excluded.school_id,
			date_of_birth = excluded.date_of_birth,
			date_of_enrollment = excluded.date_of_enrollment,
			version = excluded.version,
			active = excluded.active;
	`

	active := agg.data.Status == student.StudentStatus_ACTIVE
	dob := agg.data.DateOfBirth
	dateOfBirth := time.Date(int(dob.Year), time.Month(dob.Month), int(dob.Day), 0, 0, 0, 0, time.UTC)
	doe := agg.data.DateOfEnrollment
	var dateOfEnrollment sql.NullTime

	if doe != nil {
		dateOfEnrollment.Time = time.Date(int(doe.Year), time.Month(doe.Month), int(doe.Day), 0, 0, 0, 0, time.UTC)
		dateOfEnrollment.Valid = true
	}

	_, err := r.db.Exec(
		query,
		agg.ID,
		agg.data.FirstName,
		agg.data.LastName,
		agg.data.SchoolId,
		dateOfBirth,
		dateOfEnrollment,
		agg.Version,
		active,
	)

	if err != nil {
		return fmt.Errorf("failed to upsert student: %w", err)
	}

	return nil
}

// loadStudent - loads the student aggregate from the event store
func (r *sqlRepository) loadStudent(ctx context.Context, id string) (*Student, error) {
	studentAgg := &Student{}
	studentAgg.SetID(id)

	if err := r.eventSourcing.Load(ctx, studentAgg, nil); err != nil {
		return nil, fmt.Errorf("failed to load student events: %w", err)
	}

	return studentAgg, nil
}
