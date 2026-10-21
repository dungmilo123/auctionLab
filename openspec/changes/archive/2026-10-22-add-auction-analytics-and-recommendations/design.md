# Design

## Context

See `proposal.md` for motivation and scope. Read-only inspection found:

- `internal/store/schema.go` supports version 5 with embedded migrations 1–5. `AGENTS.md` still mentions version 4 and Change 7 previews; the current code, main specs, roadmap and Change 7 archive establish implemented order history/fulfillment/reset. Apply must update those stale current-state descriptions, preserving archived evidence.
- `commerce_bids`, `commerce_auction_results`, `commerce_orders` and `commerce_demo_payments` hold authoritative participation/outcomes. A sold result creates one owned order atomically; one receipt records the paid total including delivery. `OrderRepository.Confirm` already handles same-intent replay and a fresh ID on a paid order.
- `AccountRepository.Membership` uses `ON CONFLICT DO NOTHING`; current membership has `saved_at` but removed membership history does not exist. There is no view or recommendation service.
- `AuctionRepository.read` demonstrates bounded read-only repeatable-read transactions. Writes share the leased connection; nested repository writes would be unsafe. Capture hooks must use the existing transaction directly.
- `catalog.PublishedLots` produces explicit safe projections from published events. `era` is optional authored text, without a controlled numeric period model.
- `web/src/lib/route.ts`, `routes/admin-preview/AdminPage.tsx` and `layouts/SiteLayout.tsx` own website routing/admin navigation. The retained `routes/admin/AdminView.tsx` belongs to the old game and is not the website integration target.
- Home and lot detail already load public data independently from commerce identity/execution. API decoders, EN/VI copy, native links, cancellable reads and stale-response guards provide the integration pattern.
- `internal/store/demo.go` inspects an explicit known-object allowlist and clears a fixed table set. `internal/store/sqliteimport.go` checks target occupancy explicitly. New tables must join both policies or migration 6 would break safe maintenance.

## Goals / Non-Goals

**Goals:** calculate auditable metrics from committed facts; make suggestions deterministic and independently recoverable; preserve ownership and replay guarantees when recording actions; distinguish observed coverage from reconstructable history; support offline Vite and embedded rehearsals.

**Non-Goals:** a warehouse, global tracking identity, changing auction/payment rules, inferred historical views, trained ML, effectiveness evaluation, public operational metrics, arbitrary time windows or new frontend charting dependencies. The interaction ledger is preparation for possible later analysis, not proof of recommendation quality.

## Decisions

### 1. Keep reporting, suggestions and capture as small separate services

Add `internal/analytics`, `internal/recommendation` and `internal/interaction` with repository interfaces implemented under `internal/store`. Add explicit APIs under `internal/api` and wire them in `cmd/server/main.go`. Owner suggestions extend the existing `AccountAPI` dispatch under `/api/account/`; do not register a competing account-prefix handler.

The analytics endpoint uses `CatalogAdminGuard`, including unknown subtree requests, plus the existing document gate on `/admin/*`. Role-based account authentication is not an alternative capability. Public recommendation reads ignore cookies; owner recommendations validate the live session within their read snapshot.

Alternative: put all behavior into catalog management. Rejected because private aggregate reporting, owner preferences and optional observations have different authorization and failure boundaries. No event bus, external analytics SDK, background exporter or new dependency is needed.

### 2. Use lifetime published-lot cohorts and direct fact queries

Build the selected cohort from published event membership, using `event=all|<slug>` and `dataset=all|demo|non-demo`. Omitted filters default to all. Parse strictly; reject unknown, duplicate, empty and malformed parameters with 422; missing/draft selected events share 404. Event rows include selected published events even when a dataset filter gives them zero matching lots, so comparison emptiness is explainable.

Demo lot classification is `lot.demo OR event.demo`; account demo classification is a separate breakdown on accepted bids. A demo actor on a non-demo lot does not move that lot into the demo dataset. All receipt values are simulated regardless of these flags. No date filter is supplied: payments are attributed to the lot's original event, even when paid later.

