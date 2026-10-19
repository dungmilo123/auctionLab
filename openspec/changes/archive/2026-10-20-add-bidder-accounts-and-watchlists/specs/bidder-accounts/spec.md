# Spec Delta

## Purpose

Provide persistent commerce identities and private account sessions for auction participants without inheriting retained classroom-game credentials or privileges.

## ADDED Requirements

### Requirement: Bidder registration
The system SHALL register a bidder using a unique normalized email, password, and display name. It SHALL validate all fields, reject unknown fields, and assign the bidder role on the server. Successful registration SHALL commit the account and a fresh session together before reporting success.

#### Scenario: Register and return later
- **WHEN** a visitor registers with a valid unused email, display name, and password
- **THEN** registration returns HTTP 201 with that account's safe profile and sets a new session cookie
- **AND** later login after logout or restart resolves the same stable account identity

#### Scenario: Normalize email and reject invalid input
- **WHEN** an email differs only by surrounding whitespace or ASCII letter case from an existing email
- **THEN** registration returns a conflict without creating another account
- **AND** malformed email, blank or overlong name, password outside the accepted bounds, or a supplied role/account ID is rejected without a partial account or session

#### Scenario: Simultaneous registration
- **WHEN** two requests register the same normalized email concurrently
- **THEN** at most one account is created and the other receives a conflict
- **AND** no password or database detail appears in either response

### Requirement: Password credential protection
Passwords SHALL be stored only as salted, computationally expensive verifiers, never plaintext or reversible credentials. Password input SHALL accept 12 through 128 Unicode characters without trimming or silent truncation. Authentication failures SHALL use one invalid-credentials response for unknown email and incorrect password.

#### Scenario: Incorrect credentials
- **WHEN** login supplies an unknown email or the wrong password for an existing email
- **THEN** it returns JSON HTTP 401 with the same error code and no session is issued
- **AND** both cases perform comparable password-verification work

#### Scenario: Secret handling
- **WHEN** an account is registered, authenticated, seeded, read, or updated
- **THEN** profile JSON, application logs, frontend persistence, and acceptance reports contain no plaintext password, verifier, salt, raw session token, or admin capability
- **AND** passwords are absent from URLs and browser storage

### Requirement: Durable bounded sessions
Successful login SHALL issue a new unpredictable opaque session credential in a host-only HttpOnly, SameSite=Lax cookie with Path=/ and Secure on HTTPS. Sessions SHALL have a fixed 30-day lifetime, be validated against durable server state on every private request, and survive server restart until expiry or revocation. Session credentials SHALL NOT be accepted through query parameters or game tokens.

#### Scenario: Login and reopen the site
- **WHEN** valid credentials are submitted and the browser later revisits with the issued cookie before its expiry
- **THEN** the account overview and watchlist resolve that account after a full document load or server restart
- **AND** a successful login replaces the current browser credential rather than adopting a client-selected token

#### Scenario: Expired or revoked session
- **WHEN** an account API receives an absent, forged, expired, or revoked session
- **THEN** it returns JSON HTTP 401 without private data, HTML, or redirects
- **AND** a database outage returns a sanitized service error rather than treating the session as authenticated or claiming the account is empty

### Requirement: Explicit logout
Logout SHALL revoke the current server-side session before reporting success and expire its browser cookie. Repeating logout with an absent or already revoked session SHALL succeed harmlessly. Logout SHALL preserve the account, its watchlist, and other browser sessions.

#### Scenario: Reuse a logged-out cookie
- **WHEN** a user logs out and a client replays the old session cookie
- **THEN** private account requests return HTTP 401
- **AND** logging in again restores the previously saved watchlist

#### Scenario: Failed revocation
- **WHEN** the session revocation cannot be confirmed because storage fails
- **THEN** logout reports a service failure without claiming server-side logout succeeded
- **AND** the UI offers recovery instead of displaying a fabricated successful logout

### Requirement: Private self-profile
Authenticated users SHALL read their own safe profile and update only their display name. The profile SHALL expose only stable account ID, email, display name, server-resolved role, and demo status. Requests SHALL derive ownership from the validated session and SHALL NOT accept another owner's ID, email changes, role changes, or password changes.

