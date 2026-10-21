# Design

## Context

See `proposal.md` for motivation and scope. The following are observed in this checkout:

- `internal/store/schema.go` supports PostgreSQL version 3; `003_timed_auctions.sql` stores accepted bids and sold/unsold results with winning bid/account/amount consistency constraints. `AGENTS.md` still describes the earlier schema-2 feature boundary; implementation/operator docs already describe bidding.
- `internal/store/auction.go` serializes writes through the leased writer connection. `advanceAuction` records terminal result and state in one transaction, then returns early for terminal lots. It creates no order. `Reconcile` does bounded batches; the service drains startup reconciliation before HTTP and subsequently ticks every 250 milliseconds.
- `cmd/server/main.go` exits maintenance modes before starting auction reconciliation. `internal/api/account.go` owns the whole private account namespace, validates commerce sessions, and implements strict bounded JSON/custom-header/origin protection. A separate mux registration for `/api/account/` would collide with that owner.
- Published `catalog.Fulfillment` already freezes shipping/pickup enablement, optional shipping fee and instructions; enabled shipping requires a fee, and at least one fulfillment option is required at publication. `catalog.MaxMoney` is 1,000,000,000,000 cents per component.
- `auction.Participation.CheckoutAvailable` currently stays false and `web/src/api/auction.ts` rejects any participation response where it is true. `web/src/routes/account/Bids.tsx` shows unavailable checkout for won lots. `AccountPage.tsx` keeps orders as a preview; `route.ts` has no wins or checkout route. `safeReturn` derives its allowlist from recognized routes.
- Private-read cancellation/account identity guards already exist. Vite proxies the same Go APIs; embedded serving already supports nested SPA documents. `internal/store/sqliteimport.go` explicitly checks all known tables for target occupancy.

Earlier Change 4 and 5 archive reports retain incomplete actual-browser zoom/reduced-motion verification. This proposal does not treat those checks as passed or edit their evidence.

## Goals / Non-Goals

**Goals:** Enforce the result → owned order → single simulated receipt chain at durable transaction boundaries; support existing winners without fabricated historical activity; keep public execution data separate from private purchase data; make confirmation and retry behavior deterministic.

**Non-Goals:** A general payment platform, draft-address/profile management, fulfillment lifecycle or new order administration. No event bus, external gateway, asynchronous payment processing, payment failure simulator, or new library is needed. `/account/orders` remains the later history feature; the new checkout URL supplies the minimum paid readback required now.

## Decisions

### 1. Small commerce order domain with additive schema 4

Add `internal/order` for checkout DTOs, normalization/validation, exact pricing and service/repository contracts, `internal/store/order.go` for persistence, and private order handlers dispatched by the existing `AccountAPI`. Wire the service explicitly alongside accounts/auctions in `cmd/server/main.go`. Keep public auction and catalog response types free of purchase fields.

Migration `004_winner_checkout.sql` adds:

| Storage | Durable fields and constraints |
|---|---|
| `commerce_orders` | Random 128-bit hex ID, unique result `lot_id`, winning bid/account/amount, currency USD, immutable public purchase snapshot and close/result times, creation time, `awaiting_payment`/`paid`, optional paid checkout contact/fulfillment/fee/total/time |
| `commerce_demo_payments` | Random receipt ID, unique order ID, owner account, account-scoped unique UUID request ID, normalized intent digest, `demo-card`/`demo-bank-transfer`, currency/total, paid timestamp, explicit simulation marker |

Add a composite unique constraint to the existing result table on lot/winning-bid/account/amount and reference that tuple from non-null order fields. Existing result checks ensure such an order can reference only a sold result. Add an order ID/owner unique key for the payment's composite ownership reference. Keep old migration files byte-for-byte unchanged. Use checks for states, component/total bounds, USD, supported method and awaiting-versus-paid field shape; the transaction enforces correspondence between receipt and paid state. Index unpaid orders by owner/result-recorded time/order ID.

The purchase snapshot includes lot and event identity, lot slug/title/condition, first public image path/alt, original result/close times, all enabled fulfillment terms and fee, plus lot/event/account demo flags. Store copied public fields, not serialized private catalog records. Keep the terms immutable after insertion; paid contact and chosen method are set once and thereafter immutable through application operations. Do not expose an order update/delete endpoint.

Alternative: derive all order terms from live catalog/profile joins. Rejected because durable purchase review must retain its own terms/contact, and the roadmap explicitly requires snapshots. A generic payment-attempt ledger is unnecessary for a synchronous local success simulation.

### 2. Atomic new closure, bounded adoption of historical sold results

Factor a transaction-local order-insertion helper callable by `advanceAuction` after inserting a sold result and before committing auction state. It reads the immutable catalog projection, validates required snapshot terms and inserts the order inside the existing writer transaction. No nested writer acquisition. Unsold closure does not call it. Snapshot/order failure rolls back the new result and state so normal reconciliation retries without changing the winner.

