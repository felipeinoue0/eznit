# The local environment files are optional. The leading `-` makes a missing
# file a non-error, so a fresh clone can run `make check`, `make clean` and
# `make build` without a secret file. The targets that genuinely need the
# variables check for themselves (see `require_db_vars` below).
#
# Not inside the sandbox. There the configuration is the container's: compose
# sets DB_HOST/DB_PORT/DB_USER/DB_NAME explicitly (the in-network address) and
# passes the rest through `env_file`, which strips the quotes `.env` is written
# with. Importing the file here *again* overrides all of that with the host
# values — and make keeps the quotes, so `DB_HOST="localhost"` reaches the API's
# DSN as a literal `"localhost"` and the sandbox cannot reach the database.
# DEV_SANDBOX is set by the `dev` service in docker-compose.yml.
ifeq ($(DEV_SANDBOX),1)
else
-include .env .env.local
endif
# `-include` tolerates a missing file, but the catch-all at the bottom of this
# file would try to *build* `.env` and print a "no rule" error on a fresh
# clone. A recipe (even an empty one) stops the pattern rule from being used;
# `true` says there is nothing to make, which is what `-include` means.
.env .env.local:
	@:
export

APP_NAME := eznit
APP_NAME_SHORT := ez
SCRIPT_NAME := add_to_path
INJECT_VERSION:= main.Version

# Host ports. WEB_PORT is shared by both entry points: the product's `web`
# service and the loop's `web-dev` publish the same one. The API port has TWO
# names, because the two API processes are independent services: the sandbox
# `dev` publishes `${HOST_PORT}` and the product's `api` publishes
# `${API_PORT}` (see docker-compose.yml). Both default to 4000.
#
# `make up` (the product) and `make dev-up` (the developer loop) therefore want
# the same pair, whichever names they use, and `preflight-ports` probes exactly
# what the entry point about to start will bind — that is what turns "the other
# one is already running", or a listener belonging to neither, into a message
# instead of a Docker bind error.
HOST_PORT ?= 4000
API_PORT ?= 4000
WEB_PORT ?= 5173

# `.env` is shell syntax, not make syntax, and make does not strip the quotes it
# is written with — `API_PORT="4000"` (see .env.example) arrives here as the
# literal `"4000"`. That is harmless inside a recipe, because the shell removes
# the quotes when it expands the variable, but it is fatal in the *environment*:
# Compose only unquotes when it parses `.env` itself, so an exported `"4000"`
# reaches it as a five-character port and every command fails with
#
#     invalid hostPort: "4000"
#
# including `down` and `ps`. Normalise to what the file meant. Single quotes are
# stripped too, since Compose strips them.
HOST_PORT := $(patsubst "%",%,$(patsubst '%',%,$(HOST_PORT)))
API_PORT := $(patsubst "%",%,$(patsubst '%',%,$(API_PORT)))
WEB_PORT := $(patsubst "%",%,$(patsubst '%',%,$(WEB_PORT)))

CLI_FOLDER := ./cmd/cli
GUI_FOLDER := ./cmd/gui
WEB_FOLDER := ./web

DOCKER_COMPOSE := ./docker-compose.yml
MIGRATIONS := ./db/migrations
SCRIPT_MIGRATION := ./scripts/run_migrate.sh
SECRETS_EXAMPLE := ./secrets/db_password.txt.example
ENV_EXAMPLE := ./.env.example

# Detect last tag and increment patch
VERSION := $(shell git describe --tags --always)

# The desktop app is built for the host, with cgo: it is not a cross-compile.
HOST_GOOS := $(shell go env GOOS)
HOST_GOARCH := $(shell go env GOARCH)

# The Fyne window is a GUI-subsystem program on Windows; without -H windowsgui
# a double-click opens a console behind it.
GUI_LDFLAGS := -X main.Version=$(VERSION)
ifeq ($(HOST_GOOS),windows)
GUI_LDFLAGS += -H windowsgui
endif
GUI_EXT :=
ifeq ($(HOST_GOOS),windows)
GUI_EXT := .exe
endif

# Scripts
SCRIPTS_DIR := scripts
WINDOWS_SCRIPT :=  $(SCRIPTS_DIR)/$(SCRIPT_NAME).ps1
LINUX_SCRIPT :=  $(SCRIPTS_DIR)/$(SCRIPT_NAME).sh

# Builds
BUILD_DIR := ./build
BUILD_CLI := $(BUILD_DIR)/cli
BUILD_GUI := $(BUILD_DIR)/gui

# Release packaging: one folder per archive, named
# <name>_<version>_<os>_<arch>, compressed from its parent so extraction
# creates exactly that folder. One archive holds the whole product — client,
# server, migrations and the local stack — because two archives carried the same
# 33 MB window and a user who wanted both downloaded it twice (#242).
DIST_DIR := $(BUILD_DIR)/dist
STAGE_DIR := $(BUILD_DIR)/stage
FAMILY := $(APP_NAME)

# The platform lives in the archive name, never in the binary name: the README
# tells a user to run `eznit`, and a suffixed binary is unreachable by it.
PLATFORMS ?= windows/amd64 windows/arm64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

# --- Windows resources -------------------------------------------------------
# Go has no flag for an icon, a manifest or a version block; the only hook is a
# .syso beside the package. `winres` writes one from the single icon source and
# reads a built PE back, and `verify-release` reads the finished archives.
ICON ?= assets/logo.png
WINRES ?= internal/tools/winres
COMPANY ?= eznit
APP_ID ?= com.fiwon123.eznit
DESCRIPTION ?= Self-hosted project setup (API, CLI and web UI)

# What a release is checked against. The commit is empty locally, where
# `git describe` is not a release claim; the release workflow sets it to the
# tagged commit so a binary from another revision (or a dirty tree) fails.
PROVENANCE_COMMIT ?=
# What `make verify-release` requires locally, where `make release` has no window
# (the window is cgo, and a machine without GL cannot build it). The release
# workflow adds the window's licences to this list.
VERIFY_FILES ?= README.md,LICENSE,RUNNING.md,docker-compose.yml

## Builds
.PHONY: clean
.PHONY: build build_cli build_gui verify
.PHONY: release release-platform verify-release checksums
.PHONY: check test_integration

# The local developer build: the binaries the README tells you to run from
# ./build. `release` below is the packaging path — staging and compressing a
# folder is a different job from producing a binary, and mixing the two is what
# produced flat archives with no architecture in the name.
build: ## Build the local binaries into ./build (the GUI is `make build_gui`)
	@$(MAKE) --no-print-directory web_build
	@$(MAKE) --no-print-directory build_cli

# Create the build folders if missing
verify:
	@mkdir -p $(BUILD_CLI)/windows $(BUILD_CLI)/linux $(BUILD_GUI)

build_cli: verify
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(BUILD_CLI)/windows/$(APP_NAME).exe $(CLI_FOLDER)
	cp $(BUILD_CLI)/windows/$(APP_NAME).exe $(BUILD_CLI)/windows/$(APP_NAME_SHORT).exe

	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X $(INJECT_VERSION)=$(VERSION)" -o $(BUILD_CLI)/linux/$(APP_NAME) $(CLI_FOLDER)
	cp $(BUILD_CLI)/linux/$(APP_NAME) $(BUILD_CLI)/linux/$(APP_NAME_SHORT)

	chmod -R a+r $(BUILD_CLI)
	chmod a+x $(BUILD_CLI)/linux/$(APP_NAME)
	chmod a+x $(BUILD_CLI)/linux/$(APP_NAME_SHORT)