| Metric | Definition |
| --- | --- |
| Published lots | Count of cohort lots, including not-yet-initialized executions |
| Bids | Count of accepted bid rows for cohort lots |
| Bidders | Distinct account IDs across those accepted rows; not registered accounts or public aliases |
| Demo/ordinary participation | Bid and distinct-bidder counts partitioned by actor demo flag |
| Sold / unsold / terminal | Committed result counts; terminal = sold + unsold |
| Pending result | Published lots minus terminal; includes upcoming/open/elapsed-unreconciled lots |
| Sell-through | Sold / terminal, null when terminal is zero |
| Winning value | Sum of sold-result winning cents, regardless of payment |
| Paid orders | Count of one confirmed simulated receipt per owned order |
| Unpaid sold orders | Sold orders still awaiting payment |
| Paid winning value / delivery fees | Separate sums from confirmed paid orders |
| Simulated paid value | Sum of receipt totals; equals paid winning value + paid delivery fees |

Use independent grouped bid/result/payment subqueries or CTEs before joining to avoid bid × payment fanout. Do not derive business totals from the interaction ledger or sum public snapshot counters. Validate result/order/receipt bindings and paid-state/amount consistency without selecting checkout contact JSON; corrupt required facts return unavailable rather than partial metrics.

Compute global distinct bidders directly across the cohort. Additive event totals reconcile with summary; event bidder counts cannot be summed. Return every selected event row in slug C-order. Run all reads, integrity checks, interaction counts, definitions metadata and response timestamp inside one bounded read-only repeatable-read transaction. Refresh is explicit, with a visible as-of timestamp; there is no analytics SSE subscription.

Alternative: reporting from a frontend aggregation or raw joined rows. Rejected because it leaks source detail, risks fanout and produces inconsistent snapshots. Materialized aggregates and caches add unnecessary synchronization for the small demo catalog.

### 3. Preserve exact aggregate arithmetic across the API

Use PostgreSQL exact aggregate arithmetic (`SUM(bigint)` yields numeric), scan decimal strings and validate nonnegative integer syntax. Counts and all new aggregate `*_minor` fields are decimal strings, including zero. Do not constrain an aggregate to the existing per-order money bound. Browser decoders/formatters use exact integer arithmetic; they never coerce large totals through `Number` or reuse the bounded single-lot formatter.

Return sell-through as null or `{sold: "2", terminal: "3"}`; display a rounded percentage alongside those exact counts using integer arithmetic. Metadata includes USD, millisecond `as_of`, resolved filters, demo definitions, collection start, and provenance counts. Existing order/catalog DTO amount types stay unchanged. A response has `{data: {summary, events, interactions, definitions}, meta: {...}}`; every success/error uses no-store. Failed/malformed data becomes error, never a row of zeroes.

Alternative: JSON numeric totals. Rejected because many valid individual orders can exceed JavaScript's safe integer range when aggregated.

### 4. Add a minimal private interaction ledger with migration 6

Create `commerce_interactions` and `commerce_interaction_coverage`. A record contains a server-generated opaque ID, supported kind, lot/event IDs, nullable authenticated actor ID, original occurrence and recording timestamps, derived lot/event/actor demo flags, provenance (`observed`, `backfilled`, `snapshot`) and a bounded source key. Add kind/provenance checks, published-identity validation in writes and foreign keys to existing identities. No generic metadata JSON or submitted payload is stored.

Use unique `(kind, source_key)` bindings. Bid keys derive from accepted bid IDs; payment keys derive from immutable receipt IDs. A watchlist-add transition uses a fresh server ID only after insertion actually changes membership; duplicate saves add nothing. Removed memberships leave historical events; re-add gets a new event. Views use a globally unique observation UUID bound to one lot; same-ID/same-lot acknowledges the existing record, changed-lot reuse conflicts. Add event/kind/time and actor/lot indexes for the actual queries, with no new identity sequence required.

Migration 6 backfills bids and payments from durable rows using original timestamps. Existing memberships get one `snapshot` row with their saved time; no removed membership or view is inferred. On a populated commerce upgrade, migration writes collection metadata at the upgrade instant. On a completely unused schema, migration leaves coverage empty so the existing migrate-then-import workflow still accepts an unused target. Normal startup initializes coverage idempotently under writer ownership before HTTP readiness; prepare/reset creates fresh coverage in its own transaction. A marker in an already-started target intentionally counts as occupancy. Restart never redoes backfill or advances collection start.

Reported interactions group kind by provenance. View counts represent received page observations only; they are neither unique visitors nor a complete visit history. Watchlist snapshots are shown separately from actual observed add transitions. Records persist with the application schema until explicit bounded demo reset; this change adds no general purge/export or personal activity UI.

