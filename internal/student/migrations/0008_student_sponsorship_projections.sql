-- +goose Up 
CREATE TABLE IF NOT EXISTS student_sponsorship_projections (
    student_id TEXT NOT NULL,
    sponsor_id TEXT NOT NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Add indexes for common queries
CREATE INDEX idx_sponsorship_sponsor_dates ON student_sponsorship_projections (sponsor_id, start_date, end_date);
CREATE INDEX idx_sponsorship_student_dates ON student_sponsorship_projections (student_id, start_date, end_date);

-- +goose Down
DROP TABLE IF EXISTS student_sponsorship_projections; 