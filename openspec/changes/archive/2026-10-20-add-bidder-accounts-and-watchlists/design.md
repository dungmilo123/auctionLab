# Design

## Context

See `proposal.md` for motivation and the five delta specs for behavior. Discovery is implemented; `web/src/routes/account/AccountPage.tsx` is still a single unavailable-state component for overview/watchlist/bids/orders. `App.tsx`, `lib/route.ts`, and `layouts/SiteLayout.tsx` provide explicit route/title tables, EN/VI copy, and native document links. `LotDetailPage.tsx` loads the public projection and renders a disabled bid panel. There is no commerce auth service, account API, or account credential persistence.

`internal/store/schema.go` currently embeds only `migrations/001_initial.sql` and supports PostgreSQL schema version 1. `Store` has an eight-connection pool and a dedicated leased writer connection; `withWriter` serializes work and permanently fails closed after ownership loss. Catalog writes currently use pool transactions after checking ownership. Catalog public aggregates use Repeatable Read and explicit DTOs. `internal/testpg` requires `TEST_DATABASE_URL` and provisions disposable schemas.

`AdminGate` protects Go `/admin` documents and `CatalogAdminGuard` independently requires the `auction_admin` cookie for every private catalog request. The existing origin helper supports browser-facing schemes through a loopback proxy. `cmd/server/main.go` has explicit offline import/catalog-seed modes before HTTP/game startup. The SQLite importer has an explicit occupied-target table inventory. All of these are integration points rather than replaced subsystems.

The current `website-navigation` spec prohibits all account/watchlist mutations, and `catalog-administration` says accounts remain unavailable. Their deltas deliberately supersede those preview restrictions. The admin capability authorization requirement remains in force.

## Goals / Non-Goals

**Goals:**
- Keep account identity server-resolved, durable, independent of game state, and reusable by Change 5 without adding bids now.
- Make registration/session issuance and membership changes report committed results, with deterministic owner isolation across the PostgreSQL pool.
- Let public catalog loading succeed independently of optional account-state loading.
- Preserve native navigation, offline operation with local PostgreSQL, the no-CGO embedded binary, and the existing capability gate.

**Non-Goals:**
- Replacing catalog capability authorization with account login, creating role-management screens, or enabling a game/admin WebSocket from the website.
- Password/email changes, recovery, verification, identity federation, account deletion, or production abuse-prevention infrastructure.
- Local-storage authentication, offline credential acceptance, watchlist guest merging, automatic saving after login, or notifications.

## Decisions

### 1. Add an independent account domain and additive schema version 2

Create `internal/account` for validation, safe profile types, credential/session policy, and repository interfaces; add its PostgreSQL repository in `internal/store` and handlers in `internal/api`. Keep a small watchlist service beside it, depending on catalog public projection types rather than game types. Do not import `internal/game` or `internal/hub` into the new domain. Add `002_accounts_and_watchlists.sql` and append it to the migration array; never edit migration 1.

| Table | Proposed stored contract |
|---|---|
| `commerce_accounts` | Random stable text ID; unique canonical email; display name; encoded password verifier; checked `bidder`/`admin` role; demo flag; UTC millisecond creation/update timestamps |
| `commerce_sessions` | SHA-256 token digest primary key; account foreign key; creation/expiry UTC milliseconds; nullable revoked time; expiry later than creation; account/expiry indexes |
| `commerce_watchlist` | `(account_id, lot_id)` primary key; foreign keys to commerce account and `catalog_lots`; saved UTC milliseconds; index supporting account/newest-first reads |
| `commerce_account_seed_runs` | Seed-version primary key and completion time, committed with all seeded accounts |

Use application-generated random account IDs from `crypto/rand`, not legacy player IDs. Do not store profile shipping/contact fields before checkout needs them. Store only one display name and one email per account; display names are not unique. Email is trimmed, ASCII lowercased, limited to 254 bytes, and validated as a bare address with no display-name syntax, whitespace, or non-ASCII characters. This explicit demo identity policy avoids claiming complete internationalized email support. Names are trimmed, 1–80 Unicode characters, and reject controls. Passwords are not normalized or trimmed: 12–128 Unicode characters, at most 512 UTF-8 bytes, rejecting invalid UTF-8. All string rendering remains escaped.

