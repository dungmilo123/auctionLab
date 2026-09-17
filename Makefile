SHELL := /bin/sh

GO          ?= go
BIN         := bin/auctionlab
SERVER_PORT ?= 8080
VITE_PORT   ?= 5173
DB_PATH     ?= ./auction.db

.PHONY: dev dev-go dev-web build run tunnel loadtest loadtest-20 loadtest-race loadtest-tunnel test test-go test-web test-race test-lint clean

# One command, two processes, zero extra dependencies. `kill 0` reaps the whole
# process group — including the binary `go run` spawns, which a PID-based kill
# would miss. Teardown from a script must signal the group: kill -INT -<pgid>.
dev: web/node_modules
	@trap 'kill 0' INT TERM EXIT; \
	$(GO) run ./cmd/server --port $(SERVER_PORT) & \
	( cd web && npm run dev -- --port $(VITE_PORT) ) & \
	wait

dev-go:
	$(GO) run ./cmd/server --port $(SERVER_PORT)

dev-web: web/node_modules
	cd web && npm run dev -- --port $(VITE_PORT)

# Conditional install: runs npm ci only when node_modules is missing or the
# lockfile is newer, so a clean clone still builds and a rebuild stays fast.
web/node_modules: web/package-lock.json
	cd web && npm ci

# Frontend first — the embed is a compile-time input, so building Go first
# would embed the previous build (or nothing) with no error.
build: web/node_modules
	cd web && npm run build
	CGO_ENABLED=0 $(GO) build -o $(BIN) ./cmd/server

run: build
	DB_PATH=$(DB_PATH) ./$(BIN) --port $(SERVER_PORT)

tunnel: build
	DB_PATH=$(DB_PATH) ./$(BIN) --port $(SERVER_PORT) --tunnel

loadtest:
	$(GO) run ./scripts/loadtest

# D-135/D-136's acceptance layer. `loadtest` above stays the two-bot smoke the
# Phase 2 gate has always run; these targets run the fleet — 20 bots, a bounded
# burst, one deliberately stalled reader — against a REAL server on a temporary
# database, plus the tunnel-death drill. LOADTEST_PORT keeps the run off the dev
# server's 8080, and LOADTEST_RACE=1 makes the HARNESS race-instrumented too, so
# `loadtest-race` instruments both sides of the wire (the script verifies the
# server binary carries the race runtime).
LOADTEST_PORT    ?= 8199
LOADTEST_BOTS    ?= 20
LOADTEST_BURST   ?= 40
LOADTEST_ROUNDS  ?= 3
LOADTEST_STALLED ?= 19

loadtest-20:
	$(GO) build -o bin/auctionlab-loadtest ./cmd/server
	LOADTEST_PORT=$(LOADTEST_PORT) ./scripts/loadtest-acceptance.sh ./bin/auctionlab-loadtest --bots $(LOADTEST_BOTS) --rapid-fire --burst $(LOADTEST_BURST) --rounds $(LOADTEST_ROUNDS) --stalled-index $(LOADTEST_STALLED)

loadtest-race:
	$(GO) build -race -o bin/auctionlab-race ./cmd/server
	LOADTEST_RACE=1 LOADTEST_PORT=$(LOADTEST_PORT) ./scripts/loadtest-acceptance.sh ./bin/auctionlab-race --bots $(LOADTEST_BOTS) --rapid-fire --burst $(LOADTEST_BURST) --rounds $(LOADTEST_ROUNDS) --stalled-index $(LOADTEST_STALLED)

# D-137's desk drill: the stub tunnel is killed mid-game and the production
# wire (manager → sink → hub frame + engine projection) must publish the death,
# fall back to LAN, restart with a rotated URL and keep local play settling.
loadtest-tunnel:
	$(GO) test ./cmd/server -run TestTunnelDeathDrill -count=1

test: test-go test-web test-lint

test-go:
	$(GO) test ./...

# D-43: the ordering and resync contracts are only meaningful under the race
# detector. -timeout is not optional — a lock-order mistake deadlocks, and a
# hung gate with no goroutine dump is undiagnosable (research Pitfall 11).
test-race:
	$(GO) test -race -timeout 120s ./...

test-web: web/node_modules
	cd web && npm run test

# oxlint exits non-zero on errors and only prints warnings without failing;
# running it inside `make test` is what keeps the lint signal exercised at all.
test-lint: web/node_modules
	cd web && npm run lint

clean:
	rm -rf bin web/dist/assets web/dist/index.html
