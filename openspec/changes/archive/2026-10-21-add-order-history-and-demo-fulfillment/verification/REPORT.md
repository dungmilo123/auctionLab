# Change 7 implementation and acceptance evidence

Date: 2026-10-21, Asia/Bangkok. Change: `add-order-history-and-demo-fulfillment`, schema `spec-driven`.

Implementation is present for schema 5, owner order history/detail, capability-authorized order inspection/fulfillment, and isolated prepare/status/reset maintenance. Acceptance is **39/41 tasks** when the task ledger is reconciled: 7.5 and 7.6 remain unchecked. Actual 200% browser zoom and an active reduced-motion preference could not be established with the available in-app browser controls. The remaining full accessibility/recovery matrix is therefore not accepted. Earlier archived gaps remain unchanged.

## Automated verification

| Command/check | Result | Evidence |
| --- | --- | --- |
| `make test`, with dedicated `TEST_DATABASE_URL` | PASS: all Go packages, 34 frontend files / 543 tests, lint without warnings | `make-test.log` |
| `make test-race`, same isolated PostgreSQL 18 service | PASS | `make-test-race.log` |
| frontend-first `make build` | PASS: TypeScript/Vite then `CGO_ENABLED=0` Go binary | `make-build.log` |
| `openspec validate add-order-history-and-demo-fulfillment --strict` | PASS | `validate-change.log` |
| `openspec validate --specs --strict` | 8 pass, 5 fail solely on existing requirement-length warnings | `validate-specs.log` |
| migrations 1–4 / main specs / archived evidence unchanged | PASS: corresponding `git diff --exit-code` checks | unchanged tracked sources |
| `git diff --check` | PASS | final review |

Tests used a newly created loopback-only PostgreSQL 18 Docker service and owned disposable fixture schemas. They never used the operational database, silently skipped required PostgreSQL tests, or substituted SQLite. SQLite fixtures were limited to the existing read-only importer tests. Connection credentials, passwords, capability values and session tokens are omitted from this report and its JSON evidence.

Main-spec strict failures are pre-existing in unchanged `auction-discovery`, `auction-event-publishing`, `catalog-administration`, `postgresql-persistence`, and `website-navigation`: their requirement paragraphs exceed 500 characters. No main spec or archive was rewritten to conceal these warnings. This apply operation does not sync or archive specifications.

## Requirement-to-task evidence

