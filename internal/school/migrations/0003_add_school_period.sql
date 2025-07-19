-- +goose Up
ALTER TABLE schools ADD COLUMN school_start_month INTEGER;
ALTER TABLE schools ADD COLUMN school_start_day INTEGER;
ALTER TABLE schools ADD COLUMN school_end_month INTEGER;
ALTER TABLE schools ADD COLUMN school_end_day INTEGER;

-- +goose Down
ALTER TABLE schools DROP COLUMN school_start_month;
ALTER TABLE schools DROP COLUMN school_start_day;
ALTER TABLE schools DROP COLUMN school_end_month;
ALTER TABLE schools DROP COLUMN school_end_day;
