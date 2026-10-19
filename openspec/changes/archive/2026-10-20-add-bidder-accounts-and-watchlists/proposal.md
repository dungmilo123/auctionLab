# Proposal

## Why

Change 3 provides real published auction discovery, but `/account` and `/account/watchlist` still show unavailable previews. Change 4 needs durable commerce identities and private saved lots before Change 5 can attribute bids to authenticated participants.

## What Changes

- Add email/password registration, login, logout, persistent server-side sessions, and an account overview with an editable display name. Email verification, password recovery/change, social login, and account deletion are outside this change.
- Establish server-resolved bidder/admin account roles. Public registration creates bidders only; explicit operator maintenance provisions demo bidders and an admin account. Preserve the existing admin document gate and the mandatory catalog capability cookie; an account session never substitutes for that credential.
- Add authenticated, per-account watchlists for published lots, with save/remove on lot detail and a working `/account/watchlist` page. Preserve saved lots across logout, later login, and application restarts.
- Keep public discovery anonymous and usable when account services fail. Keep bidding, bid history, winners, checkout, orders, analytics, and recommendations unavailable until their roadmap changes.
- Keep commerce accounts and their credentials independent of retained game players, nickname/token identity, budgets, truth values, and WebSockets.
- Add explicit, repeat-safe account seeding and update operator/frontend documentation and notices to distinguish working accounts/watchlists from future transactions.

## Capabilities

### New Capabilities

- `bidder-accounts`: Commerce account validation, password credentials, durable sessions, self-profile access, role boundaries, and explicit demo account provisioning.
- `lot-watchlists`: Authenticated ownership, repeat-safe saving/removal of published lots, private public-field projections, and accessible watchlist interactions.

### Modified Capabilities

- `website-navigation`: Recognize login/register destinations, load real account/watchlist pages, and narrow preview restrictions to remaining unimplemented transactions.
- `catalog-administration`: Update management notices that currently describe accounts and watchlists as unavailable while preserving the capability authorization contract.
- `postgresql-persistence`: Extend ordered migrations and isolated database acceptance to accounts, sessions, watchlists, and account seed records; retain unused-target import protection.

## Impact

- Backend: new account/watchlist domain and repository code, JSON handlers in `internal/api`, additive PostgreSQL migration after `001_initial.sql`, and service/maintenance wiring in `cmd/server/main.go`.
- Frontend: `web/src/routes/account`, route/title tables, account state and API client, shared layouts/copy, and lot-detail watch controls using existing EN/VI design components and native document navigation.
- APIs: new `/api/auth/*`, `/api/account`, and `/api/account/watchlist*`; existing public catalog DTOs, catalog admin authorization, and retained game protocol remain compatible.
- Operations: PostgreSQL 18 remains required, with no new external service or CGO dependency. Password hashing uses Go's standard-library PBKDF2-HMAC-SHA256; credentials and raw session tokens are excluded from source, browser storage, and verification reports.
- Verification: real PostgreSQL migration/restart/isolation checks, handler authorization and request-boundary checks, frontend state/navigation checks, and browser acceptance in Vite and the embedded binary. Archived evidence and roadmap completion status are not rewritten by this proposal.
