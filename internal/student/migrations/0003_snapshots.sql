-- +goose Up 
CREATE TABLE IF NOT EXISTS "student_snapshots" (
    "id" TEXT PRIMARY KEY,
    "version" INTEGER NOT NULL,
    "data" JSONB NOT NULL,
    "timestamp" INT NOT NULL
);


-- +goose Down 
DROP TABLE IF EXISTS "student_snapshots";