# The one binary is the client and the server, and it carries the built web UI
# (web/embed.go), so `build` builds the UI first. `make check` deliberately does
# not: the committed web/dist/.gitkeep keeps the embed directive valid without a
# build, which is what lets the Go gate run on a machine with no Node. A bare
# `go build ./cmd/cli` therefore embeds no UI, and `eznit serve` says so in its
# log instead of pretending.

# The desktop app needs cgo and the host's GL/X11 headers, so it is not part of
# `make build` or `make check` — a machine with neither must still pass the
# gate. CI builds it with -tags fyne; see .github/workflows/ci.gui.yml.
build_gui: verify ## Build the Fyne desktop app for this host (needs cgo + GL/X11 headers)
	@set -e; \
	if [ "$(HOST_GOOS)" = "windows" ]; then \
	  syso="cmd/gui/rsrc_windows_$(HOST_GOARCH).syso"; \
	  trap 'rm -f "$$syso"' EXIT; \
	  GOOS=$$(go env GOHOSTOS) GOARCH=$$(go env GOHOSTARCH) CGO_ENABLED=0 \
	    go run ./internal/tools/winres -icon assets/logo.png -out "$$syso" -arch $(HOST_GOARCH) \
	      -name "$(APP_NAME)-gui" -description "eznit desktop app" \
	      -company "$(COMPANY)" -app-id "$(APP_ID).gui" -version "$(VERSION)"; \
	fi; \
	CGO_ENABLED=1 go build -tags fyne -trimpath \
	  -ldflags "$(GUI_LDFLAGS)" -o $(BUILD_GUI)/eznit-gui$(GUI_EXT) ./cmd/gui; \
	chmod -R a+r $(BUILD_GUI); \
	chmod a+x $(BUILD_GUI)/eznit-gui$(GUI_EXT)

gui_check: ## vet and test the Fyne code (needs cgo + GL/X11 headers)
	@if ! command -v go >/dev/null 2>&1; then \
	  echo "ERROR: go is not installed — the GUI check cannot run" >&2; exit 1; \
	fi; \
	set -e; \
	CGO_ENABLED=1 go vet -tags fyne ./...; \
	CGO_ENABLED=1 go test -tags fyne ./...

# The window's suite is the one place a data race can hide: the test driver runs
# fyne.Do *inline on the calling goroutine*, so a worker's widget work can
# overlap the test that started it, and plain `go test` says nothing about it.
# Run the detector here rather than adding it to gui_check: it triples the time
# (10s -> 2m) for a check that only this package needs (#333).
fyne_race: ## run the window suite under the race detector (needs cgo)
	@if ! command -v go >/dev/null 2>&1; then \
	  echo "ERROR: go is not installed — the race check cannot run" >&2; exit 1; \
	fi; \
	CGO_ENABLED=1 go test -race -tags "fyne ci" -count=1 ./internal/fyneui/

# --- Release packaging -------------------------------------------------------
# One archive per platform, each extracting into one folder named after it. A
# flat archive scatters the binaries, the installer and the licenses into the
# directory the user ran the command in; `add_to_path` then adds that directory
# to PATH instead of the extracted tool.
#
# `make release` builds the web UI once (the one binary embeds it), then one
# platform at a time so the output reads in order, then the checksums.
release: ## Build one-folder archives for every platform into build/dist
	@rm -rf $(DIST_DIR) $(STAGE_DIR)
	@$(MAKE) --no-print-directory web_build
	@for platform in $(PLATFORMS); do \
	  $(MAKE) --no-print-directory release-platform \
	    GOOS=$${platform%/*} GOARCH=$${platform#*/} || exit 1; \
	done
	@$(MAKE) --no-print-directory verify-release
	@$(MAKE) --no-print-directory checksums
	@echo "Release $(VERSION) ready in $(DIST_DIR)"

release-platform:
	@set -eu; \
	if [ ! -f "$(WEB_FOLDER)/dist/index.html" ]; then \
	  echo "ERROR: $(WEB_FOLDER)/dist/index.html is missing -- the server embeds the UI;" >&2; \
	  echo "       run 'make web_build' first ('make release' does it for you)." >&2; exit 1; \
	fi; \
	folder="$(FAMILY)_$(VERSION)_$(GOOS)_$(GOARCH)"; \
	stage="$(STAGE_DIR)/$$folder"; \
	ext=""; [ "$(GOOS)" = "windows" ] && ext=".exe"; \
	rm -rf "$$stage"; mkdir -p "$$stage/db" "$$stage/scripts" "$$stage/secrets" "$(DIST_DIR)"; \
	echo "eznit  $(VERSION) $(GOOS)/$(GOARCH)"; \
	cli_syso=""; \
	if [ "$(GOOS)" = "windows" ]; then \
	  if [ ! -f "$(ICON)" ]; then \
	    echo "ERROR: $(ICON) is missing -- it is the one icon source; restore it (see assets/README.md)" >&2; exit 1; \
	  fi; \
	  if [ ! -f "$(WINRES)/main.go" ]; then \
	    echo "ERROR: $(WINRES) is missing -- copy templates/go/internal/tools/winres there" >&2; exit 1; \
	  fi; \
	  cli_syso="$(CLI_FOLDER)/rsrc_windows_$(GOARCH).syso"; \
	  trap 'rm -f "$$cli_syso"' EXIT; \
	  GOOS="$$(go env GOHOSTOS)" GOARCH="$$(go env GOHOSTARCH)" CGO_ENABLED=0 \
	    go run "./$(WINRES)" -icon "$(ICON)" -out "$$cli_syso" -arch "$(GOARCH)" \
	      -name "$(APP_NAME)" -description "$(DESCRIPTION)" -company "$(COMPANY)" \
	      -app-id "$(APP_ID)" -version "$(VERSION)"; \
	fi; \
	CGO_ENABLED=0 GOOS="$(GOOS)" GOARCH="$(GOARCH)" go build -trimpath \
	  -ldflags "-s -w -X $(INJECT_VERSION)=$(VERSION)" \
	  -o "$$stage/$(APP_NAME)$$ext" $(CLI_FOLDER); \
	cp "$$stage/$(APP_NAME)$$ext" "$$stage/$(APP_NAME_SHORT)$$ext"; \
	if [ -n "$$cli_syso" ]; then \
	  GOOS="$$(go env GOHOSTOS)" GOARCH="$$(go env GOHOSTARCH)" CGO_ENABLED=0 \
	    go run "./$(WINRES)" -require-resources -verify-console "$$stage/$(APP_NAME).exe"; \
	fi; \
	cp -R $(MIGRATIONS) "$$stage/db/"; \
	cp $(SCRIPT_MIGRATION) "$$stage/scripts/"; \
	cp $(DOCKER_COMPOSE) $(ENV_EXAMPLE) README.md LICENSE RUNNING.md "$$stage/"; \
	cp $(SECRETS_EXAMPLE) "$$stage/secrets/"; \
	if [ "$(GOOS)" = "windows" ]; then cp $(WINDOWS_SCRIPT) "$$stage/"; \
	else cp $(LINUX_SCRIPT) "$$stage/"; fi; \
	archive="$(abspath $(DIST_DIR))/$$folder"; \
	if [ "$(GOOS)" = "windows" ]; then \
	  archive="$$archive.zip"; rm -f "$$archive"; \
	  (cd "$(STAGE_DIR)" && zip -q -r "$$archive" "$$folder"); \
	  tops="$$(zip -sf "$$archive" | sed -n 's/^  \([^/]*\).*$$/\1/p' | sort -u)"; \
	else \
	  archive="$$archive.tar.gz"; rm -f "$$archive"; \
	  tar -czf "$$archive" -C "$(STAGE_DIR)" "$$folder"; \
	  tops="$$(tar -tzf "$$archive" | sed 's|^\./||' | cut -d/ -f1 | sort -u)"; \
	fi; \
	if [ "$$tops" != "$$folder" ]; then \
	  echo "ERROR: $$archive does not carry one folder $$folder (found: $$tops)" >&2; exit 1; \
	fi; \
	echo "       $$archive"

