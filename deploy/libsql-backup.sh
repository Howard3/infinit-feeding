#!/bin/sh
# Online backup of the local libSQL file. Does not stop the app.
# sqlite3 .backup is safe while writers are active.
set -eu

DB="${LIBSQL_DB_PATH:-/var/lib/libsql/feeding.db}"
DEST="${LIBSQL_BACKUP_DIR:-/backups}"
KEEP_DAYS="${LIBSQL_BACKUP_KEEP_DAYS:-14}"

if ! command -v sqlite3 >/dev/null 2>&1; then
  apk add --no-cache sqlite
fi

mkdir -p "$DEST"

while true; do
  if [ ! -f "$DB" ]; then
    echo "waiting for $DB"
    sleep 30
    continue
  fi
  stamp=$(date +%Y%m%d)
  sqlite3 "$DB" ".backup '$DEST/feeding-$stamp.db'"
  echo "backed up $DB to $DEST/feeding-$stamp.db"
  find "$DEST" -name 'feeding-*.db' -mtime +"$KEEP_DAYS" -delete
  sleep 86400
done
