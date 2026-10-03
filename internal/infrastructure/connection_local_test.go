package infrastructure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/tursodatabase/go-libsql"
)

func TestOpenDBRejectsRemoteURI(t *testing.T) {
	t.Setenv("GO_ENV", "production")
	_, err := openDB("libsql", "libsql://example.turso.io?authToken=secret")
	if err == nil || !strings.Contains(err.Error(), "refusing remote database URI in production") {
		t.Fatalf("production remote URI: got %v", err)
	}

	t.Setenv("GO_ENV", "development")
	_, err = openDB("libsql", "https://example.turso.io")
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("development remote URI: got %v", err)
	}
}

func TestOpenDBLocalFileSetsPragmas(t *testing.T) {
	t.Setenv("GO_ENV", "development")
	dir := t.TempDir()
	uri := "file:" + filepath.Join(dir, "feeding.db")

	db, err := openDB("libsql", uri)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(CloseAllConnectors)

	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}

	var timeout int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&timeout); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if timeout != 10000 {
		t.Fatalf("busy_timeout = %d, want 10000", timeout)
	}

	if _, err := os.Stat(filepath.Join(dir, "feeding.db")); err != nil {
		t.Fatalf("database file: %v", err)
	}
}
