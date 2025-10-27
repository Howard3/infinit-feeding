# Project Guidelines

This document explains how to work in this codebase using Domain-Driven Design (DDD) with event sourcing and CQRS. It adds concrete, actionable conventions so contributors can make consistent changes with minimal friction.

Tech stack: Go, TailwindCSS, Turso (SQLite), Protobuf for events/aggregates, Templ for Web UI.

## Repository Layout (high level)
- events: Protobuf definitions for events and aggregates
- gen/: Generated code (do not edit by hand)
- internal/
  - <domain>/
    - aggregates, services, repositories, event handlers, domain logic, migrations
  - webapi/: User-facing HTTP layer and templates
- Taskfile.yaml: Common tasks. Prefer using task over raw commands

## DDD Boundaries and Module Ownership
- Keep domain code inside internal/<domain>. Domains are wholly unaware of other domains. Do not import from sibling domains directly. Prefer published events for cross-domain collaboration; if synchronous interaction is truly required, depend on an interface (ACL) owned by the consuming domain, not the provider.
- Allowed imports:
  - A domain may depend on shared infrastructure under internal/infrastructure
  - WebAPI can depend on multiple domains but should not contain domain logic
- Organize domain packages:
  - aggregates: Aggregate roots and command handling
  - services: Orchestrate domain use cases, purely in-domain
  - repositories: Aggregate persistence and event store interactions
  - event_handlers: Projectors and process managers for domain events
  - migrations: SQL for read models (per domain)
- Anti-Corruption Layer (ACL) and interfaces:
  - Interfaces live in the consuming domain (e.g., internal/<consumer>/acl or under services as small interfaces). The provider never imports these.
  - Adapters that implement those interfaces live at the composition boundary (webapi or infrastructure) and may import the provider domain to call into its queries/read models.
  - Never pass provider domain models directly into the consumer. Map into consumer-owned DTOs/value objects to avoid leaking concepts.
  - Prefer event-driven integration. Use ACL only for unavoidable synchronous reads or command orchestration where latency budgets require it.

### Cross-domain Integration: ACL and Interfaces
- Principles:
  - Dependency direction points inward to the consuming domain’s interface. Providers do not depend on consumers.
  - Keep interfaces minimal and use language of the consuming domain. Avoid wide “god” interfaces.
  - Favor eventual consistency; design UI and workflows tolerant to lag when possible.
- Placement:
  - Define the interface in internal/<consumer>/acl (or a minimal subpackage under services).
  - Provide an adapter in internal/infrastructure or internal/webapi composition layer that implements the interface by delegating to the provider’s read models or services.
- Data contracts:
  - Use consumer-owned DTOs. Map from provider read models/events to these DTOs.
  - Handle missing/unknown fields and version drift defensively.
- Testing:
  - Unit test the consumer against a mock of the interface.
  - Add integration tests for the adapter wiring if it has logic (mapping, error translation).

## Events and Protobufs
- Location: events/
- Scope: Model only facts that happened. Avoid “SetX” events unless they represent a meaningful business fact
- Naming: Past tense, explicit. Example: StudentRegistered, HeightRecorded, BMIComputed
- Versioning and evolution:
  - Prefer additive changes (add new fields with sensible defaults)
  - If semantics change, create a new event name (e.g., StudentRegisteredV2) and keep handlers backward compatible
  - Do not reuse fields for new meanings
- Backward compatibility:
  - Event handlers must tolerate missing fields and unknown fields
  - Projection code should handle both old and new event versions until a full backfill is completed
- Generation:
  - Keep .proto sources as the single source of truth
  - Regenerate code with task commands defined in Taskfile.yaml (see that file for exact targets)

## Aggregates (Command side)
- Handle intent through explicit command methods returning domain events
- Validate invariants inside aggregates; services should not duplicate invariant checks
- Keep aggregates small and focused. Derive state from applied events only
- Do not reach out to external resources inside aggregates

## Repositories and Event Store
- Repository is the abstraction that loads/saves aggregates by appending/reading events
- Concurrency:
  - Use optimistic concurrency with expected version checks when appending events
  - On conflict, reload and retry command with care, or surface a domain-level concurrency error
- Serialization:
  - Store the full protobuf-encoded event with type and version metadata

## Projections and CQRS Read Models
- Read models live under internal/<domain>/migrations and corresponding repository/query code
- Consistency:
  - Projections should be idempotent and able to re-run from stream start
  - Use upsert/ON CONFLICT rules in SQL to keep handlers idempotent
- Rebuild strategy:
  - Provide a way to rebuild a projection from event history (script or task). Document it in README or Taskfile.yaml
  - During rebuilds, prefer building into temp tables then swap to minimize downtime
- Ownership:
  - Each domain maintains its own read models and migrations

## Database Migrations (Turso/SQLite)
- Location: internal/<domain>/migrations
- Filename convention: zero-padded sequence + short description, e.g., 0007_add_bmi_projection.sql
- Migration rules:
  - Keep migrations idempotent where possible
  - Never modify a migration that has shipped; add a new migration
  - Include indexes needed by the projection queries

## WebAPI and Templates
- Templ usage:
  - Edit only .templ files. Never edit generated .go files
  - Verify templates compile using: task build:templates
- Separation of concerns:
  - Keep controllers thin. Map HTTP -> commands/queries, then render
  - No domain logic in templates; compute in services or queries first

## Testing Strategy
- Unit tests:
  - Aggregate tests: command -> events, events -> state transitions
  - Service tests: orchestrations with mocked repositories
- Integration tests:
  - Projection tests against an in-memory or temp SQLite/Turso instance
  - Seed with events, run handlers, assert read models
- Fixtures:
  - Prefer building events via helper builders to keep tests readable

## Error Handling, Logging, and Observability
- Errors:
  - Use wrapped errors with context. Surface domain errors distinctly from infrastructure errors
- Logging:
  - Use structured logs (key/value). No printf in libraries
  - Avoid logging sensitive data (PII)
- Metrics/Tracing (if/when added):
  - Wrap external calls; propagate context.Context

## Performance and Backfills
- When introducing a new projection or event version, plan a backfill:
  - Add handlers that can process both legacy and new events
  - Provide a rebuild path and measure duration on a copy first
- Index strategically. Verify query plans for reports and admin views

## Security and Configuration
- Secrets and API keys belong in environment variables (.env for local only). Do not commit secrets
- Validate and sanitize user input in WebAPI
- Principle of least privilege for DB connections and file access

## Contribution Workflow
- Prefer task commands defined in Taskfile.yaml over ad-hoc scripts
- Before submitting PR:
  - Run linters/formatters and build templates
  - Ensure migrations apply cleanly on a fresh DB
  - Add or update tests for aggregates, services, and projections
  - Update docs if behavior or commands change

## Quick Checklists
- Adding a new domain feature:
  - Define events (protobuf) and regenerate code
  - Update aggregate to emit events enforcing invariants
  - Implement repository/service changes as needed
  - Add/adjust projection handlers and migrations
  - Add tests (aggregate + projection)
  - Update WebAPI flows and templates if user-facing
- Changing an event:
  - Prefer new event name/version; keep old handlers
  - Make projections tolerant; plan rebuild/backfill

## References
- See Taskfile.yaml for available tasks
- See internal/webapi/templates for Templ-based views
- Per-domain examples: browse internal/student for a full set of aggregate, handlers, migrations, and WebAPI usage

