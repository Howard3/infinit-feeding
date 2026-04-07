package infrastructure

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Howard3/gosignal/drivers/eventstore"
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

// GetSourcingConnection returns a connection to the sourcing database
func (c SQLConnection) GetSourcingConnection(db *sql.DB, tableName string) eventstore.SQLStore {
	es := eventstore.SQLStore{
		DB:        db,
		TableName: tableName,
		PositionalPlaceholderFn: func(i int) string {
			return "?"
		},
	}

	return es
}

// Process-wide singleton cache. The app constructs SQLConnection by value in
// several places, but every domain points at the same logical database — we
// must hand them the same *sql.DB so the embedded replica file isn't opened
// multiple times concurrently.
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

	db, err := openDB(string(c.Type), c.URI)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	dbCache[c.URI] = db
	c.db = db
	return db, nil
}

// openDB opens a database connection. For libsql remote URIs we use an
// embedded replica so reads/writes go through the local file (sidestepping
// the Hrana HTTP "baton" session protocol that has been the source of
// "invalid baton" errors). Local file:// URIs fall through to the standard
// driver path.
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

	connector, err := libsql.NewEmbeddedReplicaConnector(
		dbPath,
		primaryURL,
		libsql.WithAuthToken(authToken),
		libsql.WithSyncInterval(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("create embedded replica connector: %w", err)
	}

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
// Honors LIBSQL_REPLICA_PATH; otherwise defaults to a path under the OS
// temp dir so the container doesn't require a mounted volume to boot.
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
