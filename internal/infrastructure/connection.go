package infrastructure

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Howard3/gosignal"
	"github.com/Howard3/gosignal/drivers/eventstore"
	"github.com/Howard3/gosignal/sourcing"
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
// gosignal SQLStore, wrapped in a retry layer for transient SQLite lock
// errors. The database is a local libSQL file; there is no remote primary.
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
	dbCacheMu sync.Mutex
	dbCache   = map[string]*sql.DB{}

	checkpointLoopCancel context.CancelFunc
	checkpointLoopDone   chan struct{}
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

	// SQLite is single-writer. The checkpoint loop also takes a lock,
	// so keep the pool small.
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	dbCache[c.URI] = db
	c.db = db
	return db, nil
}

// openDB opens a local libSQL file. Remote libsql:// and http(s):// URIs are
// refused so a deploy cannot attach this process to Turso. In production
// (GO_ENV is not development) that refusal is a hard boot error.
func openDB(driverName, uri string) (*sql.DB, error) {
	if isRemoteLibsqlURI(uri) {
		if os.Getenv("GO_ENV") != "development" {
			return nil, fmt.Errorf("refusing remote database URI in production; set DB_URI=file:/var/lib/libsql/feeding.db")
		}
		return nil, fmt.Errorf("remote database URIs are not supported; set DB_URI to a file: path")
	}
	if driverName != "libsql" {
		return sql.Open(driverName, uri)
	}
	if err := ensureLibsqlDir(uri); err != nil {
		return nil, err
	}

	probe, err := sql.Open("libsql", ":memory:")
	if err != nil {
		return nil, err
	}
	drv := probe.Driver()
	_ = probe.Close()

	dc, ok := drv.(driver.DriverContext)
	if !ok {
		return nil, fmt.Errorf("libsql driver does not implement OpenConnector")
	}
	inner, err := dc.OpenConnector(uri)
	if err != nil {
		return nil, fmt.Errorf("open local libsql: %w", err)
	}
	db := sql.OpenDB(pragmaConnector{
		inner: inner,
		pragmas: []string{
			"PRAGMA busy_timeout = 10000",
		},
	})

	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode = WAL`).Scan(&mode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set journal_mode=WAL: %w", err)
	}
	if !strings.EqualFold(mode, "wal") {
		_ = db.Close()
		return nil, fmt.Errorf("journal_mode = %s, want wal", mode)
	}

	slog.Info("opening local libsql database", "uri", uri)
	startCheckpointLoop(db)
	return db, nil
}

func isRemoteLibsqlURI(uri string) bool {
	return strings.HasPrefix(uri, "libsql://") ||
		strings.HasPrefix(uri, "https://") ||
		strings.HasPrefix(uri, "http://")
}

// ensureLibsqlDir creates the parent directory of a file: database path.
func ensureLibsqlDir(uri string) error {
	path := strings.TrimPrefix(uri, "file:")
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	path = strings.TrimPrefix(path, "//localhost")
	if path == "" || strings.HasPrefix(path, ":memory:") {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create database dir: %w", err)
	}
	return nil
}

const checkpointInterval = 30 * time.Second

// startCheckpointLoop runs PRAGMA wal_checkpoint(PASSIVE) on one goroutine.
// It does not sync to a remote primary.
func startCheckpointLoop(db *sql.DB) {
	if checkpointLoopCancel != nil {
		checkpointLoopCancel()
		if checkpointLoopDone != nil {
			<-checkpointLoopDone
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	checkpointLoopCancel = cancel
	checkpointLoopDone = done

	go func() {
		defer close(done)
		runCheckpoint(db)
		ticker := time.NewTicker(checkpointInterval)
		defer ticker.Stop()
		consecutiveBusy := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := runCheckpoint(db); err != nil {
					if IsBusyError(err) {
						consecutiveBusy++
						if consecutiveBusy >= 3 {
							slog.Error("wal_checkpoint repeatedly busy", "consecutive", consecutiveBusy, "error", err)
						}
					} else {
						consecutiveBusy = 0
					}
					continue
				}
				consecutiveBusy = 0
			}
		}
	}()
}

func runCheckpoint(db *sql.DB) error {
	var busy, logPages, checkpointed int
	err := db.QueryRow(`PRAGMA wal_checkpoint(PASSIVE)`).Scan(&busy, &logPages, &checkpointed)
	if err != nil {
		slog.Warn("wal_checkpoint(PASSIVE) failed", "error", err)
		return err
	}
	slog.Info("wal_checkpoint", "busy", busy, "log", logPages, "checkpointed", checkpointed)
	if busy != 0 {
		return fmt.Errorf("wal_checkpoint busy=1 log=%d checkpointed=%d", logPages, checkpointed)
	}
	return nil
}

func (c SQLConnection) Close() error {
	if c.db != nil {
		c.db.Close()
	}

	return nil
}

// CloseAllConnectors stops the checkpoint loop, then closes cached databases.
// Call on process shutdown so a checkpoint cannot run against a closing DB.
func CloseAllConnectors() {
	dbCacheMu.Lock()
	cancel := checkpointLoopCancel
	done := checkpointLoopDone
	checkpointLoopCancel = nil
	checkpointLoopDone = nil
	dbs := make([]*sql.DB, 0, len(dbCache))
	for _, db := range dbCache {
		dbs = append(dbs, db)
	}
	dbCache = map[string]*sql.DB{}
	dbCacheMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	for _, db := range dbs {
		_ = db.Close()
	}
}

// PingAll pings every cached *sql.DB once. Does not retry SQLITE_BUSY —
// a locked database during checkpoint must not stretch the Docker health
// window into autoheal restarts. Returns the first error encountered.
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
			if err := drainRows(rows); err != nil {
				rows.Close()
				conn.Close()
				return nil, fmt.Errorf("read pragma %q: %w", pragma, err)
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

func drainRows(rows driver.Rows) error {
	dest := make([]driver.Value, len(rows.Columns()))
	for {
		err := rows.Next(dest)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

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

// IsBusyError reports whether err is SQLite SQLITE_BUSY / "database is locked".
// The checkpoint loop and app queries share one file; this is expected
// under contention and should be retried, not surfaced to the user.
func IsBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "SQLITE_BUSY")
}

// RetryTransient retries fn up to maxAttempts times on baton or SQLITE_BUSY
// errors. Honors ctx cancellation between attempts and does not sleep after
// the last attempt.
func RetryTransient(ctx context.Context, maxAttempts int, fn func() error) error {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = fn()
		if err == nil {
			return nil
		}
		if !IsBatonError(err) && !IsBusyError(err) {
			return err
		}
		if IsBusyError(err) {
			slog.Warn("retrying after database is locked", "attempt", attempt+1, "max_attempts", maxAttempts)
		}
		if attempt == maxAttempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1200+200*attempt) * time.Millisecond):
		}
	}
	return err
}

// RetryOnBaton is kept for call sites that do not yet pass a context.
// Prefer RetryTransient.
func RetryOnBaton(maxAttempts int, fn func() error) error {
	return RetryTransient(context.Background(), maxAttempts, fn)
}

// retryEventStore wraps an EventStore and retries the whole operation when
// it fails with a libsql baton error.
type retryEventStore struct {
	inner sourcing.EventStore
}

const batonRetryAttempts = 4

func (r retryEventStore) Store(ctx context.Context, events []gosignal.Event) error {
	return RetryTransient(ctx, batonRetryAttempts, func() error {
		return r.inner.Store(ctx, events)
	})
}

func (r retryEventStore) Load(ctx context.Context, aggID string, opts sourcing.LoadEventsOptions) ([]gosignal.Event, error) {
	var out []gosignal.Event
	err := RetryTransient(ctx, batonRetryAttempts, func() error {
		var err error
		out, err = r.inner.Load(ctx, aggID, opts)
		return err
	})
	return out, err
}

func (r retryEventStore) Replace(ctx context.Context, id string, version uint64, event gosignal.Event) error {
	return RetryTransient(ctx, batonRetryAttempts, func() error {
		return r.inner.Replace(ctx, id, version, event)
	})
}