**Alternative:** extending `players` would inherit nickname uniqueness, game tokens, budgets, and reset behavior. Independent commerce tables preserve both lifecycles and make ownership explicit.

### 2. Use a versioned password verifier with bounded work

Use standard-library `crypto/pbkdf2` with HMAC-SHA256, 600,000 iterations, a fresh 16-byte random salt, and 32-byte output. Encode algorithm/version/iteration count/salt/output in one private verifier string; parse with fixed supported bounds and compare derived bytes with `crypto/subtle`. Unknown emails use a fixed-format dummy verifier so normal invalid-credential paths perform the same password work. The checked-in Go toolchain exposes `crypto/pbkdf2`, so this adds no runtime service or third-party dependency.

Hash/verify before acquiring a database writer transaction or game lock. Limit active registration/login password work to four requests per process, with immediate JSON 429 and `Retry-After` when full. Add a shared registration/login limit of ten attempts per minute per direct peer IP, with a bounded 4,096-entry expiring map; reject new keys with 429 if all slots are active. Ignore `X-Forwarded-For` and similar client IP headers. Keep clock/hash interfaces injectable for fast deterministic tests, while retaining a small real-policy hashing check. Do not place fake verifiers into integration/demo data.

**Alternatives:** fast digests do not provide suitable password work; adding Argon2id introduces a dependency and a separate memory/concurrency policy. PBKDF2 is a deliberate self-contained baseline, with algorithm tagging to permit later migration. The bounded limiter is demo-scale and is not a distributed abuse-control claim.

### 3. Persist opaque sessions and revoke them explicitly

Generate a fresh 32-byte random token per successful login/registration, base64url encode it for the `auction_account` cookie, and persist only its SHA-256 digest. Session expiry is fixed at creation plus 30 days, with no sliding extension or refresh endpoint. The cookie is host-only, HttpOnly, SameSite=Lax, Path=/, with Max-Age/Expires aligned to server expiry and Secure for the effective HTTPS scheme. Reuse the established loopback-only scheme trust rule; do not trust forwarded Host. No raw token appears in JSON, logs, URLs, localStorage, or sessionStorage.

Registration inserts account and session in one transaction. Login verifies credentials, then creates the session and revokes the supplied current valid browser session in one transaction; invalid old cookies do not choose or fix the new token. Other browsers' sessions remain valid. Set the new cookie only after commit. `/api/account` reads the current session and account together from storage on every request; role/expiry are not cached as frontend authority. Expiry and revocation are checked even if the cookie remains present.

Logout revokes the matching current session and then clears the cookie using identical path/security attributes. Absent/unknown/revoked sessions succeed idempotently; a storage failure is an explicit failure, not confirmed logout. Delete at most 100 expired session rows opportunistically within successful session-writing transactions; no new worker is needed.

**Alternative:** client-stored JWTs or game tokens make explicit durable revocation and identity separation harder. Server sessions fit the existing PostgreSQL requirement and the fixed 30-day visit-persistence criterion.

### 4. Preserve the catalog capability and distinguish it from account roles

Public registration fixes role to `bidder` and rejects extra role/ID input. Only operator maintenance provisions the initial `admin` account. The authenticated account principal includes stable ID and the server-stored role; it is the future input to bidder/admin guards. Current self-profile/watchlist operations allow either account role and always enforce ownership.

The existing catalog and document capability requirements are authoritative here: neither a bidder nor an admin account session alone authorizes catalog management. Existing capability-only operator access remains compatible; a capability alone does not authorize account APIs or select a private watchlist. The seeded admin account establishes a durable account role, but its catalog work still requires the gate cookie. Explain this in operator docs so the account role is not presented as replacing the gate. No public role assignment, promotion endpoint, or admin-account UI is added.

