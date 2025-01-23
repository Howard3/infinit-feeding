-- +goose Up 
ALTER TABLE student_projections ADD COLUMN max_sponsorship_date DATE;

-- +goose Down
ALTER TABLE student_projections DROP COLUMN max_sponsorship_date; 