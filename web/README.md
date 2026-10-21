# AuctionLab — Collected Stories

A vintage auction website using React, Vite, and TypeScript. Authorized catalog administration stores drafts and publishes complete events on PostgreSQL 18. Public discovery reads published records with search, category/phase filters, sorting, galleries and planned schedules. Commerce registration, sessions, self-profiles and private watchlists are available. Independent timed auctions, manual bids and private participation are available. Private winner checkout with simulated payment is available; order history and fulfillment remain Change 7 work.

## Run and build

From the repository root:

| Command | Behavior |
|---|---|
| `make dev` | Go :8080 and Vite :5173; API, WS, runtime images proxy to Go |
| `make dev-web` | Vite only; discovery needs a matching Go API, account views need the matching Go account API |
| `make test` | Existing Go tests, frontend unit tests, oxlint |
| `make build` | Typecheck + Vite build **before** `CGO_ENABLED=0 go build -o bin/auctionlab ./cmd/server` |

The built binary embeds `web/dist`; Node is not needed at runtime. `dist/.gitkeep` stays committed and is restored by the Vite closeBundle hook after every build. Fonts and illustrations are local; running offline needs the application server, a local PostgreSQL 18 service, and media, with no CDN dependency. A first dependency install/build may require network access.

## Routes and document navigation

| Path | Layout | Contents |
|---|---|---|
| `/` | Storefront | Brand introduction, featured published events, nearest planned closes and category links |
| `/auctions` | Storefront | Published events with GET planned-phase filter |
| `/auctions/:slug` | Storefront | Matching published event with cover, story, planned times and ordered lots |
| `/lots` | Storefront | Published lots with GET search/category/phase/sort controls and count |
| `/lots/:slug` | Storefront | Matching published lot, ordered gallery, content, disclosures and a separate live auction panel |
| `/account` | Account | Session-validated self-profile or sign-in recovery |
| `/account/login` | Account | Email/password sign-in with safe return navigation |
| `/account/register` | Account | Bidder registration with safe return navigation |
| `/account/watchlist` | Account | Private saved published lots and confirmed removal |
| `/account/bids` | Account | Session-owned accepted participation and leading/outbid/won/lost filters |
| `/account/wins` | Account | Owned awaiting-payment wins |
| `/account/checkout/:orderId` | Account | Owned checkout or stored simulated confirmation |
| `/account/orders` | Account | Change 7 order-history preview |
| `/admin` | Admin | Management overview |
| `/admin/auctions` | Admin | Stored event list, draft editor, schedule/order, preview and publish |
| `/admin/lots` | Admin | Stored lot list and draft content editor |
| `/admin/orders` | Admin | Processing unavailable |
| All other paths, including `/screen` and descendants | Storefront | Explicit 404 with a home recovery link |

`src/lib/route.ts` matches case-sensitive segments, ignoring query/fragment and allowing one trailing slash. Detail routes accept exactly one nonempty slug and resolve the published record or a common draft/missing not-found view. Slugs are not inserted into HTML. No game alias is provided. The old player/admin/projector source and tests remain unmounted for future evaluation; the backend, database, protocol, and existing game data are preserved. Saved player tokens are not read or restored by the website.

Navigation uses normal `<a href>` document links, with no click interception or History API router. Refresh, direct load, Back/Forward, and new-tab links retain normal browser semantics and the Go document gate. Full navigation reloads stored catalog records; unsaved local inputs are transient. Exact active destinations use `aria-current="page"`; each layout has a skip link to `main#main` and a shared footer. Context navigation wraps on mobile and becomes a sidebar at 1024px.

The frontend 404 is a view: Go document fallback returns HTTP 200 with `Cache-Control: no-cache`. Missing `/assets/*` and `/site-assets/*` files return real HTTP 404 without SPA HTML in Vite and Go. Only hashed `/assets/*` files are immutable; hand-managed files belong in `public/site-assets/`. `/api`, `/ws`, and runtime `/images` remain owned by the backend.

## Admin and account boundary

