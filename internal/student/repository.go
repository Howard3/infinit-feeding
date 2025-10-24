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
	"strings"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/drivers/snapshots"

	src "github.com/Howard3/gosignal/sourcing"
)

const MaxPageSize = 100

//go:embed migrations/*.sql
var migrations embed.FS

// ProjectedStudent - what information should be returned from a projected student
type ProjectedStudent struct {
	ID                     uint
	FirstName              string
	LastName               string
	SchoolID               string
	DateOfBirth            time.Time
	StudentID              string
	Grade                  uint
	Version                uint
	Active                 bool
	Age                    uint
	ProfilePhotoID         string
	EligibleForSponsorship bool
	QRCode                 string
}

// Add this new struct for filter options
type StudentListFilters struct {
	ActiveOnly                 bool
	EligibleForSponsorshipOnly bool
	SchoolIDs                  []uint64
	MinBirthDate               *time.Time
	MaxBirthDate               *time.Time
	NameSearch                 string
}

// Add these new types
type SponsorshipProjection struct {
	StudentID     string    `db:"student_id"`
	SponsorID     string    `db:"sponsor_id"`
	StartDate     time.Time `db:"start_date"`
	EndDate       time.Time `db:"end_date"`
	PaymentID     string    `db:"payment_id"`
	PaymentAmount float64   `db:"payment_amount"`
}

// Repository incorporates the methods for persisting and loading student aggregates and projections
type Repository interface {
	upsertStudent(student *Aggregate) error
	upsertStudentProfilePhoto(student *Aggregate) error
	upsertFeedingEventProjection(student *Aggregate) error
	saveEvents(ctx context.Context, evts []gosignal.Event) error
	loadStudent(ctx context.Context, id uint64) (*Aggregate, error)
	CountStudents(ctx context.Context, filters StudentListFilters) (uint, error)
	ListStudents(ctx context.Context, limit, page uint, filters StudentListFilters) ([]*ProjectedStudent, error)
	ListStudentsForSchool(ctx context.Context, schoolID string) ([]*ProjectedStudent, error)
	GetNewID(ctx context.Context) (uint64, error)
	getEventHistory(ctx context.Context, id uint64) ([]gosignal.Event, error)
	insertStudentCode(ctx context.Context, id uint64, code []byte) error
	getStudentIDByCode(ctx context.Context, code []byte) (uint64, error)
	getStudentIDByStudentSchoolID(ctx context.Context, studentSchoolID string) (uint64, error)
	getEvent(ctx context.Context, id, version uint64) (*gosignal.Event, error)
	QueryFeedingHistory(ctx context.Context, query FeedingHistoryQuery) (*StudentFeedingProjections, error)
	GetCurrentSponsorships(ctx context.Context, sponsorID string) ([]*SponsorshipProjection, error)
	upsertSponsorshipProjections(student *Aggregate) error
	GetAllSponsorshipsByID(ctx context.Context, sponsorID string) ([]*SponsorshipProjection, error)
	CountFeedingEventsInPeriod(ctx context.Context, studentID string, startDate, endDate time.Time) (int64, error)
	GetFeedingEventsForSponsorships(ctx context.Context, sponsorships []*SponsorshipProjection, limit, page uint) ([]*SponsorFeedingEvent, int64, error)
	GetAllCurrentSponsorships(ctx context.Context) ([]*SponsorshipProjection, error)
	GetAllFeedingEvents(ctx context.Context, limit, page uint) ([]*SponsorFeedingEvent, int64, error)
	getStudentByStudentAndSchoolID(ctx context.Context, studentSchoolID, schoolID string) (uint64, error)
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
	FeedingImageID  string
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
			go r.rebuildStudentFeedingProjections(ctx)
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
			// BUG: doesn't consider the school at the time of feeding, just the current student state.
			timestamp := time.Unix(int64(report.UnixTimestamp), 0)
			projection := ProjectedFeedingEvent{
				StudentID:       student.GetID(),
				FeedingID:       report.GetUnixTimestamp(),
				SchoolID:        student.data.SchoolId,
				FeedingDateTime: timestamp,
				FeedingImageID:  report.FileId,
			}

			projections = append(projections, projection)
		}
	}

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
		slog.Info("inserting feeding projection", "student_id", projection.StudentID, "feeding_id", projection.FeedingID, "feeding_image_id", projection.FeedingImageID)
		if err := r.insertFeedingProjection(tx, projection); err != nil {
			return fmt.Errorf("failed to insert feeding projection: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}

		tx, err = r.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
	}

	return nil
}

