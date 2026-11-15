#!/bin/bash
# Refuse a dev sandbox whose Go toolchain is older than go.mod requires.
#
# `make dev-up` deliberately does not pass `--build` (#103): Compose starts the
# image that exists, whatever the Dockerfile now says. After a Go pin bump
# (#256, for GO-2026-6859) that image's toolchain can be older than the `go`
# directive in go.mod — and the API then dies *inside the log stream* with
#
#   go: go.mod requires go >= 1.27.4 (running go 1.27.3; GOTOOLCHAIN=local)
#
# which names neither the cause nor the repair (#322). This script is the gate
# that fails first and says `make dev-build`.
#
# Two modes, because the version is known in two different places:
#   --image  (host, the default) ask the image: `docker compose run … go version`
#   --local  (inside the sandbox) ask this container: `go version`
#
# A missing image passes: Compose builds it on the spot, and a freshly built
# image is current by construction. A newer toolchain also passes — only an
# older one is a failure, because GOTOOLCHAIN=local in the official Go image.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:---image}"

case "$MODE" in
    --image|--local) ;;
    *)
        printf 'usage: %s [--image|--local]\n' "$(basename "$0")" >&2
        exit 2
        ;;
esac

fail() {
    printf 'ERROR: %s\n' "$1" >&2
    shift
    for line in "$@"; do
        printf '  %s\n' "$line" >&2
    done
    exit 1
}

# The `go` directive, which is what the compiler enforces: `go 1.27.4`.
# Since Go 1.21 the patch level is significant when GOTOOLCHAIN=local.
if [[ ! -f "$ROOT/go.mod" ]]; then
    fail "no go.mod at $ROOT/go.mod — cannot tell which toolchain this checkout needs."
fi
required="$(awk '$1 == "go" { print $2; exit }' "$ROOT/go.mod")"
if [[ -z "$required" ]]; then
    fail "go.mod has no 'go' directive — cannot tell which toolchain this checkout needs." \
        "Expected a line such as: go 1.27.4"
fi

# `go version` prints "go version go1.27.4 linux/amd64". Take the numeric core
# only: a toolchain can carry a suffix (`go1.26.10-X:nodwarf5`, `go1.28rc1`),
# and a comparison against "27.4rc1" is not arithmetic.
toolchain_of() {
    local raw="$1"
    printf '%s\n' "$raw" | sed -nE 's/.*go([0-9]+(\.[0-9]+)*).*/\1/p' | head -n 1
}

if [[ "$MODE" == "--local" ]]; then
    if ! command -v go >/dev/null 2>&1; then
        fail "no go on PATH inside the sandbox — the dev image is broken; run make dev-build on the host."
    fi
    # GOTOOLCHAIN=local forces the image's own toolchain. Without it, a
    # GOTOOLCHAIN=auto go command downloads the version go.mod asks for and
    # reports *that* — masking the stale image this gate exists to catch (and
    # turning the sandbox into one that only works with network access). The
    # official Go images set local already; this makes the comparison mean the
    # same thing regardless.
    have="$(toolchain_of "$(GOTOOLCHAIN=local go version)")"
else
    if ! command -v docker >/dev/null 2>&1; then
        fail "docker is not on PATH, so the dev image's toolchain cannot be checked." \
            "Start Docker (or use: make dev-build to rebuild the image anyway)."
    fi
    # One throwaway container, no dependencies, ~0.4s. Compose prints its
    # "Creating/Created" chatter on stderr; keep that out of a healthy run and
    # show it only when the read failed, where it is the explanation.
    errfile="$(mktemp)"
    trap 'rm -f "$errfile"' EXIT
    if ! out="$(cd "$ROOT" && docker compose --profile dev -f docker-compose.yml \
        run --rm --no-deps -T --entrypoint go dev version 2>"$errfile")"; then
        cat "$errfile" >&2
        fail "could not read the dev image's Go version." \
            "Fix the error above, or rebuild the image: make dev-build"
    fi
    have="$(toolchain_of "$out")"
fi

if [[ -z "$have" ]]; then
    fail "could not parse the Go toolchain version from the sandbox." \
        "Rebuild the image: make dev-build"
fi

# Numeric, component by component: 1.27.10 is newer than 1.27.9, and 1.27 is
# 1.27.0. `sort -V` is GNU-only (macOS BSD sort lacks it), so compare in awk.
version_lt() {
    awk -v a="$1" -v b="$2" 'BEGIN {
        na = split(a, A, "."); nb = split(b, B, ".");
        n = (na > nb) ? na : nb;
        for (i = 1; i <= n; i++) {
            x = A[i] + 0; y = B[i] + 0;
            if (x < y) exit 0;
            if (x > y) exit 1;
        }
        exit 1;
    }'
}

if version_lt "$have" "$required"; then
    if [[ "$MODE" == "--local" ]]; then
        what="this sandbox has Go $have"
        fix="Repair (on the host, not in the sandbox): make dev-build"
    else
        what="the dev sandbox image has Go $have"
        fix="Repair: make dev-build"
    fi
    fail "$what, but go.mod requires >= $required." \
        "" \
        "The image predates the pin bump in go.mod. dev-up does not rebuild" \
        "images on purpose, so it would start this one and the API would die in" \
        "the log stream with:" \
        "  go: go.mod requires go >= $required (running go $have; GOTOOLCHAIN=local)" \
        "" \
        "$fix" \
        "Native Windows:                        python scripts/dev-up.py dev-build"
fi

printf '[dev] Go %s satisfies go.mod (>= %s).\n' "$have" "$required"
