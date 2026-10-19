# Design

## Context

See `proposal.md` for motivation. Discovery currently reads editorial `published` records through `internal/catalog/projection.go`; `web/src/lib/discovery.ts` derives schedule phases from response server time. `LotDetailPage.tsx` has a disabled bid panel, and `AccountPage.tsx` treats bids/orders as previews. Commerce sessions and the account mutation boundary already exist in `internal/api/account.go` and `internal/store/account.go`.

The retained `internal/game/auction.go` selects one current auction, validates game players/budgets, can extend deadlines, and changes budget/truth-value totals at settlement. Its `internal/hub` projects game roles through `/ws`. Calling that engine for commerce would import incompatible identity, money, lifecycle, and disclosure semantics. The transferable ideas are a server-derived minimum, checking deadlines inside the serialized mutation, committing before success/publication, and idempotent terminal transitions.

`internal/store/store.go` owns a dedicated leased connection and never reacquires it after loss. Account writes transact on that connection; catalog writes use a separate transaction after an ownership check. PostgreSQL schema version is currently 2. `internal/store/sqliteimport.go` explicitly checks target-table occupancy. The website has an `/api` development proxy and no commerce realtime connection.

## Goals / Non-Goals

**Goals:** Use PostgreSQL as the auction source of truth, support independently timed lots across overlapping events, make ordered updates recoverable without a persistent event broker, and leave a stable private winning-account/result reference for Change 6.

**Non-Goals:** No multi-process auction service, durable replay broker, production-scale fanout, modification of the game protocol, account-admin replacement for the capability gate, or order/payment creation. Keep discovery query values and opening-price sorting compatible; realtime is initially concentrated on the lot detail and account participation surfaces.

## Decisions

### 1. Independent database-backed commerce service

Add `internal/auction` for lifecycle, validation, safe DTOs and repository contracts, with `internal/store/auction.go` for SQL. Reuse catalog IDs, immutable schedule/pricing and commerce account IDs. Do not call the legacy `Engine.PlaceBid`, settlement, or hub. Pure arithmetic helpers can be shared only if doing so preserves existing game tests and contracts; sharing code is not a deliverable.

All commerce auction mutations use the existing leased connection through a cancellation-aware writer transaction, like account writes. Lock the target auction row and revalidate within that transaction. This serializes writes in this single-process demo while preserving per-lot durable state; many lots can be open simultaneously without parallel in-memory engines. Reads use repeatable-read snapshots from the pool. An alternative of a map of game engines would duplicate incompatible player state; a new WebSocket command protocol would add unnecessary authentication and ordering work.

### 2. Additive migration 3 and durable records

Add `internal/store/migrations/003_timed_auctions.sql`, embed it after migrations 1–2, and advance `UserVersion`/readiness expectations to 3. Do not edit either existing migration.

| Proposed table | Durable contract |
|---|---|
| `commerce_lot_auctions` | One row keyed by catalog lot ID; immutable event ID, opening/close instants, USD starting price/increment; execution state, per-lot revision, accepted count and leading-bid reference |
| `commerce_bids` | Immutable accepted bid ID, lot ID, private account ID, strictly increasing per-lot sequence, integer `amount_minor`, acceptance instant, client request ID and per-lot bidder alias |
| `commerce_auction_results` | At most one immutable result per lot; sold/unsold, scheduled close and actual recorded time; winning bid/account/amount or all-null winner fields |

Use foreign keys and uniqueness for lot rows, `(lot_id, sequence)`, `(account_id, request_id)`, and result-per-lot. Add indexes for due-state scans, newest bid history and account participation. Enforce valid states, integer money in `[0, catalog.MaxMoney]`, positive increments, and consistent sold/unsold nullability. Winning bid references must resolve to the same lot/account/amount; enforce with composite relationships or transactional validation plus tests. Request IDs are bounded UUID strings. Public DTOs never embed persistence structs.

Keep the catalog DTO and editorial `state: published` unchanged. New execution DTOs carry `auction_state` and `revision`; publication versions and game revisions do not become auction revisions. Extend import occupancy checks to all three new tables. Migration itself creates storage only, never accepts bids or settles auctions.

### 3. Adopt published schedules once and reconcile independently

At normal startup after migration and before listening, initialize missing auction rows from every committed published event/lot and its existing derived schedule, then reconcile due transitions. Never load drafts or rebase dates. Startup failure leaves the listener closed. Existing future lots become scheduled; currently eligible lots open; already expired lots with no accepted commerce bids close unsold with their original deadline and the current recording time. No earlier bids or sale are inferred. State explicitly in operator guidance that Change 5 activates previously published schedules, so a fresh demo requires new future-scheduled drafts/publication rather than editing expired publications.

Run one cancellable worker every 250 ms to discover newly published lots, open due lots and close overdue lots in bounded batches/transactions. No publication hook is needed for correctness; a newly published lot can briefly show execution pending. A bid can initialize a missing published lot row transactionally and advance it to open, avoiding a false refusal while the worker catches up. Reads and SSE never initialize or settle anything. Late bids are refused even if the worker has not closed the row.