Vite serves the admin document publicly; every catalog data request independently requires the Go API capability. Without it, management shows an explicit access/backend error. The Go origin protects `/admin` and its subtree with the existing admin capability gate. Read the entry link from the server log. A correct key sets the existing HttpOnly, SameSite=Lax, 12-hour capability cookie and redirects to literal `/admin`, removing the key from the URL. Nested navigation/refresh then uses the cookie. Missing capability gets the bilingual gate; a wrong nonempty key returns 401 without echoing it. The gate uses self-contained CSS and system fallbacks. Commerce sign-in and safe return navigation use their independent account APIs and cookie. Account roles never substitute for the catalog capability; that capability never selects a private account.

## Presentation components

Tokens live in `src/index.css`: ivory `#F6F1E8`, charcoal `#24211E`, burgundy `#722F37`, olive status accents, decorative brass. Headings use local Noto Serif 400/600 and Georgia fallback; body uses Noto Sans 400/600 and system-ui fallback. Type roles are 14/16/20/28px with 36–56px fluid hero, 1.5 body / 1.15 heading leading. Prices/times use `.numeric` tabular numerals. Spacing is 4/8/12/16/24/32/48/64px. Controls use 1px borders, 4px radius, 44px minimum targets, visible focus with an ivory gap, and readable disabled/error states. No hard offset shadows. Reduced motion disables nonessential transitions. Grid: 1 column below 640px, 2 at 640px, 3 at 1024px; content max 1200px with 16/24/32px gutters.

- `components/ui/`: existing `Button` (default/outline/secondary/ghost/destructive/link; `asChild` for a real link), `Input`, `Label`, and `Card` family use the shared tokens. Use `type="button"` for UI actions. Never use an anchor as a disabled button.
- `components/site/Presentation.tsx`: `LotCard({title, metadata, status, image?, href})` has one primary detail link; `StatusBadge({status})` accepts upcoming/live/ended/sold/unavailable and always includes text; `LotImage({src?, alt})` handles failures with a local fallback plus translated caption; `PreviewNotice`, `PageHeading({title, intro?})`, `UnavailableState({title, children})` are shared presentation components.
- `Field({id, label, help, error?, ...inputProps})` associates its label, help, and optional invalid/error text. Example: `<Field id="sample" label="Collection" help="Preview only" error="Example error" defaultValue="" />`. Discovery forms submit native GET requests. Catalog controls use the same design tokens and associations, without frontend credential inputs.
- `fixtures/site.ts`: three illustrative objects, independent of game wire types and API clients. A long title exercises wrapping. These retained fixtures are not mounted by public discovery.
- `lib/site-copy.ts`: typed EN/VI website dictionary and `useSiteCopy()` integrate with the existing language provider and `auctionlab.lang` preference. Retained previews can use `t('imageAlt', {object: title})`; published galleries/cards use authored alternative text. React escapes output. Game copy and its fixed-count tests remain unchanged. App updates document title/language for each route and language.
- `layouts/SiteLayout.tsx`: shared header/footer plus storefront, account, admin shells, all composed with native links.

Asset sources, exact package releases, font metadata/licenses, checksums, and illustration provenance are in `src/assets/README.md`. Fonts/illustrations imported from `src/assets` are fingerprinted (small SVGs may be embedded as local data URLs); the shared fallback is `/site-assets/image-unavailable.svg`. No remote images or styles.

## Verification

Route and bilingual-copy tests cover matching, misses, completeness, and interpolation. Static HTTP tests cover deep links, fonts/images, reserved asset misses and cache policy; admin tests retain key/cookie/redirect and bilingual error semantics. Existing game tests remain in place. Change-local browser observations, screenshots, computed contrast, and acceptance mapping are under `openspec/changes/archive/2026-10-19-establish-vintage-design-and-site-foundation/verification/` (this completed archive and its main specs are explicitly tracked; the general OpenSpec ignore rule remains in place).


## PostgreSQL runtime

See [setup and operations](../docs/postgresql.md) for local PostgreSQL 18 provisioning, separate app/test roles, required `DATABASE_URL`, `DATABASE_SCHEMA`, explicit `TEST_DATABASE_URL`, readiness, and disposable test/load schemas. Runtime `DB_PATH` and SQLite fallback are removed. `PROJECT.md` and `PLAN.md` from the previous generated guidance are absent in this checkout; archived implementation evidence remains historical.

## Stored catalog contract

