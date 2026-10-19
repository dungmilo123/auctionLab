# Accounts and watchlists verification

Report finalized 2026-10-20, Asia/Bangkok. Change: `add-bidder-accounts-and-watchlists`; schema: `spec-driven`.

Implementation is present for accounts, sessions, private watchlists, explicit demo provisioning and the bilingual website. **32/34 tracked tasks are complete.** Tasks **7.3 and 8.3 remain unchecked solely for exact 200% browser-zoom acceptance**. The available browser exposes responsive viewport sizing but no working browser-zoom control. The user explicitly selected **“Leave exact zoom check pending”**; no equivalent reflow check has been substituted or claimed as that acceptance.

## Environment and data boundary

- PostgreSQL 18 ran in a dedicated disposable local container, bound to loopback port 55439, with an ephemeral database. Required suites used explicit `TEST_DATABASE_URL` and `internal/testpg` owned schemas. Browser/maintenance acceptance used one independently created `scripts/dbfixture` schema in that same disposable database.
- No operational schema, existing `auction.db` or its sidecars was used, imported, reset or modified. SQLite tests created their own temporary source files. Migration 1 and archived evidence were preserved.
- Vite ran at `127.0.0.1:5173`; the embedded binary ran at port 8080. Separate `127.0.0.1` and `localhost` host-only cookie contexts exercised two independent accounts. These were host-isolated cookie contexts, not two browser profiles.
- Browser catalog records were explicit disposable fixtures: catalog seed drafts were made public inside the fixture schema, and one public lot was assigned a long title and no image. Production publication rules were separately covered by the existing catalog suite; SQL fixture preparation is not evidence of a production publication operation.
- Seed passwords and capability were generated only for the disposable fixture and kept in a private temporary operator environment. No password, verifier, salt, raw token, cookie, capability value, database row dump or real profile is included here. Screenshots contain public authored fixture content, counts and interface states only.

## Executed checks

All commands below completed successfully unless explicitly described as an expected refusal. Connection values and credential inputs are intentionally omitted.

| Command/check | Outcome |
|---|---|
| `go test ./internal/store -run 'TestSchema\|TestCommerceConstraints' -count=1` with the isolated test URL | Fresh/repeated migrations and commerce constraints passed. |
| Store upgrade/rollback/import focused tests | Populated actual version 1 upgraded to 2 without changing retained data; failed step 2 rolled back; newer ledger refused; occupied import refused; first/repeat/read-only import behavior passed. |
| `go test ./internal/account -count=1` | Identity bounds, Unicode preservation, real-cost PBKDF2, distinct salts, malformed verifier rejection, bounded limiter and recovery passed. |
| Store account/watchlist/seed focused tests | Atomic/concurrent registration, session rotation/revocation/expiry/restart, queued cancellation, physical writer termination, owner isolation, stable ordering, idempotency and seed collision/repeat passed. |
| `go test ./internal/api -run TestAccountTwoOwnerRehearsal -count=1` | Full two-owner rehearsal, including both real engine reset modes, passed. |
| Account/watchlist HTTP focused tests | Strict JSON/header/origin boundaries, safe errors, cookie scheme policy, capability separation and recovery passed. |
| `make test` with explicit isolated `TEST_DATABASE_URL` | All Go packages passed; **28 frontend files / 514 tests** passed; oxlint passed without warnings after fixes. |
| `make test-race` with explicit isolated `TEST_DATABASE_URL` | All Go packages passed under `-race -timeout 120s`. |
| Frontend-first `make build` | Typecheck, Vite production build and `CGO_ENABLED=0` embedded Go build passed. After the final keyboard-focus refinement, frontend tests/lint/build and the no-CGO Go build were rerun successfully. |
| `go test ./internal/api -run TestHealth -count=1` | Supported-version health assertion passed after replacing the mock's hard-coded schema version with `store.UserVersion`. |
| Live embedded health | HTTP 200; `database=postgresql`, `ready=true`, `schema_version=2`, `status=ok`. |
| Embedded direct GETs for `/account`, `/account/login`, `/account/register`, `/account/watchlist` | All returned 200 SPA documents with `Cache-Control: no-cache`. |
| `go version -m bin/auctionlab` | Build metadata confirmed `CGO_ENABLED=0`. |
| `--seed-accounts` and repeat on the owned fixture | Status-only `seeded`, then `already_applied`; real seeded-account login succeeded. |
| Seed invocation while server owned the schema | Expected nonzero refusal for writer ownership. |
| `--seed-accounts --seed-catalog` and `--seed-accounts --tunnel` | Expected nonzero mutually-exclusive-mode/tunnel refusals. |
| Three local real-policy login samples | 96.3, 90.1 and 89.7 ms; median **90.1 ms**. This is a small local sample, not a load or deployment performance claim. |
| Production website JS inspection | No `WebSocket`, `/ws`, or legacy `auctionlab.token` marker; artifact SHA-256 recorded in `bundle-boundary.json`. This is static bundle evidence, not a runtime network trace. Vite's development HMR socket is independent tooling. |
| `git diff --check` | Passed. |
| `openspec validate add-bidder-accounts-and-watchlists --strict` | Passed. |

