package student

import (
	"context"
	"database/sql"
	"embed"
	_ "embed"
	"fmt"
	"geevly/gen/go/eda"
	"geevly/internal/infrastructure"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/drivers/eventstore"
	"github.com/Howard3/gosignal/drivers/snapshots"

	src "github.com/Howard3/gosignal/sourcing"
)

const MaxPageSize = 100

//go:embed migrations/*.sql
var migrations embed.FS

// Repository incorporates the methods for persisting and loading student aggregates and projections
type Repository interface {
	upsertStudent(student *Aggregate) error
	saveEvents(ctx context.Context, evts []gosignal.Event) error
	loadStudent(ctx context.Context, id uint64) (*Aggregate, error)
	CountStudents(ctx context.Context) (uint, error)
	ListStudents(ctx context.Context, limit, page uint) ([]*ProjectedStudent, error)
	GetNewID(ctx context.Context) (uint64, error)
	getEventHistory(ctx context.Context, id uint64) ([]gosignal.Event, error)
	insertStudentCode(ctx context.Context, id uint64, code []byte) error
	getStudentIDByCode(ctx context.Context, code []byte) (uint64, error)
}

type ProjectedStudent struct {
	ID               uint
	FirstName        string
	LastName         string
	SchoolID         string
	DateOfBirth      time.Time
	DateOfEnrollment time.Time
	Version          uint
	Active           bool
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

// GetNewID - returns a new unique ID
// given the table structure
//
//	CREATE TABLE IF NOT EXISTS aggregate_id_tracking (
//		type VARCHAR(255) NOT NULL,
//	 next_id INT NOT NULL,
//	);
//
// get the next ID for the given type
func (r *sqlRepository) GetNewID(ctx context.Context) (uint64, error) {
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

func (r *sqlRepository) setupEventSourcing() {
	repoOptions := []src.NewRepoOptions{
		src.WithEventStore(eventstore.SQLStore{DB: r.db, TableName: "student_events"}),
		src.WithQueue(r.queue),
		src.WithSnapshotStrategy(&snapshots.VersionIntervalStrategy{
			EveryNth: 10,
			Store: snapshots.SQLStore{
				DB:        r.db,
				TableName: "student_snapshots",
			},
		}),
	}

	r.eventSourcing = src.NewRepository(repoOptions...)
}

// SaveEvents - persists the generated events to the event store
func (r *sqlRepository) saveEvents(ctx context.Context, evts []gosignal.Event) error {
	return r.eventSourcing.Store(ctx, evts)
}

func (r *sqlRepository) CountStudents(ctx context.Context) (uint, error) {
	query := `SELECT COUNT(*) FROM student_projections`
	var count uint
	if err := r.db.QueryRow(query).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count students: %w", err)
	}

	return count, nil
}

func (r *sqlRepository) ListStudents(ctx context.Context, limit, page uint) ([]*ProjectedStudent, error) {
	query := `SELECT
		id, first_name, last_name, school_id, date_of_birth, date_of_enrollment, version, active
		FROM student_projections
		LIMIT ? OFFSET ?;
	`

	if limit > MaxPageSize {
		limit = MaxPageSize
	}

	if page < 1 {
		page = 1
	}

	page--

	rows, err := r.db.Query(query, limit, limit*page)
	if err != nil {
		return nil, fmt.Errorf("failed to list students: %w", err)
	}
	defer rows.Close()

	students := []*ProjectedStudent{}
	for rows.Next() {
		student := &ProjectedStudent{}
		var dateOfBirth, dateOfEnrollment sql.NullTime

		if err := rows.Scan(
			&student.ID,
			&student.FirstName,
			&student.LastName,
			&student.SchoolID,
			&dateOfBirth,
			&dateOfEnrollment,
			&student.Version,
			&student.Active,
		); err != nil {
			return nil, fmt.Errorf("failed to scan student: %w", err)
		}

		students = append(students, student)
	}

	return students, nil
}

// upsertStudent - persists the student projection to the database
func (r *sqlRepository) upsertStudent(agg *Aggregate) error {
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
			active = excluded.active,
			updated_at = CURRENT_TIMESTAMP;
	`

	active := agg.data.Status == eda.Student_ACTIVE
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
func (r *sqlRepository) loadStudent(ctx context.Context, id uint64) (*Aggregate, error) {
	studentAgg := &Aggregate{}
	studentAgg.SetIDUint64(id)

	if err := r.eventSourcing.Load(ctx, studentAgg, nil); err != nil {
		return nil, fmt.Errorf("failed to load student events: %w", err)
	}

	return studentAgg, nil
}

// getEventHistory - returns the event history for a student aggregate
func (r *sqlRepository) getEventHistory(ctx context.Context, id uint64) ([]gosignal.Event, error) {
	cfg := src.NewRepoLoaderConfigurator().SkipSnapshot(true).Build()
	sID := fmt.Sprintf("%d", id)
	return r.eventSourcing.LoadEvents(ctx, sID, cfg)
}

// insert code into lookup table
func (r *sqlRepository) insertStudentCode(ctx context.Context, id uint64, code []byte) error {
	query := `INSERT INTO student_code_lookup (id, code)
		VALUES (?, ?)
		ON CONFLICT (id) DO UPDATE SET code = excluded.code;
	`

	_, err := r.db.Exec(query, id, code)
	if err != nil {
		return fmt.Errorf("failed to insert student code: %w", err)
	}

	return nil
}

// getStudentIDByCode - returns the student ID by the given code
func (r *sqlRepository) getStudentIDByCode(ctx context.Context, code []byte) (uint64, error) {
	query := `SELECT id FROM student_code_lookup WHERE code = ?`
	var id uint64

	if err := r.db.QueryRow(query, code).Scan(&id); err != nil {
		return 0, fmt.Errorf("failed to get student ID by code: %w", err)
	}

	return id, nil
}
