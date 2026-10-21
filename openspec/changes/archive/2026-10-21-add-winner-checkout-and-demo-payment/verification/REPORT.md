# Change 6 implementation verification

Measured on 2026-10-21 (Asia/Bangkok). Change: `add-winner-checkout-and-demo-payment`, schema `spec-driven`. Implementation is delivered with **35/36 tasks complete**. Task **7.3 remains incomplete**; this is not full accessibility acceptance. The change has not been archived or committed.

## Delivered behavior

Sold commerce closure commits a single winner-owned order and immutable purchase snapshot with its result. Historical sold results are adopted in bounded startup reconciliation before HTTP readiness, preserving original result/close times and recording a separate creation time. Unsold results create no order. Offline migration/import/account/catalog maintenance does not perform adoption or payment.

Private unpaid wins, single-order reads, stateless review and atomic simulated confirmation share the existing AccountAPI namespace/session/header/origin boundary. Contact and selected fulfillment are saved only with the confirmed payment. Shipping uses the frozen flat fee; pickup saves no address. Accepted intent IDs replay matching normalized payloads; changed accepted intents conflict, and fresh IDs cannot create another payment on an already-paid order. Unknown outcomes require owner readback or deliberate same-intent retry.

The frontend provides EN/VI wins, contact/fulfillment, review, pending/unknown recovery and immutable paid confirmation. Owner/order/lifecycle guards discard obsolete responses, suppress double submit and clear private values on logout/account change/restoration. A Vite StrictMode cancellation bug found during browser acceptance was corrected with per-lifecycle operation tokens and regression coverage. No payment credentials, gateway, order-history collection, admin order operations or fulfillment tracking were added.

## Commands and retained evidence

| Check | Result | Evidence |
| --- | --- | --- |
| `make test` with dedicated disposable PostgreSQL service | PASS; all Go packages, 534 frontend tests, lint without warnings | `test.log` |
| `make test-race` with the same dedicated service | PASS; required PostgreSQL fixtures execute, not skipped or replaced with SQLite | `race.log` |
| Frontend-first `make build` | PASS; TypeScript/Vite and CGO-disabled Go binary | `build.log` |
| `openspec validate add-winner-checkout-and-demo-payment --strict` | PASS | `openspec.log` |
| `git diff --check` | PASS | final workspace check |
| Prior migration files | Migrations 1–3 have no diff | final workspace diff |
| Embedded health | HTTP 200, ready true, PostgreSQL, supported schema 4 | `http-checks.json` |
| Deep links / unauthenticated APIs | Documents HTTP 200/no-cache; private APIs HTTP 401/no-store | `http-checks.json` |
| Rehearsal database audit | 12 sold results/orders; 4 paid orders and exactly 4 receipts; 8 unpaid; matching owner/amount/time and no saved pickup address; game revision 0 | `database-audit.json` |

Credentials, capability values, cookies, connection URLs and raw server startup logs are excluded. The PostgreSQL container is the previously provisioned dedicated test service; tests generate/drop owned disposable schemas. Browser acceptance used a separately generated `test_load_*` schema in that test database. No operational schema or existing SQLite source was used or changed.

## Requirement/task mapping