Catalog persistence is independent of retained game items, budgets, truth values and revisions. PostgreSQL migration 1 creates the eight legacy tables, `schema_migrations`, `sqlite_import_runs`, and six catalog tables:

| Table | Contract |
|---|---|
| `catalog_categories` | Five stable choices: film-cameras, watches, vinyl-records, audio-equipment, decorative-objects; EN/VI names and sort order |
| `catalog_lots` | Stable ID/unique slug; title/category, description, nullable brand/era/story, condition, ordered specifications JSONB, fulfillment JSONB, nullable money, USD, editorial state, private notes, version, timestamps, demo flag |
| `catalog_lot_images` | `(lot_id, position)` primary key; local path and alt text, zero-based order |
| `catalog_events` | Stable ID/unique slug, title/theme/description, cover/alt, nullable schedule, state, private notes, version, timestamps, demo flag |
| `catalog_event_lots` | `(event_id, position)` key, unique lot membership; draft detach before reassignment |
| `catalog_seed_runs` | Seed version/reference time, committed only with the complete dataset |

Title-only create yields a durable draft and generated ASCII slug. Slugs remain stable on renaming, may be edited while draft, and accept lowercase ASCII letters/digits with hyphens (maximum 96 characters). Brand, era and story can remain absent; a supplied story is not independently verified provenance. Drafts permit missing publication fields but reject invalid supplied values. `version` guards all edits. An assigned lot edit advances its parent event version too. Stale saves return conflicts.

Commerce uses integer USD cents, with explicit `starting_price_minor`, `bid_increment_minor`, and `fulfillment.shipping_fee_minor`, each capped at **1,000,000,000,000 cents**. Opening price and delivery fee permit zero; increments must be positive. Decimal input preserves cents exactly (`125.50` → `12550`); excess precision, negative/over-limit amounts and non-USD currency fail. Nullable means absent, distinct from zero. The independent helpers in `lib/catalog-money.ts` do not change legacy `formatUSD` or convert game money.

`fulfillment` contains `shipping_enabled`, `pickup_enabled`, `shipping_fee_minor`, `shipping_instructions`, and `pickup_instructions`. Publication requires at least one enabled option; delivery requires disclosed fee/instructions, pickup requires instructions. `specifications` is an ordered array of `{name,value}`; `images` is an ordered array of `{path,alt}`.

Media is public storage, even before publication: only canonical `/images/<relative-file>` (under `IMAGES_DIR`; default `images`, Makefile `./image`) and `/site-assets/catalog/<file>` (embedded) paths are accepted. JPEG/PNG/WebP/SVG are supported. No query/fragment, remote URL, encoded path bypass, traversal or symlink escape is accepted. Draft paths may name files to install later; publication requires readable image files and nonempty alt text, including the event cover. There is no upload or remote fetching. Removed/missing images retain the accessible local fallback. Seed artwork provenance is in `public/site-assets/catalog/README.md`.

Schedule fields are nullable integer UTC Unix milliseconds: `opens_at`, `first_closes_at`, `close_interval_ms`. The browser explicitly shows its timezone and converts local controls without changing the underlying instant. For zero-based member position `k`, all lots open at the event opening and close at `first_closes_at + k * close_interval_ms`. The first close follows opening; the interval is positive; instants and derived closes must be within 0–8,640,000,000,000,000 ms. The server derives the displayed schedule after saving/reordering. Publication requires a future opening; overlapping events are allowed.

An event owns publication of every member. Publication validates complete event content/cover/schedule, at least one lot, and each lot's category/description/condition/gallery/prices/fulfillment, then commits all states atomically. Every catalog mutation takes the schema-scoped transaction advisory lock before loading data; expected versions and constraints apply across pool connections. Publication failures expose nothing. Repeated publication returns the existing result before stale-version/date checks. Published content, slugs, money, timing and membership are immutable; there is no unpublish/delete/relisting action. Passing planned times does not start an auction, create bids/winners/orders, settle a lot or change editorial status. Commerce execution remains a later change.

Public projections explicitly allow content, media, specifications/story, fulfillment, USD opening price/increment, event identity/order, planned times, editorial state and demo flag. They exclude private notes, versions, internal timestamps/seed markers, credentials and every legacy game/player secret. Public aggregates use one consistent Repeatable Read snapshot. Draft and missing slugs have identical 404 responses. Preview uses the same projection but requires authorization and returns `preview: true`.

