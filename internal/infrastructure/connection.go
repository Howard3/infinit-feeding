package infrastructure

type ConnectionType string

const (
	SQLite ConnectionType = "sqlite"
)

type SQLConnection struct {
	Type ConnectionType
	URI  string
}
