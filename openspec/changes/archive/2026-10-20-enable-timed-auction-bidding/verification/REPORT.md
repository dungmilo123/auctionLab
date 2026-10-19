# Change 5 implementation and acceptance evidence

Date: **2026-10-20, Asia/Bangkok**. Change: `enable-timed-auction-bidding` (spec-driven).

## Result and remaining acceptance

Implementation is present for schema-3 storage, independent timed lots, transactional manual bidding, public snapshots/history/SSE, owner-only participation, the lot panel and `/account/bids`. **38/39 tasks complete** after this report; **task 8.4 remains unchecked**. No archive or main-spec synchronization was performed.

The local two-session rehearsal accepted four ascending bids on `demo-lot-01` and recorded exactly one sold result: **Bidder 2, 16007 USD cents ($160.07)**. Another lot and the overlapping event remained independently open. A separate restart across the original deadlines recovered `demo-lot-14` sold for **21975 cents**, and `demo-lot-13` unsold. No payment/order was created or implied.

Measured browser visibility (not a production SLA):

| Observation | Time |
|---|---:|
| A's opening submit action to B displaying the accepted history | 893 ms |
| B's jump submit action to A displaying the accepted history | 543 ms |
| Scheduled close to A displaying the committed sold result | 882 ms |
| Scheduled close to B displaying the committed sold result | 571 ms |

Bid timing includes the submit action and request/commit/display time. Result timing is measured from the immutable scheduled close, so it also includes worker settlement time. Measurements use the automation host's monotonic action interval/wall-clock epoch against the local server schedule. They do not estimate remote network or production throughput. See `latency.json`, `two-bidders-*.txt`, `sold-*.txt`, and `sold-desktop-en.png`.

## Test environment and exact commands

An existing stopped PostgreSQL 18 container, `auctionlab-change2-pg18`, was found and started on **127.0.0.1:55432**. A new dedicated, non-superuser owner and database `auctionlab_change5_test` were created inside it. No existing application databases/schemas were used for tests. Credentials are outside the repository in a mode-0600 file, `/home/milo/.local/state/auctionlab/change5-test.env`; no credential value is in this report. The existing container and dedicated test database remain available.

Tests used `internal/testpg` owned disposable schemas. Browser rehearsal used one separate `scripts/dbfixture` owned `test_load_*` schema. Demo credentials were generated privately and supplied through operator environment inputs to stopped-writer `--seed-accounts`; normal boot installed none. `prepare/main.go` seeded drafts and updated/published only new draft schedules through the catalog service, without editing any existing publication. A third draft event was published through the existing admin API while serving, and accepted a bid without restart.

From the repository root, after sourcing the private test environment:

| Command | Result / saved output |
|---|---|
| `go test ./internal/store -run '^TestSchema$' -count=1` | Passed against the discovered instance; also passed from a fresh normal zsh after sourcing the private configuration. |
| `go test ./internal/store -run 'Test(AuctionMigrationConstraints\|UpgradeFromVersionTwo\|Schema\|MigrationRollback)$' -count=1` | Fresh/repeat/constraint/v2-upgrade/rollback checks passed. |
| `go test ./internal/auction ./internal/store ./internal/api -run 'Test(Auction\|Commerce\|Minimum)' -count=1` | Domain, PostgreSQL and HTTP/stream checks passed after corrections; final full suites below include them. |
| `go test ./... -count=1 -timeout 120s` | Passed all packages: `go-tests.txt`. No required DB checks skipped or substituted with SQLite. |
| `go test -race -count=1 -timeout 120s ./...` | Passed all packages without race reports: `go-race.txt`. |
| `cd web && npm test` | **29 files / 525 tests passed**: `frontend-tests.txt`. |
| `cd web && npm run lint` | Passed: `frontend-lint.txt`. |
| `cd web && npm run build` | TypeScript and Vite passed: `frontend-build.txt`. |
| `CGO_ENABLED=0 go build -o bin/auctionlab ./cmd/server` | Passed after the final frontend build: `embedded-build.txt`. |
| `openspec validate enable-timed-auction-bidding --strict` | Passed. This validates the planning package, not browser acceptance. |
| `git diff --check` | Passed. |

An early stream-capacity test had a race in its **test reservation setup** while disconnected streams were releasing slots. The test now waits for disconnect cleanup before reserving capacity; the final normal/race suites pass. Two abandoned schemas from interrupted test processes were removed only from the new dedicated test database. `cleanup.json` records zero remaining owned test schemas. Temporary Go/Vite rehearsal processes were stopped and the rehearsal schema dropped; the screenshots are saved evidence, not a currently running demo.

## Capability requirement mapping

