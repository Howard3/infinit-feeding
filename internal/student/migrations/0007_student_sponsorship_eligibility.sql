-- +goose Up 
ALTER TABLE student_projections ADD COLUMN eligible_for_sponsorship BOOLEAN NOT NULL DEFAULT false;

-- +goose Down 
ALTER TABLE student_projections DROP COLUMN eligible_for_sponsorship; 