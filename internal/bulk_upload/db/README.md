# Bulk Upload Database Layer

This directory contains the database layer for the bulk upload module, implemented using [sqlc](https://sqlc.dev/) with SQLite (Turso).

## Directory Structure

- `query/`: Contains SQL queries used by sqlc to generate Go code
- `migrations/`: Contains the SQLite database schema migrations, using [goose](https://github.com/pressly/goose)
- `sqlc/`: Contains the generated Go code (models, queries, interfaces)
- `sqlc.yaml`: Configuration file for sqlc

## Generating Code

To generate the database code, you'll need to install sqlc and run it in this directory.

### 1. Install sqlc

```bash
# Using Go:
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Or using Homebrew:
brew install sqlc

# Or using Docker:
docker pull sqlc/sqlc
```

### 2. Generate the code

Run sqlc using the task command from the project root:

```bash
# Using the project's task command:
task build:sqlc:bulk_uploads

# If installed with Go or Homebrew and not using task:
cd internal/bulk_upload/db
sqlc generate

# If using Docker and not using task:
docker run --rm -v $(pwd):/src -w /src/internal/bulk_upload/db sqlc/sqlc generate
```

## Available Queries

The following queries are implemented:

- `UpsertBulkUploadProjection`: Insert or replace a bulk upload projection (SQLite upsert)
- `ListBulkUploads`: Get a paginated list of bulk uploads ordered by initiated_at in descending order
- `CountBulkUploads`: Count total number of bulk uploads
- `GetBulkUploadByID`: Get a specific bulk upload by ID

## Using the Generated Code

The generated code provides a `Querier` interface and a concrete implementation that can be used to interact with the database:

```go
import "infinit-feeding/internal/bulk_upload/db/sqlc"

// Create a new querier
querier := sqlc.New(dbConn)

// Use the queries
uploads, err := querier.ListBulkUploads(ctx, 10, 0)
```

When adding new queries, add them to the `query/bulk_upload.sql` file and regenerate the code using the task command:

```bash
task build:sqlc:bulk_uploads
```

## SQLite-Specific Considerations

This implementation uses SQLite (via Turso) instead of PostgreSQL:

1. All queries use `?` placeholders instead of `$1`, `$2`, etc.
2. JSON data is stored as TEXT in SQLite, so we need to manually marshal/unmarshal
3. The schema uses SQLite-specific data types (INTEGER instead of INT, BLOB instead of BYTEA)
4. Upserts use `INSERT OR REPLACE` syntax instead of `ON CONFLICT`
5. Timestamps use SQLite's `datetime()` function
