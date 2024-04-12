-- +goose Up 
ALTER TABLE student_projections ADD COLUMN student_id TEXT;
ALTER TABLE student_projections ADD COLUMN age INT;
ALTER TABLE student_projections ADD COLUMN grade INT;
ALTER TABLE student_projections DROP COLUMN date_of_enrollment;

CREATE TABLE IF NOT EXISTS student_projection_updates (
    what TEXT NOT NULL
);

INSERT INTO student_projection_updates (what) VALUES ('student_projections');

-- +goose Down 
ALTER TABLE student_projections DROP COLUMN student_id;
ALTER TABLE student_projections DROP COLUMN age;
ALTER TABLE student_projections DROP COLUMN grade;
ALTER TABLE student_projections ADD COLUMN date_of_enrollment DATE;
DROP TABLE IF EXISTS projection_updates;

