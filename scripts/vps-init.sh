#!/bin/sh
# Prepare this checkout to run as a public server on a VPS (#398).
#
#   scripts/vps-init.sh DOMAIN          (make vps-init DOMAIN=eznit.example.com)
#
# Run it once, after `make setup` (the make target runs that first). It
#   - replaces the example database password with a random one, but only while
#     no database exists yet: Postgres applies the password when it creates its
#     volume, so changing it afterwards would lock the API out of the data;
#   - keeps the API and the UI ports on 127.0.0.1, so only the HTTPS proxy is
#     public, and lets the API trust the proxy's forwarded client address;
#   - turns on the `caddy` service (COMPOSE_PROFILES=https) for DOMAIN.
#
# After it, `docker compose up -d --build` starts everything, HTTPS included.
# Running it again is safe: it changes only what is not set yet, and DOMAIN.
# The password is never printed.
set -eu

cd "$(dirname "$0")/.."
umask 077

DOMAIN="${1:-}"
ENV_FILE=".env"
SECRET="secrets/db_password.txt"
SECRET_EXAMPLE="secrets/db_password.txt.example"

say() { printf 'vps-init: %s\n' "$1"; }
fail() {
    printf 'vps-init: %s\n' "$1" >&2
    exit 1
}

[ -n "$DOMAIN" ] || fail "usage: make vps-init DOMAIN=eznit.example.com (the name whose DNS points at this server)"
case "$DOMAIN" in
    *[!A-Za-z0-9.-]* | .* | *. | *..* | -*)
        fail "\"$DOMAIN\" is not a host name: give only the name, like eznit.example.com, with no https:// or path" ;;
    *.*) ;;
    *) fail "\"$DOMAIN\" is not a public host name: it needs a dot, like eznit.example.com" ;;
esac

command -v docker >/dev/null 2>&1 || fail "docker is not on PATH — install Docker and its Compose plugin first"
[ -f "$ENV_FILE" ] || fail "$ENV_FILE is missing — run make setup first"
[ -f "$SECRET" ] || fail "$SECRET is missing — run make setup first"

# A database exists once its volume or its container does. The project name is
# `name:` in docker-compose.yml unless COMPOSE_PROJECT_NAME overrides it.
database_exists() {
    docker volume inspect "${COMPOSE_PROJECT_NAME:-eznit}_postgres-data" >/dev/null 2>&1 && return 0
    [ -n "$(docker compose ps -aq db 2>/dev/null)" ]
}

# set_env KEY VALUE writes KEY=VALUE into .env, replacing an existing line, and
# keeps the file mode 600. Values here never hold a secret.
set_env() {
    tmp="$ENV_FILE.vps-init.tmp"
    if grep -q "^$1=" "$ENV_FILE"; then
        awk -v k="$1" -v v="$2" 'index($0, k "=") == 1 { print k "=" v; next } { print }' "$ENV_FILE" >"$tmp"
    else
        cp "$ENV_FILE" "$tmp"
        [ ! -s "$tmp" ] || [ -z "$(tail -c 1 "$tmp")" ] || printf '\n' >>"$tmp"
        printf '%s=%s\n' "$1" "$2" >>"$tmp"
    fi
    chmod 600 "$tmp"
    mv "$tmp" "$ENV_FILE"
}

# get_env KEY prints KEY's value in .env, without surrounding quotes. Used only
# for the non-secret COMPOSE_PROFILES.
get_env() {
    sed -n "s/^$1=//p" "$ENV_FILE" | tail -n 1 | tr -d "\"'"
}

if [ ! -s "$SECRET" ] || { [ -f "$SECRET_EXAMPLE" ] && cmp -s "$SECRET" "$SECRET_EXAMPLE"; }; then
    if database_exists; then
        say "WARNING: the database already exists and $SECRET still holds the example password."
        say "         It is left as it is, because the database keeps the password it was created with."
        say "         Change both together: https://github.com/fiwon123/eznit/wiki/Deploying-on-a-VPS#-changing-the-database-password"
    else
        tmp="$SECRET.tmp"
        LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 40 >"$tmp"
        [ "$(wc -c <"$tmp" | tr -d ' ')" = 40 ] || {
            rm -f "$tmp"
            fail "could not generate a password"
        }
        chmod 600 "$tmp"
        mv "$tmp" "$SECRET"
        say "generated a random database password in $SECRET (not shown — keep a copy in your password manager)"
    fi
else
    say "kept the database password already in $SECRET"
fi

set_env API_BIND 127.0.0.1
set_env WEB_BIND 127.0.0.1
set_env TRUST_PROXY true
set_env DOMAIN "$DOMAIN"

profiles="$(get_env COMPOSE_PROFILES)"
case ",$profiles," in
    *,https,*) ;;
    ,,) set_env COMPOSE_PROFILES https ;;
    *) set_env COMPOSE_PROFILES "$profiles,https" ;;
esac
say "set API_BIND, WEB_BIND, TRUST_PROXY, DOMAIN=$DOMAIN and COMPOSE_PROFILES in $ENV_FILE"

cat <<EOF

Ready. Before the first start:
  - DOMAIN's DNS (an A record) must point at this server's address;
  - ports 80 and 443 must be open and free (no other web server running).

Start it:          docker compose up -d --build
Then open:         https://$DOMAIN   and sign up first — the first account is the admin.
Stop it:           docker compose down        (keeps the data; never add -v)
Update it later:   make vps-update
EOF
