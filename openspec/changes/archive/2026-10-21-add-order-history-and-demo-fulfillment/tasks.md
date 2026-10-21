# Tasks

Checkboxes record verified implementation and acceptance evidence. Unverified cases remain open. Use a dedicated PostgreSQL 18 `TEST_DATABASE_URL` and owned disposable schemas; never reset an operational schema. Preserve archived acceptance limitations.

## 1. Schema 5 and upgrade safety

- [x] 1.1 Add migration 5 fulfillment/progress/event/environment structures, references, checks and history/filter indexes without editing migrations 1–4; verify fresh initialization, constraints and repeated migration against real PostgreSQL and compare prior migration files unchanged.
- [x] 1.2 Backfill paid progress/event version 1 from each existing receipt and selected method without rewriting purchases or adding unpaid/later-stage progress; verify populated schema-4 fixtures with both methods, historic adoption times, unchanged receipt/snapshot digests and injected backfill failure rollback.
- [x] 1.3 Register the migration, schema version 5 and health/test expectations; verify supported upgrades, unsupported-newer refusal, failed-migration ledger preservation and sanitized health readiness.
- [x] 1.4 Include fulfillment and demo-environment rows in SQLite-import target occupancy; verify importer refusal leaves source/target unchanged and existing empty-target import/verification still succeeds.
- [x] 1.5 Update `docs/postgresql.md` with schema-5 initialization, stopped-writer backup, historical progress, readiness and compatible rollback; verify examples match the implemented version behavior and disclose post-backup data loss.

## 2. Order history and fulfillment persistence

- [x] 2.1 Add owner history summaries with deterministic recorded-time/ID ordering and minimal fields; verify two-owner isolation, all payment/fulfillment stages, null unpaid method/total, successful empty results and failures distinct from empty.
- [x] 2.2 Add consistent owner/admin detail projections and durable timelines while preserving existing checkout DTO payment semantics; verify snapshot/receipt equality, historic result-versus-creation times, event version/time ordering and unavailable responses for corrupt required progress.
- [x] 2.3 Initialize paid progress and its initial event within `OrderRepository.Confirm`; verify injected initialization failure rolls back payment/contact/state, and concurrent confirmations plus accepted/fresh retries preserve one receipt and later fulfillment progress.
- [x] 2.4 Implement version-checked method-specific transitions and shipment normalization through the leased writer; verify both complete paths, terminal completion, unpaid/skipped/backward/wrong-method rejection, exact text bounds and unchanged purchase/payment data.
- [x] 2.5 Cover concurrent/stale/duplicate submissions, atomic update/event failure, unknown commit, clock rollback, ownership loss and restart; verify at most one version/event per accepted step, no false success or stale-lease reacquisition, and matching owner/admin readback.
- [x] 2.6 Update `internal/order/README.md` with payment-versus-fulfillment contracts, projections, timeline sources, transition versions and unknown-outcome recovery; verify documentation against repository tests and keep simulation/attribution limits explicit.

## 3. Private account and operator HTTP APIs

- [x] 3.1 Dispatch exact `GET /api/account/orders` before existing detail/review/confirm routing; verify session-owned collection, unsupported owner selection, foreign/missing 404 parity, invalid-session 401 and existing wins/checkout regression tests.
- [x] 3.2 Register exact/subtree admin order guards and list/detail/transition handlers in the server; verify capability-only operator access, account-role-only refusal, protected unknown subpaths and unchanged account-namespace ownership in both server wiring and API tests.
- [x] 3.3 Validate status/method queries and bounded strict transition JSON with the existing admin origin/header boundary; verify 400/403/404/409/415/422/503 behavior, duplicate/unknown queries, oversized bodies, unsupported financial/identity/contact fields and no mutation on rejected inputs.
- [x] 3.4 Add HTTP privacy and recovery cases using distinguishable owner/contact/receipt markers; verify minimal summaries, no-store success/errors, public catalog/history/stream nonleakage, sanitized logs/errors and durable recovery after a committed response is lost.
- [x] 3.5 Document account history and dedicated admin order API shapes, filters, auth matrix, error codes and shipping limits in `web/README.md` and the order contract; verify examples exercise the actual namespace and same-origin requirements.

## 4. Buyer history and order detail

- [x] 4.1 Extend route/detail dispatch and safe auth returns for exact lowercase-hex owner order IDs; verify direct/trailing-slash/query URLs, malformed/uppercase/extra-segment rejection and unchanged checkout/public routes with route/return tests.
- [x] 4.2 Replace the history placeholder and add stored order detail/timeline, exact money, demo labels, shipping text and unpaid checkout/paid receipt links; verify unpaid/paid/shipping/pickup/completed, empty, missing and service-failure rendering with representative valid API records.
- [x] 4.3 Add owner-bound API/DTO validation for summaries and additive fulfillment detail; verify rejection of foreign owners/IDs, inconsistent method/status/version/timeline or malformed amounts while existing paid checkout validators continue to pass.
- [x] 4.4 Apply private lifecycle guards to reads, navigation, focus/page restoration, logout and account switching; verify delayed responses cannot restore a prior owner's contact/order/receipt and login never triggers payment or fulfillment automatically.
- [x] 4.5 Add EN/VI history/detail labels and recovery copy, update checkout/won-lot links and active account documentation; verify locale-key parity, safe display of authored text, retained language selections and removal of obsolete Change 7 buyer previews.

