package student

import (
	"context"
	"database/sql"
	"embed"
	_ "embed"
	"fmt"
	"geevly/gen/go/eda"
	"geevly/internal/infrastructure"
	"log/slog"
	"regexp"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/drivers/snapshots"

	src "github.com/Howard3/gosignal/sourcing"
)

const MaxPageSize = 100

//go:embed migrations/*.sql
var migrations embed.FS

// Repository incorporates the methods for persisting and loading student aggregates and projections
type Repository interface {
	upsertStudent(student *Aggregate) error
	upsertStudentProfilePhoto(student *Aggregate) error
	saveEvents(ctx context.Context, evts []gosignal.Event) error
	loadStudent(ctx context.Context, id uint64) (*Aggregate, error)
	CountStudents(ctx context.Context) (uint, error)
	ListStudents(ctx context.Context, limit, page uint) ([]*ProjectedStudent, error)
	ListStudentsForSchool(ctx context.Context, schoolID string) ([]*ProjectedStudent, error)
	GetNewID(ctx context.Context) (uint64, error)
	getEventHistory(ctx context.Context, id uint64) ([]gosignal.Event, error)
	insertStudentCode(ctx context.Context, id uint64, code []byte) error
	getStudentIDByCode(ctx context.Context, code []byte) (uint64, error)
	getStudentIDByStudentSchoolID(ctx context.Context, studentSchoolID string) (uint64, error)
	getEvent(ctx context.Context, id, version uint64) (*gosignal.Event, error)
	QueryFeedingHistory(ctx context.Context, query FeedingHistoryQuery) (*StudentFeedingProjections, error)
}

type ProjectedStudent struct {
	ID          uint
	FirstName   string
	LastName    string
	SchoolID    string
	DateOfBirth time.Time
	StudentID   string
	Grade       uint
	Version     uint
	Active      bool
	Age         uint
}

// source schema:
// CREATE TABLE IF NOT EXISTS student_feeding_projections (
//
//		student_id TEXT NOT NULL,
//	 feeding_id INT NOT NULL,
//		feeding_id INT NOT NULL,
//		school_id TEXT NOT NULL,
//		feeding_timestamp TIMESTAMPTZ NOT NULL,
//		PRIMARY KEY(student_id, feeding_id)
//
// );
type ProjectedFeedingEvent struct {
	StudentID       string
	FeedingID       uint64
	SchoolID        string
	FeedingDateTime time.Time
}

// sqlRepository is the implementation of the Repository interface using SQL
type sqlRepository struct {
	db            *sql.DB
	eventSourcing *src.Repository
	queue         gosignal.Queue
}

// NewRepository creates a new instance of the sqlRepository
func NewRepository(conn infrastructure.SQLConnection, queue gosignal.Queue) Repository {
	db, err := conn.Open()
	if err != nil {
		panic(fmt.Errorf("failed to open database: %w", err))
	}

	repo := &sqlRepository{db: db}

	if err := infrastructure.MigrateSQLDatabase(`student`, string(conn.Type), db, migrations); err != nil {
		panic(fmt.Errorf("failed to migrate database: %w", err))
	}

	repo.queue = queue
	repo.setupEventSourcing(conn)
	repo.updateProjections(context.Background()) // TODO: consider bubbling up the context further

	return repo
}

func (r *sqlRepository) updateProjections(ctx context.Context) {
	slog.Info("updating projections")
	whatToUpdate := r.checkForProjectionUpdates()
	if len(whatToUpdate) == 0 {
		slog.Info("no projections to update")
		return
	}

	for _, v := range whatToUpdate {
		switch v {
		case "student_projections":
			r.updateStudentProjections()
		case "student_feeding_projections":
			r.rebuildStudentFeedingProjections(ctx)
		default:
			panic(fmt.Errorf("unsupported projection: %s", v))
		}
	}

	r.clearProjectionUpdates()
}

