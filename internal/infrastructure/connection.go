package infrastructure

import (
	"database/sql"
	"fmt"

	"github.com/Howard3/gosignal/drivers/eventstore"
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

func (c *SQLConnection) Open() (*sql.DB, error) {
	if c.db != nil {
		return c.db, nil
	}

	db, err := sql.Open(string(c.Type), c.URI)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	c.db = db

	return db, nil
}

func (c SQLConnection) Close() error {
	if c.db != nil {
		c.db.Close()
	}

	return nil
}
