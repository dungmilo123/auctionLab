# Implementation verification — 2026-10-19

All 58 tracked tasks are implemented and verified. `make test`, `make test-race`, `make build`, the two-client smoke, the 20-client ordering/resync harness, browser acceptance, offline import, restart, outage, backup/restore and prior-binary rollback checks passed. Strict OpenSpec validation passed. These rehearsals used disposable PostgreSQL schemas/databases and generated SQLite fixtures; no operational SQLite source was imported or cut over.

## Delivered behavior

Runtime persistence is PostgreSQL 18 through pgx v5.11.0 and `database/sql`, with bounded pooling/deadlines, per-connection schema selection, migration ledger version 1, exclusive writer ownership and live readiness. Retained game behavior runs against PostgreSQL. SQLite remains an explicit read-only, standalone v4 import source. The importer preserves typed values/raw legacy text and sequence history, commits data/sequence restarts/marker atomically, and independently verifies actual rows without writes. Replaying a marker reports `already_imported` without claiming fresh verification/readiness.

Catalog preparation stores independent lots/events, exact USD cents, ordered media/specifications/membership, versions, fulfillment and derived UTC schedules. Publication validates and freezes the whole aggregate atomically. Public DTOs exclude draft/private/game data. Both admin routes provide EN/VI stored lists/editors, previews, explicit publication, validation/conflict/access/backend recovery, and read-only published views. Seed-only mode creates 18 illustrative lots in three draft events with 39 bundled project-authored CC0 SVGs and repeat-safe fixed dates. Publication does not execute commerce auctions.

## Checks and evidence

| Check | Actual result | Evidence |
|---|---|---|
| Full suite | All Go packages passed; 20 frontend files / 483 tests passed; oxlint passed without warnings | `make-test.log` |
| Race suite | All database-backed game, hub, store, catalog, API, server and other packages passed | `make-test-race.log` |
| Build | TypeScript + Vite completed before `CGO_ENABLED=0` Go build; 39 catalog SVGs embedded; placeholder preserved | `make-build.log`, `build-results.json` |
| Startup/modes | Eight invalid/missing/unreachable configuration/flag cases failed before listening with sanitized diagnostics | `startup-results.json` |
| Missing test service | Explicitly unset test configuration failed with `TEST_DATABASE_URL is required`; no skip/fallback | `internal/testpg` negative run (expected failure, not a full-suite failure) |
| Separate roles | Nonsuperuser app role seeded its own database; separate nonsuperuser test role passed parallel isolated fixtures | `provisioning-results.json` |
| Two-client smoke | Owned disposable schema; real English round, private rejection, settlement and convergence passed | `loadtest-smoke.log` |
| 20-client fleet | All 20 clients converged; deliberately stalled client detected a gap and recovered via snapshot | `loadtest-20.log` |
| Catalog HTTP | Created/edited complete lots and event; exact cents; membership/order; preview; corrected field-specific publication failure; immutable/repeat-safe publication; anonymous private-field exclusion | `catalog-results.json`, `catalog-acceptance.py` |
| Browser | 12 groups passed; no page exceptions or game sockets; creation/refresh, failed-save retention, stale/uncertain reload, lost authorization, keyboard order, missing record, preview boundaries, Vite access/backend recovery | `browser-results.json`, `browser-acceptance.cjs` |
| SQLite import | Eight tables compared before first normal boot; Unicode, nullable fields, raw JSON, sealed submissions, IDs/key/tokens and identity high-water history preserved | `import-report.json`, `verify-report.json`, source/import unit tests |
| Import replay | Existing marker returns only replay/source identity, with no new row-digest/readiness claim or overwrite | `import-replay-result.json` |
| Token continuity | Imported player token resumed the same retained-game identity | `token-result.json` |
| Operations | Application/PostgreSQL restart, seed replay, writer refusal, outage 503/no false save, backup/restore equality and prior-binary rollback passed | `operations-results.json`, `operations-rehearsal.py` |
| Final built binary | Restarted the final binary and compared its public aggregate with the pre-restart baseline; byte-for-byte logical JSON equality | `build-results.json` |
| Specs | Five deltas strict-valid; no validation issues | `openspec-validation.json` |

The importer/source tests also reject unsupported versions, invalid exact integer/text/reference data, WAL sidecars and exhausted identities. They inject failures after row insertion and after identity RESTART, verify rollback, reject occupied/different-source targets, preserve edits on replay, and detect actual drift without trusting a marker. Catalog tests exercise multi-connection edit/publication races, expected-version invalidation from member edits, membership rollback/detach/reattach, failed publication after lot updates, schedule overflow/future-time checks, media confinement, seeding collisions/rollback and game reset isolation.

