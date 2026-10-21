# Tasks

Planning only: every implementation/acceptance task starts unchecked. Follow `design.md` and the seven capability deltas; record actual evidence before checking a task. Earlier archived Change 4–7 accessibility gaps remain unchanged.

## 1. Interaction persistence and schema compatibility

- [x] 1.1 Add embedded migration 6 for the constrained interaction/coverage tables, source uniqueness and query indexes; raise supported schema version to 6. Verify fresh/repeated migration and newer-schema refusal with real PostgreSQL, and confirm migrations 1–5 are byte-for-byte unchanged.
- [x] 1.2 Backfill accepted bids/receipts and snapshot current watchlists with original times and explicit provenance. Verify populated 5→6 counts/bindings, no invented views or removed memberships, repeat safety and rollback on injected migration/backfill failure.
- [x] 1.3 Initialize collection coverage idempotently before normal HTTP readiness while leaving fresh unused migration targets importable. Verify startup/restart preserve collection start and the migrate-before-first-start SQLite import succeeds without changing its read-only source.
- [x] 1.4 Extend SQLite-import occupancy checks to both new tables. Verify interaction-only and coverage-only occupied targets are refused without changing source/destination data and repeated completed import retains its existing contract.
- [x] 1.5 Document schema-6 upgrade, historical coverage, startup initialization and backup-based rollback in `docs/postgresql.md`; review against the migration tests and verify health reports version 6 without credentials.

## 2. Transactional commerce interaction capture

- [x] 2.1 Add transaction-local interaction insertion and strict private record projection. Verify source-key deduplication, allowed kind/provenance combinations, derived demo flags and absence of contact/address/token/payload fields with repository tests.
- [x] 2.2 Capture only actual watchlist insertions using affected-row results. Verify duplicate saves add nothing, removal preserves history, remove-then-add creates one new event and upgraded membership snapshots remain distinguishable.
- [x] 2.3 Capture accepted bids and first confirmed simulated receipts within their existing transactions. Verify rejected bids, bid replay, checkout review, same-ID/fresh-ID paid confirmation and fulfillment changes never add duplicate or fabricated action events.
- [x] 2.4 Inject capture failure, unknown commit and writer-lease loss across watch/bid/payment paths. Verify atomic rollback, durable retry reconciliation, unchanged game/catalog facts and no nested/reacquired writer, with real PostgreSQL tests.
- [x] 2.5 Document event kinds, provenance and commit/replay behavior beside the new interaction service and in relevant commerce docs; check descriptions against the focused action-capture tests.

## 3. Optional anonymous lot-view collection

- [x] 3.1 Add `POST /api/catalog/lot-views` with strict 1 KiB body, exact lot/observation fields, interaction header and origin checks. Verify published-only lookup, malformed/oversized/unknown/invalid input, JSON/origin rejection, no-store errors and absence of cookie issuance or account dependence in API tests.
- [x] 3.2 Add confirmed-commit view acknowledgement and global observation-ID binding. Verify duplicate same-ID/same-lot submission records once, changed-lot reuse conflicts and storage/unknown commit returns no false success.
- [x] 3.3 Add the bounded direct-peer limiter at 30 attempts/minute and at most 4096 peer keys. Verify 429 behavior, expiry/capacity handling, untrusted forwarding headers and no persisted peer/request metadata.
- [x] 3.4 Add one per-lot/per-document in-memory observation ID after successful lot-detail rendering. Verify effect remounts, language changes and execution refreshes reuse it, new documents can create a new view, and failed recording causes no blocking UI or retry loop; ensure cards and GET loaders record nothing.
- [x] 3.5 Document view meaning, privacy limits, error boundaries and incomplete coverage in `web/README.md` and interaction API documentation; verify the documented request against the endpoint and inspect stored columns/log output for prohibited data.

## 4. Private auction analytics service and API

- [x] 4.1 Implement lifetime published-event/lot cohort selection and strict event/dataset filters. Verify all/demo/non-demo combinations, missing/draft indistinguishability, duplicate/unknown/empty queries, zero-cohort event rows and actor-demo separation using PostgreSQL fixtures.
- [x] 4.2 Implement independent participation/result/payment aggregates in one read-only repeatable-read snapshot. Verify the design reference fixture yields six bids, two distinct bidders, 3/4 sell-through, 35000 winning cents and 16500 simulated paid cents, reconciled with independent SQL.
- [x] 4.3 Add decimal-string counts/totals, exact sell-through operands, paid winning/fee components, receipt consistency validation and interaction provenance/coverage counts. Verify safe-integer overflow, zero values, null zero-denominator rates, rejected/replayed actions, corruption errors and concurrent-commit snapshot consistency.
- [x] 4.4 Expose no-store `GET /api/admin/analytics` behind the capability guard, including unknown subtree paths. Verify absent/invalid capability and commerce-admin-only requests fail, valid capability succeeds without an account and payloads contain no raw identities/orders/interactions or secrets.
- [x] 4.5 Document metric formulas, cohort/actor demo semantics, lifetime event attribution, source integrity and non-additive bidder counts in operator/API docs. Verify every documented metric against independent SQL and distinguish observed interaction coverage from full durable commerce history.

## 5. Deterministic lot recommendations

