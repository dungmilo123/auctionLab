# Design

## Context

See `proposal.md` for motivation and capability scope. Discovery found these integration points:

- `internal/order/order.go` and `internal/store/order.go` define immutable purchase snapshots, two payment states (`awaiting_payment`, `paid`), owner reads and transactional confirmation. Receipt loading and frontend validators branch explicitly on those two states.
- `004_winner_checkout.sql` enforces that unpaid orders have no selection/payment values and paid orders retain them. `OrderRepository.Confirm` already locks the order, checks the session after waiting and returns the original receipt for accepted/fresh paid retries.
- `internal/api/order.go` serves wins and order detail/review/confirm; collection `/api/account/orders` currently returns 404. `CatalogAdminGuard` supplies capability, no-store JSON and origin/header checks suitable for a distinct order namespace.
- `web/src/lib/route.ts` recognizes history/admin placeholders and exact checkout IDs. `AccountPage.tsx` and `AdminPage.tsx` still render order previews. `AccountProvider`, `usePrivateRead` and checkout lifecycle guards already clear private account data on identity/navigation changes.
- `cmd/server/main.go` dispatches import/account/catalog maintenance before auction lifecycle and HTTP startup. Existing seeds own their transactions, preserve prior dates and create no winners/payments. They cannot yet participate in one atomic clear-and-reseed transaction.
- Schema 4 and active operator docs describe checkout; main specs still expressly forbid capability purchase access and describe Change 7 as unavailable. The seven modified deltas deliberately reconcile those statements. Archived Change 4–6 accessibility limitations are evidence, not implementation tasks silently completed by this change.

## Goals / Non-Goals

**Goals:**

- Keep the existing payment contract intact while adding method-specific fulfillment, one durable history and consistent buyer/operator reads.
- Reuse existing authorization, writer ownership, bounded transaction and private-view lifecycle patterns.
- Make demo reset independently reviewable and executable only inside a tool-marked disposable schema, as selected by the user.

**Non-Goals:**

- No change to auction execution, financial amounts, receipt identity, account authorization model, public catalog projections or published schedules.
- No carrier API, external tracking link generation, automatic fulfillment worker, notifications or authenticated operator-person attribution. The shared capability can identify an operator action but cannot establish which human performed it.
- No pagination/export/search platform for the small demo collection, no arbitrary tracking edits after shipment and no operational-schema deletion workflow. These can be later proposals if needed.

## Decisions

### 1. Keep payment and fulfillment separate

Keep `commerce_orders.state` and its public `state` values exactly `awaiting_payment|paid`. Add a separate fulfillment projection with `status`, `version`, `updated_at`, optional `shipment`, and ordered timeline. An unpaid order projects `awaiting_payment`, version 0, no method/shipment; a paid order always has a stored fulfillment record starting at `paid`, version 1.

| Paid method | Allowed operator transitions |
| --- | --- |
| shipping | paid → preparing → shipped → completed |
| pickup | paid → ready_for_pickup → completed |

The method comes from committed checkout selection. It cannot be chosen by the operator. Paid receipt reads, review and accepted confirmation replays remain valid at every later fulfillment stage. The history page gives unpaid purchases a checkout link and paid purchases a receipt/timeline view; paid checkout retains its original confirmation and adds a history-detail link.

Alternative: extending `commerce_orders.state` to every fulfillment status would require weakening migration-4 payment checks and changing every paid branch in Go and TypeScript. Separate progress preserves the proven checkout contract and makes payment versus delivery explicit.

### 2. Add schema 5 with safe historical initialization

Create `005_order_fulfillment.sql`, embed it in `internal/store/schema.go`, and raise `UserVersion`/health expectations to 5. Keep migrations 1–4 byte-for-byte unchanged.

- `commerce_order_fulfillment`: one row per paid order, matching receipt reference, immutable selected method, allowed status, positive version, last event time and nullable carrier/reference. Enforce referential integrity, allowed method/status combinations, shipment bounds and uniqueness. Add any composite receipt key required to guarantee the receipt belongs to that order through migration 5.
- `commerce_order_fulfillment_events`: `(order_id, version)` unique; from/to status, effective server timestamp, action source (`payment` or `operator`) and optional shipment data. No account attribution, internal notes or editable history. Initial payment event has version 1; each operator event advances version once.
- `demo_environments`: single environment marker with schema name, format version, committed generation identifier, seed reference and preparation timestamp. Normal migration creates the empty structure; only explicit eligible preparation inserts the marker.
- Owner-history index on `(owner_id, recorded_at DESC, id)` for all payment states; add minimal status/method indexes for admin filtering.

