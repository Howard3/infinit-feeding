-- +goose Up
ALTER TABLE student_feeding_projections ADD COLUMN feeding_image_id TEXT;

-- Trigger projection rebuild by adding a new record
INSERT INTO student_projection_updates (what) VALUES ('student_feeding_projections');

-- Delete all snapshots
DELETE FROM student_snapshots;

-- +goose Down
ALTER TABLE student_feeding_projections DROP COLUMN feeding_image_id; 