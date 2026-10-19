# Proposal

## Why

Published vintage lots already have immutable schedules and commerce accounts, but visitors cannot compete for them or receive a durable result. Roadmap Change 5 connects those foundations with concurrent timed auctions so two bidders can complete the bid-to-win portion of the demo before checkout is introduced.

## What Changes

- Add independent commerce auction state, accepted bid history, per-lot revisions, and immutable sold/unsold results in additive PostgreSQL migration 3; retain USD integer cents and the existing writer lease.
- Execute each published lot's common opening and staggered closing schedule, including overlapping events, new publication, startup adoption of previously published lots, and overdue closure after restart.
- Accept authenticated manual ascending bids with a server-computed minimum, fixed per-lot increment, strict server deadlines, transactional serialization, and retry-safe accepted submissions. A current leader cannot raise their own bid; no legacy budget is charged.
- Provide public read-only realtime snapshots and pseudonymous bid history through a separate commerce SSE surface; keep personal leading/outbid/won/lost state private. Show closure as pending until its result is committed.
- Activate the lot-detail bid panel and `/account/bids`, including server-confirmed feedback, reconnect recovery, EN/VI copy, and accessible mobile layouts.
- Replace earlier specs' blanket bidding-disabled rules while retaining publication immutability, private watchlists, capability-gated catalog administration, and unavailable checkout/orders.
- Do not add auto-bidding, reserve prices, anti-sniping, withdrawals, relisting, seller controls, payments, orders, analytics, or ML. Creating planning artifacts does not implement these features.

## Capabilities

### New Capabilities

- `timed-auction-bidding`: Per-lot lifecycle, money/identity/deadline validation, durable accepted bids/results, retry safety, and private account bid participation.
- `auction-realtime`: Public commerce snapshots, pseudonymous history, monotonic revisions, SSE delivery, disconnect recovery, and separation from private account/game data.

### Modified Capabilities

- `auction-event-publishing`: Published immutable schedules become inputs to separate commerce execution; publication itself does not accept a bid or determine a result.
- `auction-discovery`: Activate lot-detail bidding and show server-confirmed execution independently of schedule filters and opening-price sorts.
- `website-navigation`: Make `/account/bids` functional and update shared availability notices while checkout/orders stay previews.
- `lot-watchlists`: Remove assumptions that the neighboring bid control is always disabled; saving still never implies participation or a result.
- `catalog-administration`: Explain scheduled bidding after publication without adding auction operations to the preparation workspace.
- `postgresql-persistence`: Add commerce auction storage, writer/commit guarantees, migration/readiness version 3, import occupancy protection, and real PostgreSQL acceptance.

## Impact

Proposed backend work adds `internal/auction` and `internal/store/auction.go`, extends account/API wiring in `internal/api` and `cmd/server/main.go`, and adds migration `003_timed_auctions.sql` without changing migrations 1–2. New `/api/auctions/lots/...` read/stream endpoints and session-owned `/api/account/bids...` operations coexist with unchanged catalog content DTOs and retained `/ws`.

Frontend work targets `web/src/routes/storefront/LotDetailPage.tsx`, account routes, typed API/state helpers, and shared EN/VI notices; SSE uses the existing `/api` development proxy. `README.md`, `docs/postgresql.md`, and `web/README.md` need operational/rehearsal updates during apply. No new runtime dependency is proposed. Existing `AGENTS.md` and archived evidence remain untouched. Change 4's archived 200-percent zoom tasks 7.3 and 8.3 remain unverified; this change must record its own accessibility evidence without retroactively completing them.