**Alternatives:** replacing the gate now conflicts with the current project constraint; requiring both account login and capability for existing operators would change the established access contract. Keeping separate authority checks is additive and leaves replacement to an explicit later decision.

### 5. Define small JSON APIs with strict request boundaries

Register an account API alongside `RegisterCatalog` in `cmd/server/main.go`. Authenticated account routes, including unknown paths under the private account namespace, first resolve the session and then route the request. Use explicit DTOs instead of serializing repository records.

| Method | Route | Request / confirmed response |
|---|---|---|
| POST | `/api/auth/register` | `{email,password,display_name}` → 201 `{data: profile}` and session cookie |
| POST | `/api/auth/login` | `{email,password}` → 200 `{data: profile}` and new session cookie |
| POST | `/api/auth/logout` | `{}` → 200 `{data: {signed_out: true}}` and expired cookie |
| GET | `/api/account` | 200 `{data: {id,email,display_name,role,demo}}`; 401 if signed out |
| PATCH | `/api/account` | `{display_name}` → 200 `{data: profile}` |
| GET | `/api/account/watchlist` | 200 `{data: [publicLot, ...], meta: {server_time}}` |
| GET | `/api/account/watchlist/{lotID}` | 200 `{data: {saved: boolean}}` for a published lot; common 404 otherwise |
| PUT | `/api/account/watchlist/{lotID}` | `{}` → 200 `{data: {saved: true}}` |
| DELETE | `/api/account/watchlist/{lotID}` | `{}` → 200 `{data: {saved: false}}`, including an absent membership |

Profile exposes the five fields in the table only. List projections reuse the public catalog lot shape, including its stable ID and authored media/fulfillment; no owner IDs or save timestamps need to be returned. Membership state is private and never appended to cacheable/public catalog DTOs. List `server_time` follows discovery's UTC-millisecond clock contract.

Every mutation, including register/login/logout, requires `Content-Type: application/json`, `X-AuctionLab-Account: 1`, a matching supplied Origin, a maximum 8 KiB request body, known fields only, and exactly one JSON object. Requests without Origin are allowed for explicit non-browser clients only under the same JSON/header rules. Reuse the existing origin algorithm without weakening the admin guard or enabling CORS. Return no-store JSON for success/errors and no credential redirects from APIs. Frontend calls use same-origin credentials through the existing Vite `/api` proxy.

Use the catalog-style `{error:{code,fields?}}` shape with codes for validation (422), malformed request (400), duplicate email (409), invalid credentials/access required (401), request rejected (403), unsupported media (415), not found (404), throttled (429), and unavailable/commit unknown (503). Domain-specific errors must not expose SQL, email existence through login, credential values, or connection strings. Registration's duplicate-email conflict is a deliberate convenience boundary in this unverified demo flow.

**Alternative:** reusing admin endpoints or query-key authorization would mix unrelated credentials; adding a general auth framework is unnecessary for this bounded session API.

### 6. Serialize new writes on the existing leased connection

Implement account/session/watchlist repository writes inside `Store.withWriter` using transactions on its dedicated connection. Check ownership before mutation and keep the lease lifecycle unchanged; never reconnect to reacquire it. Use parameterized SQL and the existing bounded operation deadline, respecting earlier request cancellation. Hash work stays outside this critical section. Read operations use the pool and bounded contexts. Resolve and revalidate the active session inside private write transactions so a logout committed first prevents a later profile/watch write.

Handle commit errors with operation-specific sanitized classifications. Extend `safeError.CommitUncertain` deliberately for new account transaction commits rather than mislabeling them as ordinary validation or retryable reads. A login/registration commit uncertainty issues no cookie and reports no success; the user can retry login, and a possibly committed registration can resolve via login instead of blind account creation. A watchlist uncertainty reloads membership before another action. Do not broaden the catalog transaction pattern or game engine as part of this work.

**Alternative:** issuing account writes on arbitrary pool connections after a one-time lease check leaves a larger ownership gap. Holding the leased writer during these short writes gives simple serialization for this single-process demo without holding it during password computation.