# Check the finished archives. The verifier reads the artifact that would be
# uploaded: the folder layout, the names inside it, the PE in each Windows
# archive, and — with PROVENANCE_COMMIT — the revision and cleanliness the
# binary claims.
verify-release: ## Verify build/dist: layout, names, PE and provenance
	@set -eu; \
	if [ ! -f "$(WINRES)/main.go" ]; then \
	  echo "ERROR: $(WINRES) is missing -- the release cannot be verified" >&2; exit 1; \
	fi; \
	rm -rf "$(BUILD_DIR)/verify"; mkdir -p "$(BUILD_DIR)/verify/all"; \
	for archive in "$(DIST_DIR)"/*.tar.gz "$(DIST_DIR)"/*.zip; do \
	  [ -e "$$archive" ] || continue; ln "$$archive" "$(BUILD_DIR)/verify/all/"; \
	done; \
	host="GOOS=$$(go env GOHOSTOS) GOARCH=$$(go env GOHOSTARCH) CGO_ENABLED=0"; \
	env $$host go run "./$(WINRES)" -verify-archives "$(BUILD_DIR)/verify/all" \
	  -app "$(APP_NAME)" -short "$(APP_NAME_SHORT)" -files "$(VERIFY_FILES)" \
	  $(if $(PROVENANCE_COMMIT),-commit "$(PROVENANCE_COMMIT)",)

# A download nobody can verify is only assumed intact. The hash list sits next
# to the archives, not inside one.
checksums: ## Write checksums.txt next to the built archives
	@set -eu; cd "$(DIST_DIR)"; \
	archives="$$(find . -maxdepth 1 -type f | sed -n 's|^\./||p' | grep -E '\.(tar\.gz|zip)$$' | sort)"; \
	if [ -z "$$archives" ]; then echo "ERROR: no archives in $(DIST_DIR) to hash" >&2; exit 1; fi; \
	if command -v sha256sum >/dev/null 2>&1; then sha256sum $$archives > checksums.txt; \
	elif command -v shasum >/dev/null 2>&1; then shasum -a 256 $$archives > checksums.txt; \
	else echo "ERROR: neither sha256sum nor shasum is available -- cannot verify a download" >&2; exit 1; fi; \
	cat checksums.txt

clean:
	rm -rf $(BUILD_DIR)

## Migrations
CONN := "postgresql://$(DB_USER):$(DB_PWD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable"
FOLDER := ./db/migrations

# The connection string carries the database password, and `make` echoes a
# recipe before running it. Every migration recipe below is therefore silenced
# with `@`, so the password never reaches the terminal or a CI log. The
# `migrate` tool still prints what it does.
#
# The environment files are optional (see the `-include` at the top of this
# file), so the targets that need the connection check for it themselves
# instead of failing with an empty DSN.
define require_db_vars
	@if [ -z "$(DB_USER)" ] || [ -z "$(DB_HOST)" ] || [ -z "$(DB_NAME)" ]; then \
	    echo "ERROR: DB_USER, DB_HOST and DB_NAME are not set." >&2; \
	    echo "       Copy .env.example to .env and fill it in, then try again." >&2; \
	    exit 1; \
	fi
endef

.PHONY: migrate_up migrate_down migrate_create migrate_force

migrate_create:
	@if [ -z "$(VAL)" ]; then\
	    echo "VAL is required. Usage: make create VAL=NEW_NAME"; exit 1;\
	fi
	@echo "Running migrate create $(VAL)"
	migrate create -ext sql -dir $(FOLDER) -seq $(VAL)

migrate_up:
	$(require_db_vars)
	@if [ -z "$(VAL)" ]; then \
	    echo "Running just migrate up"; \
	    @migrate -path "$(FOLDER)" -database "$(CONN)" up; \
	else \
	    echo "Running migrate up $(VAL)"; \
	    @migrate -path "$(FOLDER)" -database "$(CONN)" up "$(VAL)"; \
	fi

migrate_down_all:
	$(require_db_vars)
	@echo "Running just migrate down"
	@migrate -path "$(FOLDER)" -database "$(CONN)" down

migrate_down: VAL=1
migrate_down:
	$(require_db_vars)
	@echo "Running migrate down $(VAL)"
	@migrate -path "$(FOLDER)" -database "$(CONN)" down "$(VAL)"

migrate_force:
	$(require_db_vars)
	@if [ -z "$(VAL)" ]; then\
	    echo "VAL is required. Usage: make force VAL=YOUR_STEP"; exit 1;\
	fi
	@echo "Running migrate force $(VAL)"
	@migrate -path $(FOLDER) -database "$(CONN)" force $(VAL)

## Docker
unexport

.PHONY: docker_up docker_down docker_up_all docker_down_v up preflight-ports

# Which entry point is starting. Set by the target that depends on
# `preflight-ports`, and only read there.
MODE ?= dev

# `make up` and `make dev-up` publish the same host ports, so they are
# alternatives. This guard runs before either one starts anything, and names the
# command that clears the way — instead of starting half a stack and failing on
# `bind: address already in use`.
#
# A port held by *this* entry point's own containers is not a collision:
# `make dev-up` twice in a row must work.
preflight-ports:
	@services="$$(docker compose --profile dev ps --services --status running 2>/dev/null || true)"; \
	running() { printf '%s\n' "$$services" | grep -qx "$$1"; }; \
	probe() { bash -c "exec 3<>/dev/tcp/127.0.0.1/$$1" 2>/dev/null; }; \
	die() { echo "ERROR: $$1" >&2; shift; for line in "$$@"; do echo "       $$line" >&2; done; exit 1; }; \
	if [ "$(MODE)" = "dev" ]; then \
	    if running api || running web; then \
	        die "the product stack is running (api/web) and already holds the host ports." \
	            "make up and make dev-up are alternatives — stop the product first:" \
	            "    make docker_down"; \
	    fi; \
	    if ! running dev && probe "$(HOST_PORT)"; then \
	        die "host port $(HOST_PORT) is already in use by something that is not this stack." \
	            "Free it, or start the dev loop elsewhere: HOST_PORT=<port> make dev-up"; \
	    fi; \
	    if ! running web-dev && probe "$(WEB_PORT)"; then \
	        die "host port $(WEB_PORT) is already in use by something that is not this stack." \
	            "Free it, or start the dev loop elsewhere: WEB_PORT=<port> make dev-up"; \
	    fi; \
	else \
	    if running dev || running web-dev; then \
	        die "the dev loop is running (dev/web-dev) and already holds the host ports." \
	            "make up and make dev-up are alternatives — stop the dev loop first:" \
	            "    make dev-down"; \
	    fi; \
	    if ! running api && probe "$(API_PORT)"; then \
	        die "host port $(API_PORT) is already in use by something that is not this stack." \
	            "Free it, or run the product elsewhere: API_PORT=<port> make up"; \
	    fi; \
	    if ! running web && probe "$(WEB_PORT)"; then \
	        die "host port $(WEB_PORT) is already in use by something that is not this stack." \
	            "Free it, or run the product elsewhere: WEB_PORT=<port> make up"; \
	    fi; \
	fi

up: MODE = product
up: preflight-ports ## Build and run the product: API :4000, web UI :5173, db, migrations
	docker compose up --build db migrate api web

docker_up:
	docker compose up db migrate

docker_up_all:
	docker compose --profile dev up --build dev db migrate

docker_down:
	docker compose down

docker_down_v:
	docker compose down -v

## Development
.PHONY: run_api run_cli run_jq

run_api:
	go run $(CLI_FOLDER) serve $(filter-out $@,$(MAKECMDGOALS))

run_cli:
	go run $(CLI_FOLDER) $(filter-out $@,$(MAKECMDGOALS))

run_jq:
	@if [ -z "$(VAL)" ]; then \
	    echo "need to specify filepath using VAL"; \
	    exit 1; \
	fi

	cat $(VAL) | jq -C . | sed 's/\\n/\n/g; s/\\t/\t/g'

# ---------------------------------------------------------------------------
# Local verification gate
#
# gofmt, vet, test, build — in that order, because a formatting failure hides
# everything after it.
#
# It needs no database, no secret file and no container: the API and the CLI
# are compiled, not run. The `-include` at the top of this file is what makes
# that true on a fresh clone.
#
# Two properties this target deliberately has:
#
#   - A missing toolchain FAILS the target. It does not skip. A gate that
#     cannot fail is worse than no gate: it reads green and proves nothing.
#   - The first failing step STOPS the target. The recipe is one shell, so
#     `set -e` is what turns a failed `go vet`, `go test` or `go build` into a
#     non-zero exit; chaining the steps with `;` alone lets the shell return
#     the status of the *last* command, and the notested report at the end
#     always succeeds. That is #61.
#   - After the run it names the packages that have no test files. `go test`
#     prints "[no test files]" for those and still exits 0, so a green run
#     would otherwise look like coverage that does not exist.
#
# The suite is partial: packages without a test file show up in that list.
# Read a green run as "what has tests passed, the rest compiled", not as
# coverage of the whole repository.
# ---------------------------------------------------------------------------
check: ## gofmt, vet, test, build — the local verification gate
	@if ! command -v go >/dev/null 2>&1; then \
		echo "ERROR: go is not installed — the check cannot run, so nothing is verified" >&2; \
		exit 1; \
	fi; \
	set -e; \
	out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi; \
	go vet ./...; \
	go test ./...; \
	go build ./...; \
	notested="$$(go list -f '{{if and (not .TestGoFiles) (not .XTestGoFiles)}}{{.ImportPath}}{{end}}' ./... | grep -v '^$$')"; \
	if [ -n "$$notested" ]; then \
		echo ""; \
		echo "NOTE: these packages have no test files — this run does not cover them:"; \
		echo "$$notested" | sed 's/^/  /'; \
	fi

# ---------------------------------------------------------------------------
# Live-PostgreSQL integration suite
#
# The tests behind the `integration` build tag that run the real migrations and
# the repository SQL against a real database. `make check` stays database-free
# on purpose: this target exists so the SQL is verified somewhere rather than
# only compiled.
#
# TEST_DATABASE_URL must point at a SCRATCH database whose name ends in `_test`
# (CI uses `eznit_test`). The suite drops and truncates tables; a name that is
# not clearly a test database aborts the run before the first destructive
# statement, so a copied product connection string fails instead of wiping.
#
# A missing variable FAILS the target rather than skipping: a gate that cannot
# fail is worse than no gate. CI runs the same commands as a step of its own.
# ---------------------------------------------------------------------------
test_integration: ## run the migrations + repository suite against a real PostgreSQL (needs TEST_DATABASE_URL=..._test)
	@if [ -z "$${TEST_DATABASE_URL}" ]; then \
		echo "ERROR: TEST_DATABASE_URL is not set — the suite needs a scratch database whose name ends in _test" >&2; \
		echo "       example: TEST_DATABASE_URL='postgres://user:pass@localhost:5432/eznit_test?sslmode=disable' make test_integration" >&2; \
		exit 1; \
	fi; \
	set -e; \
	go vet -tags=integration ./...; \
	go test -tags=integration -count=1 -p 1 ./...

# ---------------------------------------------------------------------------
# Dependency vulnerability scan
#
# Deliberately NOT part of `make check`: the scan downloads the tool and the
# vulnerability database, and the verification gate has to work offline on a
# fresh clone. CI runs the same command as a step of its own.
#
# Symbol-level (the default) fails only when this code actually reaches a
# vulnerable function. `-scan=module` cannot go green here: GO-2026-5932 on
# x/crypto's openpgp package is "Fixed in: N/A" and the package is not
# imported, while x/crypto itself cannot be dropped (it provides `bcrypt`).
# See the comment in .github/workflows/ci.go.yml.
#
# A missing toolchain FAILS the target, like every other gate in this file.
# ---------------------------------------------------------------------------
.PHONY: vuln
vuln: ## scan for reachable dependency vulnerabilities (needs network)
	@if ! command -v go >/dev/null 2>&1; then \
		echo "ERROR: go is not installed — the scan cannot run, so nothing is verified" >&2; \
		exit 1; \
	fi; \
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

# ---------------------------------------------------------------------------
# The web UI (web/)
#
# A second toolchain, so a second gate. `make check` stays Go-only on purpose:
# the Go gate must not start failing because an npm package moved.
#
# The same rules as the Go gate:
#
#   - A missing toolchain FAILS the target. It does not skip. A gate that
#     cannot fail is worse than no gate: it reads green and proves nothing.
#   - Every step can fail on its own. `oxlint` exits non-zero on a lint error
#     and `oxfmt --check` exits non-zero on a misformatted file, so the gate
#     fails when the code is wrong and not only when it is missing.
#
# What this gate does NOT prove: the web has no test suite, so nothing here
# checks what the pages do in a browser. It proves that the code lints,
# formats, typechecks and builds.
# ---------------------------------------------------------------------------
.PHONY: web_install web_check web_build web_dev
.PHONY: screenshots

web_install: ## Install the web dependencies from the lockfile (npm ci)
	@if ! command -v npm >/dev/null 2>&1; then \
		echo "ERROR: npm is not installed — mise install puts the pinned Node on PATH (see mise.toml)" >&2; \
		exit 1; \
	fi; \
	(cd $(WEB_FOLDER) && npm ci)

web_check: ## lint, format check, typecheck and build the web UI — the web gate
	@if ! command -v node >/dev/null 2>&1; then \
		echo "ERROR: node is not installed — the check cannot run, so nothing is verified" >&2; \
		echo "       mise install puts the pinned version on PATH (see mise.toml)" >&2; \
		exit 1; \
	fi; \
	if [ ! -d "$(WEB_FOLDER)/node_modules" ]; then \
		echo "[web] node_modules is missing — installing the pinned dependencies..."; \
		(cd $(WEB_FOLDER) && npm ci); \
	fi; \
	cd $(WEB_FOLDER) && \
		npm run lint && \
		npm run format:check && \
		npm run typecheck && \
		npm run build && \
		touch dist/.gitkeep

web_build: ## Build the web UI into web/dist
	@if ! command -v node >/dev/null 2>&1; then \
		echo "ERROR: node is not installed — mise install puts the pinned Node on PATH (see mise.toml)" >&2; \
		exit 1; \
	fi; \
	if [ ! -d "$(WEB_FOLDER)/node_modules" ]; then \
		echo "[web] node_modules is missing — installing the pinned dependencies..."; \
		(cd $(WEB_FOLDER) && npm ci); \
	fi; \
	cd $(WEB_FOLDER) && npm run build && touch dist/.gitkeep

web_dev: ## Run the web UI dev server on http://localhost:5173
	@if ! command -v node >/dev/null 2>&1; then \
		echo "ERROR: node is not installed — mise install puts the pinned Node on PATH (see mise.toml)" >&2; \
		exit 1; \
	fi; \
	cd $(WEB_FOLDER) && npm run dev

# --- README screenshots ------------------------------------------------------
# The committed tour a reader sees before running anything. The harness refuses
# to write a blank or error-state capture, so a green run means the pages
# rendered. Not part of `make check`: it needs the app running and a browser.
SCREENSHOTS_URL ?= http://localhost:5173

screenshots: ## Capture README screenshots into docs/screenshots (needs the app running)
	@command -v node >/dev/null 2>&1 || { echo "ERROR: node is not installed — mise install puts the pinned Node on PATH (see mise.toml)" >&2; exit 1; }
	@[ -d "$(WEB_FOLDER)/node_modules" ] || { echo "ERROR: $(WEB_FOLDER)/node_modules is missing — run 'make web_install'" >&2; exit 1; }
	@curl -fsS -o /dev/null "$(SCREENSHOTS_URL)" || { echo "ERROR: $(SCREENSHOTS_URL) is not answering — start the app with 'make up' (or 'make dev-up')" >&2; exit 1; }
	@cd $(WEB_FOLDER) && BASE_URL="$(SCREENSHOTS_URL)" npm run screenshots

# --- Wiki --------------------------------------------------------------------
# The wiki's source is docs/wiki/. GitHub keeps a wiki in a separate git
# repository (<repo>.wiki.git), so this copies the pages there and pushes them.
# Edit pages here, through a pull request: a change made only in the wiki UI is
# not in docs/wiki/ and the next sync overwrites it.
WIKI_REPO ?= https://github.com/fiwon123/eznit.wiki.git
WIKI_BRANCH ?= master
WIKI_DIR := build/wiki

wiki_push: ## Publish docs/wiki/*.md to the GitHub wiki
	@command -v git >/dev/null 2>&1 || { echo "ERROR: git is required" >&2; exit 1; }
	@if ! git ls-remote "$(WIKI_REPO)" >/dev/null 2>&1; then \
		echo "ERROR: the wiki repository does not exist yet." >&2; \
		echo "       GitHub creates <repo>.wiki.git only after the first page is" >&2; \
		echo "       saved in the web UI — there is no API for it. Once only:" >&2; \
		echo "         open https://github.com/fiwon123/eznit/wiki" >&2; \
		echo "         click 'Create the first page', title it 'Home', save" >&2; \
		echo "       then rerun 'make wiki_push'." >&2; \
		exit 1; \
	fi
	@rm -rf "$(WIKI_DIR)"
	@git clone --quiet "$(WIKI_REPO)" "$(WIKI_DIR)"
	@cp docs/wiki/*.md "$(WIKI_DIR)"/
	@git -C "$(WIKI_DIR)" add -A
	@if git -C "$(WIKI_DIR)" commit -q -m "docs: sync wiki from docs/wiki"; then \
		echo "[wiki] committed"; \
	else \
		echo "[wiki] no changes to publish"; \
	fi
	@if command -v gh >/dev/null 2>&1; then \
		git -C "$(WIKI_DIR)" -c credential.helper='!gh auth git-credential' push origin HEAD:$(WIKI_BRANCH); \
	else \
		git -C "$(WIKI_DIR)" push origin HEAD:$(WIKI_BRANCH); \
	fi
	@echo "[wiki] published to $(WIKI_REPO) ($(WIKI_BRANCH))"

# ---------------------------------------------------------------------------
# Dev sandbox (multiplatform)
#
# A pinned toolchain and a shell that behave the same on every machine. The
# sandbox is optional: the project still builds and tests with a plain host Go
# toolchain (see the `check` target above).
#
# Cross-platform note: this Makefile targets POSIX hosts — Linux, macOS and
# WSL2 — where `make` runs recipes under a POSIX shell. Native Windows users
# should use `python scripts/dev-up.py dev-up` (or `scripts/dev-up.ps1`), which
# provides the same targets without a POSIX shell. Everyone can run the product
# with a plain `docker compose up`, which needs no Make at all, and the sandbox
# with `docker compose up dev db migrate`.
# ---------------------------------------------------------------------------

# --- Host identity -----------------------------------------------------------
# Run bind-mounted dev services as the host developer instead of root. Derived
# from the workspace OWNER, not the current process, so invoking make inside an
# already-running bind-mounted container still sends the host identity.
# GNU stat (-c) first, then BSD/macOS stat (-f), then plain `id`. The `id`
# fallback is what makes this work everywhere `make` does.
HOST_UID ?= $(shell stat -c '%u' . 2>/dev/null || stat -f '%u' . 2>/dev/null || id -u)
HOST_GID ?= $(shell stat -c '%g' . 2>/dev/null || stat -f '%g' . 2>/dev/null || id -g)
HOST_HOME ?= $(HOME)
# Exported by name. A bare `unexport` cancels the *default* export of the
# variables defined after it, but it cannot cancel an export that names the
# variable — so naming them here is what keeps the host identity reaching
# compose, whatever the surrounding sections do with export attributes.
# Without a named export the sandbox would start as UID/GID 1000 on a Linux
# host whose uid differs, and the bind mount would end up root-owned.
export HOST_UID HOST_GID HOST_HOME HOST_PORT API_PORT WEB_PORT

# --- In-container AI agents --------------------------------------------------
# opencode, Claude Code and Codex are installed in the dev image (pinned in
# Dockerfile.template), so they run on every host OS and nothing here depends on
# a host install. Each one asks for its own login on first run — an API key you
# paste, or an account you sign in with — and keeps it in a named volume
# (docker-compose.yml), so you sign in once per project, not once per start.
#
# Every agent has a plain target and an `-auto` target. The command lines are
# variables so a project can change one without touching the recipe:
#
#   opencode --auto            approves what is not explicitly denied; the
#                              secret-file deny rules in opencode.json still apply
#   claude --dangerously-skip-permissions
#                              no prompts — the container is the boundary, as for
#                              codex-auto. Claude's Read/Edit deny rules for `.env`
#                              in .claude/settings.json still apply (verified). It
#                              asks once to accept bypass mode; the answer is kept
#                              in its volume. For a reviewed middle ground:
#                              CLAUDE_AUTO_CMD="claude --permission-mode auto"
#   codex --sandbox danger-full-access
#                              Codex's own sandbox (bubblewrap) cannot create a
#                              namespace inside an unprivileged container, so the
#                              CONTAINER is the sandbox and Codex still asks first
#   codex --dangerously-bypass-approvals-and-sandbox
#                              no prompts at all — the flag Codex documents for an
#                              externally sandboxed environment, which this is.
#                              Codex has no file deny list: in this mode nothing
#                              stops it reading `.env`. Use it where that is fine.
#
# Extra flags for one run: `make claude AGENT_ARGS="--model sonnet"`.
OPENCODE_CMD      ?= opencode
OPENCODE_AUTO_CMD ?= opencode --auto
CLAUDE_CMD        ?= claude
CLAUDE_AUTO_CMD   ?= claude --dangerously-skip-permissions
CODEX_CMD         ?= codex --sandbox danger-full-access
CODEX_AUTO_CMD    ?= codex --dangerously-bypass-approvals-and-sandbox
AGENT_ARGS        ?=

# The overlay carries only the host's gh config and ~/.gitconfig, and
# `prepare-agent-mounts` creates both sources first, so it is always merged.
COMPOSE_UP := docker compose --profile dev -f docker-compose.yml -f docker-compose.agent.yml

# Python runs the agent-pin helper (`agents-upgrade`). Starting the sandbox
# never needs it. Resolved with fallbacks: a Windows Python often provides only
# `python` or the `py` launcher.
PYTHON ?= $(shell command -v python3 2>/dev/null || command -v python 2>/dev/null || command -v py 2>/dev/null)

# --- Is the dev image current? -----------------------------------------------
# `dev-up` does not rebuild, so an edited Dockerfile keeps running the old image
# until someone remembers `make dev-build`. The image therefore carries a label
# with a hash of the files it was built from, and the entry points refuse an
# image whose label does not match the files on disk.
#
# IMAGE_INPUTS lists the Dockerfile and EVERY file it COPYs. Add a file here
# when you add a COPY — `make check` compares the two. The hash is the first 16
# hex digits of SHA-256 over the files in this order; scripts/dev-up.py computes
# the same value, so both entry points agree on what "current" means.
IMAGE_INPUTS ?= Dockerfile scripts/dev-sandbox-rc.sh
IMAGE_INPUTS_HASH = $(shell cat $(IMAGE_INPUTS) 2>/dev/null | { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-16)
# Compose names a built image `<project>-<service>`; the project is `name:`.
DEV_IMAGE ?= $(shell sed -n 's/^name:[[:space:]]*//p' docker-compose.yml | head -n 1)-dev


