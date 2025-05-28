-- name: UpsertBulkUploadProjection :exec
INSERT
OR REPLACE INTO bulk_upload_projections (
	id,
	status,
	target_domain,
	file_id,
	initiated_at,
	completed_at,
	invalidation_started_at,
	invalidation_completed_at,
	total_records,
	processed_records,
	failed_records,
	upload_metadata,
	version,
	updated_at
)
VALUES
	(
		?,
		?,
		?,
		?,
		?,
		?,
		?,
		?,
		?,
		?,
		?,
		?,
		?,
		datetime ('now')
	);

-- name: ListBulkUploads :many
SELECT
	id,
	status,
	target_domain,
	file_id,
	initiated_at,
	completed_at,
	invalidation_started_at,
	invalidation_completed_at,
	total_records,
	processed_records,
	failed_records,
	upload_metadata,
	version,
	updated_at
FROM
	bulk_upload_projections
ORDER BY
	initiated_at DESC
LIMIT
	?
OFFSET
	?;

-- name: CountBulkUploads :one
SELECT
	COUNT(*)
FROM
	bulk_upload_projections;

-- name: GetBulkUploadByID :one
SELECT
	id,
	status,
	target_domain,
	file_id,
	initiated_at,
	completed_at,
	invalidation_started_at,
	invalidation_completed_at,
	total_records,
	processed_records,
	failed_records,
	upload_metadata,
	version,
	updated_at
FROM
	bulk_upload_projections
WHERE
	id = ?
LIMIT
	1;