## 5. Operator order workspace

- [x] 5.1 Replace `/admin/orders` preview with capability-authorized stored list, AND-combined status/method URL filters and `view` selection; verify direct/refresh/navigation, unpaid unselected-method filtering, invalid query recovery, empty results and missing selected orders.
- [x] 5.2 Implement immutable purchase/contact/receipt inspection, shared timeline and next-step controls for each paid method; verify unpaid/completed orders expose no invalid controls and shipment review requires carrier/reference only before shipping.
- [x] 5.3 Implement pending/field/conflict/unknown-commit recovery with durable reload and deliberate resubmission; verify duplicate clicks cannot advance twice, reload failure keeps the outcome unknown, and language changes preserve authorized unsaved shipping text.
- [x] 5.4 Clear sensitive admin detail on selection change, access loss and page restoration and suppress obsolete read/update responses; verify late responses do not repopulate cleared detail or overwrite a newer selected order, and gate recovery never replays a transition.
- [x] 5.5 Add EN/VI admin copy and accessible status/timeline/form controls, update overview links and operator/API docs; verify next-step labels, text-only shipment rendering, validation associations and no secret input/browser persistence.

## 6. Disposable demo preparation and reset

- [x] 6.1 Add explicit prepare/reset/status modes, validated demo target/confirmation/seed options and early mode dispatch; verify incompatible/stray flags, missing target and default-schema fallback are refused before writes and no service/lifecycle starts in maintenance modes.
- [x] 6.2 Extract transaction-aware account/catalog seed helpers while preserving existing seed-mode contracts; verify repeat-safe standalone seeds, original schedule offsets, collision rollback, operator-supplied password handling and no nested writer acquisition.
- [x] 6.3 Implement eligible empty-target preparation and atomic marker/accounts/draft-catalog seeding; verify 18 lots/three events, no winners/orders/payments, occupied/unmarked refusal, no default credentials and no-op repeated preparation without credential reads or date rebasing.
- [x] 6.4 Implement prefix/marker/exact-confirmation/stopped-writer reset with an explicit schema-qualified table allowlist, one clear/reseed transaction and a fresh committed generation; verify old sessions/data disappear only in that target while migrations/categories/capability configuration remain.
- [x] 6.5 Verify failure/unknown reset recovery, fresh identities and generation status using injected pre-commit failures and a lost commit response; demonstrate full rollback or an inspectable committed generation without an automatic second reset.
- [x] 6.6 Implement genuinely read-only demo status with no migrations/schema initialization; verify missing/unmarked targets cause no writes, current generation/reference/time are returned and no credentials or purchase details appear.
- [x] 6.7 Test unsafe-target, active-writer, import-marker, unknown-object/dependency and cross-schema-reference refusal with control-schema/media sentinels; verify no cascade/drop, no other-schema/media changes, retained game reset still preserves commerce and normal startup never resets/rebases it.
- [x] 6.8 Document complete prepare/status/reset/reseed/restart/publication/rehearsal commands and the erased/preserved table scope in root README and `docs/postgresql.md`; verify the drill on an owned disposable demo schema, including required passwords, backup guidance and explicit runtime schema selection.

## 7. Integrated acceptance and review evidence

- [x] 7.1 Run `make test`, `make test-race` and frontend-first `make build` with isolated PostgreSQL configured; record commands/results and distinguish existing failures from change regressions without skipping required DB coverage or substituting SQLite.
- [x] 7.2 Rehearse discovery → two-account bidding → sold closure → owner simulated payment → admin shipping completion and buyer history in Vite; verify the same committed order/receipt/timeline, owner-only access and no public purchase data/game socket.
- [x] 7.3 Repeat the full journey with pickup in the embedded build, including Go gate, direct entry, refresh, new tabs and Back/Forward; verify timeline/read-only paid checkout before and after server/PostgreSQL restart.
- [x] 7.4 Rehearse preparation → complete demo → status → explicit stopped-writer reset → new draft publication/run on a marked disposable schema; record generation changes, old-session invalidation and unchanged control-schema/media evidence.
- [ ] 7.5 Verify buyer/admin flows in EN/VI using keyboard, 320/375 CSS-pixel and desktop layouts plus actual 200% browser zoom with long titles, maximum-length shipping values, field errors, empty/access/conflict/unknown states; record exact browser settings and leave any unverified case unchecked.
- [ ] 7.6 Verify active reduced-motion behavior and private-state recovery across logout/account change/capability loss/page restoration with delayed reads/mutations; record measured settings and observable results separately from viewport emulation or static CSS inspection.
- [x] 7.7 Produce a requirement-to-task evidence report under this change's `verification/`, reconcile active README/web notices/ROADMAP status to measured implementation, and run strict change/spec validation; preserve archived gaps and report any missing acceptance instead of marking it complete.
