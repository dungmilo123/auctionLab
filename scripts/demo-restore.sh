#!/bin/sh
# Restore the pre-played demo snapshot (demo/auctionlab_demo.dump) into the
# PostgreSQL database named by DATABASE_URL, as schema auctionlab_demo_submission.
#
# The snapshot is a disposable, tool-marked demo schema: it contains published
# auctions, accepted bids, sold/unsold results, paid and unpaid orders and
# simulated fulfillment, so every page has data immediately after restore.
#
# Usage:  DATABASE_URL=postgres://user@localhost:5432/auctionlab ./scripts/demo-restore.sh
#         FORCE=1 ...   drop an existing auctionlab_demo_submission schema first
set -eu

SCHEMA=auctionlab_demo_submission
DUMP=${DEMO_DUMP:-demo/auctionlab_demo.dump}

if [ -z "${DATABASE_URL:-}" ]; then
	echo "DATABASE_URL is required, e.g. postgres://$(whoami)@localhost:5432/auctionlab?sslmode=disable" >&2
	exit 1
fi
for tool in psql pg_restore; do
	command -v "$tool" >/dev/null 2>&1 || { echo "$tool not found: install the PostgreSQL 18 client tools" >&2; exit 1; }
done
[ -f "$DUMP" ] || { echo "snapshot not found: $DUMP" >&2; exit 1; }

exists=$(psql "$DATABASE_URL" -Atc "select 1 from pg_namespace where nspname = '$SCHEMA'")
if [ "$exists" = "1" ]; then
	if [ "${FORCE:-}" = "1" ]; then
		echo "dropping existing schema $SCHEMA (FORCE=1)"
		PGOPTIONS='-c client_min_messages=warning' psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -qc "drop schema $SCHEMA cascade"
	else
		echo "schema $SCHEMA already exists; run with FORCE=1 to replace it (its data will be lost)" >&2
		exit 1
	fi
fi

pg_restore --no-owner --no-privileges --exit-on-error -d "$DATABASE_URL" "$DUMP"
echo "restored $SCHEMA from $DUMP"
echo "start it with: make demo   (then open http://localhost:8080)"
