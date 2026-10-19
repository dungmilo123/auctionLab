#!/bin/sh
# 07-05's race acceptance (D-135/D-136/D-137, REH-01): run the loadtest fleet
# against a REAL server that no one else's data can reach and no one else's
# process can leak.
#
#   usage: scripts/loadtest-acceptance.sh <server-binary> [harness args...]
#
# What it guarantees, in order:
#   1. The database uses a fresh owned PostgreSQL test schema and the admin
#      capability is a pinned constant (ADMIN_KEY), so the run never touches the
#      developer's application schema and never needs the presenter's key (D-74).
#   2. The server is one child of this script on LOADTEST_PORT (default 8199,
#      deliberately not the dev server's 8080), started with no --tunnel: the
#      acceptance target is the room's ordering, not the venue's tunnel.
#   3. The harness starts only after GET /api/health answers, within a bounded
#      wait — a server that dies at boot fails the run with its log, not a hang.
#   4. Every exit path (success, harness failure, SIGINT/SIGTERM) reaps the
#      server child and removes the temp directory, and the script exits with
#      the harness's own code. `make loadtest-race` is only as trustworthy as
#      this: a nonzero result must never leave a server behind.
#   5. LOADTEST_RACE=1 runs the harness under `go run -race` AND verifies the
#      server binary really carries the race runtime (runtime.raceinit), so
#      "both sides are race-instrumented" is checked, not asserted.
set -u

die() {
	printf 'loadtest-acceptance: %s\n' "$*" >&2
	exit 2
}

bin=${1:-}
[ -n "$bin" ] || die "usage: scripts/loadtest-acceptance.sh <server-binary> [harness args...]"
shift
[ -x "$bin" ] || die "server binary '$bin' is missing or not executable (build it first: go build -o $bin ./cmd/server)"

port=${LOADTEST_PORT:-8199}
case "$port" in
	'' | *[!0-9]*) die "LOADTEST_PORT must be a port number, got '$port'" ;;
esac
race=${LOADTEST_RACE:-0}
command -v curl >/dev/null 2>&1 || die "curl is required to wait for /api/health"

# A pinned capability: 64 hex characters in the production SHAPE (D-74) that
# could never match a real server's key. The temp database is what makes it
# safe — nothing this run writes outlives the trap below.
admin_key=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef

tmp=$(mktemp -d "${TMPDIR:-/tmp}/auctionlab-loadtest.XXXXXX") || die "mktemp failed"
[ -n "${TEST_DATABASE_URL:-}" ] || die "TEST_DATABASE_URL required (dedicated PostgreSQL 18 test database)"
schema=$(go run ./scripts/dbfixture) || die "could not create isolated PostgreSQL schema"
server_log="$tmp/server.log"
server_pid=""

cleanup() {
	status=$?
	trap - EXIT INT TERM
	if [ -n "$server_pid" ]; then
		kill "$server_pid" 2>/dev/null
		waited=0
		while kill -0 "$server_pid" 2>/dev/null && [ "$waited" -lt 50 ]; do
			sleep 0.1
			waited=$((waited + 1))
		done
		kill -9 "$server_pid" 2>/dev/null
		wait "$server_pid" 2>/dev/null
	fi
	go run ./scripts/dbfixture --drop "$schema" || status=1
	rm -rf "$tmp"
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [ "$race" = 1 ]; then
	# The server half of "both race-instrumented": a binary built without -race
	# has no race runtime to instrument, so the flag would be a lie.
	if ! go tool nm "$bin" 2>/dev/null | grep -q 'runtime\.raceinit'; then
		printf 'loadtest-acceptance: LOADTEST_RACE=1 but %s carries no race runtime — rebuild it with: go build -race -o %s ./cmd/server\n' "$bin" "$bin" >&2
		exit 2
	fi
fi

printf 'loadtest-acceptance: server=%s race=%s port=%s schema=%s\n' "$bin" "$race" "$port" "$schema"

# The child owns the port; nothing else in this script may need it.
DB_PATH= DATABASE_URL="$TEST_DATABASE_URL" DATABASE_SCHEMA="$schema" ADMIN_KEY="$admin_key" "$bin" --port "$port" >"$server_log" 2>&1 &
server_pid=$!

# Bounded health wait: 200 × 50 ms = 10 s. The server prints its boot lines to
# the log, so a failure here reports why instead of hanging the gate.
waited=0
while ! curl -fsS "http://127.0.0.1:$port/api/health" >/dev/null 2>&1; do
	if ! kill -0 "$server_pid" 2>/dev/null; then
		printf 'loadtest-acceptance: the server exited before /api/health answered:\n' >&2
		sed -n '1,40p' "$server_log" >&2
		exit 1
	fi
	waited=$((waited + 1))
	if [ "$waited" -ge 200 ]; then
		printf 'loadtest-acceptance: /api/health did not answer within 10s:\n' >&2
		sed -n '1,40p' "$server_log" >&2
		exit 1
	fi
	sleep 0.05
done

# Print the harness command: the evidence that both sides are instrumented has
# to be readable from the target's output, not inferred from the Makefile.
if [ "$race" = 1 ]; then
	printf 'loadtest-acceptance: harness=go run -race ./scripts/loadtest\n'
	go run -race ./scripts/loadtest "ws://127.0.0.1:$port/ws" --admin-key "$admin_key" "$@"
else
	printf 'loadtest-acceptance: harness=go run ./scripts/loadtest\n'
	go run ./scripts/loadtest "ws://127.0.0.1:$port/ws" --admin-key "$admin_key" "$@"
fi
code=$?

if [ "$code" -ne 0 ]; then
	printf 'loadtest-acceptance: the harness exited %d; server log tail:\n' "$code" >&2
	tail -n 20 "$server_log" >&2
fi

# The EXIT trap reaps the child and removes the temp directory; exiting with
# $code here is what makes `make loadtest-race` propagate the verdict.
exit "$code"