| Capability and requirements | Implementation | Automated / rehearsal evidence |
|---|---|---|
| timed-auction-bidding: independent published lot auctions; scheduled activation/adoption | `internal/auction`, leased SQL repository; published-only initialization, immutable schedule inputs; pre-listener reconciliation and bounded 250 ms worker | `TestAuctionLifecycleAndIndependentPublication`, `TestAuctionParticipationOrderAndWorker`; overlapping published events, new third publication while serving, `restart-rehearsal.json` |
| Server-authoritative bid window | Session/time sampled after writer/row waiting; half-open interval; reads only present opening/closing | `TestAuctionLockWaitRevalidatesDeadlineAndSession`, boundary/late bid tests; exact deadline and session-expiry refusals, restart late refusal |
| Exact ascending commerce amounts | USD integer cents, zero opening, arbitrary jumps, fixed positive increment, exhausted minimum | `TestMinimumAndIntent`, `TestAuctionBidLadderReplayAndPrivateReads`, `TestAuctionRacesAndMaximum`; 14001-cent jump and final 16007-cent bid; exact decimal parser tests |
| Commerce identity/request protection | Existing whole-account namespace guard; in-transaction session recheck; bounded strict JSON and same-origin/header rules | `TestCommerceHTTPBidBoundaryPrivacyAndIsolation`; malformed/missing/fractional/negative/over-limit/unknown-field/origin/header/draft/missing/expired cases |
| Atomic ordered acceptance; retry-safe accepted intents | Accepted bid/count/leader/revision in one transaction; account-scoped UUID; fresh receipt replay, no rejected reservations | Bid/close races, identical concurrent submissions, no-self-raise, rollback/lost writer tests; `http-rehearsal.json` discards a successful response then replays the same intent with one durable bid; frontend unknown-intent API test |
| Durable once-only terminal results | Unique result plus composite winning bid/account/amount reference; no terminal reopen | Schema invalid-row tests, lifecycle/race/history/reopen tests; `restart-rehearsal.json` verifies sold+unsold, original dates, owner won state, replay after closure, unique results and watchlist restoration |
| Private participation reads | Repeatable Read session-owned unique-lot list/detail and owner request IDs, deterministic ordering | `TestAuctionParticipationOrderAndWorker`, `TestAuctionBidLadderReplayAndPrivateReads`, HTTP isolation tests; `switched-*.txt` shows two distinct owner lists after account switching; won/lost/leading/outbid/closing/not-participating backend checks |
| Accessible confirmed bid interaction | EN/VI copy, labeled exact-money input, polite result messages, keyboard form, honest loading/refusal/unknown/identity errors | 11 focused auction frontend tests, keyboard submit and focus observation, mobile evidence below. **Task 8.4 remains incomplete** for actual zoom and reduced-motion runtime checks. |
| auction-realtime: coherent public snapshots; pseudonymous history | Explicit public DTO, Repeatable Read, null pre-bid price, newest 50 and exclusive sequence cursor, per-lot Bidder N | History/alias pagination test with 55 same-time bids; privacy/HTTP checks; public result/history agree in browser captures |
| Read-only commerce streaming; monotonic recovery | Dedicated SSE, initial/current full snapshots, revision/state sampling, heartbeat, client merge/fallback/backoff/stale handling | `TestCommerceStreamFreshReconnectCapacityAndShutdown`, frontend controlled timers, measured local visibility; `offline-stale.txt` / `reconnected.txt` show retained content, disabled stale form, then fresh eligible recovery |
| Bounded stream lifecycle | 64 slots, at most 50 public history rows, five-second write deadlines, cancellation before HTTP shutdown | Capacity/disconnect/shutdown stream test, `TestCommerceStalledStreamWriteBound`, normal SIGTERM with active streams in rehearsal; no stream retains a writer transaction between samples |
| Separate private participation freshness; explicit realtime privacy | Session-owned visible polling/focus/revision refresh; logout/expiry/account changes clear state; explicit public safe field set | Obsolete/aborted private response tests and slow-read recovery test; `signed-out-*.txt`, `switched-*.txt`; HTTP privacy sentinels exclude account IDs/emails/names/request IDs, private notes, capability and game secrets |
| postgresql-persistence: versioned schema; import occupancy; real PG acceptance; commit/ownership safety | Additive migration 3, health version 3, new-table occupancy checks, shared leased writer and safe commit classification | Fresh/repeat/failed-step/v2-preservation/newer-refusal tests; SQLite read-only import and verify-only suites; fault injection, lease termination, cancelled queue, game-reset independence and full/race suites |
| auction-event-publishing: publication separate from execution | Publication remains editorial and immutable; independent worker adopts without publication hooks/game commands | Lifecycle test; all three public event responses compared structurally to their publication manifests across restart (`all_publications_unchanged`); game revision 0 and game bid count 0 in live check |
| auction-discovery: prices/fulfillment; honest schedule phases; bilingual/session boundary; separate execution loading | Content/gallery/condition/fulfillment precede auction panel; schedule URL phases and opening-price sorts unchanged | Existing discovery/catalog/route tests pass; browser outage preserves loaded catalog content while auction/private reads fail; `offline-stale.txt` |
| website-navigation: bid destination and honest preview notices | `/account/bids` functional; shared/footer/admin EN/VI copy updated; orders remain unavailable | Direct load/refresh, safe login return, native lot link, Back/Forward in Vite and embedded build; signed-out and switched captures; won screenshot explicitly says no checkout/order |
| lot-watchlists: published membership and separate save interaction | Watchlist operations remain independent of bids; account capability boundaries unchanged | Full account/watchlist tests; explicit save survives restart without generating a bid; runtime counts and owner isolation checks |
| catalog-administration: honest management/development boundaries | Existing capability gate retained; notices explain separate scheduled execution and unavailable checkout/orders | Existing gate/catalog/account suites pass; no competing account prefix, game UI/socket or account-role substitution introduced |

