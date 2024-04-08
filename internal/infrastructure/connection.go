package infrastructure

import (
	"database/sql"

	"github.com/Howard3/gosignal/drivers/eventstore"
)

type ConnectionType string

const (
	SQLite ConnectionType = "sqlite"
)

type SQLConnection struct {
	Type ConnectionType
	URI  string
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