# The marker file is the dev entrypoint's opt-in (scripts/dev-entrypoint.sh):
# with it present the sandbox runs `make run_api` instead of sleeping. It is
# gitignored per-checkout state, created by `dev-up` and removed by `dev-down`.
DEV_START_MARKER := .dev/sandbox-start

help: ## List the available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(firstword $(MAKEFILE_LIST)) | sort | awk 'BEGIN{FS=":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
	@printf '\n  \033[36m%-30s\033[0m %s\n' "docker compose up" "run the product with no host prerequisites (any OS)"
	@printf '  \033[36m%-30s\033[0m %s\n' "docker compose up dev web-dev db migrate" "the dev loop without Make (needs .dev/sandbox-start)"
	@printf '  \033[36m%-30s\033[0m %s\n' "scripts/dev-up.py" "native Windows: python scripts/dev-up.py dev-up"
	@printf '\n  %s\n' "dev-up is the dev loop: UI :5173 (Vite, hot reload) and API :4000, both"
	@printf '  %s\n' "from the source. make up is the product: same ports, built images."
	@printf '  %s\n' "dev-up does not rebuild: run make dev-build after a Dockerfile or entrypoint"
	@printf '  %s\n' "change. dev-restart is dev-down followed by dev-up. A stale image (a Go pin"
	@printf '  %s\n' "bump in go.mod) is refused with that instruction instead of being started."

