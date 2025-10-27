-- +goose Up 
-- Health/BMI projections for reporting
CREATE TABLE IF NOT EXISTS student_health_projections (
    student_id TEXT NOT NULL,
    school_id TEXT NOT NULL,
    assessment_date TIMESTAMPTZ NOT NULL,
    height_cm REAL NOT NULL,
    weight_kg REAL NOT NULL,
    bmi REAL,
    nutritional_status TEXT,
    associated_bulk_upload_id TEXT,
    PRIMARY KEY(student_id, associated_bulk_upload_id),
    FOREIGN KEY(student_id) REFERENCES student_projections(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_shp_school_date ON student_health_projections (school_id, assessment_date);
CREATE INDEX IF NOT EXISTS idx_shp_student_date ON student_health_projections (student_id, assessment_date);

-- Grade projections for reporting
CREATE TABLE IF NOT EXISTS student_grade_projections (
    student_id TEXT NOT NULL,
    school_id TEXT NOT NULL,
    test_date DATE NOT NULL,
    grade INT NOT NULL,
    school_year TEXT,
    grading_period TEXT,
    associated_bulk_upload_id TEXT,
    PRIMARY KEY(student_id, associated_bulk_upload_id),
    FOREIGN KEY(student_id) REFERENCES student_projections(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sgp_school_date ON student_grade_projections (school_id, test_date);
CREATE INDEX IF NOT EXISTS idx_sgp_student_date ON student_grade_projections (student_id, test_date);

INSERT INTO student_projection_updates (what) VALUES ('student_health_projections'), ( 'student_grade_projections');

-- +goose Down
DROP TABLE IF EXISTS student_health_projections;
DROP TABLE IF EXISTS student_grade_projections;
