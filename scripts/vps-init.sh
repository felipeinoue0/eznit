#!/bin/sh
# Prepare this checkout to run as a public server on a VPS (#398).
#
#   scripts/vps-init.sh DOMAIN [PROXY]  (make vps-init DOMAIN=eznit.example.com [PROXY=traefik])
#
# PROXY says what serves HTTPS:
#   caddy    (default) the `caddy` service in docker-compose.yml, on 80 and 443
#   traefik  a Traefik the server already runs: writes docker-compose.override.yml
#            with the labels and the network for the `web` service (#401)
#   none     your own reverse proxy, pointed at 127.0.0.1:5173
#
# Run it once, after `make setup` (the make target runs that first). It
#   - replaces the example database password with a random one, but only while
#     no database exists yet: Postgres applies the password when it creates its
#     volume, so changing it afterwards would lock the API out of the data;
#   - keeps the API and the UI ports on 127.0.0.1, so only the HTTPS proxy is
#     public, and lets the API trust the proxy's forwarded client address;
#   - turns on the `caddy` service (COMPOSE_PROFILES=https) for DOMAIN, or for
#     PROXY=traefik|none turns it off, so it does not fight that proxy for 80/443.
#
# After it, `docker compose up -d --build` starts everything, HTTPS included.
# Running it again is safe: it changes only what is not set yet, DOMAIN and the
# proxy choice. An existing docker-compose.override.yml is never overwritten.
# The password is never printed.
set -eu

cd "$(dirname "$0")/.."
umask 077

DOMAIN="${1:-}"
PROXY="${2:-caddy}"
OVERRIDE="docker-compose.override.yml"
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

case "$PROXY" in
    caddy | traefik | none) ;;
    *) fail "PROXY=$PROXY is not one of caddy (the default), traefik or none" ;;
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

# The `https` profile is the built-in Caddy: on for PROXY=caddy, off otherwise.
profiles="$(get_env COMPOSE_PROFILES)"
others="$(printf '%s' "$profiles" | tr ',' '\n' | grep -vx 'https' | grep . | paste -sd, - || true)"
if [ "$PROXY" = caddy ]; then
    set_env COMPOSE_PROFILES "${others:+$others,}https"
else
    set_env COMPOSE_PROFILES "$others"
fi
say "set API_BIND, WEB_BIND, TRUST_PROXY, DOMAIN=$DOMAIN and COMPOSE_PROFILES in $ENV_FILE"

# write_traefik_override puts the `web` service on Traefik's network with the
# labels that route DOMAIN to it. `default` stays in the list: it is how `web`
# reaches `api`, and dropping it turns every request into a 502.
write_traefik_override() {
    cat >"$OVERRIDE" <<'YAML'
# Written by `make vps-init PROXY=traefik` (#401). Local to this server:
# gitignored, merged into docker-compose.yml by every `docker compose` command,
# and never touched by a fetch or `make vps-update`. Edit it freely.
#
# Names come from .env, with these defaults: TRAEFIK_NETWORK=traefik,
# TRAEFIK_ENTRYPOINT=websecure, TRAEFIK_CERTRESOLVER=letsencrypt.
services:
  web:
    networks:
      - default    # how web reaches the api: keep it
      - traefik
    labels:
      - "traefik.enable=true"
      # Which network Traefik dials: web is on two.
      - "traefik.docker.network=${TRAEFIK_NETWORK:-traefik}"
      - "traefik.http.routers.eznit.rule=Host(`${DOMAIN:?set DOMAIN in .env (make vps-init DOMAIN=...)}`)"
      - "traefik.http.routers.eznit.entrypoints=${TRAEFIK_ENTRYPOINT:-websecure}"
      - "traefik.http.routers.eznit.tls=true"
      - "traefik.http.routers.eznit.tls.certresolver=${TRAEFIK_CERTRESOLVER:-letsencrypt}"
      # nginx in the web image listens on 80.
      - "traefik.http.services.eznit.loadbalancer.server.port=80"

networks:
  traefik:
    external: true
    name: ${TRAEFIK_NETWORK:-traefik}
YAML
    chmod 644 "$OVERRIDE"
}

case "$PROXY" in
    traefik)
        if [ -e "$OVERRIDE" ]; then
            say "kept the existing $OVERRIDE (delete it and run this again for a fresh one)"
            grep -q 'traefik' "$OVERRIDE" ||
                say "WARNING: $OVERRIDE does not mention traefik — add the labels yourself, see the wiki"
        else
            write_traefik_override
            say "wrote $OVERRIDE: Traefik routes $DOMAIN to the web service"
        fi
        network="$(get_env TRAEFIK_NETWORK)"
        network="${network:-traefik}"
        docker network inspect "$network" >/dev/null 2>&1 ||
            say "WARNING: there is no Docker network \"$network\" — set TRAEFIK_NETWORK in $ENV_FILE to the one your Traefik uses (docker network ls)"
        ;;
    caddy)
        [ ! -e "$OVERRIDE" ] || ! grep -q 'traefik' "$OVERRIDE" ||
            say "WARNING: $OVERRIDE still routes through Traefik — delete it, or Traefik and Caddy both serve $DOMAIN"
        ;;
    none)
        say "the built-in HTTPS is off: point your reverse proxy at 127.0.0.1:${WEB_PORT:-5173}"
        ;;
esac

cat <<EOF

Ready. Before the first start:
  - DOMAIN's DNS (an A record) must point at this server's address;
$(case "$PROXY" in
    caddy) echo "  - ports 80 and 443 must be open and free (no other web server running)." ;;
    traefik) echo "  - your Traefik must be running, with the entrypoint and resolver named in $OVERRIDE." ;;
    none) echo "  - your reverse proxy must send https://$DOMAIN to 127.0.0.1:${WEB_PORT:-5173}." ;;
esac)

Start it:          docker compose up -d --build
Then open:         https://$DOMAIN   and sign up first — the first account is the admin.
Stop it:           docker compose down        (keeps the data; never add -v)
Update it later:   make vps-update
EOF
