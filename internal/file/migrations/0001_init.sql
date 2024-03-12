-- +goose Up 
CREATE TABLE IF NOT EXISTS file_events (
	type VARCHAR(255) NOT NULL,
	data BYTEA NOT NULL,
	version INT NOT NULL,
	timestamp INT NOT NULL,
	aggregate_id INT NOT NULL,
	UNIQUE (aggregate_id, version)
);


CREATE TABLE IF NOT EXISTS files (
    id TEXT PRIMARY KEY,
    domain TEXT NOT NULL,
    name TEXT NOT NULL,
    deleted BOOLEAN NOT NULL,
    version INT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down 
DROP TABLE IF EXISTS file_events; 
DROP TABLE IF EXISTS files;
