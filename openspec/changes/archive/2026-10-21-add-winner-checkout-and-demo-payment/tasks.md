# Tasks

## 1. Schema and purchase storage

- [x] 1.1 Add migration `004_winner_checkout.sql` with orders, simulated receipts, sold-result/owner references, unique result/order/request keys, state/amount checks and unpaid-owner index; verify invalid owners, unsold references, duplicate orders/payments and invalid amounts fail in real PostgreSQL constraint tests.
- [x] 1.2 Register migration 4 and supported readiness version 4, updating existing version-sensitive schema/auction/health fixtures without weakening their original checks; verify fresh initialization, version-3 upgrade, repeat migration and failed-migration rollback preserve all prior data and migrations 1–3 are unchanged.
- [x] 1.3 Implement the explicit immutable purchase snapshot and order read mappings in `internal/order` and `internal/store/order.go`; verify persisted snapshots contain published terms and provenance but no private catalog fields, and reopening after restart/profile change preserves them.
- [x] 1.4 Extend SQLite-import target-occupancy checks for both new tables; verify an occupied target is refused without source/destination mutation and existing read-only import tests still pass.
- [x] 1.5 Document schema 4, additive upgrade, separate historical-order creation time and stop/backup/roll-forward recovery in `docs/postgresql.md`; verify instructions match startup modes and disclose no credentials.

## 2. Result-to-order lifecycle

- [x] 2.1 Add a transaction-local order-insertion helper to new sold closure in `internal/store/auction.go`; verify result/order commit together, forced insertion failure rolls back both, unsold creates none, and repeated closure preserves exactly one order and the original winner.
- [x] 2.2 Extend bounded reconciliation to adopt sold results lacking orders and include adoption in progress counts; verify more than one batch drains, historical close/result times remain unchanged, and repeat reconciliation preserves order IDs/snapshots/paid state.
- [x] 2.3 Keep startup adoption after maintenance early returns and before HTTP readiness; verify failed adoption prevents serving, while migrate/import/account/catalog seed modes create no orders or payments.
- [x] 2.4 Verify result/order writes respect writer loss, cancellation and commit uncertainty and survive retained game reset/restart; add focused PostgreSQL/startup tests proving no game revision, budget or published schedule changes.
- [x] 2.5 Document automatic order creation, existing-winner adoption and future-scheduled demo preparation in `README.md`; verify the steps do not suggest editing terminal results, resetting commerce through the game or installing credentials on startup.

## 3. Private checkout reads and review

- [x] 3.1 Implement owner-scoped unpaid-wins/order reads and deterministic ordering; verify empty/unavailable distinction, paid removal from unpaid wins, and missing/foreign order 404 versus invalid-session 401 with real session fixtures.
- [x] 3.2 Implement contact/address normalization and field validation with the design's limits and fulfillment enablement; verify Unicode boundaries, malformed email/country, shipping-only/pickup-only terms, optional address parts and pickup address discard through focused domain tests.
- [x] 3.3 Implement checked integer totals and stateless review using frozen terms; verify 12550+1500=14050, pickup/free shipping/zero wins, maximum combined amount, absent-fee failure and no persistent change from review.
- [x] 3.4 Dispatch wins/order/review routes through the existing `AccountAPI` guard and wire the order service; verify strict bounded JSON, unsupported amount/owner fields, header/origin checks, no-store responses, paid review readback and no mux collision or new order-history/admin API.
- [x] 3.5 Document private route/request/error contracts and checkout field limits alongside the order domain; verify API tests cover winner, losing bidder, commerce admin-as-owner, capability-only and game-token callers.

## 4. Simulated confirmation and retries

- [x] 4.1 Implement confirmation with writer/order locking, session recheck after waiting, and atomic contact/selection/receipt/paid fields; verify both allowed simulation methods succeed locally, expired-after-wait sessions fail and invalid methods/financial fields create no payment.
- [x] 4.2 Enforce account-scoped accepted-intent digest/replay and order-scoped one-payment semantics; verify same-intent retries after restart, altered accepted payload conflict, validation correction, accepted-ID reuse across owned orders and fresh-ID already-paid readback.
- [x] 4.3 Verify concurrent distinct/same-ID confirmations, failed transaction rollback, unknown commit and lost-response recovery against real PostgreSQL/API fixtures; assert exactly one payment, immutable paid selection/time and no false successful response.
- [x] 4.4 Document simulation-only behavior and recovery codes near the order API/domain contract; verify no external payment calls, credential fields, request bodies or checkout PII appear in logs/errors and public catalog/auction/SSE fixture-marker checks remain clean.

