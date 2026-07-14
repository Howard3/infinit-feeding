package student

import (
	"database/sql"
	"path/filepath"
	"testing"

	"geevly/gen/go/eda"

	// Use libsql (same as production) — mattn/go-sqlite3 duplicates SQLite symbols with go-libsql.
	_ "github.com/tursodatabase/go-libsql"
)

func setupProjectionTestDB(t *testing.T) *sqlRepository {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("libsql", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE student_projections (
			id TEXT PRIMARY KEY,
			first_name TEXT NOT NULL,
			last_name TEXT NOT NULL,
			school_id TEXT NOT NULL,
			date_of_birth DATE NOT NULL,
			version INT NOT NULL,
			active BOOLEAN NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			student_id TEXT,
			age INT,
			grade INT,
			eligible_for_sponsorship BOOLEAN NOT NULL DEFAULT false,
			max_sponsorship_date DATE
		);
		CREATE TABLE student_events (
			type VARCHAR(255) NOT NULL,
			data BLOB NOT NULL,
			version INT NOT NULL,
			timestamp INT NOT NULL,
			aggregate_id INT NOT NULL,
			UNIQUE (aggregate_id, version)
		);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return &sqlRepository{db: db}
}

func makeTestAgg(id uint64, version uint64, status eda.Student_Status, schoolID string) *Aggregate {
	agg := &Aggregate{
		data: &eda.Student{
			FirstName:       "Jane",
			LastName:        "Doe",
			SchoolId:        schoolID,
			Status:          status,
			StudentSchoolId: "LRN123",
			GradeLevel:      3,
			DateOfBirth:     &eda.Date{Year: 2015, Month: 3, Day: 15},
		},
	}
	agg.SetIDUint64(id)
	agg.SetVersion(version)
	return agg
}

func readProjection(t *testing.T, repo *sqlRepository, id string) (active bool, schoolID string, version int) {
	t.Helper()
	err := repo.db.QueryRow(
		`SELECT active, school_id, version FROM student_projections WHERE id = ?`,
		id,
	).Scan(&active, &schoolID, &version)
	if err != nil {
		t.Fatalf("read projection: %v", err)
	}
	return active, schoolID, version
}

// Reproduces the bulk-import race: a stale Create/Enroll projection must not
// overwrite a newer SetStatus projection that already made the student active.
func TestUpsertStudent_IgnoresStaleLowerVersion(t *testing.T) {
	repo := setupProjectionTestDB(t)

	current := makeTestAgg(42, 4, eda.Student_ACTIVE, "school-7")
	if err := repo.upsertStudent(current); err != nil {
		t.Fatalf("upsert current: %v", err)
	}

	stale := makeTestAgg(42, 1, eda.Student_INACTIVE, "")
	if err := repo.upsertStudent(stale); err != nil {
		t.Fatalf("upsert stale: %v", err)
	}

	active, schoolID, version := readProjection(t, repo, "42")
	if !active {
		t.Fatalf("expected active=true after stale upsert, got false")
	}
	if schoolID != "school-7" {
		t.Fatalf("expected school_id=school-7, got %q", schoolID)
	}
	if version != 4 {
		t.Fatalf("expected version=4, got %d", version)
	}
}

func TestUpsertStudent_AllowsHigherVersion(t *testing.T) {
	repo := setupProjectionTestDB(t)

	inactive := makeTestAgg(7, 1, eda.Student_INACTIVE, "")
	if err := repo.upsertStudent(inactive); err != nil {
		t.Fatalf("upsert inactive: %v", err)
	}

	active := makeTestAgg(7, 3, eda.Student_ACTIVE, "school-9")
	if err := repo.upsertStudent(active); err != nil {
		t.Fatalf("upsert active: %v", err)
	}

	gotActive, schoolID, version := readProjection(t, repo, "7")
	if !gotActive {
		t.Fatalf("expected active=true")
	}
	if schoolID != "school-9" {
		t.Fatalf("expected school_id=school-9, got %q", schoolID)
	}
	if version != 3 {
		t.Fatalf("expected version=3, got %d", version)
	}
}

func TestForceUpsertStudent_RepairsCorruptSameVersionRow(t *testing.T) {
	repo := setupProjectionTestDB(t)

	_, err := repo.db.Exec(`
		INSERT INTO student_projections
			(id, first_name, last_name, school_id, date_of_birth, version, active, student_id, age, grade)
		VALUES ('55', 'Jane', 'Doe', '', '2015-03-15', 5, 0, 'LRN55', 10, 3)
	`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	correct := makeTestAgg(55, 5, eda.Student_ACTIVE, "school-1")
	if err := repo.upsertStudentForced(correct); err != nil {
		t.Fatalf("force upsert: %v", err)
	}

	active, schoolID, version := readProjection(t, repo, "55")
	if !active {
		t.Fatalf("expected active after force upsert")
	}
	if schoolID != "school-1" {
		t.Fatalf("expected school-1, got %q", schoolID)
	}
	if version != 5 {
		t.Fatalf("expected version 5, got %d", version)
	}
}
