#!/bin/sh
# Restore a backup made by scripts/backup.sh into this Compose deployment.
#
#   scripts/restore.sh BACKUP_DIR      (make restore BACKUP=backups/<stamp>)
#
# REPLACES what the deployment holds now: the database's public schema is
# dropped and rebuilt from the dump, and the uploads volume is emptied before
# the archive is unpacked. So the current data is never lost by mistake, a
# safety backup of it is taken first into backups/before-restore/ — the
# restore does not start unless that succeeds.
#
# Works on the same machine or a new one: start the stack once (`docker compose
# up -d`, which creates the volumes and applies the migrations), copy the backup
# folder over, then run this. The `api` service is stopped while its data is
# swapped and started again at the end.
set -eu

cd "$(dirname "$0")/.."

fail() {
    printf 'restore: %s\n' "$1" >&2
    exit 1
}

[ $# -eq 1 ] || fail "usage: scripts/restore.sh BACKUP_DIR"

SRC="$1"

# Everything is checked before anything is stopped or dropped.
[ -d "$SRC" ] || fail "$SRC is not a folder"
[ -s "$SRC/database.dump" ] || fail "$SRC has no database.dump — not a backup made by scripts/backup.sh"
[ -s "$SRC/uploads.tgz" ] || fail "$SRC has no uploads.tgz — not a backup made by scripts/backup.sh"
command -v docker >/dev/null 2>&1 || fail "docker is not on PATH"

[ -n "$(docker compose ps -q --status running db 2>/dev/null)" ] ||
    fail "the db service is not running — start the stack first (docker compose up -d)"

api="$(docker compose ps -aq api 2>/dev/null)"
[ -n "$api" ] || fail "there is no api container yet — run docker compose up -d once first"

SRC_ABS="$(cd "$SRC" && pwd)"

printf 'restore: taking a safety backup of the current data first\n'
sh scripts/backup.sh backups/before-restore || fail "the safety backup failed — nothing was changed"

printf 'restore: stopping api while its data is replaced\n'
docker compose stop api

# The api comes back whatever happens below: a failed restore must not leave
# the service down without saying so.
trap 'printf "restore: FAILED — starting api again; the data may be partly restored\n" >&2; docker compose start api >/dev/null 2>&1 || true' EXIT

printf 'restore: restoring the database from %s/database.dump\n' "$SRC"
# The schema is emptied rather than restored over with --clean: a backup from
# an older release lacks the tables newer migrations added, and --clean would
# leave them behind for the migration step below to trip over.
docker compose exec -T db sh -c \
    'psql -v ON_ERROR_STOP=1 -q -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"'
# --no-owner: the dump may come from a deployment whose user had another name.
docker compose exec -T db sh -c \
    'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --single-transaction' \
    <"$SRC/database.dump"

printf 'restore: replacing the uploads from %s/uploads.tgz\n' "$SRC"
docker run --rm --volumes-from "$api" -v "$SRC_ABS:/backup:ro" alpine:3 sh -c \
    'find /data/uploads -mindepth 1 -delete && tar -xzpf /backup/uploads.tgz --numeric-owner -C /data/uploads'

# A backup from an older release carries an older schema: bring it up to the
# migrations this checkout ships before the api reads it.
printf 'restore: applying any newer migrations\n'
docker compose run --rm migrate

trap - EXIT
docker compose start api
printf 'restore: done — api is starting on the restored data\n'