Alternative: retroactively label all reconstructable records observed, or use current watchlists as complete addition history. Rejected because both invent collection coverage. A queue/outbox is unnecessary because all authoritative actions already commit to the same database.

### 5. Capture mutations inside their existing transaction

Add transaction-local insertion helpers to `internal/store/watchlist.go`, `auction.go` and `order.go`. Inspect `RowsAffected` for membership creation; place bid capture immediately after the accepted row is inserted; place payment capture alongside the first receipt/state/progress write. Existing replay/already-paid branches exit before capture.

Insertion failure rolls back the parent action; unknown commit preserves existing durable recovery behavior. Never call a repository writer recursively, publish before commit, or acquire a replacement writer after ownership loss. Read-only dashboard/suggestion calls never initialize auctions or create business state.

Alternative: frontend emits all event kinds or a best-effort post-commit callback. Rejected because clients can fabricate actions and callbacks can lose confirmed facts. A local database failure can now prevent a commerce mutation; this is deliberate atomicity and requires fault-injection coverage.

### 6. Treat anonymous views as optional bounded observations

`POST /api/catalog/lot-views` accepts exactly `{lot_id, observation_id}` with a strict 1 KiB JSON boundary, interaction header and existing same-origin checks. It sets no cookie and ignores incoming account credentials. No permissive CORS is added. Validation returns 422, missing/draft lot 404, conflicting ID 409, boundary errors 400/413/403/415, limit 429 and storage/unknown commit 503; acknowledged first/duplicate commits return 200 `{data:{recorded:true}}` with no IDs or raw inputs.

Reuse the bounded in-memory limiter pattern for direct-peer attempts: 30/minute/peer, at most 4096 retained peer keys; proxy headers cannot choose identity. The address exists only in limiter memory. A client can submit fabricated views, so dashboard copy labels them observations, not authenticated traffic or conversion evidence.

The successful lot-detail renderer obtains one in-memory UUID per lot per document through a small registry. This survives component/effect remounts and language/auction refreshes; no local/session storage identifier is created. Request failure is nonblocking and has no automatic retry loop. Native navigation to a new document gets a new ID; a restored document reuses its ID. The public catalog GET remains read-only, and a card impression never emits a view.

### 7. Use deterministic source/watchlist similarity with public fallback

Read eligible candidates and, for an owner request, the live session/watchlist in one repeatable-read snapshot. Reuse public projection helpers rather than serializing internal catalog rows. Eligibility requires published lot and parent, future planned close and no committed terminal result. Include upcoming lots, exclude source and owner-saved candidates, and never infer live auction execution from suggestions.

Score category equality as 4 and trimmed lowercased nonempty era equality as 2. With a source, use its category/era only; otherwise use membership in the owner's set of saved published categories/eras, with each feature counted once. Sort by score descending, planned close ascending and slug C-order, take six. Zero-score candidates fill remaining positions and use closing-soon reason. Reasons are bounded codes: `category_and_era`, `category`, `era`, `watchlist_category_and_era`, `watchlist_category`, `watchlist_era`, `closing_soon`.

`GET /api/catalog/recommendations[?source=<slug>]` omits credentials and supplies source similarity or fallback. `GET /api/account/recommendations` accepts the same optional source, validates only the current owner and otherwise returns 401; no client account ID is accepted. Both return `{data: [{lot, reason}], meta:{server_time, strategy:"rules-v1"}}` with no-store. Source draft/missing share 404; malformed/unknown/duplicate/empty queries are 422. Private preference sources and raw scores are omitted.

Alternative: parsing eras into date ranges, learning from views/bids, or popularity/collaborative ranking. Rejected because current authored era data and demo interactions do not justify those assumptions. The explicit rule baseline is independently useful and leaves a later ML comparison possible.

### 8. Integrate with existing website state and language boundaries

Add `adminAnalytics` to route matching, admin navigation and `AdminPage` dispatch; use existing guarded admin client patterns. Preserve all other exact-route/deep-link rules. Analytics uses labeled summary values, definition text and a semantic comparison table; a labeled local table scroller is acceptable for many columns while controls/cards must not cause page-level overflow. Charts are optional and cannot be the sole data representation.

Create a recommendation section shared by home and lot detail using existing cards. Public content does not wait for account validation. While identity is checking/unavailable, show independent public fallback with a label and separate account recovery; never treat that state as confirmed logout. Once a current session validates, replace it with private suggestions. A private failure clears private cards, offers retry and can keep explicitly labeled public suggestions; a 401 triggers existing session revalidation. Abort/ignore old source and account generations on logout, expiry, switch, pagehide or retry.