| Tasks | Measured implementation and evidence |
| --- | --- |
| 1.1–1.2 | `TestWinnerSchemaConstraints`, `TestWinnerMigrationFourUpgradeAndRollback`, `TestWinnerUpgradePreservesPopulatedVersionThree`, existing schema/health/auction upgrade suites: invalid owners/unsold result references, duplicate orders/receipts and bounds rejected; fresh/repeat migration and populated version-3 execution/account/session/watchlist/game preservation; failed migration ledger/DDL rollback. |
| 1.3 | Explicit snapshot DTO and `TestWinnerCheckoutSnapshotIsolationAndPayment`, `TestWinnerAdoptionMultipleBatchesAndRestart`: no catalog private-marker data, unchanged purchase after profile change/restart, copied lot/event/account provenance. |
| 1.4–1.5 | `TestImportPurchaseOccupied` plus existing read-only SQLite import tests: source bytes and destination purchase counts remain unchanged; both tables included in occupancy guard. Schema-4 upgrade/backup/roll-forward guidance in `docs/postgresql.md`. |
| 2.1–2.2 | `TestWinnerOrderAtomicClosureAndAdoption`, multi-batch/restart test and original auction lifecycle tests: forced order insertion failure rolls back result/state; unsold creates none; 101 historical winners drain across batches; original result times and existing IDs/snapshots/paid state preserved. |
| 2.3–2.5 | `TestWinnerStartupAdoptionFailureAndMaintenance` runs child server startup: bad historical snapshot refuses normal startup, account/catalog seed modes return with no adoption/payment. Migration/import behavior separately tested. Original auction cancellation/ownership and new confirmation writer-loss tests retain game invariants. README documents future draft preparation and automatic adoption without replay/reset of terminal commerce. |
| 3.1–3.5 | Owner order/wins reads, domain `TestNormalizationAndLimits`/`TestExactReview`, `TestCheckoutHTTPBoundariesPrivacyAndRecovery`: deterministic list/paid removal, empty vs unavailable, 401 vs shared 404, winner/loser/admin-owner/capability-only/game-token access, bounded strict JSON including nested null/unsupported financial/owner fields, origin/header checks and no-store. Unicode limits, malformed email/country, shipping/pickup enablement, optional address parts, zero/max totals and absent-fee errors covered. Contract/limits are in `internal/order/README.md`. |
| 4.1–4.4 | PostgreSQL confirmation concurrency/rollback/restart/lock-wait tests: both simulation methods, expiry after row wait, correction without reserving ID, accepted-ID cross-order conflict, immutable paid selection/time and exactly one receipt. `TestCheckoutUnknownCommitAndDurableRecovery` wraps a **real committed PostgreSQL confirmation** with a simulated lost commit response: API returns 503/commit_unknown, GET reads paid, original intent replays once. This tests uncertain-response handling, not an injected PostgreSQL wire-level COMMIT acknowledgement loss. Existing public stream projection tests plus public HTTP fixture-marker checks remain clean. No new body/PII logging or external payment code exists. |
| 5.1–5.5 | `api/order.test.ts`, `lib/order.test.ts`, existing route/return/catalog suites: owner/order/receipt/state/method/amount consistency, malformed success rejection, 2e12 total display with unchanged component guards, coherent private participation links, exact checkout routing and safe auth returns. Web/API boundaries documented; history/admin-order routes remain previews. |
| 6.1–6.5 | Private-read cancellation and `checkout-session.test.ts` verify obsolete transport results, repeated setup, identity loss, double-submit exclusion and stable retry IDs that clear on edit/close. Browser checks cover actual form/review/field correction, language preservation, paid refresh, logout and losing-account isolation. Shared copy distinguishes simulated payment from future history/fulfillment. Source audit finds no local/session storage, console or WebSocket use in new checkout modules. Embedded bundle has no WebSocket constructor. |
| 7.1 | `prepare/main.go` created two actual commerce accounts and accepted competing bids through the commerce service on published fixture terms using an explicit controlled fixture clock, then reconciled sold results/orders. The browser exercised those durable wins in Vite and embedded builds: shipping 12550 + 1500 = 14050, both simulation methods, paid refresh/removal, losing-account 404, logout clearing and unchanged receipt after real server restart. This is a fixture-clock rehearsal, not a new manual real-time auction usability study. |
| 7.2 | Browser pickup-only page omits address controls; two independently reviewed tabs converge on the identical receipt/time. Zero winning amount with free shipping confirms 0 + 0 = 0. Stopping the app between review/confirm produces unknown, no paid heading/pay control; after restart deliberate GET and same-intent retry recover. Public and private namespaces stay separate; anonymous catalog/public auction fixture tests continue to pass. Audit proves no pickup address and one receipt per paid order. |
| 7.3 | **INCOMPLETE.** Partial measured responsive/keyboard evidence below; actual browser 200% zoom, actual reduced-motion preference and the complete long-title/field-error matrix were not verified. |
| 7.4 | Existing README, PostgreSQL/web docs and AGENTS feature/schema summary updated. ROADMAP records measured delivery and remaining acceptance. Earlier Change 4/5 incomplete statements and archived evidence unchanged. |
| 8.1–8.3 | Broad tests/race/build/strict validation and this report; task checkboxes reflect the single remaining acceptance limitation. |

## Browser observations

The in-app browser exercised the running Vite frontend on port 5176, proxied to the embedded Go service on port 8176. Its temporary proxy override was process-local; the repository Vite configuration is unchanged. Screenshots contain only invented fixture identities/contact/address data.

- Vite unpaid wins → shipping review → simulated card → paid refresh, total **USD 140.50**. Paid lot disappears from wins (12 links before, 11 after). The same receipt survived subsequent server restarts.
- Embedded shipping review → simulated bank transfer → paid refresh, **USD 140.50**; two pickup tabs show the same receipt, **USD 125.50** with zero fee; free-shipping/zero win shows **USD 0.00** throughout.
- Server interruption between review and confirmation: explicit unknown state, zero paid headings and zero pay buttons. Recovery requires deliberate stored-order read and deliberate confirmation. No automatic confirmation after restoration.
- Logout leaves zero old-contact matches. Logging in as the losing fixture account at the paid URL shows “Order not found for this account,” without previous contact/receipt data.
- Server country-field validation retained the valid contact name, marked one field invalid and focused the heading/error context. Keyboard Tab from contact name reached contact email.
- Review, wins and paid screens were measured in EN/VI at actual **320/375 CSS-pixel viewports**. Document scroll widths were 305/360 respectively (scrollbar excluded), with no horizontal overflow. Desktop width 1280 measured scroll width 1265. Language changes preserved the active review.
- `browser-results.json` retains measurements. Representative screenshots: `vite-wins.png`, `vite-review-{320,375}-{en,vi}.png`, `vite-paid-375-vi.png`, `embedded-review-desktop-en.png`, `embedded-paid-desktop-en.png`, `embedded-service-interruption.png`, `embedded-pickup-duplicate-tabs.png`, `embedded-zero-free-shipping-desktop-en.png`, `embedded-field-error-desktop-en.png`, `embedded-form-375-en.png`, and mobile wins/paid images.
- Some first screenshot attempts targeted a non-active tab and retained desktop width. Those files were renamed to **desktop** based on measured dimensions; they are not mobile evidence.

## Remaining acceptance

Task 7.3 stays unchecked. The available browser exposes viewport resizing but no zoom/reduced-motion preference control; native app control is disabled in this session. A browser zoom shortcut attempt left both width **1280** and device-pixel ratio **1** unchanged, so it supplies **no actual-zoom evidence**. Viewport resizing, text scaling and CSS zoom are not substitutes. Complete actual 200% browser zoom, actual reduced-motion preference and the full long-title/field-error/keyboard matrix on wins/form/review/paid pages in EN/VI before checking 7.3.

General order history, fulfillment timelines/admin operations and reset tooling remain Change 7 scope. Payment is always local simulation, including non-demo records. This report does not alter or resolve the archived Change 4/5 acceptance gaps.

Cleanup completed: acceptance tabs/processes closed, owned rehearsal schema dropped, temporary fixture credentials removed, and the dedicated PostgreSQL container returned to its original stopped state. Saved evidence remains in this change directory.