## Requirement-to-evidence mapping

Paths below are repository-relative, with exact test names available in the listed files.

| Requirements / tracked tasks | Evidence and result |
|---|---|
| Versioned PostgreSQL schema and existing-data upgrade; 1.1–1.2 | `internal/store/schema_test.go`: `TestSchema`, `TestCommerceConstraints`, `TestMigrationRollback`, `TestUpgradeFromVersionOne`. Unique email/session/member/seed keys, role/expiry/foreign-key checks; populated catalog/game/capability/revision preservation; no inferred accounts; atomic failed-step rollback; supported constant/newer rejection. |
| Atomic repeat-safe import and occupied commerce target; 1.3 | `internal/store/sqliteimport_test.go`: existing independent verification/import tests and `TestImportAccountOccupied`; all four commerce tables are in the occupied inventory. Completed-source repeat remains checked before occupancy; source fixture stays read-only. |
| Migration/health/operator contract; 1.4 | `docs/postgresql.md`, `internal/api/health_test.go`, live health/direct-route/build checks above. Compatible-build or explicit stopped-service backup restore is documented; no automatic down-migration. |
| Bidder registration, credential protection and bounded authentication work; 2.1–2.4 | `internal/account/account_test.go` and `internal/account/README.md`; actual PBKDF2-HMAC-SHA256 policy is versioned at 600,000 iterations, with 16-byte salts and 32-byte output. `internal/api/account_test.go` verifies comparable unknown-address verification work and forwarded-IP non-authority. |
| Durable bounded sessions, explicit logout, private self-profile; 3.1 | `internal/store/account_test.go`: `TestAccountsDurableSessions`, `TestAccountAtomicConcurrentRegistration`, `TestAccountCancellationWhileWriterQueued`. Account/session inserts are atomic; fixed 30-day expiry; old browser-session replay denied; another browser stays valid; profile persists after reopen; cancellation does not poison a queued lease. |
| Account writes respect ownership and commit boundaries; 3.2 | `TestAccountLostWriterAndClassification`, `TestAccountPhysicalWriterLoss`; `TestAccountCommitFailureAndRecovery` in API tests. Physical leased-connection termination fails closed without reacquisition. The uncertain-commit API test injects an error after an actual successful registration commit: no cookie/success is issued and login recovers. Arbitrary network interruption at every commit phase was not exhaustively fault-injected. |
| Account request boundary and safe HTTP/cookie contract; 3.3 | `TestAccountHTTPBoundaries`, `TestAccountCookieScheme`, `TestAccountInvalidLoginAndLimiter`: no-body/null/trailing/oversize/unknown-field rejection; 8 KiB boundary; JSON/header/origin checks; no-store envelopes; HTTP/TLS/loopback-proxy cookie flags; idempotent logout. |
| Bidder/admin roles independent of catalog authority; 3.4–3.5 | `TestAccountCapabilityIndependence`, main-server wiring, browser `/admin` denial while signed in. Both roles alone fail private catalog reads/previews/writes/misses. Capability-only catalog access remains compatible and cannot select an account. API and operator docs explain the distinction. |
| Published membership and repeat-safe owner-scoped changes; 4.1 | `internal/store/watchlist_test.go`: `TestWatchlistMembership`; draft/missing indistinguishable 404, overlapping saves create one row, original saved time retained, repeat/absent removal harmless, revoked sessions fail later writes. |
| Private persistent listing, coherent public projection and ordering; 4.2 | `TestWatchlistSnapshotProjectionAndIsolation`: two-owner isolation, newest-first with stable C-collation ID ties, elapsed retention, nonpublic lot/event omission and no private field leakage. Session/membership/catalog are read in one Repeatable Read transaction; explicit existing public lot DTOs are reused. |
| Watchlist API and recovery semantics; 4.3–4.4 | `TestWatchlistHTTP`, account/capability tests, typed client tests, browser recovery and `web/README.md`. Foreign owner fields, capability-only requests, expired/missing sessions and cross-origin mutations are rejected. UI does not claim an uncertain mutation; it requires a committed-state reload. Public catalog DTO shape is unchanged. |
| Explicit repeat-safe demo provisioning; 5.1–5.3 | `internal/store/account_seed_test.go`, `cmd/server/accounts_test.go`, live maintenance checks. Three real-policy credential accounts, roles/demo labels/stable IDs, all-or-nothing collision behavior and repeat preservation of password/session/watchlist. Maintenance precedes engine/listener/tunnel; no normal-startup defaults or secret output. |
| Explicit destinations and safe authentication return; 6.1 | `web/src/lib/route.test.ts`, `account-return.test.ts`; direct routes, one trailing slash, recognized queries, external/encoded/backslash/control/path-traversal and auth-loop rejection. Browser safe lot return and external target fallback both passed; no automatic save followed authentication. |
| Optional account state and restoration without private-data guesses; 6.2 | `web/src/account/state.test.ts`, `web/src/api/account.test.ts`, `usePrivateRead.test.ts`; checking/signed-out/signed-in/unavailable distinction; abort plus obsolete-response suppression; identity clearing; native Back after logout visibly required login. |
| Accessible auth forms and private profile; 6.3–6.4 | HTTP/client tests plus Vite/embedded observations: invalid registration focused its alert and associated email error; failed login showed invalid credentials; invalid profile name retained entered text; confirmed name survived refresh; read-only email; failed logout left an explicit unconfirmed error instead of claiming sign-out. |
| Bilingual honest notices, docs and no game restoration; 6.5–6.6 | `account-copy.test.ts`, existing `site-copy.test.ts`, unchanged game-copy suite, App route/title tables, production bundle inspection, active README/AGENTS updates. Direct account/auth loads worked in Vite and embedded build. Bids/orders visibly remained unavailable previews. |
| Lot detail save interaction; 7.1 | Separate watch control and explained disabled bid were observed together. Anonymous link returned to the same lot. Keyboard save reported only confirmed membership. Stale-account/lot reads are dropped by tested cleanup guards. Public lot content stayed usable when account storage failed. |
| Recoverable real watchlist; 7.2 | Real cards/count/links; A's saved entry was absent in B's list. B saved and removed explicitly; A retained its entry. Failure was not rendered as empty. A backend restart produced a recovery state and explicit reload restored the saved entry. Confirmed keyboard removal moved focus to its announced result. |
| EN/VI, keyboard and narrow layout; 7.3 **partial, unchecked** | EN/VI long-title/missing-image watchlist at 320 CSS pixels; document scroll width equaled content client width (305 pixels with the vertical scrollbar). Vietnamese keyboard removal showed disabled pending action, then confirmed result and focused status. **Exact 200% zoom remains pending.** |
| Watchlist usage docs and transaction boundaries; 7.4 | `web/README.md` reflects actual controls, idempotency, ordering, safe return, recovery and planned schedule wording. Delivery/pickup disclosures and disabled explained bidding remain present. |
| Complete independent lifecycle and game-reset boundary; 8.1 | `internal/api/account_rehearsal_test.go`: `TestAccountTwoOwnerRehearsal` registers two owners, updates profile, saves/removes independently, logs out/replays/logs in, repeats seed, reopens the same owned schema, checks expiry and invokes `game.ResetKeepPlayers` and `game.ResetClearPlayers`. Accounts do not change game revisions/legacy identity/publication; both reset modes preserve private lists/sessions. |
| Required real PostgreSQL acceptance and build; 8.2 | Full normal/race suites and embedded schema-2/no-CGO/live-health checks above. Required fixtures fail when the explicit service is missing rather than silently skipping or using SQLite. |
| Browser acceptance; 8.3 **partial, unchecked** | Vite and embedded direct loads/refresh, host-isolated accounts, native Back after logout, safe return, capability denial, EN/VI, keyboard, 320-pixel long/missing-image view, account failure/public discovery, restart recovery and unavailable bid/order views passed. **Exact 200% zoom remains pending.** |
| Evidence hygiene and report; 8.4 | This report, four sanitized UI screenshots and a production-bundle digest/marker summary. No raw credentials, account rows, cookies or capability values are included. No acceptance checkbox is set for the two remaining zoom-dependent tasks. |