Recommendations and observations cannot disable working catalog or commerce controls. Update EN/VI copy in the actual shared notices and footers as well as new sections; preserve simulation and demo labels and describe suggestions as rule-based. Editorial content remains authored. Do not mount retained game views or sockets.

## Risks / Trade-offs

- [View observations can be blocked, duplicated across documents or fabricated] → bound and deduplicate collection, label its meaning and never use it for authoritative bid/payment metrics or effectiveness claims.
- [Historical tracking is incomplete] → retain backfilled/snapshot provenance and collection-start metadata; metrics still use full durable commerce facts.
- [Action capture adds transactional coupling] → small indexed inserts, operation timeouts and explicit rollback/unknown-commit tests; no post-commit publication gap.
- [Aggregate joins can inflate values] → preaggregate facts and reconcile an independently calculated fixture, including repeat bidders and receipts with many bids.
- [Numeric precision or demo cohort confusion] → decimal-string aggregates, exact formatters, definition labels and mixed-provenance fixtures.
- [Era matching is coarse] → exact normalized strings and reasons only; no historical inference or model claims.
- [Optional private data can outlive a session in UI] → generation-bound requests and immediate clearing on identity changes, with cross-account delayed-response tests.
- [Existing exact-zoom/reduced-motion acceptance is incomplete] → maintain those archived gaps; new checks require actual browser evidence and remain unchecked if unavailable.

## Migration Plan

1. During apply, add `006_auction_analytics_interactions.sql`, embed it, raise supported version to 6 and preserve files 1–5 byte-for-byte. Test fresh, populated 5→6, repeated, failed and newer-than-supported upgrades on owned PostgreSQL schemas. Prove fresh migrated targets remain importable before first normal boot.
2. Back up and stop the operator writer before upgrade. Migration/backfill is atomic under ownership. Initialize coverage before HTTP readiness; no deployment creates views, bids, winners, payments or personalized preferences.
3. Extend demo known-table checks and reset lists to both new tables. Atomically reseed coverage with the new demo generation; preserve migrations/config/categories/media and other schemas. Extend import occupancy checks to both tables, without changing legacy SQLite source semantics.
4. Run focused repository/API/frontend checks, then `make test`, `make test-race`, `make build` using explicit disposable PostgreSQL configuration. Run Vite and embedded rehearsals and compare UI metrics to independent SQL. Record restart and reset/replay evidence, two-owner authorization and delayed response behavior.
5. Record exact 200 percent browser zoom and active reduced-motion checks separately from responsive-width checks; preserve every unverified criterion. Update `README.md`, `docs/postgresql.md`, `web/README.md` and stale implementation statements in the existing `AGENTS.md` during apply without replacing its guidance wholesale.
6. Rollback requires stopping the version-6 writer and restoring the pre-upgrade backup with its compatible binary, or retaining version 6 with the matching build. An older binary must refuse schema 6. There is no destructive down-migration or automatic fallback; preserve post-upgrade data separately if restoring a backup.

## Acceptance Reference Fixture

Use two published events and at least one unpublished event, two bidding accounts plus one non-bidding account, and mixed demo content/actors. In event A, sell two lots for 10000/20000 cents, close one unsold and leave one pending; A has four accepted bids from two accounts. In event B, sell one lot for 5000 cents, using two accepted bids from both of A's bidders. Pay A's first lot with 1500-cent delivery and B's lot by pickup. Keep A's second win unpaid.

Expected combined values: five lots, six bids, two distinct bidders (A=2, B=2), sold=3, unsold=1, pending=1, sell-through=3/4, winning=35000, paid winning=15000, paid delivery=1500, simulated paid=16500 cents, paid orders=2, unpaid sold orders=1. Reject/replay bids, repeat paid confirmations and advance fulfillment; those values must not change. Views/watchlist additions affect only their own observed counts. Add a separate sufficiently large valid fixture to exceed JavaScript's safe integer range, and empty/zero-price/zero-fee fixtures.

Run independent SQL against the same owned fixture, not assertions copied from the reporting function. Recommendations use a separate published candidate matrix covering six/four/two/zero scores, ties, absent eras, past closes, terminal exclusions, owner-saved exclusions, new-user fallback and session switching. These checks establish implementation correctness; they do not establish user recommendation effectiveness.
