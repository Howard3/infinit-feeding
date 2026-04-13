package infrastructure

import (
	"context"
	"database/sql"
	"database/sql/driver"
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

	// Turso's server-side Hrana streams expire after 10s of inactivity
	// (see go-libsql#13). Close idle connections before that threshold
	// so database/sql never hands out a connection with a dead stream.
	db.SetConnMaxIdleTime(9 * time.Second)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(10)

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

	slog.Info("opening synced database (offline writes)", "primary", primaryURL, "replica_path", dbPath)

	connector, err := libsql.NewSyncedDatabaseConnector(
		dbPath,
		primaryURL,
		libsql.WithAuthToken(authToken),
		libsql.WithSyncInterval(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("create synced database connector: %w", err)
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

	// Compose connector wrappers:
	// 1. recoveryConnector: convert dead-stream errors to driver.ErrBadConn
	// 2. pragmaConnector: set busy_timeout on every new connection so SQLite
	//    waits for locks instead of failing immediately with "database is locked"
	recovery := recoveryConnector{inner: connector}
	withPragmas := pragmaConnector{
		inner: recovery,
		pragmas: []string{
			"PRAGMA busy_timeout = 5000",
		},
	}
	return sql.OpenDB(withPragmas), nil
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

// PingAll pings every cached *sql.DB. Returns the first error encountered,
// or nil if all databases are reachable. Used by the health check endpoint
// so that Docker/autoheal can detect a dead database connection.
func PingAll(ctx context.Context) error {
	dbCacheMu.Lock()
	dbs := make([]*sql.DB, 0, len(dbCache))
	for _, db := range dbCache {
		dbs = append(dbs, db)
	}
	dbCacheMu.Unlock()

	for _, db := range dbs {
		if err := db.PingContext(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// pragmaConnector: execute PRAGMAs on every new connection from the pool.
// go-libsql doesn't support PRAGMAs in the connection string, so we run
// them in Connect() before handing the connection to database/sql.
// ---------------------------------------------------------------------------

type pragmaConnector struct {
	inner   driver.Connector
	pragmas []string
}

func (pc pragmaConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := pc.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	for _, pragma := range pc.pragmas {
		// PRAGMAs return rows (the current/new value), so we must use
		// Query rather than Exec — go-libsql rejects Exec on statements
		// that produce result sets.
		if qc, ok := conn.(driver.QueryerContext); ok {
			rows, err := qc.QueryContext(context.Background(), pragma, nil)
			if err != nil {
				conn.Close()
				return nil, fmt.Errorf("query pragma %q: %w", pragma, err)
			}
			rows.Close()
			continue
		}
		stmt, err := conn.Prepare(pragma)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("prepare pragma %q: %w", pragma, err)
		}
		rows, err := stmt.Query(nil) //nolint:staticcheck
		if err != nil {
			stmt.Close()
			conn.Close()
			return nil, fmt.Errorf("query pragma %q: %w", pragma, err)
		}
		rows.Close()
		stmt.Close()
	}
	return conn, nil
}

func (pc pragmaConnector) Driver() driver.Driver { return pc.inner.Driver() }

// ---------------------------------------------------------------------------
// driver-level recovery: convert Hrana stream/baton errors to
// driver.ErrBadConn so database/sql automatically discards the broken
// connection and retries with a fresh one from the pool (which calls
// Connector.Connect → fresh C connection → fresh Hrana stream).
// This lets bulk uploads survive dead streams without a process restart.
// ---------------------------------------------------------------------------

// recoveryConnector wraps a driver.Connector. Connections it hands out
// translate baton errors into driver.ErrBadConn.
type recoveryConnector struct {
	inner driver.Connector
}

func (rc recoveryConnector) Connect(ctx context.Context) (driver.Conn, error) {
	c, err := rc.inner.Connect(ctx)
	if err != nil {
		if IsBatonError(err) {
			return nil, driver.ErrBadConn
		}
		return nil, err
	}
	return &recoveryConn{inner: c}, nil
}

func (rc recoveryConnector) Driver() driver.Driver { return rc.inner.Driver() }

// recoveryConn wraps a driver.Conn. Any operation that fails with a
// baton/stream error returns driver.ErrBadConn instead, causing
// database/sql to discard this connection and retry on a new one.
type recoveryConn struct {
	inner driver.Conn
}

func batonOrOrig(err error) error {
	if err != nil && isConnectionDead(err) {
		slog.Warn("converting Hrana stream error to ErrBadConn for automatic recovery", "original_error", err)
		return driver.ErrBadConn
	}
	return err
}

func (c *recoveryConn) Prepare(query string) (driver.Stmt, error) {
	s, err := c.inner.Prepare(query)
	return s, batonOrOrig(err)
}

func (c *recoveryConn) Close() error { return c.inner.Close() }

func (c *recoveryConn) Begin() (driver.Tx, error) {
	tx, err := c.inner.Begin() //nolint:staticcheck // required by driver.Conn
	return tx, batonOrOrig(err)
}

// PrepareContext implements driver.ConnPrepareContext.
func (c *recoveryConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if pc, ok := c.inner.(driver.ConnPrepareContext); ok {
		s, err := pc.PrepareContext(ctx, query)
		return s, batonOrOrig(err)
	}
	return c.Prepare(query)
}

// ExecContext implements driver.ExecerContext.
func (c *recoveryConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if ec, ok := c.inner.(driver.ExecerContext); ok {
		r, err := ec.ExecContext(ctx, query, args)
		return r, batonOrOrig(err)
	}
	return nil, driver.ErrSkip
}

// QueryContext implements driver.QueryerContext.
func (c *recoveryConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if qc, ok := c.inner.(driver.QueryerContext); ok {
		rows, err := qc.QueryContext(ctx, query, args)
		return rows, batonOrOrig(err)
	}
	return nil, driver.ErrSkip
}

// BeginTx implements driver.ConnBeginTx.
func (c *recoveryConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if bt, ok := c.inner.(driver.ConnBeginTx); ok {
		tx, err := bt.BeginTx(ctx, opts)
		return tx, batonOrOrig(err)
	}
	return c.Begin()
}

// isConnectionDead reports whether err indicates the connection itself is
// broken (Hrana stream died, transaction poisoned). These warrant
// discarding the connection via driver.ErrBadConn.
func isConnectionDead(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid baton") ||
		strings.Contains(msg, "stream not found") ||
		strings.Contains(msg, "stream expired") ||
		strings.Contains(msg, "baton expired") ||
		strings.Contains(msg, "invalid state")
}

// IsBatonError reports whether err is a transient libsql error that can
// be recovered by retrying the operation on a fresh connection.
func IsBatonError(err error) bool {
	return isConnectionDead(err)
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