| Capability / requirement | Tasks | Implementation and verification |
| --- | --- | --- |
| Versioned PostgreSQL schema, immutable prior migrations | 1.1, 1.3 | `005_order_fulfillment.sql`, schema version 5; `TestSchema`, upgrade/rollback/newer-version tests, health tests; migrations 1–4 have no diff |
| Historical paid progress and rollback | 1.2 | `TestFulfillmentMigrationBackfillAndRollback`: shipping and pickup, original payment time, later creation/adoption time, unpaid order without progress, receipt/purchase equality, failed backfill retains ledger 4 |
| Import occupancy includes purchases/progress/demo marker | 1.4 | `TestFulfillmentImportOccupied`, `TestDemoRejectsUnmarkedAndImportOccupancy`, existing empty-target import/verification tests; source bytes unchanged |
| Stopped-writer upgrade, backup and compatible rollback | 1.5 | `docs/postgresql.md`; documented version-5 readiness and loss of post-backup writes on restore; real backup taken before the isolated reset |
| Owned history, deterministic summaries, empty/failure distinction | 2.1, 3.1 | `History`, exact collection dispatch; `TestOrderHistoryOrderingAndFailure`, fulfillment stage tests, two-owner/API rejection tests; unpaid method/total null and summaries exclude contact/receipts |
| Consistent detail and durable ordered timeline | 2.2 | shared repeatable-read projection; `TestFulfillmentPathsAndPaymentRollback`, migration tests; inconsistent/missing events fail unavailable; byte-equivalent owner/admin JSON |
| Atomic one-payment transition and unchanged paid semantics | 2.3 | initialization inside `Confirm`; injected initial-event failure rolls back receipt/contact/state; concurrent confirmations and accepted/fresh paid retries preserve later progress |
| Method-specific forward progression and manual shipment | 2.4 | shipping versions 1–4, pickup 1–3; invalid/unpaid/skipped/method/terminal steps rejected; `TestShipmentBounds`, complete repository paths, live carrier-error/correction drill |
| Atomic versioned fulfillment / unknown commit / lease / restart | 2.5 | concurrent identical submissions advance once; event failure rolls back update; time clamps to predecessor; stale lease never reacquired; unknown-commit HTTP wrapper and live proxy drill; restart digests identical |
| Payment-versus-fulfillment contract and attribution limits | 2.6 | `internal/order/README.md`: original payment state remains paid, system/payment/operator sources, shared capability is not a named-person audit |
| Dedicated operator authorization and namespace separation | 3.2 | guarded exact collection and subtree registration in server; `TestAdminOrderHTTPBoundaryAndHistory`; capability-only account access 401, account-role-only admin access refused, foreign account detail 404 |
| Protected request boundary / query filters | 3.3 | strict 8-KiB object decoder, same-origin/admin header; 400/403/404/409/415/422/503 coverage, unknown/duplicate filters, unsupported identity/contact/financial fields, no rejected-input mutations |
| Private no-store projections and durable recovery | 3.4 | existing checkout privacy suite plus new admin/unknown tests; contact/address/receipt markers absent from summaries and public projections; `orders-before-restart.json`, `orders-after-restart.json` |
| API/operator documentation | 3.5 | `web/README.md`, `internal/order/README.md`: exact namespaces, AND filters, allowed status/method values, errors, text bounds and request boundary |
| Exact routes / safe authentication returns | 4.1 | lowercase 32-hex order detail route, one trailing slash, query/fragment route identity; malformed/uppercase/extra segments rejected; targeted route/return tests |
| Stored purchase detail, simulated receipt and timeline navigation | 4.2 | `OrderHistory.tsx`, shared `OrderInspection`, exact money; SSR tests for unpaid, shipping/pickup/terminal and inert authored text; live history/checkout/receipt links, empty and foreign-order recovery |
| Owner-bound API and additive progress validation | 4.3 | summary ordering, amount/method/stage/version/timeline/receipt validation in `fulfillment.ts`; malformed/foreign records rejected; original checkout validator tests continue passing |
| Accessible private navigation / identity lifecycle | 4.4 | existing account provider and cancellable private reads; identity/checkout-session tests; live account switch removes prior contact/receipt; native keyboard history link, Back/Forward/direct/new-tab restoration; login never replays payment |
| EN/VI buyer copy and honest discovery/management notices | 4.5 | buyer/history/detail dictionaries, checkout and wins links, active account/discovery/catalog/site notices and docs; parity tests and SSR escaping; no obsolete Change-7 preview copy in active notices |
| Filterable operator workspace | 5.1 | `/admin/orders` native query form with status/method AND semantics and independent `view`; parser/API tests and live filter/empty/selected-order inspection |
| Immutable inspection / next-step controls | 5.2 | shared inspection; only next paid method step offered; shipment review before shipped; paid contact/price/receipt unchanged in both complete paths; terminal orders have no next control |
| Pending, field, conflict and unknown recovery | 5.3 | pending ref prevents double-submit; live 121-character carrier rejection then exact 120/200 success; locale switch preserves values; fault proxy hides a confirmed commit, fails recovery reads, then permits durable recovery; stale conflict requires reload |
| Selection/access/restoration response guards | 5.4 | abort/generation checks in workspace; live 401 clears contact/receipt and controls; delayed obsolete detail and accepted mutation responses do not replace newer selection; gate recovery does not submit transitions |
| EN/VI operator controls and guidance | 5.5 | localized status/timeline/field/action labels, associated errors, text-only carrier/reference; overview links and API/operator docs; no frontend secret input or private-data browser persistence |
| Explicit isolated maintenance dispatch and validation | 6.1 | `demoOptions`, early demo dispatch; missing target/confirmation, incompatible or stray options refused; mode exits before game/HTTP/lifecycle services |
| Transaction-aware repeat-safe seeds | 6.2 | `PrepareSeed`, `seedAccountsTx`, `SeedTx`; original standalone account/catalog tests plus maintenance drill; passwords supplied by operator; no nested writer acquisition |
| Explicit disposable preparation | 6.3 | eligible unused target only; marker/accounts/18 lots/three draft events in one transaction; no winners/payment; repeated preparation reads no credentials and preserves original reference |
| Explicit bounded atomic reset | 6.4 | prefix/marker/exact confirmation/leased writer; explicit schema-qualified table allowlist, no CASCADE/drop/identity restart; old sessions removed, fresh account IDs; migrations/categories/capability configuration preserved |
| Failure/unknown reset recovery | 6.5 | pre-commit reseed/media failure rolls back prior sessions/data/generation; committed generation readback demonstrates response-loss recovery without a second reset; commit-uncertainty classification retained |
| Genuinely read-only generation status | 6.6 | read-only transaction without migration/initialization; missing target causes no schema creation, no credentials or purchase data returned; CLI repeat/status tests |
| Unsafe layouts / ownership / control scope | 6.7 | `TestDemoRefusesUnsafeLayouts`: unknown tables/views/sequences, routines/triggers, cross-schema references, import marker, unmarked/occupied refusal; exclusive writer tests; control/media sentinels; retained-game reset preserves fulfillment |
| Operator replay guide and command drill | 6.8 | README/PostgreSQL prepare/status/reset/restart/publication commands; actual isolated prepare/repeat, stopped backup/reset/status, explicit runtime schema, fresh publication/bidding/payment/fulfillment |
| Integrated verification | 7.1–7.4 | suite/build logs, two browser journeys below, restart digest comparison, generation/reset/media audit, fresh normal-API journey |
| Full actual-zoom/accessibility matrix | 7.5 | **OPEN**: representative keyboard/EN/VI/320/375/desktop and maximum shipment field checks passed; actual 200% browser zoom and the complete long-title/recovery matrix not established |
| Active reduced motion and complete delayed private-state matrix | 7.6 | **OPEN**: read preference was false, no active preference override available; identity/access/obsolete-read and unknown recovery cases tested, but full active reduced-motion/delayed-mutation matrix not accepted |
| Evidence and honest active status | 7.7 | this report, JSON/screenshot evidence, task ledger, active README/web/ROADMAP reconciliation, strict validation results with unchanged-main-spec warnings disclosed |

