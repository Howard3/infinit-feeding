# Project Guidelines
    
This is a domain-driven design (DDD) and event-sourced project. It's written in Go, TailwindCSS for the styling, and Turso (SQLite) for the database. Data is first and foremost stored as events, feeding the aggregate. Projections are used for easy reading following CQRS principles.

Each domain has its own directory, and for each domain that has a database, there is a `migrations` directory.

All events and aggregates as formed as protobufs, and the protobufs are stored in the `events` directory.

Domains can be found in the `internal` directory. You should adhere to domain separation.

# Commands 
Most commands are already defined via TaskFile in `Taskfile.yml`.

# Domains
Here's some specific documentation on domains.

Domains are comprised of the following:
- services
- repositories
- aggregates
- event handlers
- domain business logic
- anti-corruption layer definitions (if needed)
- migrations

## WebAPI
Is all user-facing code, including admin interface. Uses Templ engine for templating. Only edit .templ files, not the generated .go files. You can verify templates compile by running `task build:templates`.

