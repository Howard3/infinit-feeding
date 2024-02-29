-- +goose Up 
CREATE TABLE IF NOT EXISTS school_events (
	type VARCHAR(255) NOT NULL,
	data BYTEA NOT NULL,
	version INT NOT NULL,
	timestamp INT NOT NULL,
	aggregate_id INT NOT NULL,
	UNIQUE (aggregate_id, version)
);

-- track next aggregate id (uint)
CREATE TABLE IF NOT EXISTS aggregate_id_tracking (
    type VARCHAR(255) NOT NULL,
    next_id INT NOT NULL,
    UNIQUE (type)
);

CREATE TABLE IF NOT EXISTS schools (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL,
    version INT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down 
DROP TABLE IF EXISTS school_events; 
DROP TABLE IF EXISTS aggregate_id_tracking;
DROP TABLE IF EXISTS schools;