// insertFeedingProjection - inserts a feeding projection into the database
func (r *sqlRepository) insertFeedingProjection(tx *sql.Tx, pfe ProjectedFeedingEvent) error {
	query := `INSERT INTO student_feeding_projections
		(student_id, feeding_id, school_id, feeding_timestamp, feeding_image_id)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (student_id, feeding_id) DO NOTHING;
	`

	_, err := tx.Exec(query, pfe.StudentID, pfe.FeedingID, pfe.SchoolID, pfe.FeedingDateTime, pfe.FeedingImageID)
	if err != nil {
		return fmt.Errorf("failed to insert student feeding projection: %w", err)
	}

	return nil
}

// upsertFeedingEventProjection - persists the feeding event projection to the database
func (r *sqlRepository) upsertFeedingEventProjection(student *Aggregate) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	// BUG: doesn't consider the school at the time of feeding, just the current student state.
	pfe := ProjectedFeedingEvent{
		StudentID:       student.GetID(),
		FeedingID:       student.data.FeedingReport[len(student.data.FeedingReport)-1].GetUnixTimestamp(),
		SchoolID:        student.data.SchoolId,
		FeedingDateTime: time.Unix(int64(student.data.FeedingReport[len(student.data.FeedingReport)-1].UnixTimestamp), 0),
		FeedingImageID:  student.data.FeedingReport[len(student.data.FeedingReport)-1].FileId,
	}

	if err := r.insertFeedingProjection(tx, pfe); err != nil {
		return fmt.Errorf("failed to insert feeding projection: %w", err)
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

func (r *sqlRepository) CountStudents(ctx context.Context, filters StudentListFilters) (uint, error) {
	query := `SELECT COUNT(*) FROM student_projections`
	where := []string{}
	args := []interface{}{}

	if filters.ActiveOnly {
		where = append(where, "active = true")
	}

	if filters.EligibleForSponsorshipOnly {
		where = append(where, "eligible_for_sponsorship = true AND (max_sponsorship_date <= CURRENT_TIMESTAMP OR max_sponsorship_date IS NULL)")
	}

	if len(filters.SchoolIDs) > 0 {
		placeholders := make([]string, len(filters.SchoolIDs))
		for i, id := range filters.SchoolIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where = append(where, fmt.Sprintf("school_id IN (%s)", strings.Join(placeholders, ",")))
	}

	if filters.MinBirthDate != nil {
		where = append(where, "date_of_birth <= ?")
		args = append(args, filters.MinBirthDate)
	}

	if filters.MaxBirthDate != nil {
		where = append(where, "date_of_birth >= ?")
		args = append(args, filters.MaxBirthDate)
	}

	if filters.NameSearch != "" {
		where = append(where, "(LOWER(first_name) LIKE ? OR LOWER(last_name) LIKE ?)")
		searchTerm := "%" + strings.ToLower(filters.NameSearch) + "%"
		args = append(args, searchTerm, searchTerm)
	}

	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	var count uint
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count students: %w", err)
	}

	return count, nil
}

