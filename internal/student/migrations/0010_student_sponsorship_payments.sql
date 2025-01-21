-- +goose Up
ALTER TABLE student_sponsorship_projections ADD COLUMN payment_id TEXT;
ALTER TABLE student_sponsorship_projections ADD COLUMN payment_amount DECIMAL(10,2);

-- +goose Down
ALTER TABLE student_sponsorship_projections 
DROP COLUMN payment_id,
DROP COLUMN payment_amount; 