Extend `AuctionRepository.Reconcile` with a bounded batch of sold results missing orders, using the same transaction-local helper. Include adopted rows in its progress count so startup drains all batches before accepting traffic. An existing terminal result remains immutable; historical order `created_at` is the adoption instant, distinct from original `closes_at` and `recorded_at`. Existing orders are never rewritten. The unique result key is the last duplicate barrier. Background ticks use the same bounded pass, without a second scheduler.

Offline migration/import/catalog/account seed modes install schema when appropriate but return before adoption. A failed adoption prevents normal startup rather than exposing winners who silently lack payable orders. Do not rebase published schedules, fabricate bids, or automatically pay any historical winner.

Alternative: create orders when `/account/wins` is viewed or rely only on a separate asynchronous closure hook. Rejected because reads must remain read-only and the new result/order must not have an observable partial-success gap.

### 3. One deliberate confirmation stores all checkout details

Use these account-owned endpoints, dispatched inside the current namespace guard:

| Endpoint | Behavior |
|---|---|
| `GET /api/account/wins` | Owner's awaiting-payment orders, result-recorded time descending then ID ascending |
| `GET /api/account/orders/:orderId` | Owner order snapshot and available fulfillment totals; paid state includes receipt/contact/selected fulfillment |
| `POST /api/account/orders/:orderId/review` | Validate/normalize checkout fields and return a server-calculated review; persist nothing |
| `POST /api/account/orders/:orderId/confirm` | Validate the reviewed selection again and atomically record the single local simulated payment |

This is not the Change 7 history API: no `GET /api/account/orders` collection, admin read/update endpoint, timeline or fulfillment mutation is introduced. Reviews have no durable side effect; users can correct the current in-memory form until payment. Reload discards unsent personal data and retrieves committed order state. A paid order always renders its stored confirmation instead of a mutable form.

The review/confirm input uses `contact` (name, email, phone), `fulfillment_method` (`shipping` or `pickup`), `shipping_address` when shipping, and `payment_method`. Confirm additionally requires `request_id` (the same lowercase UUID format as bids). Reject all price/currency/owner/status fields. Bound JSON at the existing 8 KiB limit; enforce UTF-8, reject unknown fields, null/malformed bodies and control characters in text. Trim surrounding whitespace; lowercase email for normalized comparison without modifying the account. Limits in Unicode code points: name 120, email 254, phone 40, each address line 200, locality/region 120, postal code 32. Require a syntactically valid contact email and a two-letter uppercase country code; do not perform email delivery or geographic/carrier validation. Shipping requires line 1/locality/country; line 2/region/postal are optional, to avoid imposing a single country's address format. Pickup normalizes address to absent and stores none. The review echoes normalized fields for explicit inspection and field errors are keyed to controls.

Use the order snapshot for `fee`: published flat shipping cents for shipping, zero for pickup. Both components allow `[0, 10^12]`; checked addition allows `[0, 2*10^12]` for the total, which remains exactly representable in existing frontend integer arithmetic. Keep the bid/catalog per-component limit unchanged. Missing/corrupt enabled shipping fees are unavailable errors, never zero. GET supplies totals for each available method; review and confirm recompute them. No tax/premium/discount, geography multiplier or external quote. Confirmation does not trust any reviewed/client total, so no signed quote token is needed for immutable terms.

`web/src/lib/catalog-money.ts` explicitly throws above the component limit, including during display. Add an order-specific validated total formatter and response decoder supporting the combined limit, preserving the current catalog/bid parser guards. Extend the existing participation type/decoder together with the backend DTO so a real checkout-available response is accepted only with coherent owned order ID/state. Reject malformed paid receipts, mismatched order IDs and totals that do not equal their validated components.

Alternative: persist a draft checkout with address-update endpoints and optimistic revisions. Rejected as unnecessary for this demo; storing selection and payment in one transaction avoids stale-contact and competing-draft state.

### 4. Idempotency at both order and accepted-intent boundaries

Acquire the existing schema writer, then lock the order row and recheck the session using current server time after waiting. Owner predicates apply to reads and mutations; invalid session is 401, unknown/foreign order is indistinguishable 404, including commerce admin accounts. Reuse account JSON/header/origin checks and `Cache-Control: no-store`; do not register a competing namespace handler. Return 422 validation, 409 accepted-ID/payload conflict, 503 unavailable/commit-unknown as appropriate, with sanitized codes and no raw bodies or database details.

Normalize the validated intent, including order ID/contact/method/normalized address, and hash its deterministic serialization. Inside confirmation:

1. Resolve current owner and check for an already accepted account/request ID. Matching digest returns its stored receipt; different normalized content/order conflicts, even if the target order is already paid.
2. If this is a new ID and the owned order is already paid, return the existing receipt with `already_paid: true`; do not reserve this new ID or change selection.
3. For an unpaid order, validate enabled fulfillment and contact, recompute exact totals, insert its unique payment and update contact/selection/paid fields in the same transaction. Sample one server paid timestamp. Return success only after confirmed commit, identifying whether the result was a replay.

