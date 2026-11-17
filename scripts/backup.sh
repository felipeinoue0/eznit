#!/bin/sh
# Back up a Compose deployment's data: the database and the uploaded files.
#
#   scripts/backup.sh [OUT_DIR]        (make backup)
#
# Writes OUT_DIR/<UTC timestamp>/ with
#   database.dump   pg_dump custom format: portable across Postgres versions
#                   and volumes, restored with scripts/restore.sh
#   uploads.tgz     the bytes behind every file record (the `api-data` volume)
#
# Both are needed: a database without its uploads has records whose files are
# gone, and uploads without the database have no owner (#388).
#
# The dump runs inside the `db` container over its local socket, as the user
# and database the container was created with, so the password is never read
# here. The stack must be up (`docker compose up -d`). Copy the folder off the
# machine afterwards: a backup on the same disk dies with that disk.
set -eu

cd "$(dirname "$0")/.."

OUT="${1:-backups}"
STAMP="$(date -u +%Y-%m-%dT%H%M%SZ)"
DEST="$OUT/$STAMP"

fail() {
    printf 'backup: %s\n' "$1" >&2
    exit 1
}

command -v docker >/dev/null 2>&1 || fail "docker is not on PATH"

[ -n "$(docker compose ps -q --status running db 2>/dev/null)" ] ||
    fail "the db service is not running — start the stack first (docker compose up -d)"

api="$(docker compose ps -aq api 2>/dev/null)"
[ -n "$api" ] ||
    fail "there is no api container, so there is no uploads volume to read — start the stack first"

mkdir -p "$DEST"
chmod 700 "$OUT" "$DEST" 2>/dev/null || true
DEST_ABS="$(cd "$DEST" && pwd)"

# A failed step leaves no half-written backup that looks complete.
trap 'rm -rf "$DEST"' EXIT

printf 'backup: dumping the database into %s/database.dump\n' "$DEST"
docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
    >"$DEST/database.dump"

printf 'backup: archiving the uploads into %s/uploads.tgz\n' "$DEST"
docker run --rm --volumes-from "$api" -v "$DEST_ABS:/backup" alpine:3 \
    tar -czf /backup/uploads.tgz --numeric-owner -C /data/uploads .

[ -s "$DEST/database.dump" ] || fail "the database dump is empty"
[ -s "$DEST/uploads.tgz" ] || fail "the uploads archive is empty"

trap - EXIT
printf 'backup: done — %s (copy it off this machine)\n' "$DEST"