Persist `scheduled -> open -> sold|unsold`, allowing `scheduled -> sold|unsold` during overdue catch-up. Each committed state transition or accepted bid advances that lot's revision. At/after the deadline a nonterminal snapshot is presented as `closing` with bidding disabled until the result commit; a scheduled row past opening is `opening` until reconciled. These are response presentation states, not additional database transitions. Terminal results never reopen, extend or relist. Normal shutdown cancels streams/worker and joins them before closing storage; maintenance/import/seed modes start neither.

### 4. Bid ladder, exact money, and retry contract

Within the serialized writer transaction: validate a current commerce session, resolve a published target, check for a prior accepted request ID, lock/initialize its auction, sample server time **after** waiting for the writer/row lock, recheck session expiry with that time, then enforce window, current leader and amount rules. The time sampled immediately before validation is the acceptance/linearization instant; a valid transaction can finish committing after close, and closure serializes behind it. HTTP arrival time and the worker tick never decide eligibility.

The window is `opens_at <= now < closes_at`. First minimum is the opening price (including zero); subsequent minimum is `top.amount_minor + bid_increment_minor`. Any integer amount at least that minimum is accepted, including a jump; no multiple-of-increment grid is required. Reject fractional/negative/over-limit amounts and unsupported/unknown fields. If the next minimum exceeds `catalog.MaxMoney`, expose null minimum plus `amount_limit_reached`, disabling further bids while retaining the leader until scheduled closure. A leading account cannot self-raise. Both bidder/admin commerce account roles can bid through their own session; the catalog capability alone cannot. No budget balance, payment or watchlist prerequisite applies.

The browser creates one request ID per deliberate submit and retains it for that pending intent. The accepted bid, count, leader and revision commit atomically. Repeating an accepted `(account_id, request_id)` with the same lot and amount returns its original receipt plus a fresh current snapshot, even after closure; a different payload returns `request_conflict`. Session validity is always required. Rejections do not reserve IDs or append bid history. A new deliberate retry after a confirmed rejection uses a new ID.

Transport/uncertain commit returns no confirmed acceptance. Reconcile through the private lot participation endpoint, which includes the account's accepted request IDs, or explicitly retry that same intent with the same ID. Never generate a fresh ID automatically for an unknown outcome. No uncommitted notification is emitted. Extend safe commit-error classification to auction commits and fail closed on lost ownership; do not reacquire a connection for stale process state.

### 5. API boundary and identity privacy

| Endpoint | Access and response |
|---|---|
| `GET /api/auctions/lots/{lot_id}` | Anonymous safe coherent snapshot, recent accepted history and `meta.server_time` |
| `GET /api/auctions/lots/{lot_id}/bids?before_sequence=...` | Anonymous history pages, descending sequence, at most 50 rows, stable cursor |
| `GET /api/auctions/lots/{lot_id}/stream` | Anonymous SSE full snapshots, no account fields |
| `POST /api/account/bids` | Current commerce session; strict `{lot_id, amount_minor, request_id}` JSON, `X-AuctionLab-Account: 1`, existing origin checks |
| `GET /api/account/bids` | Private participation list, one row per lot with the account's latest accepted bid and current status |
| `GET /api/account/bids/{lot_id}` | Private own accepted bids/request IDs and participation status for a published lot, including `not_participating` |

Extend the existing `AccountAPI` switch/constructor to dispatch its bids subpaths through an injected auction service, preserving the whole-private-namespace session guard. Do not mount a competing account prefix handler that bypasses the guard. Revalidate sessions within bid writes, not only in HTTP middleware. Use the existing no-store `{data, meta}` / structured-error conventions, strict 8 KiB JSON limit, and no permissive credential CORS. GET business mutations remain forbidden.

Return 201 for a new bid, 200 for a replay receipt, 400 malformed request, 401 invalid/expired session, 403 rejected origin/header, 404 indistinguishable draft/missing target, 409 lifecycle/minimum/leader/request conflict, 422 invalid amount, and 503 unavailable/commit-unknown. Error responses carry safe reason codes and an available fresh public snapshot/minimum; they never accept caller-supplied owner/role/deadline/winner fields.

Public history exposes sequence, amount, time and a per-lot alias `Bidder N`, allocated on that account's first accepted bid on the lot. An alias is stable within a lot, not a cross-lot identity; it never uses email, display name or account ID. Public sold state includes winning bid sequence, alias and amount, never winner account ID. Private responses identify only the requesting account's bids and derive `leading`, `outbid`, `closing`, `won`, or `lost` from one coherent auction/result snapshot. Scheduled lots with no personal bids are `not_participating`; the list contains only accepted participation. Order rows by latest personal bid time descending, then lot ID; provide filter tabs for leading, outbid, won and lost, keeping closing visible in All. A win reports checkout unavailable.

### 6. SSE snapshots with bounded recovery

Use a separate read-only SSE stream per visible lot-detail page; keep `/ws` untouched. Send a full coherent public snapshot immediately, then poll the committed repository once per second and emit when revision or presentation state changes. Include up to the latest 50 accepted bids, `history_has_more`, current price (null before any bid), server-computed next minimum, state/times/count, public leader/result, lot ID and revision. SSE `id` is that revision. Send a server-time heartbeat at least every 10 seconds. No broker, event replay log or publisher-under-lock is needed; full snapshots are the recovery primitive, so a racing commit is caught by the next read.

