# Change 8 verification

Change: `add-auction-analytics-and-recommendations`. Dates: 2026-10-21–2026-10-22 (Asia/Bangkok).
Implementation is in the working tree; no archive or commit is included in apply.

## Scope and accepted fixture correction

Schema 6, private first-party interactions, capability-only aggregate reporting,
`rules-v1` recommendations, bilingual website integration and bounded maintenance
are implemented. The user approved correcting event B from one to two distinct
bidders: two accepted bids on one lot cannot come from the same account because
current leaders cannot self-outbid. A and B each have two bidders; overall there
are still two distinct bidders, six bids and unchanged outcome/payment totals.

## Automated evidence

The explicit dedicated PostgreSQL 18 service is bound to localhost:55437. Tests
use `TEST_DATABASE_URL`, generate owned disposable schemas and clean them up.
No required database suite skips or substitutes SQLite. The SQLite fixture is
only the explicit read-only import source, with byte preservation checked.
Credentials live outside this change and are not included in evidence.

Focused checks cover:

- `internal/store/interaction_test.go`: fresh/repeated/failed 5→6 upgrade,
  historical bid/receipt timestamps, current-membership snapshots, constrained
  provenance/field allowlist, idempotent coverage, restart, source import and
  both new occupancy records. Migrations 1–5 have no diff.
- Watchlist affected-row transitions, rejected/replayed bids, stateless review,
  first/replayed/fresh-ID payment and fulfillment isolation. Capture failures
  roll back the parent action. Deferred commit triggers terminate only fixture
  writers, preserve unknown-outcome classification, refuse stale writers and
  reconcile durable retry after reopening; separate lease termination fails
  closed for watch/bid/payment. Game revision/catalog terms remain unchanged.
- `internal/api/interaction_test.go` and `internal/interaction/limiter_test.go`:
  strict JSON/header/origin/1-KiB boundaries, invalid/duplicate fields, NUL IDs,
  published-only lookup, sanitized no-store/no-cookie acknowledgement/failure,
  direct-peer limit 30/minute, 4096-key capacity, expiry and forwarding rejection.
- `internal/store/analytics_test.go`: reference fixture, mixed cohort/actor demo,
  independent SQL reconciliation, zero-cohort event rows, pending uninitialized
  lots, zero-price/zero-fee payment, corruption refusal, concurrent snapshot
  consistency and an exact 10,000,000,000,000,000-cent total. Aggregates come
  from durable commerce facts, not interaction counters.
- `internal/api/analytics_test.go`: strict filters, draft/missing parity,
  capability/account-role separation including unknown subtree paths, no-store,
  aggregate-only projection and unavailable reads without fabricated zeroes.
- Recommendation repository/API/ranking tests: eligible public projection,
  upcoming inclusion, draft/terminal/expired/source/saved exclusions, two owners,
  source precedence, set preferences, normalized/absent eras, 6/4/2/0 scoring,
  close/slug ties, six-card cap, cold start and successful empty results.
- `web/src/api/insights.test.ts`: strict decoders, exact BigInt formatting,
  malformed/error data, strict URLs, independent public/private credentials and
  cancellation, complete EN/VI keys and document-local observation deduplication.
- Demo tests: both new tables participate in the known-object allowlist and
  atomic clear/reseed; failed reset preserves observations and coverage, success
  starts fresh coverage with zero published metrics. Existing refusal, old-session,
  unknown-generation inspection, other-schema/media and ownership tests remain.

One full race run exposed an existing nondeterministic test assumption in
`TestAuctionRacesAndMaximum`: its second phase always challenged account B even
when B had won phase one. The test now queries the actual leader and uses the
other account, preserving business rules and the intended bid-versus-close race.

## Browser evidence

`browser.cjs`, `privacy.cjs`, `states.cjs` use local isolated Chromium profiles and
an isolated fixture created by `rehearsal.go` on the dedicated test service.
The helper uses controlled historical clocks for synthetic accepted actions,
real repositories and receipts, published demo content and independently signed-in
buyers. Its local credential manifest is outside the repository.

- Vite :5173 and embedded :8080 each reconcile six bids, two global bidders,
  3 sold/1 unsold/1 pending, sell-through 3/4, winning 35000, paid winning 15000,
  fees 1500, simulated paid 16500 cents, two paid and one unpaid sold order.
- Two buyer sessions see only their owned orders; contextual, watchlist and
  cold-start suggestion links use stored public slugs. Dashboard filters survive
  native form navigation, Back and Forward.
- `privacy-results.json` records delayed private-response suppression for logout,
  expiry and account switching, unavailable-account public fallback and private
  failure/retry recovery in both modes. A language switch emits no extra view;
  a new document emits one new observation. Failed observations have no retry loop.
- `browser-results.json` records ready dashboard layouts at 320/375/1280 CSS
  pixels and both languages, real native tab zoom 2.0, no game `/ws` sockets,
  no external requests and no page console errors. Vite HMR is not a game socket.