// rebuildStudentFeedingProjections - completely rebuilds the student feeding projections
// NOTE: this is a very expensive operation and should only be used in exceptional circumstances
func (r *sqlRepository) rebuildStudentFeedingProjections(ctx context.Context) (err error) {
	slog.Info("updating student feeding projections")

	ids := r.getUniqueIDsForAggregates()
	projections := make([]ProjectedFeedingEvent, 0)

	slog.Info("loading student feeding reports", "count", len(ids))

	// populate the projections; this is necessary to do prior to starting the transaction because
	// the loadStudent method may make it's own inserts (snapshotting); transactions will be blocking.
	for _, id := range ids {
		student, err := r.loadStudent(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to load student: %w", err)
		}

		for _, report := range student.data.FeedingReport {
			timestamp := time.Unix(int64(report.UnixTimestamp), 0)
			projection := ProjectedFeedingEvent{
				StudentID:       student.GetID(),
				FeedingID:       report.Id,
				SchoolID:        student.data.SchoolId,
				FeedingDateTime: timestamp,
			}

			projections = append(projections, projection)
		}
	}

	// start a transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		panic(fmt.Errorf("failed to begin transaction: %w", err))
	}

	defer func() {
		if err != nil {
			slog.Error("rolling back transaction", "error", err)
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	slog.Info("deleting student feeding projections")

	// delete all feeding projections since we're doing a full update
	query := `DELETE FROM student_feeding_projections`
	if _, err := tx.Exec(query); err != nil {
		panic(fmt.Errorf("failed to delete student feeding projections: %w", err))
	}

	slog.Info("Updating student feeding projections", "count", len(ids))

	for _, projection := range projections {
		slog.Info("inserting feeding projection", "student_id", projection.StudentID, "feeding_id", projection.FeedingID)
		if err := r.insertFeedingProjection(tx, projection); err != nil {
			return fmt.Errorf("failed to insert feeding projection: %w", err)
		}
	}

	return nil
}

// insertFeedingProjection - inserts a feeding projection into the database
func (r *sqlRepository) insertFeedingProjection(tx *sql.Tx, pfe ProjectedFeedingEvent) error {
	query := `INSERT INTO student_feeding_projections
		(student_id, feeding_id, school_id, feeding_timestamp)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (student_id, feeding_id) DO NOTHING;
	`

	_, err := tx.Exec(query, pfe.StudentID, pfe.FeedingID, pfe.SchoolID, pfe.FeedingDateTime)
	if err != nil {
		return fmt.Errorf("failed to insert student feeding projection: %w", err)
	}

	return nil
}

func (r *sqlRepository) updateStudentProjections() {
	slog.Info("updating student projections")

	ids := r.getUniqueIDsForAggregates()
	for _, id := range ids {
		slog.Info("updating student projection for ID", "id", id)
		student, err := r.loadStudent(context.Background(), id)
		if err != nil {
			panic(fmt.Errorf("failed to load student: %w", err))
		}

		if err := r.upsertStudent(student); err != nil {
			panic(fmt.Errorf("failed to upsert student: %w", err))
		}
	}
}

func (r *sqlRepository) getUniqueIDsForAggregates() []uint64 {
	ids := []uint64{}
	query := `SELECT DISTINCT aggregate_id FROM student_events`

	rows, err := r.db.Query(query)
	if err != nil {
		panic(fmt.Errorf("get unique IDs for aggregates: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			panic(fmt.Errorf("scan unique ID: %w", err))
		}

		ids = append(ids, id)
	}

	return ids
}

func (r *sqlRepository) checkForProjectionUpdates() []string {
	whatToUpdate := []string{}
	query := `SELECT what from student_projection_updates`

	rows, err := r.db.Query(query)
	if err != nil {
		panic(fmt.Errorf("failed to check for projection updates: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var what string
		if err := rows.Scan(&what); err != nil {
			panic(fmt.Errorf("failed to scan projection update: %w", err))
		}

		whatToUpdate = append(whatToUpdate, what)
	}

	return whatToUpdate
}

func (r *sqlRepository) clearProjectionUpdates() {
	slog.Info("clearing projection updates")

	query := `DELETE FROM student_projection_updates`
	if _, err := r.db.Exec(query); err != nil {
		panic(fmt.Errorf("failed to clear projection updates: %w", err))
	}
}

// GetNewID - returns a new unique ID
// given the table structure
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

func (r *sqlRepository) setupEventSourcing(conn infrastructure.SQLConnection) {
	repoOptions := []src.NewRepoOptions{
		src.WithEventStore(conn.GetSourcingConnection(r.db, "student_events")),
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
		id, first_name, last_name, school_id, date_of_birth, student_id, age, grade, version, active
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
		var dateOfBirth, studentID sql.NullString
		var grade, age sql.NullInt64

		if err := rows.Scan(
			&student.ID,
			&student.FirstName,
			&student.LastName,
			&student.SchoolID,
			&dateOfBirth,
			&studentID,
			&age,
			&grade,
			&student.Version,
			&student.Active,
		); err != nil {
			return nil, fmt.Errorf("scan student: %w", err)
		}

		student.DateOfBirth = r.parseDate(dateOfBirth.String)
		student.StudentID = studentID.String
		student.Grade = uint(grade.Int64)
		student.Age = uint(age.Int64)

		students = append(students, student)
	}

	return students, nil
}

func (r *sqlRepository) parseDate(datestr string) time.Time {
	rx := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})`)
	matches := rx.FindStringSubmatch(datestr)
	if len(matches) < 2 {
		return time.Time{}
	}

	t, err := time.Parse("2006-01-02", matches[1])
	if err != nil {
		return time.Time{}
	}

	return t
}

// upsertStudent - persists the student projection to the database
func (r *sqlRepository) upsertStudent(agg *Aggregate) error {
	query := `INSERT INTO student_projections
		(id, first_name, last_name, school_id, date_of_birth, version, active, student_id, age, grade)
		VALUES (:id, :first_name, :last_name, :school_id, :date_of_birth, :version, :active, :student_id, :age, :grade)
		ON CONFLICT (id) DO UPDATE SET 
			first_name = excluded.first_name,
			last_name = excluded.last_name,
			school_id = excluded.school_id,
			date_of_birth = excluded.date_of_birth,
			version = excluded.version,
			active = excluded.active,
			student_id = excluded.student_id,
			age = excluded.age,
			grade = excluded.grade,
			updated_at = CURRENT_TIMESTAMP;
	`

	active := agg.data.Status == eda.Student_ACTIVE
	dob := agg.data.DateOfBirth
	dateOfBirth := time.Date(int(dob.Year), time.Month(dob.Month), int(dob.Day), 0, 0, 0, 0, time.UTC)
	doe := agg.data.DateOfEnrollment
	age := agg.GetAge()
	var dateOfEnrollment sql.NullTime

	if doe != nil {
		dateOfEnrollment.Time = time.Date(int(doe.Year), time.Month(doe.Month), int(doe.Day), 0, 0, 0, 0, time.UTC)
		dateOfEnrollment.Valid = true
	}

	_, err := r.db.Exec(
		query,
		sql.Named("id", agg.ID),
		sql.Named("first_name", agg.data.FirstName),
		sql.Named("last_name", agg.data.LastName),
		sql.Named("school_id", agg.data.SchoolId),
		sql.Named("date_of_birth", dateOfBirth),
		sql.Named("version", agg.Version),
		sql.Named("active", active),
		sql.Named("student_id", agg.data.StudentSchoolId),
		sql.Named("age", age),
		sql.Named("grade", agg.data.GradeLevel),
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

// GetEvent returns a single event by ID and version
func (r *sqlRepository) getEvent(ctx context.Context, id, version uint64) (*gosignal.Event, error) {
	cfg := src.NewRepoLoaderConfigurator().SkipSnapshot(true).MinVersion(version).MaxVersion(version).Build()
	sID := fmt.Sprintf("%d", id)
	evts, err := r.eventSourcing.LoadEvents(ctx, sID, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	if len(evts) == 0 {
		return nil, fmt.Errorf("event not found")
	}

	return &evts[0], nil
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

// getStudentIDByStudentSchoolID - returns the student ID by the given student school ID
func (r *sqlRepository) getStudentIDByStudentSchoolID(ctx context.Context, studentSchoolID string) (uint64, error) {
	if studentSchoolID == "" {
		return 0, fmt.Errorf("student school ID is required")
	}

	query := `SELECT id FROM student_projections WHERE student_id = ?`
	var id uint64

	if err := r.db.QueryRow(query, studentSchoolID).Scan(&id); err != nil {
		return 0, fmt.Errorf("failed to get student ID by student school ID: %w", err)
	}

	return id, nil
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

// upsertStudentProfilePhoto - persists the student profile photo to the database
func (r *sqlRepository) upsertStudentProfilePhoto(agg *Aggregate) error {
	query := `INSERT INTO student_profile_photos
		(id, file_id)
		VALUES (?, ?)
		ON CONFLICT (id) DO UPDATE SET file_id = excluded.file_id;
	`

	_, err := r.db.Exec(query, agg.ID, agg.data.ProfilePhotoId)
	if err != nil {
		return fmt.Errorf("failed to upsert student profile photo: %w", err)
	}

	return nil
}

// ListStudentsForSchool - returns all students for a school
func (r *sqlRepository) ListStudentsForSchool(ctx context.Context, schoolID string) ([]*ProjectedStudent, error) {
	query := `SELECT
		id, first_name, last_name, school_id, date_of_birth, student_id, age, grade, version, active
		FROM student_projections
		WHERE school_id = ?;
	`

	rows, err := r.db.Query(query, schoolID)
	if err != nil {
		return nil, fmt.Errorf("failed to get students for school: %w", err)
	}
	defer rows.Close()

	students := []*ProjectedStudent{}
	for rows.Next() {
		student := &ProjectedStudent{}
		var dateOfBirth, studentID sql.NullString
		var grade, age sql.NullInt64

		if err := rows.Scan(
			&student.ID,
			&student.FirstName,
			&student.LastName,
			&student.SchoolID,
			&dateOfBirth,
			&studentID,
			&age,
			&grade,
			&student.Version,
			&student.Active,
		); err != nil {
			return nil, fmt.Errorf("scan student: %w", err)
		}

		student.DateOfBirth = r.parseDate(dateOfBirth.String)
		student.StudentID = studentID.String
		student.Grade = uint(grade.Int64)
		student.Age = uint(age.Int64)

		students = append(students, student)
	}

	return students, nil
}

type FeedingHistoryQuery struct {
	SchoolID string
	From     time.Time
	To       time.Time
}

func (fhq FeedingHistoryQuery) Validate() error {
	if fhq.SchoolID == "" {
		return fmt.Errorf("school ID is required")
	}

	if fhq.From.IsZero() {
		return fmt.Errorf("from date is required")
	}

	if fhq.To.IsZero() {
		return fmt.Errorf("to date is required")
	}

	if fhq.From.After(fhq.To) {
		return fmt.Errorf("from date must be before to date")
	}

	return nil
}

type JoinedFeedingProjection struct {
	Student      ProjectedStudent
	FeedingEvent ProjectedFeedingEvent
}

type StudentFeedingProjections struct {
	projections []JoinedFeedingProjection
}

// GetAll - returns all feeding projections
func (sfp *StudentFeedingProjections) GetAll() []JoinedFeedingProjection {
	return sfp.projections
}

// GroupedByStudentReturn - represents a grouping of feeding projections by student
type GroupedByStudentReturn struct {
	Student       ProjectedStudent
	FeedingEvents []ProjectedFeedingEvent
}

func (gbsr *GroupedByStudentReturn) WasFedOnDay(t time.Time) bool {
	for _, evt := range gbsr.FeedingEvents {
		if evt.FeedingDateTime.Year() == t.Year() && evt.FeedingDateTime.Month() == t.Month() && evt.FeedingDateTime.Day() == t.Day() {
			return true
		}
	}

	return false
}

// GroupByStudent - groups the feeding projections by student ID
func (sfp *StudentFeedingProjections) GroupByStudent() []*GroupedByStudentReturn {
	grouped := []*GroupedByStudentReturn{}
	indexTracker := map[string]int{}

	for _, projection := range sfp.projections {
		key := projection.Student.StudentID
		if _, ok := indexTracker[key]; !ok {
			groupedRet := &GroupedByStudentReturn{
				Student:       projection.Student,
				FeedingEvents: []ProjectedFeedingEvent{},
			}

			grouped = append(grouped, groupedRet)
			indexTracker[key] = len(grouped) - 1
		}

		idx := indexTracker[key]
		grouped[idx].FeedingEvents = append(grouped[idx].FeedingEvents, projection.FeedingEvent)
	}

	return grouped
}

// QueryFeedingHistory - returns the feeding history for students
func (r *sqlRepository) QueryFeedingHistory(ctx context.Context, query FeedingHistoryQuery) (*StudentFeedingProjections, error) {
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}

	// query for feeding events
	q := `SELECT
		sp.id, sp.first_name, sp.last_name, sp.school_id, sp.date_of_birth, sp.student_id, sp.age, sp.grade, sp.version, sp.active,
		sfp.feeding_id, sfp.school_id, sfp.feeding_timestamp
		FROM student_feeding_projections sfp
		JOIN student_projections sp ON sp.id = sfp.student_id
		WHERE sfp.school_id = ? AND sfp.feeding_timestamp >= ? AND sfp.feeding_timestamp <= ?
		ORDER BY sp.last_name ASC, sfp.feeding_timestamp ASC;
	`
	rows, err := r.db.Query(q, query.SchoolID, query.From, query.To)
	if err != nil {
		return nil, fmt.Errorf("failed to query feeding history: %w", err)
	}
	defer rows.Close()

	projections := &StudentFeedingProjections{}
	for rows.Next() {
		var projection JoinedFeedingProjection
		var dateOfBirth, studentID, feedingTimestamp sql.NullString
		var grade, age sql.NullInt64

		if err := rows.Scan(
			&projection.Student.ID,
			&projection.Student.FirstName,
			&projection.Student.LastName,
			&projection.Student.SchoolID,
			&dateOfBirth,
			&studentID,
			&age,
			&grade,
			&projection.Student.Version,
			&projection.Student.Active,
			&projection.FeedingEvent.FeedingID,
			&projection.FeedingEvent.SchoolID,
			&feedingTimestamp,
		); err != nil {
			return nil, fmt.Errorf("scan feeding projection: %w", err)
		}

		projection.Student.DateOfBirth = r.parseDate(dateOfBirth.String)
		projection.Student.StudentID = studentID.String
		projection.Student.Grade = uint(grade.Int64)
		projection.Student.Age = uint(age.Int64)

		// parse feeding timestamp from 2024-05-05 19:59:48-05:00
		t, err := time.Parse("2006-01-02 15:04:05-07:00", feedingTimestamp.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse feeding timestamp: %w", err)
		}

		projection.FeedingEvent.FeedingDateTime = t

		projections.projections = append(projections.projections, projection)
	}

	return projections, nil
}
