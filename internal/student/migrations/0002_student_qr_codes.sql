-- +goose Up 
CREATE TABLE IF NOT EXISTS student_code_lookup (
    id TEXT PRIMARY KEY,
    code CHAR(20) NOT NULL,
    UNIQUE (code)
);

-- +goose Down 
DROP TABLE IF EXISTS student_code_lookup;