- `states-results.json` records the affected-view/state matrix: analytics/home/lot,
  EN/VI, 320/375/1280 layouts, loading/empty/access/error/malformed/retry,
  long authored titles, longest bounded rule reason and exact huge aggregate
  amounts. Stress responses are explicitly intercepted fixtures, not claimed
  commerce history. Each observation measures page overflow and visible keyboard
  focus. Comparison overflow is local to the labeled focusable semantic table.
- Actual browser zoom uses isolated Chrome extension `chrome.tabs.setZoom(2)`
  and `getZoom() === 2`, independently recording `innerWidth=640` and
  `devicePixelRatio=2` for a 1280-pixel window. This is not CSS scaling or viewport
  substitution. Screenshot capture accounts for native zoom coordinates.
- Reduced motion uses an active browser preference and records
  `matchMedia('(prefers-reduced-motion: reduce)').matches === true` plus zero
  running main-content animations across the same matrix. EN/VI stress PNGs
  include native 200% zoom plus active reduced motion. Representative screenshots
  were visually inspected after correcting the harness's initial capture clipping.

## Restart/reset and final gates

`make test`, `make test-race`, `make build` pass on the final implementation;
logs are stored beside this report. The frontend has 35 passing files / 552 tests,
with no lint warnings. The corrected bid-versus-close test also passed 12 repeated
race-instrumented runs. Required PostgreSQL checks use the explicit isolated URL.
`openspec validate add-auction-analytics-and-recommendations --strict` and
`git diff --check` pass. Migrations 1–5 remain byte-for-byte unchanged.

`restart-before.json` and `restart-after.json` are identical independent SQL
snapshots: six bids, two bidders, three sold, one unsold, 35000 winning cents,
16500 simulated paid cents, the same collection start and 295 observations.
Health after reopening reports ready/schema_version 6 without credentials.

The stopped-writer `rehearsal.go --reset` explicitly confirms its same owned,
marked demo schema through production `SeedDemo`. `reset-drafts.json` proves
fresh generation/coverage, zero published lots, zero interactions and zero old
sessions immediately after atomic reseed, before publishing. The helper then
replays through real watch/bid/closure/payment repositories with fresh session
secrets. `reset-replay.json` reconciles the reference totals once with zero old
views and a newer collection baseline. The old browser login cookie is rejected
with HTTP 401. A new observation followed by a duplicate acknowledgement produces
exactly one stored view (`reset-after-observation.json`) without changing totals.
Existing automated reset tests also preserve capability, other schemas and media;
ordinary boot and retained game reset preserve interaction history.

`states-results.json` contains 736 successful state/recovery observations. Native
200% tab zoom and active reduced motion are verified independently from viewport
checks. Previous archived acceptance gaps have not been changed.

## Scientific and historical limits

These checks establish implementation behavior and arithmetic, not effectiveness,
recommendation lift, unique visitors, complete traffic or trained ML. `rules-v1`
is an explainable baseline. Historical backfills/current-watch snapshots never
imply historical views or removed-save history. All payments/fulfillment are
simulated. Existing Change 4–7 archived accessibility gaps are unchanged; new
Change 8 evidence does not close those prior acceptance criteria.


## Capability/scenario review

| Capability delta | Implementation and acceptance evidence |
| --- | --- |
| auction-analytics | Private API guard, cohort/actor filters, accepted participation, terminal denominator, distinct winning/paid components, integrity refusal, exact aggregates, snapshot concurrency, bilingual recovery and semantic comparison. Repository/API tests, independent SQL, browser/state matrices. |
| commerce-interactions | Allowlisted private schema; anonymous bounded observation endpoint; atomic authoritative actions; dedup/replay/rollback/unknown commit/lease loss; historical provenance and persistent collection start. Migration/store/API/limiter tests and privacy browser drill. |
| lot-recommendations | Published eligible future candidates, normalized category/era/set scoring, deterministic ties/fallback, owner/source exclusions, current-session access and stale response suppression. Ranking/store/API tests plus two-owner/contextual/cold-start and privacy browser evidence. |
| auction-discovery | Independent home/lot suggestions and nonblocking per-document observation; existing published discovery and deliberate commerce remain separate. Public loader tests, browser links/no game sockets/external requests, language/view counts and recovery matrix. |
| website-navigation | Exact analytics route, website admin dispatch/navigation, separate Go document/API capability checks, direct/native/history URLs and updated EN/VI notices. Route/API tests and Vite/embedded navigation evidence. |
| postgresql-persistence | Additive version 6; immutable migrations 1–5; fresh/populated/rollback/repeat/newer refusal; import occupancy and read-only source preservation; writer safety and read snapshots. Real PostgreSQL tests, health/restart/independent SQL. |
| demo-rehearsal | Both tables in object allowlist and atomic reset; new coverage, prior-session rejection, draft-zero/no invented activity, replay/restart; default/unmarked/active-writer/unknown objects refused, capability/other-schema/media preserved. Demo/CLI tests and generation drill artifacts. |

Temporary app/Vite processes and owned rehearsal schemas were removed after
verification. The reused dedicated test container was returned to its prior
stopped state; private temporary credential/session files were removed.

All 40 tracked tasks have evidence. No implementation or acceptance item is
intentionally deferred in this change. This completion does not revise the
explicit scientific and historical limits above.