## Catalog APIs and working management

Every response is `Cache-Control: no-store`. Private success uses `{data: ...}` (201 create, 200 otherwise); public success uses `{data: ..., meta: {server_time: 1000}}` (200), where `server_time` is UTC Unix milliseconds sampled once from the catalog clock. Existing `data`-only consumers remain compatible. Private envelopes and authorization are unchanged; errors use `{error: {code, fields?}}`. Field paths include `title`, `images.0.path`, and `lots.<id>.condition`. No raw database errors are returned.

| Method | Path | Result |
|---|---|---|
| GET | `/api/catalog/categories` | Five public reference choices |
| GET | `/api/catalog/events`, `/api/catalog/events/{slug}` | Published events/aggregate |
| GET | `/api/catalog/lots`, `/api/catalog/lots/{slug}` | Published lots/details |
| GET | `/api/admin/catalog/categories` | Authorized category choices |
| GET, POST | `/api/admin/catalog/lots`, `/api/admin/catalog/events` | Stored list / title-only draft create |
| GET, PUT | `/api/admin/catalog/lots/{id}`, `/api/admin/catalog/events/{id}` | Read / replace editable content with loaded `version` |
| PUT | `/api/admin/catalog/events/{id}/lots` | `{version, lot_ids: [...]}` full ordered membership replacement |
| GET | `/api/admin/catalog/events/{id}/preview` | Authorized saved public projection |
| POST | `/api/admin/catalog/events/{id}/publish` | `{version}` explicit atomic publication |

All private reads, previews, writes and private-namespace misses require the existing `auction_admin` HttpOnly cookie. Query keys and player tokens do not authorize APIs. Missing/wrong capabilities return JSON 401 without a redirect or gate HTML. Mutations require `Content-Type: application/json` and `X-AuctionLab-Admin: 1`; a supplied Origin must match the browser-facing Host/scheme. Only a loopback Vite/cloudflared proxy may supply the effective forwarded scheme. Forwarded Host is not trusted. JSON decoding is bounded at 512 KiB and rejects unknown fields and trailing values.

Statuses: 400 malformed JSON; 401 `access_required`; 403 `request_rejected`; 404 `not_found`; 409 stale/duplicate/assigned/published/transaction conflict; 415 `json_required`; 422 `validation` with field reasons; 503 known unavailability or `commit_unknown`; 500 other sanitized storage failure. No automatic retry overwrites a newer record. On an uncertain commit, reload committed state before retrying.

Example authorized request bodies (provide the cookie through your existing session; do not put a key in frontend code or examples):

```json
{"title":"Camera draft"}
```

```json
{"title":"Camera draft","slug":"camera-draft","currency":"USD","version":1,"starting_price_minor":12550,"bid_increment_minor":500}
```

PUT is a replacement: include every editable field you intend to retain, including images/specifications/fulfillment. After editing an assigned lot, reload its event's new version. Preview is GET; publish uses `{"version": <latest-event-version>}`.

`/admin/lots?new=1` and `/admin/auctions?new=1` create drafts; `?edit=<id>` loads a stored editor. Unknown selections show not-found recovery. `routes/admin/CatalogPage.tsx`, `LotEditor.tsx`, `EventEditor.tsx`, and `Controls.tsx` supply stored lists, title-only creation, content/gallery/specification/money/fulfillment editing, membership/order, server-derived schedule, public-field preview and explicit publish. Ordered rows have labeled keyboard Move up/Move down/Remove actions. EN/VI labels and errors do not translate or clear entered content. Published records are read-only. Pending actions are disabled; failed saves retain input, and conflicts/uncertain commits require reload rather than automatic overwrite. Reload explicitly replaces entered values with the committed record, with a warning to copy unsaved content first.

The Go document gate remains unchanged. For Vite, authorize on the Go `/admin` gate at **the same hostname** (for example `http://localhost:8080/admin` and `http://localhost:5173/admin/lots`); do not mix `localhost` and `127.0.0.1`. Use the entry link from the server console, return to Vite, then reload. Host-only cookies work across ports. A frontend-only page shows access-required/backend-unavailable recovery and never offers a secret field or mutable sample fallback. Public discovery reads only published records; account/profile/watchlist views use their independent commerce session, and bids use the separate commerce auction service; private winner checkout with simulated payment is available; order history and fulfillment remain Change 7 previews. The website mounts no legacy game view or WebSocket.