Ignore Last-Event-ID as a replay demand and always send the latest complete snapshot on reconnect. Clients discard lower revisions, allow a same-revision server-time/presentation refresh, and never replace a newer accepted snapshot with an older HTTP read. Reconcile an unknown bid receipt independently of the public revision. Use bounded active streams (64 process-wide), one stream per mounted detail, cancellable reads and per-write deadlines (5 seconds); reject excess streams with retry guidance. A slow/disconnected stream holds no database transaction, row lock or writer lease. Shutdown cancels SSE contexts before `http.Server.Shutdown` waits for active handlers.

On stream failure, use public GET polling every 2 seconds with visible reconnecting/stale copy, and retry streaming with capped backoff. After 10 seconds without a successful snapshot/time heartbeat, label data stale and disable submit until a fresh read succeeds. Invalid payloads never activate bidding. Refresh immediately after accepted bids, rejected stale minimums, focus/visibility restoration and reconnect. Healthy local acceptance target: another browser sees a committed bid/result within 2 seconds, measured during rehearsal rather than treated as a production SLA.

Private lot participation and `/account/bids` poll every 2 seconds while visible and revalidate on focus, successful submit or public revision change. These independent session-owned reads supply personal feedback; public aliases are never compared to profile names to infer ownership. Account changes, logout, expiry, pagehide and obsolete async responses clear private participation and pending intent. No bid is replayed after login or account switching.

### 7. Website integration and compatibility

Replace only the lot-detail disabled panel with an independent auction loader/status, history and manual money form. Keep condition and fulfillment terms earlier in reading order and the watch control separate. Parse decimal user entry through exact cents helpers; do not use binary floating-point multiplication. Anonymous users can inspect all public history and get a safe login-return link; authentication never submits automatically. Disable submit for unavailable identity, scheduled/opening/closing/terminal state, stale data, self-leading, exhausted money range or pending intent.

Keep homepage/index/event cards' opening prices and planned phase filters (`all/upcoming/window/elapsed`) explicitly schedule-based. Those cards need not open one stream per lot. Only the detail's separate execution panel and account participation use live auction state; an elapsed filter alone never claims sold. The catalog JSON readers remain compatible. Notices/footer/admin copy must now describe scheduled bidding as available and checkout/orders as unavailable. Adjust the existing six spec boundaries listed in the proposal rather than leaving contradictory disabled-bidding clauses.

Use existing EN/VI design/copy infrastructure, tabular money/countdown, associated labels/errors, polite announcements for committed results, visible keyboard focus and reduced motion. Avoid announcing every countdown second or stealing focus on bid updates. Test at 320/375 CSS pixels, desktop and actual 200-percent browser zoom. Preserve the archived Change 4 evidence, including its unverified tasks 7.3/8.3; new observations are Change 5 evidence only.

## Risks / Trade-offs

- Single leased writer serializes auction/account writes → short transactions and bounded worker batches; no claim of production throughput.
- One-second SSE reads and private polling add database work → limit streams/history, close on unmount, pause private polls while hidden, and measure only the demo workload.
- Startup activates old published schedules → document deterministic adoption and rehearse using newly prepared future schedules; never alter immutable published dates to replay a demo.
- Server wall-clock adjustments affect eligibility → server time stays authoritative; deterministic injected clocks cover exact boundaries and restart, with no client-clock authority.
- Connection loss near commit can leave an unknown receipt → durable request IDs, private reconciliation, fail-closed writer behavior and no automatic new-ID resubmission.
- Bidder aliases reveal participation patterns within a lot → intentional public ascending-auction history, without account identifiers or cross-lot handles.
- Archived accessibility gaps could be mistaken for closure → record independent Change 5 browser results and keep historical limits unchanged.

## Migration Plan

1. Back up the stopped PostgreSQL schema, preserve migrations 1–2 and record a compatible previous binary. Provision only disposable owned schemas for acceptance tests.
2. Apply additive migration 3; verify repeat migration, v2 upgrade preservation and readiness version 3. Offline import/seed modes must not start auction reconciliation, and occupied-target checks include auction data.
3. On normal boot, adopt committed published schedules and reconcile overdue lots before accepting traffic. Start the worker/stream lifecycle with explicit shutdown ordering. Newly published lots are adopted on the next worker pass.
4. Validate with real PostgreSQL tests, focused frontend state/contract tests, and two isolated browser sessions in both Vite and embedded builds. Rehearse overlapping events/multiple lots, no-bid unsold, rejection/races, reconnect and overdue restart; save sanitized evidence under this change's `verification/` during apply.
5. If rollback is needed, stop the writer. Use a binary compatible with schema 3 or restore the pre-upgrade backup with explicit loss of later bids/results. A schema-2 binary must refuse schema 3; do not drop auction tables, downgrade the ledger, or write back to SQLite as an implicit rollback.
