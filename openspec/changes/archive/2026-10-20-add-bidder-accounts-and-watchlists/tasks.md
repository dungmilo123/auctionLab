# Tasks

## 1. Add durable commerce storage

- [x] 1.1 Add migration `002_accounts_and_watchlists.sql` for accounts, hashed sessions, watchlist memberships, and account-seed markers; append the migration and advance `UserVersion`; verify fresh/repeated migrations and required uniqueness/role/foreign-key constraints with real PostgreSQL fixtures.
- [x] 1.2 Add an actual version-1-to-version-2 upgrade fixture with populated catalog and game records; verify stable data/capability/revisions, no inferred accounts, rollback of a failing migration 2, and newer-ledger rejection; replace affected hard-coded schema-version test assumptions with the supported constant.
- [x] 1.3 Extend `internal/store/sqliteimport.go` occupied-target protection to all four commerce tables; verify first import works only on an unused upgraded fixture and account-occupied targets fail without writes, while completed-source repeat and verify-only behavior remain compatible.
- [x] 1.4 Document migration 2, required PostgreSQL/test targets, backup/compatible-build rollback, and the absence of player conversion in `docs/postgresql.md`; verify the described schema and health-version contract against migration tests without modifying migration 1 or historical evidence.

## 2. Implement account validation and credentials

- [x] 2.1 Add `internal/account` types, repository interfaces, safe profile projection, and email/name/password validation from design decision 1; verify canonical-email collisions, bounds, Unicode password preservation, rejected fields, and role defaults with focused domain tests.
- [x] 2.2 Implement the versioned PBKDF2-HMAC-SHA256 verifier, random salts/IDs/tokens, digest-only session persistence, constant-time verification, and unknown-email dummy work; verify real-policy hash round trips, distinct salts, malformed verifier rejection, and absence of secrets from safe projections.
- [x] 2.3 Add the bounded direct-peer attempt limiter and four-slot credential-work limiter with injectable clocks; verify 429/retry recovery, bounded 4,096-key state, no trust in forwarded IP headers, and no permanent account lockout.
- [x] 2.4 Record the password/identity policy and verifier-version maintenance notes in the account domain documentation; verify they match implemented policy tests and do not publish real credentials or claim email verification.

## 3. Implement account/session persistence and APIs

- [x] 3.1 Implement account registration, login session creation/replacement, session lookup/expiry, self-profile update, and current-session revocation using leased-connection write transactions; verify atomic registration, concurrent duplicate email, fixed expiry, old-session replay denial, unaffected other-browser sessions, and request-cancellation behavior against PostgreSQL.
- [x] 3.2 Classify account commit uncertainty and unavailable/lost-writer errors without exposing SQL or credentials; verify no false success or new login cookie on failure, safe registration recovery via login, and no writer reacquisition after lease loss.
- [x] 3.3 Add `/api/auth/register`, `/api/auth/login`, `/api/auth/logout`, and GET/PATCH `/api/account`, with strict 8 KiB JSON decoding, unknown-field rejection, `X-AuctionLab-Account: 1`, origin checks, no-store envelopes, and the host-only session cookie; verify statuses, no-body/trailing/null JSON rejection, HTTP/HTTPS/proxy cookie flags, and idempotent logout with handler tests.
- [x] 3.4 Wire the service and private namespace guard into `cmd/server/main.go`; verify direct private requests and private misses require the session, profile ownership cannot be supplied, and bidder/admin account cookies alone fail catalog reads/previews/writes while the existing capability still works independently.
- [x] 3.5 Document the new API/error/cookie contract and account-role/capability distinction in `web/README.md` and the operator overview; verify examples against handler checks and keep all secrets out of the examples.

## 4. Implement private published-lot watchlists

- [x] 4.1 Implement owner-scoped save/remove/state operations using stable catalog IDs and the validated session inside write transactions; verify unpublished/missing save targets share a 404, concurrent/double saves create one row without changing saved time, and repeat/absent removal is harmless.
- [x] 4.2 Implement newest-first watchlist reads with stable ID ties, one coherent membership/public-catalog snapshot, existing public projections, and `meta.server_time`; verify two-account isolation, projection non-leakage, unpublished omission, elapsed-lot retention, and stable ordering on PostgreSQL fixtures.
- [x] 4.3 Add GET `/api/account/watchlist`, GET/PUT/DELETE `/api/account/watchlist/{lotID}` with the account request/authorization policy; verify invalid sessions, cross-origin writes, foreign owner fields, capability-only access, unknown IDs, safe errors, and uncertain-result recovery semantics through HTTP tests.
- [x] 4.4 Document watchlist API shapes, idempotency, ordering, and planned-schedule semantics in `web/README.md`; verify the examples with the API checks and confirm existing public catalog response types remain unchanged.

## 5. Provision ready-to-use demo accounts

