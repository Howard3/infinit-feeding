-- +goose Up
CREATE TABLE IF NOT EXISTS bulk_upload_events (
	type TEXT NOT NULL,
	data BLOB NOT NULL,
	version INTEGER NOT NULL,
	timestamp INTEGER NOT NULL,
	aggregate_id TEXT NOT NULL,
	UNIQUE (aggregate_id, version)
);

CREATE TABLE IF NOT EXISTS bulk_upload_projections (
	id TEXT PRIMARY KEY,
	status TEXT NOT NULL,
	target_domain TEXT NOT NULL,
	file_id TEXT NOT NULL,
	initiated_at TIMESTAMP NOT NULL,
	completed_at TIMESTAMP,
	invalidation_started_at TIMESTAMP,
	invalidation_completed_at TIMESTAMP,
	total_records INTEGER NOT NULL DEFAULT 0,
	processed_records INTEGER NOT NULL DEFAULT 0,
	failed_records INTEGER NOT NULL DEFAULT 0,
	upload_metadata TEXT NOT NULL DEFAULT '{}',
	version INTEGER NOT NULL,
	updated_at TIMESTAMP DEFAULT (datetime ('now'))
);

-- +goose Down
DROP TABLE IF EXISTS bulk_upload_events;

DROP TABLE IF EXISTS bulk_upload_projections;