func (r *sqlRepository) ListStudents(ctx context.Context, limit, page uint, filters StudentListFilters) ([]*ProjectedStudent, error) {
	baseQuery := `SELECT
		sp.id, sp.first_name, sp.last_name, sp.school_id, sp.date_of_birth, sp.student_id, sp.age, sp.grade, sp.version, sp.active,
		spp.file_id as profile_photo_id, sp.eligible_for_sponsorship, spl.code
		FROM student_projections sp
		LEFT JOIN student_profile_photos spp ON sp.id = spp.id
        LEFT JOIN student_code_lookup spl on sp.id = spl.id`

	where := []string{}
	args := []interface{}{}

	if filters.ActiveOnly {
		where = append(where, "sp.active = true")
	}

	if filters.EligibleForSponsorshipOnly {
		where = append(where, "sp.eligible_for_sponsorship = true AND (sp.max_sponsorship_date <= CURRENT_TIMESTAMP OR sp.max_sponsorship_date IS NULL)")
	}

	if len(filters.SchoolIDs) > 0 {
		placeholders := make([]string, len(filters.SchoolIDs))
		for i, id := range filters.SchoolIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where = append(where, fmt.Sprintf("sp.school_id IN (%s)", strings.Join(placeholders, ",")))
	}

	if filters.MinBirthDate != nil {
		where = append(where, "sp.date_of_birth <= ?")
		args = append(args, filters.MinBirthDate)
	}

	if filters.MaxBirthDate != nil {
		where = append(where, "sp.date_of_birth >= ?")
		args = append(args, filters.MaxBirthDate)
	}

	if filters.NameSearch != "" {
		where = append(where, "(LOWER(sp.first_name) LIKE ? OR LOWER(sp.last_name) LIKE ?)")
		searchTerm := "%" + strings.ToLower(filters.NameSearch) + "%"
		args = append(args, searchTerm, searchTerm)
	}

	if len(where) > 0 {
		baseQuery += " WHERE " + strings.Join(where, " AND ")
	}

	if limit > 0 {
		baseQuery += " ORDER BY sp.last_name LIMIT ? OFFSET ?"
		args = append(args, limit, limit*(page-1))
	}

	rows, err := r.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list students: %w", err)
	}
	defer rows.Close()

	students := []*ProjectedStudent{}
	for rows.Next() {
		student := &ProjectedStudent{}
		var dateOfBirth, studentID, profilePhotoID, qrCode sql.NullString
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
			&profilePhotoID,
			&student.EligibleForSponsorship,
			&qrCode,
		); err != nil {
			return nil, fmt.Errorf("scan student: %w", err)
		}

		student.DateOfBirth = r.parseDate(dateOfBirth.String)
		student.StudentID = studentID.String
		student.Grade = uint(grade.Int64)
		student.Age = uint(age.Int64)
		student.ProfilePhotoID = profilePhotoID.String
		student.QRCode = qrCode.String

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

func (r *sqlRepository) deleteStudentProjection(id string) error {
	query := `DELETE FROM student_projections WHERE id = :id`
	_, err := r.db.Exec(query, sql.Named("id", id))
	if err != nil {
		return fmt.Errorf("when deleting student projection: %w", err)
	}
	return nil
}

