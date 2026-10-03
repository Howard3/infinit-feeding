# AGENTS.md — infinit-feeding

Guidance for coding agents working in this repository.

## What this repo is

Go backend for Infinit Feeding: event-sourced Student/School/File/BulkUpload aggregates (gosignal), HTMX admin/staff/feeding UI, Clerk auth, local libSQL file.

Sibling apps (not in this repo):

- `~/code/infinit-foundation-frontend` — sponsor/foundation frontend
- `~/code/infinit-feeding-android` — offline feeder Android app

## Remaining work

**Before starting new offline/sync features, read [`docs/remaining-work.md`](docs/remaining-work.md).** It lists P0/P1/P2 gaps (Android↔`/sync/v1` wiring, Clerk, encryption, roster deltas, etc.).

## Hard boundaries

- **Do not** copy the server libSQL file onto mobile devices. Offline feeding uses the **sync API** (`sync.v1` envelope).
- **Do not** rewrite or remove web `/feeding` for Android rollout — keep parallel; both hydrate Student via `FeedStudent`.
- Domain packages under `internal/<domain>` stay unaware of sibling domains; prefer events / ACLs.
- Secrets stay in env (`.env`); never commit credentials.

## Architecture docs

- Decision trail: [`docs/adr/`](docs/adr/) (Architecture Decision Records)
- Backlog: [`docs/remaining-work.md`](docs/remaining-work.md)
- Support: [`docs/support/`](docs/support/) (including [`replica-wal.md`](docs/support/replica-wal.md) for local libSQL backup and restore)
- Sync harness: [`docs/testing-sync.md`](docs/testing-sync.md)

Read relevant ADRs before changing sync, feeding, or auth.

## Sync (mobile)

- Protos: `events/sync/v1/`, `events/roster/v1/`, `events/feeding/v1/`
- Runtime: `internal/sync` — devices, feed intents, Pull/Push handlers
- Soft conflicts: batch never fails for same-day duplicates (`DUPLICATE_FLAGGED`)
- Transport v1: HTTP + `application/proto` (Connect optional follow-up)

## Common commands

```bash
go run .
go test ./internal/sync/... ./internal/student/ -count=1
go run ./cmd/syncharness
cd events && buf generate
```

## Style

- Match existing patterns in `internal/`
- When completing backlog items, update `docs/remaining-work.md`
