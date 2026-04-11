package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/drivers/eventstore"
	"github.com/Howard3/gosignal/sourcing"
	"github.com/tursodatabase/go-libsql"
)

type ConnectionType string

const (
	SQLite ConnectionType = "sqlite"
)

type SQLConnection struct {
	Type ConnectionType
	URI  string
	db   *sql.DB
}

// GetSourcingConnection returns an EventStore implementation backed by the
// gosignal SQLStore, wrapped in a retry layer that recovers from libsql
// "invalid baton" errors. With embedded replicas the retry layer is rarely
// needed, but it provides defense in depth for writes that are forwarded
// to the Turso primary.
func (c SQLConnection) GetSourcingConnection(db *sql.DB, tableName string) sourcing.EventStore {
	es := eventstore.SQLStore{
		DB:        db,
		TableName: tableName,
		PositionalPlaceholderFn: func(i int) string {
			return "?"
		},
	}

	return retryEventStore{inner: es}
}

// Process-wide singleton cache. SQLConnection is constructed by value in
// several places, but every domain points at the same logical database —
// hand them all the same *sql.DB so we don't open the replica file or
// remote connection multiple times.
var (
	dbCacheMu  sync.Mutex
	dbCache    = map[string]*sql.DB{}
	connectors []interface{ Close() error } // track connectors for cleanup
)

func (c *SQLConnection) Open() (*sql.DB, error) {
	if c.db != nil {
		return c.db, nil
	}

	dbCacheMu.Lock()
	defer dbCacheMu.Unlock()

	if cached, ok := dbCache[c.URI]; ok {
		c.db = cached
		return cached, nil
	}

	db, err := openDB(string(c.Type), c.URI)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	dbCache[c.URI] = db
	c.db = db
	return db, nil
}

// openDB opens a database connection. For remote libsql URIs, it creates
// an embedded replica so reads hit the local file and writes are forwarded
// to the Turso primary. This sidesteps the Hrana HTTP session protocol
// entirely. Local file:// URIs fall through to the standard driver.
func openDB(driver, uri string) (*sql.DB, error) {
	if driver != "libsql" || !isRemoteLibsqlURI(uri) {
		return sql.Open(driver, uri)
	}

	primaryURL, authToken, err := splitLibsqlURI(uri)
	if err != nil {
		return nil, err
	}

	dbPath, err := localReplicaPath()
	if err != nil {
		return nil, err
	}

	slog.Info("opening embedded replica", "primary", primaryURL, "replica_path", dbPath)

	connector, err := libsql.NewEmbeddedReplicaConnector(
		dbPath,
		primaryURL,
		libsql.WithAuthToken(authToken),
		libsql.WithSyncInterval(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("create embedded replica connector: %w", err)
	}

	// Initial sync to pull data from the primary before we start serving.
	slog.Info("performing initial sync from Turso primary")
	if _, err := connector.Sync(); err != nil {
		connector.Close()
		return nil, fmt.Errorf("initial replica sync: %w", err)
	}
	slog.Info("initial sync complete")

	// Track for cleanup
	connectors = append(connectors, connector)

	return sql.OpenDB(connector), nil
}

func isRemoteLibsqlURI(uri string) bool {
	return strings.HasPrefix(uri, "libsql://") ||
		strings.HasPrefix(uri, "https://") ||
		strings.HasPrefix(uri, "http://")
}

// splitLibsqlURI extracts the auth token from the URI's query string and
// returns the bare primary URL plus the token.
func splitLibsqlURI(uri string) (primaryURL, authToken string, err error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", "", fmt.Errorf("parse libsql uri: %w", err)
	}
	q := u.Query()
	authToken = q.Get("authToken")
	q.Del("authToken")
	u.RawQuery = q.Encode()
	return u.String(), authToken, nil
}

// localReplicaPath returns the on-disk path for the embedded replica file.
// Honors LIBSQL_REPLICA_PATH env var; otherwise defaults to a path under
// the OS temp dir so the container boots without a mounted volume.
func localReplicaPath() (string, error) {
	if p := os.Getenv("LIBSQL_REPLICA_PATH"); p != "" {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", fmt.Errorf("create replica dir: %w", err)
		}
		return p, nil
	}
	dir := filepath.Join(os.TempDir(), "geevly-libsql")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create replica dir: %w", err)
	}
	return filepath.Join(dir, "replica.db"), nil
}

func (c SQLConnection) Close() error {
	if c.db != nil {
		c.db.Close()
	}

	return nil
}

// CloseAllConnectors cleans up any embedded replica connectors.
func CloseAllConnectors() {
	dbCacheMu.Lock()
	defer dbCacheMu.Unlock()
	for _, c := range connectors {
		c.Close()
	}
	connectors = nil
}

// IsBatonError reports whether err is a transient libsql Hrana session
// error — "invalid baton", "stream not found", or similar — that can be
// recovered by retrying the operation on a fresh connection.
func IsBatonError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid baton") ||
		strings.Contains(msg, "stream not found") ||
		strings.Contains(msg, "stream expired") ||
		strings.Contains(msg, "baton expired")
}

// RetryOnBaton retries fn up to maxAttempts times if it fails with a baton
// error, with a short backoff between attempts.
func RetryOnBaton(maxAttempts int, fn func() error) error {
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = fn()
		if err == nil || !IsBatonError(err) {
			return err
		}
		time.Sleep(time.Duration(1200+200*attempt) * time.Millisecond)
	}
	return err
}

// retryEventStore wraps an EventStore and retries the whole operation when
// it fails with a libsql baton error.
type retryEventStore struct {
	inner sourcing.EventStore
}

const batonRetryAttempts = 4

func (r retryEventStore) Store(ctx context.Context, events []gosignal.Event) error {
	return RetryOnBaton(batonRetryAttempts, func() error {
		return r.inner.Store(ctx, events)
	})
}

func (r retryEventStore) Load(ctx context.Context, aggID string, opts sourcing.LoadEventsOptions) ([]gosignal.Event, error) {
	var out []gosignal.Event
	err := RetryOnBaton(batonRetryAttempts, func() error {
		var err error
		out, err = r.inner.Load(ctx, aggID, opts)
		return err
	})
	return out, err
}

func (r retryEventStore) Replace(ctx context.Context, id string, version uint64, event gosignal.Event) error {
	return RetryOnBaton(batonRetryAttempts, func() error {
		return r.inner.Replace(ctx, id, version, event)
	})
}