# --- One-time local setup ---------------------------------------------------
# Creates the two local files the stack needs, from the committed examples:
# the configuration (.env) and the database secret (secrets/db_password.txt).
# Both are gitignored, and both must be mode 600 (README.md -> Local secret
# files). Idempotent: an existing file is never overwritten.
setup: ## Create .env and the database secret from their examples (idempotent)
	@if [ -f "$(ENV_EXAMPLE)" ]; then \
	  if [ -f ".env" ]; then \
	    echo "[setup] .env already exists — leaving it alone."; \
	  else \
	    cp "$(ENV_EXAMPLE)" .env && chmod 600 .env && echo "[setup] created .env from $(ENV_EXAMPLE)"; \
	  fi; \
	fi
	@if [ -f "$(SECRETS_EXAMPLE)" ]; then \
	  if [ -f "secrets/db_password.txt" ]; then \
	    echo "[setup] secrets/db_password.txt already exists — leaving it alone."; \
	  else \
	    cp "$(SECRETS_EXAMPLE)" secrets/db_password.txt && chmod 600 secrets/db_password.txt && echo "[setup] created secrets/db_password.txt"; \
	  fi; \
	  echo "[setup] the secret is the example's placeholder password — replace it for anything but localhost."; \
	fi

# --- Preflight ---------------------------------------------------------------
# Refuses to start as root, and pre-creates the read-only mount sources the
# agent overlay needs. Missing mount sources are fatal on Windows and create
# root-owned directories on Linux, so they are created here rather than assumed.
preflight: check-identity prepare-agent-mounts