## Live Vite shipping journey

An explicitly prepared `auctionlab_demo_change7_acceptance` schema was used. Draft event 1 was assigned future real server times and published through normal catalog APIs. The Vite storefront (`127.0.0.1:5173`) navigated homepage → Analog Memories → Folding travel camera. Demo bidder one bid $125.50; after logout/login, demo bidder two bid $130.50. Server closure created the winner's order; history showed awaiting payment with unselected method/total.

The winner followed history detail → checkout, reviewed fictional contact/address and shipping, and deliberately confirmed a local simulated payment: $130.50 + $12.50 = **$143.00 USD**. The original receipt remained unchanged through preparation, shipment and completion. Operator capability was obtained through the separate Go document gate; Vite admin filtering and selected-order details loaded through guarded APIs.

Carrier length 121 was rejected with an associated error and no advancement. Correcting to 120 and retaining a 200-character reference survived EN → VI switching; review and confirmation stored shipment once. Shipping ended version 4 with five timeline entries (creation, payment, preparation, shipment, completion). Buyer readback matched operator data. A foreign logged-in account saw not-found, with previous contact and receipt absent.

## Embedded pickup, recovery and restart

The frontend-first embedded binary served Objects of Time. Two accounts bid $169.00 then $174.00 for the field watch. Server closure created a new owner order. Pickup checkout used simulated bank transfer, zero delivery fee and **$174.00 USD** total. No shipping address or second payment control was offered.

A temporary loopback test proxy (`fault-proxy.py`, never part of production startup) forwarded a real accepted pickup transition but replaced its response with 503/commit_unknown. The UI kept outcome unknown and disabled another mutation. A failed subsequent read preserved the unknown notice and removed private detail; successful reload revealed ready_for_pickup and required a new deliberate action. A separately accepted completion made the cached submission stale: the UI showed conflict, disabled mutation and reloaded terminal state. Capability denial cleared contact and receipt. A delayed prior-order read did not replace a newly selected shipping order. After reset, a real accepted transition on the secondary paid order was delayed by the proxy while navigating to the newly completed order; the prior contact and response did not overwrite the new selection.

