# ADR 0003: Soft multi-feeder conflicts

- **Status:** Accepted (amended 2026-09-10)
- **Date:** 2026-08-04

## Context

Multiple feeders/devices at one school can record the same child offline. Sync must not double-count meals, and must not fail the whole batch when duplicates appear.

## Decision

One applied feed per student per **school calendar day**. The day is the calendar date of `fed_at_utc` / `unix_timestamp` in the school's timezone (today: `Asia/Manila` for every school). The server never uses a client-sent `local_calendar_date` for dedupe; that proto field may still be present for compatibility and is ignored.

First intent wins (`APPLIED`). Later distinct intents → `DUPLICATE_FLAGGED` (audit; batch still HTTP success). Idempotent retries of the same `client_item_id` → `IDEMPOTENT_REPLAY`. Outbox clears on all three terminal statuses.

## Consequences

- Need durable `feed_intents` + admin conflict visibility
- Local “fed today” is best-effort; server is authority
- Aggregate same-day check uses full school-TZ date derived from unix only
- Sync stores the derived date on intents for lookup / admin display

## Alternatives considered

- **Hard fail sync on conflict** — strands outbox and blocks other items
- **Merge both feeds** — violates one-meal-per-day product rule
- **Device local calendar date as dedupe key** — rejected; clients can lie or drift; web path has no device date