Validation failures reserve no ID. Database uniqueness on order and account/request ID backs application serialization. A new UUID cannot force a second payment; exact replay after restart returns the same durable receipt. Use existing sanitized uncertain-commit classification. Do not mark unknown commits as failed/unpaid. An authenticated GET retrieves paid confirmation if commit succeeded; otherwise the UI permits a deliberate retry of its retained in-memory intent with the same ID. After reload, reconcile through GET before enabling a fresh deliberate intent. Never auto-submit on login, reconnect, navigation or retry of a read.

Alternative: disable the button as the only duplicate guard or key solely by request ID. Both fail duplicate tabs or newly generated IDs; uniqueness per order is essential.

### 5. Private account pages and accurate public links

Extend `route.ts`, `App.tsx` titles, account layout navigation and `AccountPage` dispatch for `/account/wins` and exact single-ID `/account/checkout/:orderId`. Pass the selected order ID explicitly rather than guessing it from a lot slug. Use native links, preserving trailing-slash/query rules and not-found behavior. `safeReturn` recognizes these local destinations through the updated route matcher; it still rejects external/admin/API/asset/auth-loop targets. Embedded SPA fallback already handles them; verify its cache policy instead of changing it without need.

Extend private participation DTOs with nullable owned order ID/state and set `checkout_available` from an existing awaiting-payment order. Won views link to checkout while unpaid and confirmation while paid. The public lot panel obtains that link only from private participation, never from public SSE or catalog data. `/account/wins` contains just unpaid orders; after confirmation it refetches and omits the paid order. Preserve `/account/orders` and `/admin/orders` as clearly explained Change 7 previews.

Use the vintage components and current account store/read guards. Bind reads/forms/mutations to current account ID and selected order, abort on changes, discard late results and revalidate on page-show/session recovery. Keep contact/address and confirmation intent only in component memory; do not put them in URL, local/session storage, analytics or console logs. Logout/account switch/401 clears them, and Back/Forward restoration must not reveal prior account data. A language switch preserves the current owned form while updating EN/VI interface labels; server editorial text remains authored.

Flow: owned order → form → server review → deliberate simulation confirmation → stored confirmation. Changing any reviewed input returns to edit/review and invalidates the unsent intent. During pending/unknown confirmation, disable competing submit/edit paths until durable reconciliation resolves the order. Announce actionable status/errors, move focus to review/confirmation headings or error summary deliberately, avoid motion dependence, and offer read recovery when the service fails.

Update shared footer/account/admin-preview/auction notices so they distinguish simulated payment availability from future order history/fulfillment. Display a simulation notice even for non-demo published objects; record provenance and payment simulation are different facts.

## Risks / Trade-offs

- [Order snapshot failure can block sold closure or startup adoption] → Published terms are already immutable and publication-validated; validate again, fail atomically and report sanitized actionable diagnostics. Never synthesize missing purchase terms.
- [Separate checkout snapshot duplicates public content] → Freeze a small explicit DTO, avoid private catalog serialization, and verify it against known published terms.
- [Large historical adoption extends startup] → Process indexed batches of 100 using the existing drain loop; no traffic before completion. No production-scale recovery guarantee is claimed.
- [Lost response leaves the user unsure] → Unknown outcome plus authenticated readback/same-intent retry; paid GET is authoritative and no second payment is possible.
- [PII remains in the app database] → Collect only the selected method's necessary fields, store once at payment, protect every read, omit bodies from logs and verify public payloads contain none of the fixture markers.
- [Actual-browser accessibility checks were previously incomplete] → Require fresh keyboard/mobile/EN/VI, actual 200-percent zoom and reduced-motion evidence for these new pages; report any unavailable environment honestly without marking those tasks complete.

## Migration Plan

1. Use a disposable owned PostgreSQL schema via `TEST_DATABASE_URL` to test fresh initialization, version-3 upgrade with real sold/unsold results, repeat migration and forced migration-4 failure/rollback. Update fixed version expectations in existing migration/health tests, preserving their original invariants.
2. Ship migration 4 plus schema/readiness support 4. Preserve migrations 1–3 and all existing data. Extend importer target-occupancy checks and test maintenance early returns create no orders/payments. Do not run against operational schemas during acceptance.
3. On normal startup, acquire the existing writer lease, migrate, then run auction/result-order reconciliation after maintenance-mode returns and before HTTP. Historic sold results receive unpaid orders with distinct current creation timestamps. Start existing background ticks only after successful drain.
4. Run two-account bidding → sold result/order → delivery or pickup review → simulated payment → paid refresh in Vite and a frontend-first embedded binary. Check concurrent/repeated confirmation, isolation, restart, unknown response recovery and unchanged public/game projections.
5. Update `README.md`, `docs/postgresql.md`, `web/README.md`, the stale runtime summary in `AGENTS.md`, and the roadmap status only with measured apply/verification results. Keep archive evidence and earlier incomplete checks unchanged.

Rollback is an operational stop-and-restore decision, not a destructive down migration. Older schema-3 binaries correctly reject version 4. Before deployment, retain a recoverable version-3 backup; after payment records exist, prefer roll-forward fixes or restore only with explicit acknowledgement that newer simulated purchase data would be lost. No automatic downgrade, deletion, reset or replay of old published schedules is part of this change.