check-identity:
	@test "$(HOST_UID)" != "0" || { echo "ERROR: refusing to run the dev sandbox as UID 0; repair workspace ownership or set HOST_UID to the host user." >&2; exit 1; }

prepare-agent-mounts:
	@mkdir -p "$(HOST_HOME)/.config/gh" && touch "$(HOST_HOME)/.gitconfig"

# Refuses a dev image that was built from different files than the ones on
# disk (see IMAGE_INPUTS). A missing image is built — by `dev-build`, so it
# carries the label. A mismatch is an error, not a rebuild: `dev-up` never
# rebuilds behind your back, it tells you to.
check-image:
	@have="$$(docker image inspect --format '{{ index .Config.Labels "dev.image-inputs" }}' '$(DEV_IMAGE)' 2>/dev/null || true)"; \
	if [ -z "$$have" ]; then \
	  echo "[sandbox] no dev image yet — building it (this runs once)."; \
	  $(MAKE) --no-print-directory dev-build; \
	elif [ "$$have" != "$(IMAGE_INPUTS_HASH)" ]; then \
	  echo "ERROR: the dev image is out of date: it was not built from the current $(IMAGE_INPUTS)." >&2; \
	  echo "       (an edited Dockerfile, a bumped agent or toolchain pin, or an image built outside make dev-build)" >&2; \
	  echo "       Rebuild it: make dev-build   — agent logins are kept, they live in volumes." >&2; \
	  exit 1; \
	fi