## Browser and visual observations

Chromium 153.0.8010.12 ran headlessly through the official installed Playwright package, explicitly authorized by the user after the UI tool reported no available browser. Both draft editor pages were checked in EN/VI at 320, 375, 768 and 1280 CSS pixels. Automated assertions found no page-level horizontal overflow and no unlabeled text/select fields. Keyboard Tab/Enter reordered and saved event membership. Language switching retained entered catalog text. Invalid money, stale versions, interrupted mutations and lost access retained input; uncertain outcomes/stale edits blocked further writes until explicit reload.

Screenshots: `lots-{en,vi}-{320,375,768,1280}.png`, `auctions-{en,vi}-{320,375,768,1280}.png`, two `*-200-percent-text.png`, and `missing-image.png` (19 images total). Representative narrow/desktop EN/VI and enlarged-text images were visually inspected: labels/buttons wrap, field groups remain within the page, membership actions remain readable, local illustrations load, and missing images retain the labeled fallback. Stored content stays in its original language while management labels change.

The 200% check doubles a snapshot of computed text sizes at 375px and verifies layout; it is a text-enlargement simulation, not a claim of browser-chrome zoom or a full assistive-technology audit. Disconnected browser networking tests save/error recovery; offline local media availability is separately covered by bundled same-origin assets and filesystem validation. The retained game WebSocket protocol remains covered by game/hub/load checks; website pages themselves opened zero game sockets.

## Import, settlement and rollback boundaries

Pre-boot `import-report.json` and independent `verify-report.json` match for all eight tables. First normal PostgreSQL boot then legitimately settled the fixture's expired English run: LIVE revision 4 became SETTLED revision 5. Its legacy game budget changed from 1,000,000 to 1,250,000 under the existing `truth value 350,000 − winning bid 100,000` payoff, with total value 350,000. This is expected game execution after verification, not import conversion/drift. Post-boot verify-only correctly detects changed player data. The imported capability authorized admin access, and the imported player token retained identity.

An actual PostgreSQL outage returned readiness/save HTTP 503. Restarting PostgreSQL did not reacquire ownership for stale memory; an explicit app restart restored readiness. A subsequent legacy mutation on a disconnected old owner failed fatally rather than publishing success. Published content/order/dates survived the PostgreSQL and app restart. `pg_dump`/`pg_restore` into a disposable database/schema reproduced the public aggregate.

The preserved prior SQLite binary booted and authorized a working copy of the fixed source snapshot. The original snapshot checksum stayed unchanged (see `operations-results.json`). A private PostgreSQL backup and the prior binary/snapshot were retained outside the repository. PostgreSQL catalog writes/publication are absent from that old SQLite snapshot: rollback after cutover would discard those writes unless separately retained. There is no reverse synchronization or lossless post-write rollback claim.

## Delta-spec map

| Delta | Implementation | Verification |
|---|---|---|
| `postgresql-persistence` | `internal/store`, `internal/sqliteimport`, maintenance/startup/health, isolated test/load helpers, operational docs | Full/race suites; source/import/store tests; startup/provisioning/import/operations/token/build reports |
| `vintage-catalog` | `internal/catalog`, catalog repositories/migrations, USD helpers, local media, explicit seed, public DTOs | Domain/storage/API tests; 18-lot/three-event seed checks; HTTP/browser/restart evidence |
| `auction-event-publishing` | Serialized catalog transactions, versions, derived schedule, aggregate validation/publication, immutable public projection | Concurrency and injected rollback tests; HTTP publish/correction/repeat/immutability; restart and clock-after-close tests |
| `catalog-administration` | JSON cookie guard, origin/header boundary, bounded HTTP contract, typed client, working EN/VI editors | Real HTTP authorization/origin tests; browser state/recovery/order/language/width/text/image/Vite checks |
| `website-navigation` | Existing routes with query-selected editors, route-specific notices, retained public/account/order previews | Route/query tests; browser direct creation/refresh/missing selection/retired screen/zero-game-socket checks |

`PROJECT.md` and `PLAN.md` referenced by old generated guidance are absent in this checkout. Active README/AGENTS/ROADMAP guidance was reconciled; archived evidence was preserved. Planning/verification files remain local under the existing OpenSpec ignore rule. No archive or production deployment was performed.
