SHELL := /bin/sh

GO          ?= go
BIN         := bin/auctionlab
SERVER_PORT ?= 8080
VITE_PORT   ?= 5173
DB_PATH     ?= ./auction.db

.PHONY: dev dev-go dev-web build run tunnel loadtest test test-go test-web test-lint clean

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

test: test-go test-web test-lint

test-go:
	$(GO) test ./...

test-web: web/node_modules
	cd web && npm run test

# oxlint exits non-zero on errors and only prints warnings without failing;
# running it inside `make test` is what keeps the lint signal exercised at all.
test-lint: web/node_modules
	cd web && npm run lint

clean:
	rm -rf bin web/dist/assets web/dist/index.html