# The value the image label is compared with. Printed for the launcher parity
# check and for debugging a mismatch.
image-inputs-hash:
	@echo "$(IMAGE_INPUTS_HASH)"

# What the RUNNING sandbox has: each agent's version and whether it is signed
# in, then (with Python) each pin next to the vendor's latest release.
agents-version: ## Show agent versions and logins in the sandbox, and the latest releases
	@test -n "$$($(COMPOSE_UP) ps -q dev 2>/dev/null)" || { echo "ERROR: the dev sandbox is not running — start it with 'make dev-up'." >&2; exit 1; }
	@$(COMPOSE_UP) exec -T dev bash /workspace/scripts/agents-status.sh
	@if [ -n "$(PYTHON)" ]; then echo; $(PYTHON) scripts/agents-pin.py show; fi

# Upgrading an agent = rewrite its pin in the Dockerfile to the vendor's latest
# release (or AGENT_VERSION=X.Y.Z for one agent), rebuild, and re-create a
# running sandbox on the new image. The pin change is an ordinary diff to
# commit, so the whole project moves together. Logins live in volumes and
# survive all of it.
AGENTS_TO_UPGRADE ?=
AGENT_VERSION ?=

agents-upgrade: preflight ## Upgrade every agent to its latest release (keeps logins)
	@test -n "$(PYTHON)" || { echo "ERROR: agents-upgrade needs python3 on the host to look up and rewrite the pins." >&2; exit 1; }
	@$(PYTHON) scripts/agents-pin.py bump $(AGENTS_TO_UPGRADE) $(if $(AGENT_VERSION),--version $(AGENT_VERSION))
	@$(MAKE) --no-print-directory dev-build
	@if [ -n "$$($(COMPOSE_UP) ps -q dev 2>/dev/null)" ]; then \
	  echo "[agents] re-creating the running sandbox on the new image (logins are kept)..."; \
	  token="$$(scripts/resolve-gh-token.sh 2>/dev/null)"; \
	  GH_TOKEN="$$token" GITHUB_TOKEN="$$token" $(COMPOSE_UP) up -d --force-recreate dev; \
	  $(MAKE) --no-print-directory agents-version; \
	fi
	@echo "[agents] commit the Dockerfile pin change so the whole project upgrades with you."

opencode-upgrade: AGENTS_TO_UPGRADE = opencode
opencode-upgrade: agents-upgrade ## Upgrade opencode in the sandbox (AGENT_VERSION=X.Y.Z to pin one)
claude-upgrade: AGENTS_TO_UPGRADE = claude
claude-upgrade: agents-upgrade ## Upgrade Claude Code in the sandbox (AGENT_VERSION=X.Y.Z to pin one)
codex-upgrade: AGENTS_TO_UPGRADE = codex
codex-upgrade: agents-upgrade ## Upgrade Codex in the sandbox (AGENT_VERSION=X.Y.Z to pin one)

# One look at everything the sandbox depends on. A failed item names its fix.
# Logins are checked by each agent's own status command (exit code only) — no
# credential is ever printed.
doctor: ## Check Docker, the dev image, the sandbox, the GitHub token and each agent's login
	@status=0; \
	ok()   { printf '  ok    %s\n' "$$1"; }; \
	bad()  { printf '  FAIL  %s\n' "$$1"; status=1; }; \
	note() { printf '  --    %s\n' "$$1"; }; \
	if docker info >/dev/null 2>&1; then ok "Docker is running"; else bad "Docker is not reachable — start Docker Desktop or the docker service"; fi; \
	have="$$(docker image inspect --format '{{ index .Config.Labels "dev.image-inputs" }}' '$(DEV_IMAGE)' 2>/dev/null || true)"; \
	if [ -z "$$have" ]; then note "no dev image yet — make dev-up builds it"; \
	elif [ "$$have" = "$(IMAGE_INPUTS_HASH)" ]; then ok "dev image is current"; \
	else bad "dev image is out of date — make dev-build"; fi; \
	if [ -n "$$(scripts/resolve-gh-token.sh 2>/dev/null)" ]; then ok "GitHub token available (gh works in the sandbox)"; \
	else note "no GitHub token — gh auth login on the host"; fi; \
	if [ -n "$$($(COMPOSE_UP) ps -q dev 2>/dev/null)" ]; then \
	  ok "sandbox is running"; \
	  $(COMPOSE_UP) exec -T dev bash /workspace/scripts/agents-status.sh || status=1; \
	else note "sandbox is not running — make dev-up (agent logins are checked while it runs)"; fi; \
	exit $$status

# --- Sandbox lifecycle -------------------------------------------------------
# HOST_UID/HOST_GID need no explicit export here: docker-compose.yml defaults
# them, and the exported values above supply the host's real identity when make
# is running on the host. Only the GitHub token is resolved at up-time.
.PHONY: dev-up dev-down dev-restart dev-log dev-build dev-reset check-dev-toolchain

# dev-up is the developer loop, all of it: the API runs inside the sandbox
# against the source, and the UI is the Vite server in `web-dev`, with hot
# reload. Both ports answer when this returns — which is the whole point of the
# target, and what it did not do before.
#
# Two steps, not one, because of the marker file. The entrypoint reads it when
# the container *starts*; a plain `up` leaves an already-running sandbox
# asleep, so `dev` is recreated to make the entrypoint run again.
#
# Both steps carry the GitHub token. Compose recreates a container whose
# configuration differs from the running one, and the token is part of `dev`'s
# environment — so a token on the first command and not on the second recreated
# `dev` a second time, restarting the API seconds after it had started. It is
# resolved once for the same reason: two substitutions can disagree.
#
# Neither step passes `--build`, and that is deliberate. Building on every start
# is what made `dev-up` look like `dev-build`: BuildKit resolves the base-image
# metadata and prints a full step list even on a cache hit, which cost ~9s per
# run on the machine this was measured on (#103). Compose still builds an image
# that does not exist yet when a started service needs it, so a cold checkout is
# unaffected. What stops being automatic is an edit to a Dockerfile or to a
# script COPYed into the image — `make dev-build` is that path, and it exists
# for exactly this reason.
#
# The consequence is that a Go pin bump leaves the old image on disk, and
# starting it fails inside the log stream with a message that names neither the
# cause nor the repair (#322). `check-dev-toolchain` is the gate that refuses
# first: it asks the image what toolchain it carries and compares that with
# go.mod. It is a prerequisite, not a warning, so `dev-up` exits non-zero before
# starting anything and names `make dev-build`.
check-dev-toolchain: ## Refuse a dev image whose Go toolchain is older than go.mod
	@scripts/check-dev-toolchain.sh --image