### 7. Store only memberships and read coherent public lot projections

Watchlists reference stable catalog lot IDs, not slugs or copied catalog snapshots. In the save transaction validate that both the lot and its parent event are published, then insert with `ON CONFLICT DO NOTHING`; preserve the original `saved_at`. Remove with an owner-scoped delete and return `saved:false` whether or not a membership existed. Never accept `account_id` from the caller. Deletion performs no draft lookup, avoiding disclosure of unsaved draft IDs.

Read the authenticated owner's memberships and corresponding public lot projections from one Repeatable Read snapshot, ordered by `saved_at DESC, lot_id COLLATE "C" ASC`. Reuse `catalog.PublicLot` projection logic; add a repository query/reader helper instead of fetching the complete public catalog and private membership in unrelated snapshots. A publication occurring concurrently can yield only a fully published set. Missing/nonpublic related content is omitted, with no private notes/version fallback. Elapsed lots remain saved; their planned phase uses the existing server-time presentation helpers.

Lot detail fetches its private membership only after account resolution; public content loading never waits for a successful account request. Avoid fetching every watchlist for every discovery card; the first implementation offers save/remove on lot detail and remove on the dedicated watchlist page. Index/card-wide actions can be added separately if useful.

**Alternative:** client-only bookmarks cannot meet cross-visit ownership; storing full lot copies risks stale content and bypassing public projection rules.

### 8. Add account views without changing the navigation model

Add `/account/login` and `/account/register` to `lib/route.ts` and the exhaustive title map. Split account rendering into auth forms, overview/profile, watchlist, and retained bids/orders previews. Reuse the vintage components, normal anchors, existing account shell, and an account-specific typed EN/VI dictionary; do not change the retained game's dictionary/fixed-count expectations.

A top-level account state provider calls `GET /api/account`, with distinct checking, signed-out, signed-in, and unavailable states. It holds safe profile data in memory only. A 401 clears private state; a network/503 error remains unavailable. Private views wait for validation and never flash a guessed identity. Abort or ignore stale requests on navigation/retry/account changes. After logout/account changes, clear watchlist state before displaying another identity. Recheck on page restoration/focus and suppress stale private views until revalidation, including `pageshow` restoration from browser history. Public discovery renders independently and treats account errors as a local watch-control error.

Signed-out overview shows login/register links, while signed-out watchlist explains the login requirement with `return_to=/account/watchlist`. Signed-in overview shows the safe profile, read-only email, editable name, demo/role labels, and explicit logout. Keep bids/orders as future-feature states; do not show fake balances, bidders, bids, wins, or orders. Update shared preview notices/footer/admin copy so they no longer say accounts/watchlists are unavailable.

On lot detail show a separate save/remove control with confirmed membership, disabled pending state, and an announced success/error region. Do not optimistically claim membership. Anonymous save links lead to login with a return target for the same lot; after login no save occurs until an explicit action. Watchlist has loading, login-required, empty, success, and recoverable failure views with real links and count.

Validate `return_to` as an origin-relative recognized storefront or account path using the existing route resolver. Reject schemes, authority-style `//`, backslashes, control characters, malformed/ambiguous encoding, unknown routes, auth-loop destinations, and admin/API/WS/asset paths; preserve ordinary discovery query strings after validation and omit fragments. Invalid targets fall back to `/account`. Form success uses native document navigation after the server response, preserving cookie and reload semantics. Never return to a URL supplied through a server redirect API.

**Alternative:** introducing a client-side router or automatic pending-watch replay adds behavior not needed by the current native navigation contract.

### 9. Provision demo accounts explicitly with stopped-writer maintenance

Add `--seed-accounts` as a mutually exclusive offline mode alongside import and catalog seeding. It migrates, uses the schema writer lease, and exits before engine/hub/tunnel/listener creation. First provisioning requires `DEMO_BIDDER_ONE_PASSWORD`, `DEMO_BIDDER_TWO_PASSWORD`, and `DEMO_ADMIN_PASSWORD`, validated by the real password policy; no defaults are compiled or written into frontend assets/docs. Provision the fixed reserved-domain emails `bidder-one@demo.auctionlab.test`, `bidder-two@demo.auctionlab.test`, and `operator@demo.auctionlab.test`, with clear demo names and flags; the first two are bidders and the third is admin.