// upsertStudent - persists the student projection to the database
func (r *sqlRepository) upsertStudent(agg *Aggregate) error {
	if agg.data.IsDeleted {
		// ensure these projections are deleted
		return r.deleteStudentProjection(agg.GetID())
	}

	query := `INSERT INTO student_projections
		(id, first_name, last_name, school_id, date_of_birth, version, active, student_id, age, grade, eligible_for_sponsorship, max_sponsorship_date)
		VALUES (:id, :first_name, :last_name, :school_id, :date_of_birth, :version, :active, :student_id, :age, :grade, :eligible_for_sponsorship, :max_sponsorship_date)
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
			eligible_for_sponsorship = excluded.eligible_for_sponsorship,
			max_sponsorship_date = excluded.max_sponsorship_date,
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
		sql.Named("eligible_for_sponsorship", agg.data.EligibleForSponsorship),
		sql.Named("max_sponsorship_date", agg.MaxSponsorshipDate()),
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

// getStudentByStudentAndSchoolID - returns the student aggregate ID by student school ID and school ID
func (r *sqlRepository) getStudentByStudentAndSchoolID(ctx context.Context, studentSchoolID, schoolID string) (uint64, error) {
	if studentSchoolID == "" {
		return 0, fmt.Errorf("student school ID is required")
	}

	if schoolID == "" {
		return 0, fmt.Errorf("school ID is required")
	}

	query := `SELECT id FROM student_projections WHERE student_id = ? AND school_id = ?`
	var id uint64

	if err := r.db.QueryRowContext(ctx, query, studentSchoolID, schoolID).Scan(&id); err != nil {
		return 0, fmt.Errorf("failed to get student ID by student school ID and school ID: %w", err)
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
		WHERE school_id = ? AND active = TRUE;
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

// GetCurrentSponsorships - returns the current sponsorships for a student
func (r *sqlRepository) GetCurrentSponsorships(ctx context.Context, sponsorID string) ([]*SponsorshipProjection, error) {
	query := `
		SELECT student_id, sponsor_id, start_date, end_date
		FROM student_sponsorship_projections
		WHERE sponsor_id = ?
		AND start_date <= CURRENT_TIMESTAMP
		AND end_date >= CURRENT_TIMESTAMP
	`

	rows, err := r.db.QueryContext(ctx, query, sponsorID)
	if err != nil {
		return nil, fmt.Errorf("failed to query sponsorships: %w", err)
	}
	defer rows.Close()

	sponsorships := []*SponsorshipProjection{}
	for rows.Next() {
		sp := &SponsorshipProjection{}
		var startDate, endDate sql.NullString
		if err := rows.Scan(&sp.StudentID, &sp.SponsorID, &startDate, &endDate); err != nil {
			return nil, fmt.Errorf("failed to scan sponsorship: %w", err)
		}

		sp.StartDate = r.parseDate(startDate.String)
		sp.EndDate = r.parseDate(endDate.String)

		sponsorships = append(sponsorships, sp)
	}

	return sponsorships, nil
}

// upsertSponsorshipProjections - persists the sponsorship projections to the database
func (r *sqlRepository) upsertSponsorshipProjections(student *Aggregate) error {
	// Start a transaction
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Delete existing sponsorships for this student
	_, err = tx.Exec("DELETE FROM student_sponsorship_projections WHERE student_id = ?", student.GetID())
	if err != nil {
		return fmt.Errorf("failed to delete existing sponsorships: %w", err)
	}

	// Insert all sponsorships from history
	for _, sponsorship := range student.GetStudent().GetSponsorshipHistory() {
		startDate := time.Date(
			int(sponsorship.StartDate.Year),
			time.Month(sponsorship.StartDate.Month),
			int(sponsorship.StartDate.Day),
			0, 0, 0, 0,
			time.UTC,
		)
		endDate := time.Date(
			int(sponsorship.EndDate.Year),
			time.Month(sponsorship.EndDate.Month),
			int(sponsorship.EndDate.Day),
			0, 0, 0, 0,
			time.UTC,
		)

		_, err = tx.Exec(`
			INSERT INTO student_sponsorship_projections
			(student_id, sponsor_id, start_date, end_date)
			VALUES (?, ?, ?, ?)
		`, student.GetID(), sponsorship.SponsorId, startDate, endDate)

		if err != nil {
			return fmt.Errorf("failed to insert sponsorship: %w", err)
		}
	}

	if maxSponsorshipDate := student.MaxSponsorshipDate(); maxSponsorshipDate != nil {
		_, err = tx.Exec("UPDATE student_projections SET max_sponsorship_date = ? WHERE id = ?", maxSponsorshipDate, student.GetID())
		if err != nil {
			return fmt.Errorf("failed to update max sponsorship date: %w", err)
		}
	}

	return tx.Commit()
}

func (r *sqlRepository) GetAllSponsorshipsByID(ctx context.Context, sponsorID string) ([]*SponsorshipProjection, error) {
	query := `
		SELECT student_id, sponsor_id, start_date, end_date
		FROM student_sponsorship_projections
		WHERE sponsor_id = ?
	`

	rows, err := r.db.QueryContext(ctx, query, sponsorID)
	if err != nil {
		return nil, fmt.Errorf("failed to query sponsorships: %w", err)
	}
	defer rows.Close()

	var sponsorships []*SponsorshipProjection
	for rows.Next() {
		sp := &SponsorshipProjection{}
		var startDate, endDate sql.NullString
		if err := rows.Scan(
			&sp.StudentID,
			&sp.SponsorID,
			&startDate,
			&endDate,
		); err != nil {
			return nil, fmt.Errorf("failed to scan sponsorship: %w", err)
		}

		sp.StartDate = r.parseDate(startDate.String)
		sp.EndDate = r.parseDate(endDate.String)
		sponsorships = append(sponsorships, sp)
	}

	return sponsorships, nil
}

func (r *sqlRepository) CountFeedingEventsInPeriod(ctx context.Context, studentID string, startDate, endDate time.Time) (int64, error) {
	query := `
		SELECT COUNT(*)
		FROM student_feeding_projections
		WHERE student_id = ?
		AND feeding_timestamp >= ?
		AND feeding_timestamp <= ?
	`

	var count int64
	err := r.db.QueryRowContext(ctx, query, studentID, startDate, endDate).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count feeding events: %w", err)
	}

	return count, nil
}

func (r *sqlRepository) GetAllFeedingEvents(ctx context.Context, limit, page uint) ([]*SponsorFeedingEvent, int64, error) {
	// Get total count first
	countQuery := `
		SELECT COUNT(*)
		FROM student_feeding_projections sfp
		JOIN student_projections sp ON sp.id = sfp.student_id
	`
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}

	query := `
		SELECT DISTINCT
			sp.id,
			sp.first_name || ' ' || sp.last_name as student_name,
			sfp.feeding_timestamp,
			sfp.school_id,
			sfp.feeding_image_id
		FROM student_feeding_projections sfp
		JOIN student_projections sp ON sp.id = sfp.student_id
		ORDER BY sfp.feeding_timestamp DESC
		LIMIT ? OFFSET ?
	`

	args := []interface{}{limit, (page - 1) * limit}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query feeding events: %w", err)
	}
	defer rows.Close()

	var events []*SponsorFeedingEvent
	for rows.Next() {
		event := &SponsorFeedingEvent{}
		var timestamp sql.NullString
		var feedingImageID sql.NullString

		if err := rows.Scan(
			&event.StudentID,
			&event.StudentName,
			&timestamp,
			&event.SchoolID,
			&feedingImageID,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan feeding event: %w", err)
		}

		if !timestamp.Valid {
			continue
		}

		event.FeedingImageID = feedingImageID.String
		event.FeedingTime = r.parseDate(timestamp.String)
		events = append(events, event)
	}

	return events, total, nil
}

func (r *sqlRepository) GetFeedingEventsForSponsorships(ctx context.Context, sponsorships []*SponsorshipProjection, limit, page uint) ([]*SponsorFeedingEvent, int64, error) {
	if len(sponsorships) == 0 {
		return []*SponsorFeedingEvent{}, 0, nil
	}

	// Build query for multiple sponsorship periods
	queryParts := make([]string, len(sponsorships))
	args := []interface{}{}

	for i, sp := range sponsorships {
		queryParts[i] = "(sfp.student_id = ? AND sfp.feeding_timestamp BETWEEN ? AND ?)"
		args = append(args, sp.StudentID, sp.StartDate, sp.EndDate)
	}

	// Base query for both count and data
	baseQuery := `
		FROM student_feeding_projections sfp
		JOIN student_projections sp ON sp.id = sfp.student_id
		WHERE ` + strings.Join(queryParts, " OR ")

	// Get total count
	countQuery := "SELECT COUNT(*) " + baseQuery
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count feeding events: %w", err)
	}

	// Get paginated results
	query := `
		SELECT DISTINCT
			sp.id,
			sp.first_name || ' ' || sp.last_name as student_name,
			sfp.feeding_timestamp,
			sfp.school_id,
			sfp.feeding_image_id
		` + baseQuery + `
		ORDER BY sfp.feeding_timestamp DESC
		LIMIT ? OFFSET ?
	`

	args = append(args, limit, (page-1)*limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query feeding events: %w", err)
	}
	defer rows.Close()

	var events []*SponsorFeedingEvent
	for rows.Next() {
		event := &SponsorFeedingEvent{}
		var timestamp sql.NullString
		var feedingImageID sql.NullString

		if err := rows.Scan(
			&event.StudentID,
			&event.StudentName,
			&timestamp,
			&event.SchoolID,
			&feedingImageID,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan feeding event: %w", err)
		}

		if !timestamp.Valid {
			continue
		}

		event.FeedingImageID = feedingImageID.String

		event.FeedingTime = r.parseDate(timestamp.String)
		events = append(events, event)
	}

	return events, total, nil
}

func (r *sqlRepository) GetAllCurrentSponsorships(ctx context.Context) ([]*SponsorshipProjection, error) {
	query := `
		SELECT sp.student_id, sp.sponsor_id, sp.start_date, sp.end_date,
			   sp.payment_id, sp.payment_amount
		FROM student_sponsorship_projections sp
		WHERE sp.start_date <= CURRENT_DATE
		AND sp.end_date >= CURRENT_DATE
		ORDER BY sp.start_date DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query sponsorships: %w", err)
	}
	defer rows.Close()

	var sponsorships []*SponsorshipProjection
	for rows.Next() {
		sp := &SponsorshipProjection{}
		var startDate, endDate, paymentID sql.NullString
		var paymentAmount sql.NullFloat64

		if err := rows.Scan(
			&sp.StudentID,
			&sp.SponsorID,
			&startDate,
			&endDate,
			&paymentID,
			&paymentAmount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan sponsorship: %w", err)
		}

		sp.StartDate = r.parseDate(startDate.String)
		sp.EndDate = r.parseDate(endDate.String)

		// Only set payment fields if they are not null
		if paymentID.Valid {
			sp.PaymentID = paymentID.String
		}
		if paymentAmount.Valid {
			sp.PaymentAmount = paymentAmount.Float64
		}

		sponsorships = append(sponsorships, sp)
	}

	return sponsorships, nil
}
