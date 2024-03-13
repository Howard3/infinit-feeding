-- +goose Up 
CREATE TABLE IF NOT EXISTS "student_profile_photos" (
    "id" TEXT PRIMARY KEY,
    "file_id" TEXT NOT NULL
);


-- +goose Down 
DROP TABLE IF EXISTS "student_profile_photos";