#### Scenario: Edit basic profile
- **WHEN** a logged-in user saves a valid display name on `/account`
- **THEN** the confirmed value persists across reload and later login
- **AND** email remains read-only and unsuccessful saves preserve entered text with an explicit error

#### Scenario: Attempt privilege or ownership changes
- **WHEN** a client supplies another account's ID or submits role, email, or password fields to the profile update
- **THEN** it receives a validation/request error and neither account nor role changes
- **AND** no API lists other accounts or exposes their private profile

### Requirement: Bidder and admin authorization boundary
Account roles SHALL be server-owned bidder or admin values. Public registration SHALL never grant admin. Account sessions SHALL NOT replace the existing admin document capability or independently authorize private catalog APIs. Possession of an admin capability SHALL NOT create an account session or reveal bidder-private information.

#### Scenario: Bidder attempts catalog administration
- **WHEN** an authenticated bidder without an admin capability requests admin documents or any private catalog read, preview, or mutation
- **THEN** the existing document gate or JSON HTTP 401 applies respectively
- **AND** neither a client role claim nor an account session grants catalog authority

#### Scenario: Account admin and capability remain distinct
- **WHEN** a provisioned admin account logs in without the catalog capability
- **THEN** its self-profile reports the server-owned admin role but catalog APIs remain inaccessible
- **AND** entering the existing capability gate retains its prior operator behavior without converting that capability into bidder identity

### Requirement: Account mutation request boundary
Registration, login, logout, profile updates, and watchlist writes SHALL require JSON and an explicit account-request header. Supplied Origin values SHALL match the browser-facing origin. GET SHALL NOT perform business mutations. Responses SHALL use no-store JSON and bounded, structured errors; cross-origin credential authorization SHALL NOT be enabled.

#### Scenario: Cross-site or malformed request
- **WHEN** a write carries a foreign/null Origin, lacks the account header, uses a form body, exceeds the body limit, or includes malformed/trailing JSON
- **THEN** the request is rejected without the requested mutation or a new session
- **AND** valid same-origin requests through Vite and the embedded server use the same checks

### Requirement: Bounded authentication work
Registration and login SHALL bound credential-processing concurrency and throttle repeated attempts without permanently locking an account. Throttled requests SHALL return HTTP 429 with retry guidance, and limiter state SHALL remain bounded. Request headers from untrusted clients SHALL NOT bypass client-address limits.

#### Scenario: Repeated authentication attempts
- **WHEN** requests exceed the authentication attempt or concurrent password-work limit
- **THEN** excess requests receive a recoverable throttling response without hashing unbounded work
- **AND** valid authentication works again after the bounded limit clears

### Requirement: Explicit repeat-safe demo accounts
An operator-only maintenance mode SHALL provision two labeled demo bidder accounts and one admin account using supplied credentials. It SHALL be explicit, transactional, repeat-safe, and separate from normal boot and catalog seeding. It SHALL neither expose credentials nor overwrite existing accounts, passwords, roles, sessions, or watchlists.

#### Scenario: Seed and repeat
- **WHEN** the operator provisions accounts on an eligible schema and repeats the command
- **THEN** the intended accounts can log in, retain stable identities and demo labels, and the repeat reports already applied
- **AND** no HTTP/tunnel/game service starts and no password is printed or shipped in frontend assets

#### Scenario: Collision or incomplete input
- **WHEN** required credentials are absent, an unowned account already uses a seed email, or another schema writer is active
- **THEN** provisioning fails clearly without partial accounts or a completion marker
- **AND** normal startup never installs default credentials

### Requirement: Independence from retained game identity
Commerce account actions SHALL NOT restore game tokens, create or alter game players, budgets, bids, runtime, revisions, or truth values, or open game WebSockets. Retained game reset SHALL NOT delete commerce accounts, sessions, or watchlists.

#### Scenario: Old player data in the same browser
- **WHEN** a browser with a saved player token registers, logs in, or opens a watchlist
- **THEN** only the commerce session determines identity and no game session is restored
- **AND** game state and revision remain unchanged by those account actions
