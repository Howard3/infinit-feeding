package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
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
// gosignal SQLStore, wrapped in a retry layer that recovers from libsql
// "invalid baton" errors. The Hrana HTTP protocol used by libsql remote
// drivers periodically invalidates session batons mid-transaction; the
// fix is to retry the entire failed operation on a fresh connection.
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
// hand them all the same *sql.DB so we don't open the remote connection
// once per domain.
var (
	dbCacheMu sync.Mutex
	dbCache   = map[string]*sql.DB{}
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

	db, err := sql.Open(string(c.Type), c.URI)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// libsql's Hrana HTTP protocol keeps per-connection session state
	// (batons, streams) that the server frequently invalidates on idle
	// connections. When database/sql hands a stale connection back out of
	// the pool, the next query fails with "invalid baton" or "stream not
	// found". Evict idle connections aggressively so we get a fresh
	// session instead of a dead one.
	db.SetConnMaxIdleTime(1 * time.Second)
	db.SetConnMaxLifetime(5 * time.Minute)

	dbCache[c.URI] = db
	c.db = db
	return db, nil
}

func (c SQLConnection) Close() error {
	if c.db != nil {
		c.db.Close()
	}

	return nil
}

// IsBatonError reports whether err is a transient libsql Hrana session
// error — "invalid baton", "stream not found", or similar — that can be
// recovered by retrying the operation on a fresh connection. The Hrana
// HTTP protocol holds per-connection session state that the server
// periodically invalidates; the fix is always the same (retry on a new
// conn), so we match all known variants here.
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
// error, with a short backoff between attempts. Use this to wrap any DB
// operation (migrations, projection upserts, etc.) that runs against the
// remote libsql connection and is safe to re-run on failure.
func RetryOnBaton(maxAttempts int, fn func() error) error {
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = fn()
		if err == nil || !IsBatonError(err) {
			return err
		}
		// Sleep longer than ConnMaxIdleTime so the failed connection
		// is reaped from the pool before we ask for another one.
		time.Sleep(time.Duration(1200+200*attempt) * time.Millisecond)
	}
	return err
}

// retryEventStore wraps an EventStore and retries the whole operation when
// it fails with a libsql baton error. Each method is idempotent at the
// gosignal layer (Store uses an explicit version, Load is read-only,
// Replace is keyed by id+version), so retrying is safe.
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