## Explicit demo dataset

```sh
make build
./bin/auctionlab --seed-catalog --seed-at 2030-01-01T00:00:00Z
./bin/auctionlab --seed-catalog --seed-at 2030-01-01T00:00:00Z
```

These commands use private `DATABASE_URL`/`DATABASE_SCHEMA` environment configuration. Seed-only mode takes writer ownership, migrates, seeds and exits without engine/listener/tunnel. It is mutually exclusive with import/verify. Seed version 1 creates six lots each in **Analog Memories**, **Objects of Time**, and **The Listening Room**, covers all five categories, and bundles project-authored CC0 illustrations (not object photographs). All records are labeled demos and start as complete **drafts**. Dates are persisted once: reference +24/+48/+72 hours, first close one hour later, then 60-second staggering. Review and publish through the normal authorized UI/API. Rerun reports `already_applied`; edits, publication state and dates are preserved. Collision/failure rolls back the entire seed. Normal startup never seeds/rebases content. Expired unpublished demos need an explicit schedule edit or a fresh disposable PostgreSQL schema; published records stay frozen.

For legacy data, follow [offline import, verify, backup/restore and rollback](../docs/sqlite-cutover.md) **before** ordinary startup or seed in the import target. There is no SQLite-v5 catalog or file-based PostgreSQL rollback.

## Published discovery

`routes/storefront/` has separate home, event index/detail and lot index/detail pages. `usePublicPage` owns cancellable loading, retries, obsolete-result guards and visibility-resume refresh. `DiscoveryPresentation` supplies published cards, explicit planned-phase badges, demo markers, gallery selection and loading/empty/no-results/not-found/unavailable/error recovery. `lib/discovery-copy.ts` supplies EN/VI interface copy; authored records are not translated. Account and order views retain notices distinguishing working accounts/watchlists/bidding/private checkout with simulated payment from future order history and fulfillment. Publication is required: explicit seeding alone leaves discovery empty, and the UI never substitutes fixtures.

`api/public-catalog.ts` performs only anonymous GET requests with `credentials: 'omit'`, no admin headers and `cache: 'no-store'`. It validates envelopes, public record shape and server time. It never calls private APIs. The primary response's time sample plus monotonic elapsed time drives presentation, independently of the device wall clock. Visible pages tick once per second; resuming a hidden tab reloads public records and a failed refresh shows retry. Absolute planned dates identify the viewer timezone. Upcoming means before opening; within planned window includes opening and excludes close; elapsed includes close. Events use planned end; lots use individual closes. Missing/inconsistent timing is unavailable. These phases do not determine execution or results.

Native GET examples:

- `/lots?q=camera&category=film-cameras&phase=upcoming&sort=price-asc`
- `/auctions?phase=window`

Lot query keys are `q/category/phase/sort`; events use `phase`. First duplicate values win, unknown keys are ignored. Unknown category slugs mean all categories. Phases: `all` (default), `upcoming`, `window`, `elapsed`. Sorts: `closing-soon` (default), `price-asc`, `price-desc`, `title`. Search trims and compares case-insensitive substrings independently across title, description, brand and era; absent text is empty. It is neither fuzzy nor accent-insensitive. Filters combine with AND. Every sort ties by ascending stored slug; missing values come last, zero cents is valid. Closing-soon puts future closes ascending before elapsed closes descending; prices use integer cents; titles use lowercase deterministic text order independent of interface language.

Home features at most three events ordered window/upcoming/elapsed/unavailable, ascending opening within window/upcoming and descending within elapsed, then slug. Up to six nearest-close lots come only from current planned windows, derived from the same event aggregate. All categories link to their slug filter. Event details preserve published member order. Gallery buttons preserve stored image order/alt text, keyboard focus and selection; missing media uses the local fallback. Condition, specifications, supplied story and enabled shipping/pickup instructions/fees precede the separate auction bid controls. Zero amounts display explicitly as `$0.00 USD`.