Migration 5 backfills exactly one `paid` record/event from each existing paid order's original receipt and selected method. Validate matching receipts and stored purchase consistency; invalid historical purchases fail the transaction rather than receive fabricated progress. Unpaid orders receive no fulfillment row. No future event is invented, and the migration creates no order, payment or disposable marker. DDL, backfill and ledger commit together.

New payment confirmation creates its initial fulfillment row/event inside the existing receipt/order transaction. Initialization failure rolls back the entire confirmation. A paid retry reads existing progress and never reinitializes it. Read helpers verify purchase/progress consistency and fail unavailable on corrupt required state.

### 3. Define bounded owner and operator API contracts

| Method and path | Authorization | Response/action |
| --- | --- | --- |
| GET `/api/account/orders` | current owner session | `{data: OrderSummary[]}` including every owned stage |
| GET `/api/account/orders/:id` | current owner session | existing order DTO with additive fulfillment/timeline fields |
| GET `/api/admin/orders?status=...&method=...` | capability | `{data: OrderSummary[]}` filtered globally |
| GET `/api/admin/orders/:id` | capability | bounded operational detail with purchase, submitted contact/address, receipt and timeline |
| POST `/api/admin/orders/:id/transition` | capability + admin mutation boundary | `{data: AdminOrderDetail}` after confirmed commit |

Collections sort by `recorded_at DESC, id ASC` using C ordering. Summaries contain order ID, lot snapshot title/image/slug/event, demo provenance, winning amount/currency, result/order times, payment state, fulfillment status/version, selected method and final total only when paid. They exclude contact/address, receipt identifiers/details, credentials and catalog private notes. Owner collection rejects owner-selection parameters. Admin status accepts `all|awaiting_payment|paid|preparing|shipped|ready_for_pickup|completed`; method accepts `all|shipping|pickup|unselected`. Filters combine with AND; absent means `all`, malformed/duplicate/unknown query fields produce 422. UI filter changes use native URLs; `view` belongs only to the document URL and is not forwarded as an API filter. An unpaid order is `unselected` regardless of its offered methods.

Reuse the account namespace's session/404 behavior. Scope guards over both exact admin collection and its subtree, including unsupported private paths, before dispatch. Use the existing document gate on embedded `/admin/*` and the existing capability cookie on every new admin API. Capability-only access to account routes stays 401; commerce roles without capability cannot administer orders. This narrow exception replaces the earlier blanket prohibition on capability purchase visibility, without granting profile/watchlist/participation reads or payment.

Admin POST accepts only `{expected_version, next_status, shipment?}`. `shipment` is exactly `{carrier, tracking_reference}` and is required only on preparing → shipped. Validate positive integer version, method transition and trimmed/control-free Unicode text (120/200 code points). Reject shipping fields on every other transition. There is no arbitrary notes, contact edit, method edit or tracking-correction endpoint. Render shipping values as inert text. Use strict object JSON, an 8 KiB limit, `X-AuctionLab-Admin: 1`, existing same-origin checks and no CORS bypass. Return 400 malformed/unknown body fields, 401 access, 403 origin/header, 404 missing, 409 stale/invalid transition, 415 non-JSON, 422 field/query validation, 503 unavailable/unknown commit. Never echo raw inputs in errors.

### 4. Serialize one deliberate fulfillment transition

Use the leased writer and existing order transaction helpers. Lock the purchase/progress consistently, require confirmed payment and matching receipt, compare expected version, derive allowed next state from the stored method, then validate shipment. Update progress and insert its event in the same transaction. Allocate effective time from the server, bounded to at least the predecessor event time so a wall-clock correction cannot reorder history; versions break equal-time ties.

A stale/duplicate submission returns 409 even if its target matches current state. It cannot skip to the next stage or append a duplicate event. A durable reload establishes the accepted state after a lost response. The UI disables controls while pending; an unknown outcome disables further mutation until a successful reload, then requires another deliberate action from the current version. It never retries automatically on login, focus, navigation or reconnect.