Pickup ended version 3 with four timeline entries. Direct entry, refresh, a new tab, keyboard link activation and Back/Forward worked. Both application and the isolated PostgreSQL container were restarted. Owner/admin JSON digests for both complete orders were identical before and after restart; paid checkout remained read-only. See `orders-before-restart.json` and `orders-after-restart.json` for IDs, receipt IDs, versions, timeline counts and hashes. Public catalog/execution/history responses excluded the synthetic private contact/receipt markers. The website continues to mount no retained game UI; its lot feed uses commerce SSE, and existing route/stream tests cover the game boundary.

## Reset and new rehearsal

After the completed browser demonstrations, the writer stopped and a custom-format schema backup was taken outside the repository. Explicit CLI reset was supplied its exact marked target, confirmation, fresh reference and operator passwords. Generation changed from `c4243b9606828aefbb40e5ab6a072db5` to `fca72defc58825a6d5e9a5bba94c8377`. Prior orders/payments/progress/sessions were erased, with three fresh accounts and 18 lots/three events reseeded. Old browser cookies returned login-required and could not restore their erased contact/receipt.

The control schema sentinel and 51 local media files retained identical values/content hashes. Migration ledger/category/capability configuration were preserved. In this drill the capability was pinned by operator environment and had no stored server_config row before or after; repository tests separately verify preservation of an existing persisted row. See `reset-audit.json` and the three demo-generation JSON files.

The replacement generation was explicitly started, its draft schedule edited/published, and two accounts placed real normal-API bids. Real server closure, owner simulated pickup payment and both operator transitions completed again, with a fresh private order/receipt and matching owner/admin readback. No fake production clock, direct winner/payment insertion or automatic publication was introduced. See `fresh-run-start.json` and `fresh-run-complete.json`.

## Browser settings and remaining acceptance

The available surface was the Codex in-app browser. Desktop width measured 1265 CSS pixels. Explicit viewport overrides of 320×850 and 375×850 produced document client widths 305 and 360 respectively (15-pixel scrollbar); scrollWidth matched clientWidth in the checked admin shipment and buyer timeline views. EN/VI labels, original authored content, long 120/200-character shipment values, field errors and keyboard Tab/Enter were checked. Screenshots in `screenshots/` record the selected cases; screenshot names distinguish desktop from actual narrow overrides.

`devicePixelRatio` measured 1. Sending the browser zoom shortcut did not change measured scale/width. `matchMedia('(prefers-reduced-motion: reduce)').matches` measured false. Advertised capabilities provided viewport and visibility only, so neither actual 200% zoom nor active reduced motion was confirmed. Responsive viewport results and static CSS are not substitutes. Tasks 7.5 and 7.6 remain open, including any unmeasured parts of their full matrix. Existing Change 4–6 archived acceptance gaps remain untouched.

The apply phase performed no commit/archive or operational reset. The subsequent explicit commit-and-archive request is recorded in `../ARCHIVE.md`; it preserves the 39/41 acceptance boundary.

Temporary Vite, application and fault-proxy processes were stopped after verification. The agent-created PostgreSQL test container `auctionlab-change7-test` was stopped and retained for optional further manual acceptance; it is separate from operational PostgreSQL.

## Archive validation follow-up

At the subsequent explicit archive request, three new main specs and seven updates were synchronized. Existing purposes/unmentioned requirements were preserved and all ten deltas compared cleanly after merging (`spec-sync.json`). Normal main-spec validation now passes 16/16. Strict main-spec validation is 8 pass / 8 fail solely because of long requirement descriptions, including additional warnings exposed by the newly synced MODIFIED text (`validate-specs-after-sync-strict.log`). The earlier 8/13 result in the apply table is the pre-sync measurement, not the archive result. Strict change validation still passes. No wording was shortened solely to suppress those warnings, and tasks 7.5–7.6 remain unchecked.