Direct load, refresh, new tabs and Back/Forward use ordinary document URLs in Vite and the embedded build. Language changes preserve loaded records and form selections. Required read failures show recovery rather than partial results. Unknown/draft details share one public not-found view. The five-route browser evidence and requirement mapping live in `openspec/changes/archive/2026-10-19-build-auction-discovery-experience/verification/`. Accounts/watchlists and timed bidding are implemented; later checkout/orders, analytics and the ML extension remain future work. Change 5 acceptance evidence records its remaining manual browser checks separately.

## Commerce account API

`POST /api/auth/register` accepts exactly `{email,password,display_name}` and returns 201 `{data: profile}`. `POST /api/auth/login` accepts `{email,password}` and returns 200 with the same profile envelope. `POST /api/auth/logout` accepts `{}` and returns `{data:{signed_out:true}}` only after durable revocation. GET `/api/account` resolves the current session; PATCH accepts only `{display_name}`. A profile contains only `id`, `email`, `display_name`, server-owned `role` and `demo`. Email is an unverified, read-only login identifier. Public registration creates bidders.

All writes require `Content-Type: application/json`, `X-AuctionLab-Account: 1`, one JSON object (8 KiB maximum), known fields, and a matching supplied Origin. No Origin is permitted for explicit non-browser clients under the same header/JSON policy. No CORS credential access is enabled. All account responses are no-store JSON. Errors use `{error:{code,fields?}}`: 400 malformed_json, 401 access_required/invalid_credentials, 403 request_rejected, 404 not_found, 409 duplicate_email, 415 json_required, 422 validation, 429 throttled (Retry-After), and 503 unavailable/commit_unknown. A failed/uncertain registration issues no cookie; login can recover a registration that possibly committed. Do not automatically resubmit account creation.

The host-only `auction_account` cookie uses Path=/, HttpOnly, SameSite=Lax and a fixed 30-day lifetime. HTTPS sets Secure; only the direct loopback proxy may supply the effective scheme using X-Forwarded-Proto. Forwarded Host/IP never grant authority. Login rotates and revokes the supplied browser session; other browsers remain signed in. Logout clears the cookie only after confirmed revocation. Raw credentials are never JSON response fields or browser storage values. The account's admin role does not grant catalog access: the independent existing capability gate/cookie remains required, and a capability never selects an account.

## Private watchlists

GET `/api/account/watchlist` returns `{data:[publicLot,...],meta:{server_time}}`. It uses exactly the existing public lot projection, with no owner IDs, saved timestamps or draft fields. One Repeatable Read snapshot covers the session, memberships and catalog; order is newest saved first, then lot ID ascending using C collation. Unpublished content is omitted; elapsed published lots remain saved with planned schedule wording and no invented sale outcome.

GET `/api/account/watchlist/{lotID}` returns `{data:{saved:boolean}}` for a published lot. PUT with `{}` returns saved true; repeat/concurrent saves retain the original time and one row. DELETE with `{}` returns saved false even for absent entries, without looking up draft content. All ownership comes from the validated session. Draft/missing save or state targets share a 404. Capability-only requests fail account authorization. On commit_unknown, reload membership before choosing another action; do not announce an unconfirmed save/remove. Public catalog responses remain independent of account membership.

## Account views and saved lots

`/account/login` and `/account/register` are native document destinations within the account shell. Labeled forms use username/new-password/current-password autocomplete, field-associated errors, disabled pending controls and a focused error summary. Confirmed login/registration navigates to validated `return_to`; unknown, external, encoded-path, control/backslash, admin/API/WS/assets or auth-loop destinations fall back to `/account`. Ordinary discovery queries are retained, fragments removed. Authentication never replays a watch or bid action.

An in-memory provider checks GET `/api/account` independently of public discovery and distinguishes checking, signed-out, signed-in and unavailable. It clears private state before restoration/focus revalidation and on pagehide/logout, aborting or ignoring obsolete responses. No game token or account credential enters browser storage. Private views wait for server validation. `/account` shows the safe profile, read-only email, editable display name, role/demo labels and explicit logout; failures retain inputs and offer recovery. `/account/bids` shows accepted owner participation; `/account/orders` remains an unavailable preview.

