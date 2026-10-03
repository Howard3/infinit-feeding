# Local libSQL file

The server database is one libSQL file, `file:/var/lib/libsql/feeding.db`, on the
`libsql_data` volume. The app does not sync that file to Turso. A remote
`libsql://` or `http(s)://` `DB_URI` fails boot when `GO_ENV` is not
`development`.

The app sets `journal_mode=WAL` once and `busy_timeout=10000` on each
connection, then runs `PRAGMA wal_checkpoint(PASSIVE)` every 30 seconds.
PASSIVE does not replace a backup.

## Backup

`db-backup` in compose runs [`deploy/libsql-backup.sh`](../../deploy/libsql-backup.sh).
Once a day it runs:

```bash
sqlite3 /var/lib/libsql/feeding.db ".backup /backups/feeding-YYYYMMDD.db"
```

Backups land on the `libsql_backups` volume. Copies older than 14 days are
deleted. `.backup` is safe while the app is writing. Turso is not a backup.

## Restore

1. Stop `infinit-feeding` and `db-backup`.
2. Replace `/var/lib/libsql/feeding.db` with a backup. If `feeding.db-wal` or
   `feeding.db-shm` exist next to the live file, remove them so SQLite does
   not replay a WAL from a different database.
3. Start the app. Confirm `/health` and a few row counts (schools, students,
   recent feeding projections).

## Cutover off the old replica path

Production used to mount `libsql-data` at `/tmp/geevly-libsql/replica.db` and
open it with a `file:` URI. `replica.db-info` is leftover Turso sync metadata.
Do not copy it onto the new path, and do not start `turso-sync-container`.

1. Stop the app. Do not start the sync container.
2. Archive `replica.db` plus `-wal` and `-shm` if they exist. Leave
   `replica.db-info` out of the new database directory.
3. Put that archive copy at `/var/lib/libsql/feeding.db` on `libsql_data`.
4. Set `DB_URI=file:/var/lib/libsql/feeding.db`. Remove any Turso URL and
   `authToken` from the app environment.
5. Start the app. Logs should say `opening local libsql database`. Migrations
   should finish and `/health` should return ok.
6. Compare schools, students, and recent feeding-projection counts with the
   archive. Then remove the old `/tmp/geevly-libsql` mount. Keep the archive
   until the new backups exist.
