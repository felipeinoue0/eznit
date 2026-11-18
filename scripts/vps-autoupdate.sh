#!/bin/sh
# Automatic updates for a VPS deployment: the server pulls new releases itself
# (#404). Nothing about the server is stored on GitHub — no address, no key, no
# workflow — and nothing new can reach it: the only traffic is the server's own
# `git fetch` of the public repository.
#
#   scripts/vps-autoupdate.sh install   (make vps-autoupdate)          hourly cron entry
#   scripts/vps-autoupdate.sh remove    (make vps-autoupdate-off)      remove it
#   scripts/vps-autoupdate.sh status    (make vps-autoupdate-status)   entry, pause, last log lines
#   scripts/vps-autoupdate.sh resume    (make vps-autoupdate-resume)   clear a pause after a failure
#   scripts/vps-autoupdate.sh run       what cron runs
#
# A run
#   1. takes a lock, so a slow update never overlaps the next check;
#   2. fetches the tags and does nothing unless a release (vX.Y.Z) newer than the
#      checked-out one exists — only releases are deployed, never a branch;
#   3. runs scripts/vps-update.sh, which backs up first and stops if that fails;
#   4. checks that the API answers /v1/healthcheck.
# Everything goes to backups/auto-update.log. Any failure writes
# backups/auto-update.paused and later runs do nothing until a person has looked
# and run `make vps-autoupdate-resume`. There is no automatic rollback on
# purpose: going back is a restore, and a restore replaces data.
#
# The whole script is one brace group, read in full before any of it runs: the
# update checks out another release, which can replace this very file.
{
    set -eu

    cd "$(dirname "$0")/.."
    ROOT="$(pwd)"

    BACKUPS="${BACKUP_DIR:-backups}"
    LOG="$BACKUPS/auto-update.log"
    PAUSED="$BACKUPS/auto-update.paused"
    LOCK="$BACKUPS/.auto-update.lock"
    TAG_MARK="# eznit-autoupdate $ROOT"
    MAX_LOG_BYTES=1048576

    say() { printf 'vps-autoupdate: %s\n' "$1"; }
    fail() {
        printf 'vps-autoupdate: %s\n' "$1" >&2
        exit 1
    }
    stamp() { date -u +%Y-%m-%dT%H:%M:%SZ; }
    log() { printf '%s %s\n' "$(stamp)" "$1"; }

    # current_tag prints the release tag HEAD is on, or nothing on a branch.
    current_tag() { git describe --tags --exact-match HEAD 2>/dev/null || true; }

    # latest_tag prints the newest vX.Y.Z tag.
    latest_tag() {
        git tag -l 'v[0-9]*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -n 1 || true
    }

    # newer A B succeeds when release B is newer than release A.
    newer() {
        [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -n 1)" = "$2" ]
    }

    # pause REASON stops later runs until `resume`, and says how to look.
    pause() {
        printf '%s %s\n' "$(stamp)" "$1" >"$PAUSED"
        log "FAILED: $1"
        log "automatic updates are paused until someone checks: make vps-autoupdate-status, then make vps-autoupdate-resume"
        exit 1
    }

    # healthy waits up to a minute for the API to answer its healthcheck.
    healthy() {
        tries=0
        until docker compose exec -T api wget -q -O /dev/null http://127.0.0.1:4000/v1/healthcheck >/dev/null 2>&1; do
            tries=$((tries + 1))
            [ "$tries" -lt 30 ] || return 1
            sleep 2
        done
    }

    cron_lines() { crontab -l 2>/dev/null || true; }

    run() {
        mkdir -p "$BACKUPS"
        chmod 700 "$BACKUPS" 2>/dev/null || true
        if [ -f "$LOG" ] && [ "$(wc -c <"$LOG" | tr -d ' ')" -gt "$MAX_LOG_BYTES" ]; then
            mv "$LOG" "$LOG.1"
        fi
        exec >>"$LOG" 2>&1

        if command -v flock >/dev/null 2>&1; then
            exec 9>"$LOCK"
            flock -n 9 || exit 0
        else
            mkdir "$LOCK.d" 2>/dev/null || exit 0
            trap 'rmdir "$LOCK.d"' EXIT
        fi

        if [ -f "$PAUSED" ]; then
            log "paused since an earlier failure ($(cat "$PAUSED")) — nothing done"
            exit 0
        fi

        current="$(current_tag)"
        [ -n "$current" ] ||
            pause "this checkout is not on a release tag ($(git rev-parse --short HEAD)); run make vps-update once by hand"

        git fetch --quiet --tags origin || {
            log "could not fetch the releases (network?) — will try again next run"
            exit 1
        }
        latest="$(latest_tag)"
        if [ -z "$latest" ] || ! newer "$current" "$latest"; then
            log "up to date ($current)"
            exit 0
        fi

        log "updating $current → $latest"
        sh scripts/vps-update.sh "$latest" || pause "make vps-update to $latest failed (see the lines above)"
        healthy || pause "$latest is running but the API does not answer /v1/healthcheck (docker compose ps -a; docker compose logs api)"
        log "updated to $latest; the API answers"
    }

    install() {
        command -v crontab >/dev/null 2>&1 || fail "crontab is not installed — sudo apt install -y cron"
        [ -f .env ] && [ -f secrets/db_password.txt ] ||
            fail "this checkout is not set up — run: make vps-init DOMAIN=<your domain>"
        [ -n "$(current_tag)" ] ||
            fail "this checkout is not on a release tag — run make vps-update once first"
        # A minute of its own, so many servers do not all fetch at :00.
        minute=$(($(date +%s) % 60))
        line="$minute * * * * cd '$ROOT' && sh scripts/vps-autoupdate.sh run $TAG_MARK"
        { cron_lines | grep -vF "$TAG_MARK" || true; printf '%s\n' "$line"; } | crontab -
        mkdir -p "$BACKUPS"
        say "on: every hour at minute $minute, this server checks for a new release and installs it"
        say "log: $ROOT/$LOG    status: make vps-autoupdate-status    off: make vps-autoupdate-off"
    }

    remove() {
        command -v crontab >/dev/null 2>&1 || fail "crontab is not installed, so there is nothing to remove"
        if cron_lines | grep -qF "$TAG_MARK"; then
            { cron_lines | grep -vF "$TAG_MARK" || true; } | crontab -
            say "off: automatic updates are removed for $ROOT"
        else
            say "automatic updates were not on for $ROOT"
        fi
    }

    status() {
        if command -v crontab >/dev/null 2>&1 && cron_lines | grep -qF "$TAG_MARK"; then
            say "on: $(cron_lines | grep -F "$TAG_MARK" | cut -d' ' -f1-5) (minute hour day month weekday)"
        else
            say "off (make vps-autoupdate turns it on)"
        fi
        say "running: $(current_tag || true)"
        if [ -f "$PAUSED" ]; then
            say "PAUSED: $(cat "$PAUSED")"
            say "after fixing it: make vps-autoupdate-resume"
        fi
        if [ -f "$LOG" ]; then
            say "last lines of $LOG:"
            tail -n 15 "$LOG"
        fi
    }

    resume() {
        if [ -f "$PAUSED" ]; then
            rm -f "$PAUSED"
            say "resumed: the next hourly run checks for a release again"
        else
            say "not paused"
        fi
    }

    case "${1:-}" in
        run) run ;;
        install) install ;;
        remove) remove ;;
        status) status ;;
        resume) resume ;;
        *) fail "usage: $0 install|remove|status|resume|run" ;;
    esac
    exit 0
}