- [x] 5.1 Implement published/future-close/nonterminal candidate eligibility and explicit public lot projections. Verify draft, terminal, expired, contextual-source and owner-saved exclusions, upcoming inclusion and successful empty results.
- [x] 5.2 Implement four-point category/two-point normalized-era rules, source precedence, watchlist preference sets and close/slug ties. Verify six/four/two/zero scoring, missing/whitespace/case era behavior, repeated preference features, up-to-six cap and deterministic fallback/reasons.
- [x] 5.3 Add anonymous `GET /api/catalog/recommendations` and current-owner `GET /api/account/recommendations` through existing account dispatch. Verify strict source-query parsing, identical missing/draft source errors, no-store, cookie-independent public results and owner-only preference access with two accounts and capability-only callers.
- [x] 5.4 Add recommendation DTO decoding and independently cancellable public/private loading. Verify malformed responses, failure/retry, source changes and delayed responses during logout/expiry/account switch cannot display stale private cards; public discovery remains usable.
- [x] 5.5 Document `rules-v1`, eligibility, reason codes, authored-era limitations and cold start in service/web docs. Verify examples against ranking fixtures and make no ML or effectiveness claim.

## 6. Website dashboard and recommendation integration

- [x] 6.1 Add exact `/admin/analytics` matching, admin navigation/overview link and website admin dispatch without mounting retained game UI. Verify direct entry, trailing slash, invalid case/extra segments, Back/Forward and Go gate behavior with route/API tests and browser evidence.
- [x] 6.2 Build the analytics page with EN/VI definitions, URL-backed event/dataset controls, exact totals, semantic per-event comparison and as-of/coverage labels. Verify loading, empty, 401, malformed/unavailable data, retry and obsolete-request handling using focused frontend tests.
- [x] 6.3 Add independent home/lot recommendation sections using existing cards, reasons, demo labels and native links. Verify new-user fallback, validated owner preferences, explicit public fallback while identity is unresolved, private-failure recovery and unchanged featured/nearest/query/bid behavior.
- [x] 6.4 Update EN/VI shared notices/footer and new copy to describe working aggregate analytics and rule-based suggestions while preserving simulation labels and authored content. Verify language changes preserve selections, translate states/reasons and never trigger a commerce mutation.
- [x] 6.5 Document dashboard URLs, recommendation loading/privacy behavior and view emission in `web/README.md`. Verify documented routes and absence of game sockets, external SDKs, tracking storage or credentials in frontend bundles/network requests.

## 7. Disposable rehearsal maintenance

- [x] 7.1 Add interaction/coverage tables to demo object inspection and atomic clear/reseed lists; start new coverage alongside the new generation. Verify prepare/repeat/reset with real PostgreSQL, new drafts yield no published metrics/activity and no new schema objects bypass the allowlist.
- [x] 7.2 Extend reset tests for prior interaction removal, rollback/unknown outcome, old session invalidation and fresh collection start. Verify active-writer/unmarked/default/unknown-object targets refuse and other-schema records/media remain intact.
- [x] 7.3 Document the publish/view/watch/bid/pay/compare/reset/replay drill and exact erased/preserved scope in `README.md` and `docs/postgresql.md`. Update stale version/Change 7 descriptions in the existing `AGENTS.md` narrowly; verify guidance matches code without replacing unrelated instructions or editing archived evidence.

## 8. Integrated acceptance and review evidence

- [x] 8.1 Run focused new checks and then `make test`, `make test-race`, `make build` with an explicit isolated `TEST_DATABASE_URL`. Record outcomes and verify no required PostgreSQL suite is skipped, falls back to SQLite or touches operational schemas.
- [x] 8.2 Rehearse the reference fixture through both Vite and the embedded binary using two independent commerce accounts and a capability-authorized operator. Verify dashboard summary/event rows against independent SQL, contextual/watchlist/cold-start links and distinct paid/unpaid/simulated/demo labels; record sanitized evidence.
- [x] 8.3 Reopen the same owned schema and perform the explicit marked demo reset/replay drill. Verify durable metrics/coverage survive restart, old-generation observations disappear only after reset, new activity reconciles once and earlier game/commerce owner boundaries remain correct.
- [x] 8.4 Verify keyboard/focus, 320/375 CSS pixels and desktop, EN/VI, long titles/reasons/large totals and all loading/empty/access/error/retry states across dashboard and recommendation sections. Record actual viewport/layout evidence, including any labeled table-local scrolling without page-level overflow.
- [x] 8.5 Verify actual 200 percent browser zoom on the dashboard, home and lot recommendation sections in EN/VI, including long text/large totals and recovery states. Record browser zoom evidence separately; keep this unchecked if only viewport or CSS scaling checks are available.
- [x] 8.6 Verify active reduced-motion preference with the same affected-view/state matrix and observable runtime evidence. Keep this unchecked if preference control or complete evidence is unavailable; do not close prior archived acceptance gaps.
- [x] 8.7 Record implemented/pending task evidence and limitations in this change's verification report, run `openspec validate add-auction-analytics-and-recommendations --strict` and `git diff --check`, and review capability/scenario coverage. Verify the report distinguishes implementation correctness from untested recommendation effectiveness and preserves every incomplete acceptance criterion.

## Workflow follow-up

- After an explicit apply request and evidence-backed verification, present completed versus pending acceptance before any requested archive/commit workflow.
- Sync/archive only through the requested lifecycle workflow; preserve existing archived evidence and force-add only specifically authorized ignored OpenSpec paths when a commit is requested.