- [x] 5.1 Implement transactional, versioned provisioning for two demo bidders and one admin using the fixed reserved-domain emails and operator-supplied passwords; verify real login, stable identities, demo/role labels, all-or-nothing collision handling, and already-applied behavior that preserves passwords/sessions/watchlists.
- [x] 5.2 Add mutually exclusive `--seed-accounts` maintenance wiring before engine/listener/tunnel startup; verify first-run missing credentials, conflicting flags, active-writer refusal, no normal-startup seeding, and sanitized status-only output with server/maintenance tests.
- [x] 5.3 Document stopped-writer provisioning and the three password environment inputs in `README.md`/`docs/postgresql.md`; verify the documented flow on an owned disposable schema without printing password values, changing the catalog seed, or installing default credentials.

## 6. Build account navigation and authenticated views

- [x] 6.1 Add `/account/login` and `/account/register` to route/title tables and implement a safe `return_to` helper preserving permitted discovery queries; verify direct routes, one-trailing-slash semantics, native history, fallback targets, external/encoded/backslash/control-character attempts, and authentication-loop exclusions with route/helper tests.
- [x] 6.2 Add the typed account API client and in-memory account state provider with checking/signed-out/signed-in/unavailable states, stale-request protection, and restoration/focus revalidation; verify 401 versus service failures, clearing private state on identity changes, and independent public catalog loading with focused client/state tests.
- [x] 6.3 Build labeled login/register forms with field errors, pending state, password autocomplete, retry guidance, and native navigation only after confirmed success; verify invalid/duplicate input, failed login, throttling, service failure, and no automatic watch mutation using the client/state checks and observable form behavior.
- [x] 6.4 Replace the account overview preview with real safe profile, read-only email, editable name, demo/role labels, and explicit logout; verify durable edit confirmation, retained failed inputs, logout failure recovery, and no private-data display before session validation.
- [x] 6.5 Add complete typed EN/VI account copy and update shared notices/footer/admin copy to distinguish available accounts/watchlists from unavailable bids/checkout/orders; verify key completeness/interpolation, route titles, unchanged game-copy tests, and no frontend capability/password constants.
- [x] 6.6 Update `web/README.md` route and account-state documentation alongside the views; verify direct login/register/account loads in Vite, keyboard focus/errors, and that bids/orders stay honest previews with no game token restoration or sockets.

## 7. Build watchlist interactions

- [x] 7.1 Add a separate authenticated save/remove control to published lot detail, with membership loading, confirmed/pending/error states, and an anonymous login-return link; verify save state cannot race across lots/accounts, no save happens automatically after login, and public content remains usable during account failure.
- [x] 7.2 Build `/account/watchlist` with real public lot cards, deterministic order/count, links/removal, and distinct checking/login-required/empty/unavailable/error states; verify confirmed removal, retry/uncertain membership reload, elapsed planned labels, and no fallback fixture or other-account data.
- [ ] 7.3 Complete EN/VI watchlist copy, announced result messages, disabled pending actions, and keyboard/mobile layout; verify client/state transitions and observable 320-pixel/200-percent-zoom behavior with a long-title/missing-image fixture.
- [x] 7.4 Update watchlist usage documentation and account/lot-page descriptions in `web/README.md`; verify examples against the implemented controls and preserve the explained disabled bid panel and existing fulfillment disclosures.

## 8. Verify the complete Change 4 boundary

- [x] 8.1 Run a two-account integration rehearsal on an owned disposable PostgreSQL schema covering register/login/profile/save/remove/logout, cookie replay, later login, restart persistence, session expiry, seed repeat, and both retained game reset modes; verify independent watchlists survive and account actions do not change game revisions, publication state, or legacy identities.
- [x] 8.2 Run `make test`, `make test-race`, and frontend-first `make build` with an explicit isolated `TEST_DATABASE_URL`; verify all existing/new suites pass, the no-CGO embedded binary starts with schema 2, health reports the supported version, and no operational schema or SQLite source is touched.
- [ ] 8.3 Perform browser acceptance in Vite and the embedded binary using isolated cookie contexts: direct links/refresh, two accounts, Back/restoration after logout, safe return navigation, capability denial, EN/VI, keyboard, mobile/zoom, backend failure, and public discovery with account failure; verify bidding/orders stay unavailable and website pages open no game sockets.
- [x] 8.4 Create `verification/REPORT.md` with requirement-to-evidence mapping, executed commands/outcomes, sanitized screenshots, and explicit unmeasured/deferred boundaries; verify no credentials/cookies/private rows appear in evidence and only mark completed tasks after their stated checks pass.

## Workflow follow-up

- Review and verify the implemented change before syncing/archiving through the explicit OpenSpec workflow.
- Update roadmap completion status only after implementation acceptance; preserve prior archived reports and defer Change 5 bidding to its own proposal.
