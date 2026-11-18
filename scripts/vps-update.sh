#!/bin/sh
# Update a VPS deployment to a newer release without touching its data (#398).
#
#   scripts/vps-update.sh [TAG]         (make vps-update [TAG=v0.39.0])
#
# 1. starts the current version (a backup needs the stack up) and backs up the
#    database and the uploads; a failed backup stops here, nothing changed;
# 2. fetches the release tags and checks out TAG, or the newest release;
# 3. rebuilds and recreates the containers. The volumes are kept, and the
#    `migrate` service applies only the migrations the database lacks.
#
# A downgrade is refused: migrations only go forward, so going back is a
# restore of the backup taken before the update (make restore BACKUP=...).
#
# The whole script is one brace group, read in full before any of it runs: the
# checkout in step 2 can replace this very file, and sh reads a script as it
# executes it.
{
    set -eu

    cd "$(dirname "$0")/.."

    TAG="${1:-}"
    BACKUPS="${BACKUP_DIR:-backups}"

    say() { printf 'vps-update: %s\n' "$1"; }
    fail() {
        printf 'vps-update: %s\n' "$1" >&2
        exit 1
    }

    command -v docker >/dev/null 2>&1 || fail "docker is not on PATH"
    command -v git >/dev/null 2>&1 || fail "git is not on PATH"
    [ -f .env ] && [ -f secrets/db_password.txt ] ||
        fail "this checkout is not set up — run: make vps-init DOMAIN=<your domain>"
    git diff --quiet HEAD -- ||
        fail "tracked files have local changes (see git status) — an update would overwrite them; commit, stash or revert them first"

    say "fetching the releases"
    git fetch --quiet --tags origin

    if [ -z "$TAG" ]; then
        TAG="$(git tag -l 'v[0-9]*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -n 1 || true)"
        [ -n "$TAG" ] || fail "no release tag (vX.Y.Z) found on origin"
    fi
    git rev-parse -q --verify "refs/tags/$TAG^{commit}" >/dev/null ||
        fail "there is no release $TAG (git tag -l 'v*' lists them)"

    current="$(git describe --tags --exact-match HEAD 2>/dev/null || true)"
    if [ -n "$current" ] && [ "$current" != "$TAG" ]; then
        oldest="$(printf '%s\n%s\n' "$current" "$TAG" | sort -V | head -n 1)"
        [ "$oldest" != "$TAG" ] ||
            fail "$TAG is older than the running $current — to go back, restore the backup taken before the update: make restore BACKUP=$BACKUPS/<timestamp>"
    fi
    say "updating ${current:-$(git rev-parse --short HEAD)} → $TAG"

    say "starting the current version so it can be backed up"
    docker compose up -d
    tries=0
    until docker compose exec -T db sh -c 'pg_isready -q -U "$POSTGRES_USER" -d "$POSTGRES_DB"' >/dev/null 2>&1; do
        tries=$((tries + 1))
        [ "$tries" -lt 60 ] || fail "the database did not become ready in 60 seconds — nothing was updated (docker compose logs db)"
        sleep 1
    done

    sh scripts/backup.sh "$BACKUPS" || fail "the backup failed, so nothing was updated"

    git -c advice.detachedHead=false checkout --quiet "$TAG"

    # VERSION in .env is what the images are built with: the web UI's About page
    # shows it (#410), and a later `docker compose up -d --build` keeps it.
    tmp=".env.vps-update.tmp"
    (
        umask 077
        if grep -q '^VERSION=' .env; then
            awk -v v="$TAG" 'index($0, "VERSION=") == 1 { print "VERSION=" v; next } { print }' .env >"$tmp"
        else
            cp .env "$tmp"
            [ ! -s "$tmp" ] || [ -z "$(tail -c 1 "$tmp")" ] || printf '\n' >>"$tmp"
            printf 'VERSION=%s\n' "$TAG" >>"$tmp"
        fi
    )
    chmod 600 "$tmp"
    mv "$tmp" .env

    say "rebuilding $TAG (the data volumes are kept)"
    VERSION="$TAG" docker compose up -d --build

    say "done — running $TAG"
    docker compose ps
    exit 0
}