## 5. Frontend contracts and navigation

- [x] 5.1 Add order/wins/review/receipt request wrappers and strict response validation in `web/src/api`; verify correct owner/order/receipt identity, supported states/methods, component sums and malformed-response rejection in focused frontend tests.
- [x] 5.2 Add a checkout total formatter supporting 2,000,000,000,000 cents without widening catalog/bid input limits; verify exact cents, zero, both maxima, fractional/unsafe rejection and existing catalog money tests.
- [x] 5.3 Extend participation type/decoder and backend DTO with coherent owner order ID/state and checkout availability; verify true availability is accepted only for unpaid owned wins and public/losing responses expose no order fields.
- [x] 5.4 Add exact `/account/wins` and `/account/checkout/:orderId` routing, account navigation, titles and explicit selected-ID dispatch; verify route normalization, malformed extra segments, direct links and safe auth returns including rejection of external/admin/API/auth-loop targets.
- [x] 5.5 Document the new page/API boundaries and Change 7 previews in `web/README.md`; verify `/account/orders` and `/admin/orders` stay unavailable and current route/copy tests reflect the supported feature set.

## 6. Winner checkout interaction

- [x] 6.1 Build private unpaid wins with lot snapshots, exact amounts, checkout links and loading/empty/error recovery; verify owner changes/401/navigation cancel reads and suppress obsolete data in frontend state tests.
- [x] 6.2 Build contact/fulfillment form and server review with EN/VI labels, field errors, selected published instructions and exact price breakdown; verify switching methods updates required fields/totals, correction retains valid input and edits require a fresh review.
- [x] 6.3 Build deliberate simulation confirmation, stable in-memory request ID, pending/unknown recovery and stored paid view; verify double-click suppression, same-intent retry, lost-response readback, already-paid handling and no automatic submit on login/reconnect/reload.
- [x] 6.4 Connect won bids and private lot participation to unpaid checkout or paid confirmation, and clear all private form/mutation state on logout/account switch/page restoration; verify late reads/reviews/confirmations cannot repopulate the previous account or leak PII through URL/browser storage.
- [x] 6.5 Replace obsolete availability copy across shared/storefront/account/admin-preview notices and update `web/README.md` interaction/recovery guidance; verify EN/VI key parity, simulation labels for demo and non-demo lots, no card/bank inputs and order-history/fulfillment still explained as future work.

## 7. Browser acceptance and operator guidance

- [x] 7.1 Rehearse two real accounts competing, closure/order creation, unpaid wins, shipping checkout and paid refresh in Vite and the frontend-first embedded build; verify exact totals, owner-only access, paid removal from unpaid wins, stable receipt after restart and no game socket, using disposable owned schemas and sanitized evidence.
- [x] 7.2 Rehearse pickup-only, free shipping/zero amount, concurrent tabs/duplicate confirmation and service interruption/recovery; verify pickup saves no address, one receipt/order remains, no false paid screen appears and public browsing survives checkout failure.
- [ ] 7.3 Verify new wins/checkout/review/paid pages in EN/VI at 320/375 CSS pixels and desktop, with keyboard, long titles/field errors, actual 200-percent browser zoom and reduced motion; capture screenshots/results and leave unavailable checks explicitly incomplete rather than substituting text scaling or CSS emulation for actual zoom.
- [x] 7.4 Update active operator guidance in `README.md`, `docs/postgresql.md`, `web/README.md` and the stale schema/feature summary in `AGENTS.md` to match implemented and measured behavior; verify existing files first, preserve historical archives and Change 4/5 incomplete-check statements, and record only measured Change 6 status in `ROADMAP.md`.

## 8. Integration verification

- [x] 8.1 Run `make test` and `make test-race` with `TEST_DATABASE_URL` pointing to the documented disposable-test service; verify all required database tests execute rather than skip/fallback and retain sanitized results for migration, closure, isolation, payment and unchanged game invariants.
- [x] 8.2 Run `make build`, verify supported health schema 4 and direct wins/checkout document/API behavior in the embedded binary, and run `openspec validate add-winner-checkout-and-demo-payment --strict`; retain build/validation outputs without operational credentials.
- [x] 8.3 Write `verification/REPORT.md` mapping requirements/tasks to measured evidence, distinguishing delivered checkout/payment from future order history/fulfillment; verify all checkboxes match actual results and list any remaining acceptance limitations without changing archived evidence.

## Workflow follow-up

- Review and verify the applied change before a separately requested archive; this proposal does not implement or archive it.
