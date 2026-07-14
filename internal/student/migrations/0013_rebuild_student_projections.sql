-- +goose Up
-- Trigger a one-time rebuild of student_projections on next app start.
-- Repairs rows left stale by concurrent async projection handlers during bulk import
-- (Create/Enroll handlers overwriting a later SetStatus projection).
INSERT INTO student_projection_updates (what) VALUES ('student_projections');

-- +goose Down
DELETE FROM student_projection_updates WHERE what = 'student_projections';