dev-up: MODE = dev
dev-up: preflight check-dev-toolchain preflight-ports check-image ## Start the dev loop: UI :5173 (Vite, live reload), API :4000
	@printf '\n  dev-up starts the developer loop in containers:\n'
	@printf '    %-14s %s\n' "UI (Vite)" "http://localhost:$(WEB_PORT)   hot reload"
	@printf '    %-14s %s\n' "API" "http://localhost:$(HOST_PORT)"
	@printf '  PostgreSQL and the migrations start first; the UI proxies /v1 to the API.\n'
	@printf '  Ctrl-C stops the log stream — the containers keep running. "make dev-down" stops them.\n\n'
	@mkdir -p .dev && touch $(DEV_START_MARKER)
	@set -e; \
	 token="$$(scripts/resolve-gh-token.sh 2>/dev/null || true)"; \
	 GH_TOKEN="$$token" GITHUB_TOKEN="$$token" HOST_PORT=$(HOST_PORT) WEB_PORT=$(WEB_PORT) $(COMPOSE_UP) up -d --force-recreate dev; \
	 GH_TOKEN="$$token" GITHUB_TOKEN="$$token" HOST_PORT=$(HOST_PORT) WEB_PORT=$(WEB_PORT) $(COMPOSE_UP) up -d web-dev
	@$(COMPOSE_UP) logs -f dev web-dev

dev-down: ## Stop the dev loop and the local stack (keeps volumes)
	@rm -f $(DEV_START_MARKER)
	@$(COMPOSE_UP) down

# The two halves are the real targets, run through $(MAKE) so this one cannot
# drift from them: the marker removal in `dev-down` and the port preflight,
# recreate and log tail in `dev-up` stay in a single place. Like `dev-up` it
# ends on the log stream, so Ctrl-C leaves the loop running.
dev-restart: ## Restart the dev loop: dev-down, then dev-up (does not rebuild)
	@$(MAKE) --no-print-directory dev-down
	@$(MAKE) --no-print-directory dev-up

dev-log: ## Tail dev sandbox logs
	@$(COMPOSE_UP) logs -f dev

# The only path that rebuilds an image. It covers `web-dev` as well as `dev`:
# since `dev-up` stopped passing `--build` (#103), this is what picks up an edit
# to `web/Dockerfile.dev` — and it used to skip that image entirely, so such an
# edit had no rebuild path at all.
dev-build: check-identity ## Rebuild the dev images (dev and web-dev) after Dockerfile/dependency changes
	@docker compose --profile dev build --build-arg DEV_IMAGE_INPUTS=$(IMAGE_INPUTS_HASH) dev web-dev

# Deliberately NOT called `reset`: `down --volumes` removes the named volumes,
# and one of them is the PostgreSQL data volume. This wipes the local database
# along with the build caches, so the name says so.
dev-reset: ## Stop everything and wipe the volumes — INCLUDING the local database
	@echo "This removes the local database volume as well as the build caches." >&2
	@rm -f $(DEV_START_MARKER)
	@$(COMPOSE_UP) down --volumes

shell: dev-shell
opencode: dev-opencode
opencode-auto: dev-opencode-auto
claude: dev-claude
claude-auto: dev-claude-auto
codex: dev-codex
codex-auto: dev-codex-auto

dev-shell: preflight check-image ## Open a shell inside the dev sandbox
	@$(COMPOSE_UP) exec dev bash || $(COMPOSE_UP) run --rm dev bash

.PHONY: dev-agent dev-opencode dev-opencode-auto dev-claude dev-claude-auto dev-codex dev-codex-auto \
        opencode opencode-auto claude claude-auto codex codex-auto agents-version agents-upgrade \
        opencode-upgrade claude-upgrade codex-upgrade doctor check-image image-inputs-hash \
        preflight check-identity prepare-agent-mounts

# One recipe for every agent; the targets below only choose AGENT_CMD (a
# target-specific variable, which make passes down to this prerequisite).
#
# The sandbox is started if it is not running, WITHOUT --force-recreate: a
# running sandbox is left alone, so an agent already working in it keeps its
# session. The agent's exit status is the recipe's — no fallback re-runs it in
# another shell when it exits non-zero.
dev-agent: preflight check-image
	@test -n "$(AGENT_CMD)" || { echo "ERROR: run an agent target (make opencode, make claude-auto, ...), not dev-agent." >&2; exit 1; }
	@if [ -z "$$($(COMPOSE_UP) ps -q dev 2>/dev/null)" ]; then \
	  echo "[agent] starting the dev sandbox..."; \
	  token="$$(scripts/resolve-gh-token.sh 2>/dev/null)"; \
	  GH_TOKEN="$$token" GITHUB_TOKEN="$$token" $(COMPOSE_UP) up -d dev; fi
	@bin="$(firstword $(AGENT_CMD))"; \
	 tty="-T"; if [ -t 0 ]; then tty="-it"; fi; \
	 $(COMPOSE_UP) exec $$tty dev zsh -lc "cd /workspace && if ! command -v $$bin >/dev/null; then echo \"ERROR: $$bin is not installed in this image — add it to SANDBOX_AGENTS in the Dockerfile, then make dev-build.\" >&2; exit 127; fi; exec $(AGENT_CMD) $(AGENT_ARGS)"

dev-opencode: AGENT_CMD = $(OPENCODE_CMD)
dev-opencode: dev-agent ## Run opencode inside the dev sandbox
dev-opencode-auto: AGENT_CMD = $(OPENCODE_AUTO_CMD)
dev-opencode-auto: dev-agent ## Run opencode in auto mode inside the dev sandbox
dev-claude: AGENT_CMD = $(CLAUDE_CMD)
dev-claude: dev-agent ## Run Claude Code inside the dev sandbox
dev-claude-auto: AGENT_CMD = $(CLAUDE_AUTO_CMD)
dev-claude-auto: dev-agent ## Run Claude Code in auto mode inside the dev sandbox
dev-codex: AGENT_CMD = $(CODEX_CMD)
dev-codex: dev-agent ## Run Codex inside the dev sandbox (the container is its sandbox)
dev-codex-auto: AGENT_CMD = $(CODEX_AUTO_CMD)
dev-codex-auto: dev-agent ## Run Codex with no approval prompts inside the dev sandbox

# --- Goals that name no target -----------------------------------------------
# `make run_api <args>` and `make run_cli <args>` forward their arguments as
# extra *goals* (that is what `$(filter-out $@,$(MAKECMDGOALS))` in those
# recipes reads), so this catch-all exists to stop make from failing on them.
# That is the only case where a goal naming no target is legitimate.
#
# Everything else is a typo, and a typo that exits 0 is the worst kind: the
# caller reads green and goes on to debug state that never changed — which is
# exactly what `make dev-restart` did while the target was missing (#104). So an
# unknown goal fails, unless the first goal is one of the two that take
# arguments.
%:
	@case "$(firstword $(MAKECMDGOALS))" in \
	  run_api|run_cli) : ;; \
	  *) echo "ERROR: no rule to make target '$@'." >&2; \
	     echo "       'make help' lists the targets." >&2; \
	     exit 1 ;; \
	esac