Published lot detail has a separate authenticated save/remove action alongside the separate commerce bid panel and existing delivery/pickup disclosures. Anonymous visitors get a login link back to that lot and must explicitly choose save after login. Membership is loaded per lot/account, confirmed before display, and uncertain results require reloading saved state before another action. `/account/watchlist` loads real server-owned cards, deterministic order/count and confirmed removal. Checking, login-required, empty, unavailable and other errors remain distinct; failed reads never become empty fixtures. EN/VI messages announce pending/results, native links remain usable, and long titles/missing images use the existing responsive fallback. Public content stays usable when the account service fails.

## Timed bidding and private participation

Lot detail loads published editorial content independently from `/api/auctions/lots/{id}`. Execution failures leave gallery/condition/fulfillment inspectable. The detail opens one read-only commerce SSE stream at `/api/auctions/lots/{id}/stream`, never `/ws`. The existing Vite `/api` proxy and embedded server serve the same endpoint; no external transport dependency is added. Full snapshots restore state after reconnect, lower revisions are discarded, and same-revision server-time/presentation refresh remains valid. Disconnect triggers capped reconnect backoff and public GET fallback every two seconds. Ten seconds without fresh data disables submission; focus/visibility restoration refreshes the snapshot. Streams and polling are cancelled on navigation/pagehide.

Manual decimal USD entry uses exact cents, without floating-point rounding. Submission requires a current commerce session and fresh open execution plus owner participation. One UUID belongs to each pending deliberate intent. No optimistic acceptance is shown. After an unknown result, the owner can check accepted request IDs or explicitly retry the same request ID/amount; the UI never silently resubmits with a new ID. Login, logout and account changes never replay bids. Watchlist saving remains independent and never bids.

`/account/bids` loads only the current session's accepted participation, one row per lot, latest own/current amounts, native lot links and All/leading/outbid/won/lost filters. Closing stays in All. Visible private reads refresh every two seconds, on focus/public revision and after submission. Loading, signed-out, failed read and empty results are distinct; obsolete responses and prior account state are cleared. EN/VI labels and polite confirmed-result announcements preserve keyboard focus; countdown ticks are not live announcements.

Discovery's planned URL phases (`all/upcoming/window/elapsed`), catalog DTOs and opening-price sorts are unchanged. Accounts, watchlists and scheduled bidding are available. Winning is a durable auction result only: checkout, payment and orders remain unavailable, including in the won view. Published schedules/results cannot be edited to replay a demo; prepare new future-scheduled drafts as described in the root README.

## Winner checkout (Change 6)

`/account/wins` lists the current owner's unpaid wins. `/account/checkout/<32-lowercase-hex-order-id>` is an exact single-order route, with safe login return and explicit selected-ID dispatch. The existing account namespace serves private wins/order reads and stateless review/atomic simulated confirmation; see `../internal/order/README.md`. Vite uses the same API guard as the embedded binary. `/account/orders` and `/admin/orders` remain unavailable Change 7 history/management previews.

The checkout flow is form → server review → deliberate simulation → stored paid confirmation. EN/VI labels cover loading, empty, signed-out, missing, field validation, pending, unknown, unavailable and paid states. Both simulation methods state that no money is charged, including for non-demo lots. No card/bank inputs exist. Item/fee components retain their catalog limit; the separate order formatter accepts their exact combined maximum of 2,000,000,000,000 USD cents.

Private reads/mutations are bound to owner/order and lifecycle tokens. Navigation, logout, account switch and page restoration abort obsolete work and suppress late responses. Language changes preserve the active owned form. Edits discard the previous review/intent. Confirmation disables competing edits/submits; unknown outcomes require stored-order recovery before deliberate same-intent retry. Reload reads durable state and clears unsent personal values; login/reconnect never replays payment. Contact/address/request intent stays out of URLs, browser storage and logs. Paid orders show their stored contact/fulfillment, exact amounts, receipt and time without pay controls. Unpaid wins omit paid orders on the next read. General order history and delivery tracking remain future work.

Current measured acceptance and any missing actual-browser zoom/reduced-motion checks are in the [Change 6 archive report](../openspec/changes/archive/2026-10-21-add-winner-checkout-and-demo-payment/verification/REPORT.md); earlier Change 4/5 archive limitations remain unchanged.
