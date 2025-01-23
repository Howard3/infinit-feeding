-- +goose Up
ALTER TABLE schools ADD COLUMN country TEXT NOT NULL DEFAULT '';
ALTER TABLE schools ADD COLUMN city TEXT NOT NULL DEFAULT '';


-- +goose Down
ALTER TABLE schools DROP COLUMN country;
ALTER TABLE schools DROP COLUMN city;