Alternative: separate idempotency UUID storage adds little value for the demo's forward-only, versioned transitions. Compare-and-set plus conflict/readback provides at-most-once advancement and explicit recovery with fewer durable intents. Payment keeps its existing stronger intent/replay contract.

Timeline derives the order-created entry from `created_at` and displays original auction close/result times separately, preserving historical adoption semantics. It joins the initial stored payment event and later operator events in ascending version order. Owner/operator detail use one consistent committed snapshot so receipt, status, version and timeline agree.

### 5. Extend existing private UI lifecycle patterns

Add `orderDetail` routing for `/account/orders/<32 lowercase hex characters>`, accepting one trailing slash and ignoring query/fragment for route identity. Reuse safe authentication returns; reject extra segments and malformed IDs. Keep `/admin/orders` as the workspace route with `?view=<id>&status=...&method=...` selection rather than introducing an admin detail path.

Implement account history/detail and admin list/detail using existing vintage components, money formatters, EN/VI copy and explicit loading/empty/access/not-found/unavailable states. Add DTO validators for stage/method/version/timeline consistency; do not relax existing receipt or owner checks to accept new data. Store contact/address and unsaved shipment text only in memory. Language changes retain authorized input; logout/account change, admin 401, pagehide/restoration and selection changes clear obsolete private detail, cancel work and suppress late results. Refresh/focus can reload committed state; no order SSE is needed for this demo.

The admin view shows immutable purchase/contact data and only the next permitted action. Unpaid orders provide no fulfillment control. Shipping metadata is entered only before shipment; completion explicitly records simulated delivery/collection. Account detail is read-only except its link to the existing unpaid checkout. No code/storage detail appears in customer flows.

Alternative: extending public auction SSE for order updates would mix private operational data into a public protocol. Explicit private reload and focus refresh satisfy the shared-order demo without that additional boundary.

### 6. Prepare and reset only tool-marked disposable schemas

Add three mutually exclusive maintenance modes to `cmd/server`:

```text
--prepare-demo --demo-schema auctionlab_demo_<suffix> --seed-at <RFC3339>
--reset-demo --demo-schema auctionlab_demo_<suffix> --confirm-demo-schema <same-name> --seed-at <RFC3339>
--demo-status --demo-schema auctionlab_demo_<suffix>
```

`DATABASE_URL` supplies the database. The explicit validated flag is the sole schema target for these modes; an absent flag never falls back to `DATABASE_SCHEMA` or `auctionlab`. Ordinary runtime still uses `DATABASE_SCHEMA`. Reject incompatible import/verify/account-seed/catalog-seed/tunnel flags and stray mode-specific options before opening a write target. Preserve existing maintenance behavior.

Preparation requires an unused target with the dedicated prefix; take its schema-scoped writer lease and inspect it before migration/marking. Refuse unknown objects, existing application content, an import marker or an unrelated schema. A matching marked prepared target returns `already_prepared` and committed generation/reference without reading password inputs or rebasing dates. For a new target, validate seed reference/media/passwords, migrate, then atomically insert marker, accounts and draft catalog content. A failed first seed can leave an empty migrated schema, but never a committed marker or partially seeded data.

Reset requires prefix + stored matching marker + exact confirmation + writer lease. Validate fresh credentials and explicit seed reference before clearing anything. All data in this dedicated schema is disposable, including manually authored records or purchases created during the rehearsal; demo booleans do not select individual records. Update the marker to a newly generated generation identifier in the same clear/reseed transaction.

Clear only explicitly enumerated application tables, schema-qualified, without `CASCADE` or schema dropping: fulfillment events/progress, payments/orders, results/bids/auctions, watchlists/sessions/accounts/account-seed markers, lot images/event membership/lots/events/catalog-seed markers, retained game runtime/sealed bids/bids/events/game/players/items, and any empty import-run bookkeeping. Preserve `schema_migrations`, `server_config`, `catalog_categories` and `demo_environments`. Refuse an actual import completion marker and unexpected table/dependency layouts instead of broadening the deletion set. Never delete local media. Cross-schema references cause refusal/transaction rollback rather than cascade. Preserve current sequences; new account/order identities avoid reusing private purchase IDs.

