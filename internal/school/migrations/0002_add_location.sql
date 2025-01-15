-- +goose Up
ALTER TABLE schools ADD COLUMN country TEXT;
ALTER TABLE schools ADD COLUMN city TEXT;

-- +goose Down
ALTER TABLE schools DROP COLUMN country;
ALTER TABLE schools DROP COLUMN city;