Insert all three accounts and seed version 1 in one transaction. An existing completion marker returns `already_applied` without resetting credentials, requiring their re-entry, or touching sessions/watchlists. Any preexisting seed-email collision without that marker fails the whole seed. Print only status and safe account identities/roles; passwords remain operator-supplied secrets. Normal boot does not read seed password variables or create demo accounts. Seed does not publish or reseed catalog content.

Extend the import occupied-target inventory to include all four commerce tables. A first import cannot run into an account-initialized schema. Existing same-source completed-import behavior remains a no-overwrite repeat. Game resets continue to affect only retained game tables; verify accounts and memberships survive both reset modes. Defer general demo reset tooling to Change 7.

**Alternative:** fixed public passwords or automatic startup seeding would silently create credentials on every deployment and confuse catalog seeding with account ownership.

## Risks / Trade-offs

- [Two administrative credentials] → Document that account role and catalog capability are separate; verify account sessions alone fail every private catalog entry point. A future replacement needs its own spec change.
- [Unverified email identity and no recovery] → Describe email as an unverified login identifier, retain read-only email, and keep verification/recovery outside this demo change.
- [HTTP demonstration traffic] → Cookies support local HTTP and become Secure on HTTPS; document the browser-facing scheme and local proxy assumptions. No password is sent in URL navigation.
- [PBKDF2 CPU and direct-peer limits behind a proxy] → Bound concurrent work and limiter size, verify representative login latency, and document that shared loopback/tunnel peers share the limit. Do not introduce distributed infrastructure for this demo.
- [Account storage failure while discovery works] → Keep the account-service error local to private views/watch controls and continue public browsing.
- [Old private DOM restored by history] → Revalidate on restoration and clear/suppress private state before displaying it; keep private API responses no-store and verify Back after logout.
- [Uncertain commit] → Separate failure from confirmed success; never issue a login cookie before commit and reload repeat-safe membership state before retrying.
- [Schema rollback] → Version 1 binaries intentionally refuse version 2. Preserve a pre-upgrade backup and use a compatible build or explicit stopped-service restore; no silent down-migration or identity deletion.

## Migration Plan

1. Implement additive migration 2, domain/repositories, HTTP routes, and frontend views; update schema-version test assumptions and health expectations to the supported constant rather than hard-coded 1/2 values.
2. On isolated PostgreSQL fixtures, verify fresh/repeated migration, an actual version-1 upgrade with populated game/catalog data, failed-step rollback, and newer-ledger rejection. Verify the offline importer accepts an unused upgraded schema and refuses an account-occupied target.
3. Stop the writer, back up the application schema, and start the compatible application to migrate. Do not edit migration 1, import an operational SQLite source, or run destructive tests on the operational schema.
4. Optionally stop the server and run documented `--seed-accounts` with operator-supplied passwords, then restart and exercise two independent browsers. Catalog sample content still requires its separate explicit seed/review/publication flow.
5. Complete unit/integration checks and `make test`, `make test-race`, and `make build` with an explicit disposable `TEST_DATABASE_URL`. Use temporary schemas for browser acceptance in Vite and the embedded binary; record outcomes and sanitized evidence under the change's `verification/` directory.
6. Acceptance covers registration/login/profile/logout, two-account isolation, membership repeat safety, fixed expiry, restart persistence, admin denial, public discovery during account failure, disabled bidding, EN/VI, keyboard/mobile/zoom, and direct links/Back after logout. Preserve all existing catalog/game checks and archived evidence.
7. Roll back with the service stopped using a compatible application build, or restore the pre-upgrade backup with explicit acknowledgement that later accounts/watchlists are absent from that snapshot. Do not drop commerce tables as an automatic rollback.