Refactor existing account/catalog seed preparation into reusable transaction-aware helpers so clear, seeds and generation update use one transaction with no nested writer acquisition. Existing standalone seeds remain repeat-safe and keep their old scope. New/reset seeds use the same 18 lots/three events and schedule offsets of +24/+48/+72 hours, first close +1 hour and 60-second staggering. Operators edit draft schedules for a short live demo before publication; no clock override or direct result/payment injection is shipped.

`--demo-status` uses a genuinely read-only connection/path, does not migrate or create missing schemas, and returns only target, marker format, generation, seed reference and preparation time. Use it before/after reset and after an uncertain response. Schema ownership, statement timeouts and commit-before-success still apply; lost ownership requires a fresh process. No normal startup or HTTP handler invokes reset. The existing game reset still preserves commerce everywhere.

Alternative selected against: deleting labeled demo rows inside an operational schema would require ownership rules across mixed bidder, publication, result and purchase relationships. The user chose a whole disposable schema so reset has one auditable target and no inferred ownership.

## Risks / Trade-offs

- [Capability now reveals operational purchase details] → Guard every dedicated API, keep summaries minimal, test namespace isolation, document the exception and never reuse buyer authorization for operator reads.
- [Payment and fulfillment projections drift] → Initialize in the payment transaction, backfill migration atomically, validate matching receipts and read consistent snapshots; fail unavailable on corrupt state.
- [Lost transition/reset responses] → No optimistic success; inspect durable order version or environment generation before a new deliberate action.
- [Accidental reset of valuable rehearsal data] → Explicit disposable creation/marker, exact target confirmation, stopped writer, documented whole-schema application-data scope, backup guidance and no cascading deletion. The tool cannot determine whether someone later placed valuable records inside a deliberately disposable environment.
- [Shared operator capability limits attribution] → Timeline labels actions as operator/system; no claim of an individual audit trail.
- [Manual shipping data cannot be corrected after shipment in this scope] → Validate and review inputs before deliberate shipment; later correction/return workflows require a separate change.
- [Small collections read all matches] → Keep summaries bounded per record and use demo-scale data. Pagination is deferred until dataset size justifies it.
- [Accessibility evidence incomplete in older archives] → Run actual new-flow checks, record settings and missing cases explicitly, and preserve earlier archives unchanged.

## Migration Plan

1. Stop the sole writer and back up the current schema-4 database. Run migration/upgrade checks on an owned disposable PostgreSQL 18 target first.
2. Build frontend first, then the no-CGO embedded binary. Start the schema-5 build; verify health/readiness and historical paid progress/receipts before using new operations. Startup still adopts missing winner orders and respects original schedules.
3. Rehearse both shipping and pickup using owner sessions and a separate capability-authorized operator; restart and verify history, paid checkout and timelines agree.
4. In a separate marked demo schema, exercise preparation, a full run, read-only status, reset and a new run. Verify an unmarked control schema and media are unchanged. Do not run reset against an operational schema.
5. Prefer roll-forward correction after new writes. Older binaries must refuse schema 5. Rollback requires a stopped writer, compatible binary and explicit restoration of the pre-upgrade backup, acknowledging loss of all post-backup activity. Never lower the ledger or edit old migrations.

## Validation Approach

Use isolated real PostgreSQL via `TEST_DATABASE_URL` for migration-4 upgrades, atomic payment initialization, owner/operator projection privacy, complete method paths, stale/concurrent/duplicate transitions, rejected field/body/origin requests, failure/unknown-commit recovery, restart and reset-scope sentinels. Test unsupported schemas/import occupancy and prove no-op repeated preparation. Missing PostgreSQL remains a hard failure.

Add targeted TypeScript tests for route shapes, API/DTO validation, allowed actions, timelines and obsolete private responses. Broader acceptance runs `make test`, `make test-race`, `make build`, then both Vite and embedded browser drills with direct entry/refresh/Back/Forward, two owners plus operator, long titles/shipping values, field errors, actual 200% browser zoom, keyboard and active reduced motion in EN/VI. Record requirement/task evidence under this change during apply; leave any unverified acceptance task unchecked. This proposal itself supplies no implementation/test-pass claim.
