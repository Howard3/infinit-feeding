-- +goose Up 
CREATE TABLE IF NOT EXISTS student_feeding_projections (
    student_id TEXT NOT NULL,
    feeding_id INT NOT NULL,
    school_id TEXT NOT NULL,
    feeding_timestamp TIMESTAMPTZ NOT NULL,
    PRIMARY KEY(student_id, feeding_id)
);

INSERT INTO student_projection_updates (what) VALUES ('student_feeding_projections');

-- clear the student snapshots
DELETE FROM student_snapshots;

CREATE INDEX idx_sfp_school_id_timestamp ON student_feeding_projections (school_id, feeding_timestamp);

-- +goose Down
DROP TABLE IF EXISTS student_feeding_projections;
