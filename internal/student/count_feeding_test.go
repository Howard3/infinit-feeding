package student

import (
	"context"
	"database/sql"
	"os"
	"testing"

	// Use libsql (same as production) — mattn/go-sqlite3 duplicates SQLite symbols with go-libsql.
	_ "github.com/tursodatabase/go-libsql"
)

func TestCountAllFeedingEvents_Integration(t *testing.T) {
	dbPath := os.Getenv("TEST_DB_PATH")
	if dbPath == "" {
		dbPath = "../../dev.db"
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Skip("dev.db not found, skipping integration test")
	}

	db, err := sql.Open("libsql", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := &sqlRepository{db: db}
	count, err := repo.CountAllFeedingEvents(context.Background())
	if err != nil {
		t.Fatalf("CountAllFeedingEvents failed: %v", err)
	}

	if count < 0 {
		t.Errorf("expected non-negative count, got %d", count)
	}

	t.Logf("Total feeding events: %d", count)
}