## Browser evidence

- [Embedded empty watchlist after confirmed removal](embedded-empty-watchlist.jpg)
- [320-pixel English watchlist, long title and missing image](watchlist-320-en.jpg)
- [320-pixel Vietnamese watchlist, same fixture](watchlist-320-vi.jpg)
- [Public lot detail during account-storage failure](public-with-account-failure.jpg)
- [Production bundle boundary and digest](bundle-boundary.json)

The screenshots are the browser's original JPEG captures; no credential/profile screenshot was retained. Temporary viewport overrides were reset.

## Pending manual acceptance and limits

1. In a browser with a working browser-zoom control, open a published long-titled/missing-image lot and its authenticated watchlist. Set **actual browser zoom to 200%**. Use the keyboard to save/remove, check focus and announced results in EN/VI, and confirm no page-level horizontal overflow. Repeat the relevant direct/history behavior in Vite and the embedded build.
2. Record that exact result, then mark tasks 7.3 and 8.3 only if their full checks pass and rerun OpenSpec apply instructions. Current progress must remain 32/34 until then.

No claim is made for production abuse resistance, distributed rate limiting, exhaustive failure injection, email verification/recovery, real bidding, checkout, winners or orders. The local three-login sample is not a capacity benchmark. Static website-bundle inspection is not a browser network capture. The change has not been synced, archived or declared accepted on the roadmap; archived evidence remains historical.

## Cleanup

The temporary browser tabs and viewport override were cleared. The owned acceptance server and Vite process were stopped, the browser fixture schema was dropped through `scripts/dbfixture`, and the disposable PostgreSQL container/ephemeral database was removed. Temporary operator inputs and server log were deleted. The implementation, binary and sanitized verification artifacts remain in the workspace.