## Browser observations and accessibility limit

Browser session A used `127.0.0.1:5173` (Vite); session B used `localhost:8080` (embedded). These are two cookie-isolated host sessions in the available in-app browser, **not two different browser engines**. They signed in as distinct demo accounts, competed on the same lot and later switched accounts through logout/login. Both returned to `/account/bids` using `return_to`; neither login replayed a bid. B saw only its $160.07 personal amount on the camera; A saw its $150.03 amount, then lost, plus its own other lots. Final switched captures confirm prior owner rows disappear.

| Check | Observation / evidence |
|---|---|
| Lot detail 320 CSS px, VI | `innerWidth=320`, document width 305; no page-level horizontal overflow. `lot-320-vi.png`, `lot-mobile-panel-vi.png`. |
| Lot detail 375 CSS px, EN | `innerWidth=375`, document width 360; no overflow; exact four-bid history and $160.07 leader. `lot-375-en.png`. |
| `/account/bids` 320 CSS px, VI | Width 320 / document 305; wrapped navigation/filter controls, readable owner amounts. `bids-320-vi.png`. |
| `/account/bids` 375 CSS px, EN | Width 375 / document no greater than 375; won filter and checkout-unavailable copy. `bids-375-en.png`. |
| Desktop | 1280 CSS px, readable lot result and owner-filter page. `sold-desktop-en.png`, `bids-won-desktop-en.png`, `bids-desktop-vi.png`. |
| Keyboard/focus | Tab reached the labeled amount and submit; Enter deliberately submitted an exact jump. A focus-loss issue from a native disabled submit button was corrected with aria-disabled plus submission guards and a read-only input. The focused BUTTON remained the bid control after confirmation; unrelated public updates do not call focus. Results use polite announcements; no countdown live region exists. |
| Actual 200% browser zoom | **Unavailable/unverified.** The browser exposed viewport controls but no zoom capability. A native Ctrl+plus attempt left width 1280, DPR 1 and visual viewport scale 1 unchanged. It was not counted as zoom, and viewport emulation was not substituted. |
| Reduced motion | Existing global `prefers-reduced-motion` rules remain and the new auction styles add no animation. The runtime reported normal motion; no supported reduced-motion override was available. **Runtime reduced-motion verification remains unverified.** |

Task 8.4 therefore remains unchecked. No archived Change 4 acceptance item was retroactively completed.

## Changed-file scope and protected evidence

Backend additions are `internal/auction/{auction.go,auction_test.go,README.md}`, `internal/store/{auction.go,auction_test.go,auction_schema_test.go,migrations/003_timed_auctions.sql}`, and `internal/api/{auction.go,auction_test.go}`. Integration changes are limited to `cmd/server/main.go`, existing account dispatch, migration embedding/version, auction commit-error classification, SQLite target occupancy and schema/import tests.

Frontend additions are `web/src/api/auction.ts`, `web/src/auction/{feed.ts,useParticipation.ts,eligibility.ts,auction.test.ts}`, `web/src/lib/auction-copy.ts`, `routes/storefront/AuctionPanel.tsx` and `routes/account/Bids.tsx`. Existing lot/account routes, four availability copy dictionaries and CSS connect these surfaces. Operator documentation is updated in `README.md`, `docs/postgresql.md`, `web/README.md` and the new auction README. No runtime dependency or lockfile changed. Planning task checkboxes and this change's verification files are local OpenSpec artifacts (Git-ignored in this checkout).

`git diff --exit-code -- AGENTS.md internal/store/migrations/001_initial.sql internal/store/migrations/002_accounts_and_watchlists.sql` passed: all three protected files are unchanged. No writes targeted archived evidence. Change 4's archived tasks **7.3 and 8.3 remain `[ ]`**, at lines 51 and 58 respectively; their current task/report hashes and pre-existing mtimes are captured in `protected-archive.json`. Existing archived limitations remain intact. Archived files are Git-ignored, so Git diff alone is not asserted as proof for them.

No catalog DTO, discovery query/sort implementation, legacy game/hub protocol or game monetary semantics changed. Checkout/orders, auto-bid, reserve, anti-sniping, relisting, analytics and ML are outside this implementation. A finite local rehearsal does not establish production capacity or remote latency. Full workflow verification and archive remain separate follow-up actions after the remaining manual check is resolved.
