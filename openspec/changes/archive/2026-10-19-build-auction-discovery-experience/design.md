# Design

## Context

See `proposal.md` for motivation and the three delta specs for behavior. Read-only discovery found:

- `StorefrontPage.tsx` imports `PREVIEW_LOTS` and `HERO_IMAGE`; detail pages ignore the matched slug. `App.tsx` passes only `page` although `routeFor()` already returns `slug`.
- `SiteLayout.tsx` shows a global preview notice except in the two working catalog editors. Shared footer and storefront copy assert all objects are illustrations. `LotCard` hardcodes illustration/unavailable-price copy, and its status union includes live/sold/ended examples unsuitable for published schedules.
- `internal/api/catalog.go` already registers anonymous category, event and lot lists and slug details. Public errors use sanitized envelopes, success uses `{data: ...}`, and responses are `no-store`. `projection.go` deliberately builds public DTOs separately from private content. Its event aggregate preserves membership order and includes planned end and per-lot closes.
- `web/src/api/catalog.ts` defines useful public types but implements only the private admin client. No anonymous discovery client exists. The existing commerce money formatter preserves cents; `catalog-schedule.ts` formats UTC instants in the browser timezone.
- Catalog service tests already cover publication, privacy, immutable schedules and reconnects using disposable PostgreSQL schemas. Frontend tests use Vitest; there is no committed browser-test suite or DOM testing dependency. `make test` runs Go tests, frontend tests and lint; `make build` builds the frontend before the no-CGO embedded binary.
- Existing `website-navigation` requirements explicitly demand generic public detail previews and unchanged public previews after publication. Those requirements are superseded by this change's delta. `auction-event-publishing` forbids claiming execution states merely from planned times; it remains unchanged. The design-system's preview examples remain available without appearing as real catalog states.

## Goals / Non-Goals

**Goals:** Separate public reads from privileged editing, reuse the published aggregate and existing design foundation, keep query behavior deterministic and shareable, and give all five public destinations explicit asynchronous and recovery behavior.

**Non-Goals:** Persistent search indexes, pagination, analytics collection, editorial featuring controls, automatic publication/seeding/rebasing, new authentication, auction lifecycle evaluation, and changes to legacy game storage or money. Roadmap Change 3's “ongoing/ended” groupings are implemented only as explicitly planned schedule phases; true execution labels await Change 5.

## Decisions

### 1. Keep public and admin HTTP boundaries explicit

Add `web/src/api/public-catalog.ts` with GET-only category/events/event/lots/lot methods under `/api/catalog/`. Reuse/export the existing `Category`, `PublicLot`, `PublicEvent` and sanitized error representation; never obtain records from the private client or fall back to admin preview. Encode slug path segments and accept an abort signal. Public fetches omit credentials, use `cache: 'no-store'`, and send no admin mutation headers. Assert the expected response envelope, collection/detail shape and usable response time before rendering; malformed/HTML success responses are errors, not sample data.

In `internal/api/catalog.go`, wrap successful public reads with `{data, meta: {server_time}}`, sampling the existing injected `service.Now()` once per response. Leave private wrappers, public DTOs, `respondCatalog` error mapping and cache policy intact. No server-side schedule fields or persisted execution status are added. An additive envelope is preferable to a new clock endpoint (extra dependency/request) or changing every immutable record with a volatile timestamp. Existing consumers of `data` remain compatible.

### 2. Use route-specific loaders and explicit request states

Pass the full resolved route to `StorefrontPage`, then dispatch to focused Home, AuctionIndex, AuctionDetail, LotIndex and LotDetail components under `web/src/routes/storefront/`. A small shared public-read hook owns loading/success/error state, retry generation and cancellation; aborted/unmounted/obsolete requests cannot replace current results. Successful empty arrays are distinct from loading or failure. Never retain the previous detail under a new slug or show the fixture grid during a request.

| Destination | Reads | Composition |
|---|---|---|
| `/` | events aggregate + categories | Brand hero, deterministic featured events, current-window nearest-close lots derived from that same aggregate, category links |
| `/auctions` | events aggregate | Phase controls and event cards |
| `/auctions/:slug` | event detail + categories | Cover/story/schedule and all members sorted by `position` |
| `/lots` | lots + categories | GET query controls, count, filtered/sorted cards |
| `/lots/:slug` | lot detail + categories | Gallery, content, disclosure, schedule, parent-event link and disabled bid |

Fetch independent page reads together. If a required read fails, show a recoverable page error; do not claim successful no-results from partial data. A detail 404 takes the common missing-record presentation for draft and absent slugs. Unknown document paths retain the existing website 404. Frontend detail-not-found is allowed to use the SPA document's HTTP 200; API misses remain real 404s.

Splitting pages makes individual failure states and route inputs reviewable; adding a client router or query-cache framework would complicate an already working native-document navigation model without a requirement.

### 3. Keep bounded discovery processing in the frontend

The initial catalog contains 18 demo lots and three events, and existing public lists return complete published content. Reuse these lists and extract pure helpers into `web/src/lib/discovery.ts` (or cohesive adjacent modules). Do not add SQL search, pagination or filtering parameters to the public APIs in this change. Homepage sections derive from one event aggregate response, avoiding a second lots snapshot with potentially different membership.

Lot matching trims the query, lowercases it and each candidate field, and performs substring comparison separately across title, description, brand and era; absent fields are empty strings. It does not promise fuzzy, accent-insensitive or tokenized search. Resolve `category` by category slug to stored category ID. Apply text, category and phase filters together. Sorting operates on integer cents, planned closes and lowercased title text, ending every tie with ascending stored slug. Use deterministic text comparison independent of the current interface language. Null/missing sort fields are last; do not use truthiness for zero cents or epoch time.

Closing-soon sorts all future closes ascending (including upcoming lots), then elapsed closes descending. Homepage nearest-close is narrower: only `window` lots, up to six. Event sections and all-events lists prioritize `window`, `upcoming`, `elapsed`, then unavailable timing; opening time is ascending in window/upcoming and descending in elapsed, then slug ascending. Featured events are the first three from that ordering. No operator featuring flag is needed. Event details preserve published order instead of applying discovery sort.

Alternative: server search/summary DTOs would reduce payloads at a larger catalog size, but introduce unnecessary contracts now. Revisit when dataset size makes full lists unsuitable; do not silently truncate this change's results.

### 4. Use GET forms and native URLs for discovery state

Use a labeled `<form method="get" action="/lots">` with `q`, `category`, `phase`, and `sort`, and a submit action. The auction index uses a GET phase selector. Category entry links are `/lots?category=<slug>`. Clear filters links to `/lots`; clear phase links to `/auctions`. Default category is all, phase is `all`, sort is `closing-soon`. Unknown category/sort/phase falls back safely; duplicate known keys use their first value and unsupported keys do not affect results. Query text is ordinary escaped input and is never inserted as HTML.

Read query parameters separately from `routeFor()` so existing path matching, trailing slash behavior, admin selections and reserved routes remain unchanged. Native links and submissions preserve history, refresh, new tabs and server document gates. The current EN/VI provider changes labels without clearing form inputs, loaded records or submitted selections. URL-driven form submissions are simpler here than intercepted navigation and synchronized History API state.

### 5. Reference the server clock without creating an auction lifecycle

Use the primary record response's `meta.server_time` as the page's time reference (events for home/event pages, lots/lot for lot pages; never a categories response sampled later). Capture a monotonic receipt time and estimate current presentation time as server sample plus elapsed monotonic duration. A modest tick updates phase-dependent sections and filters while the page is visible. On resume from a hidden tab, re-fetch to establish a fresh reference; on failure show the explicit retry state. Do not fall back to device wall time if metadata is absent or invalid. Absolute schedules remain sourced from saved UTC milliseconds and display the viewer's timezone using the existing formatting helper.

| Phase | Condition | English label |
|---|---|---|
| `upcoming` | `now < opens_at` | Upcoming schedule |
| `window` | `opens_at <= now < closes_at` | Within planned window |
| `elapsed` | `now >= closes_at` | Planned window elapsed |
| unavailable | Missing, invalid or inconsistent endpoints | Schedule unavailable |

Events substitute `planned_end` for the close. Validate nonnegative safe integer endpoints within the existing supported instant range and close strictly after opening. Label all times “planned”; provide corresponding VI labels. Show absolute dates as the stable primary schedule. A countdown is optional, and if included it is approximate, clamped at zero and not an assertive screen-reader announcement every second. The page clock has request-latency error and does not adjudicate opening/closing, bids, or winners. Browser-wall-clock derivation was rejected because the roadmap makes the server authoritative; a realtime socket or stored lifecycle was rejected because both belong to Change 5.

### 6. Adapt presentation to records and disclosures

Add public EventCard, catalog LotCard and Gallery composition using existing tokens, grid, local `LotImage` fallback and controls. Either add compatible optional props to `LotCard` or wrap a shared visual shell; preserve existing preview callers and never cast schedule phases into the live/ended/sold union. A separate planned-phase badge carries explicit EN/VI text. Stored gallery alt text must reach the image; the existing generated “Drawing of …” label must not be used for arbitrary catalog photographs. Thumbnail buttons expose names, selection and visible focus; no modal/lightbox or swipe dependency is required.

Use `formatCatalogMoney` for opening price, increment and each enabled shipping fee. Show optional brand/era/story only when present and never attach an authentication claim to supplied provenance. Reading order is gallery/content, condition/specifications/story, enabled fulfillment methods/instructions/fees, then the bid panel, with desktop styling allowed to rearrange visually only if the reading order remains usable. The bid panel repeats opening price/increment and planned dates as useful, includes a native disabled button, and explains that bidding is not available yet. There is no bid history or mocked current price.

Make the shared storefront notice describe published catalog discovery and unavailable bidding/purchases; keep account/order preview notices and catalog-management notices appropriate to those routes. Replace footer claims that all objects are illustrations with neutral collection copy. Show a per-record demo marker when `demo` is true; avoid promising photographic media or authenticity for either demos or operator content. Home can retain a bundled decorative brand illustration if labeled as such, never linked as a fake lot.

### 7. Verify behavior at the changed boundaries

Automated coverage focuses on public envelope/client error mapping, query parsing/composition/sort, time boundaries including incorrect device clocks, and URL/EN-VI regression behavior. Extend existing Go HTTP fixtures with public successful list/detail time envelopes, published visibility, privacy and unchanged sanitized errors. Use the injected clock rather than wall-clock sleeps to prove immutable data at different response times. Keep retained game and admin regression coverage.

Use real PostgreSQL fixtures only through `TEST_DATABASE_URL` and disposable owned schemas. For browser acceptance, explicitly seed/review/publish content in a disposable schema through normal supported flows; seeding alone intentionally leaves public discovery empty. Include one custom non-demo published record with multiple gallery images, distinct private-note sentinels, exact-cent opening/increment, free shipping and pickup, plus an unpublished record. A short future schedule can demonstrate phase boundaries without altering published dates; helper/clock tests cover elapsed cases deterministically. Restart the app on the same disposable schema and confirm record identity/content and discovery links survive.

Exercise all five routes, GET submission/clear/refresh/Back/Forward, missing/draft slugs, successful empty catalog, no matches, API unavailable/malformed response, retry, and missing images. Inspect at 320/375px and desktop, with keyboard, 200 percent zoom, both languages and reduced motion. Verify public browsing emits only public catalog GETs and local asset requests, never private catalog requests, game sockets or commerce mutations. Capture concise acceptance notes/screenshots under this change's `verification/` during implementation. Manual browser evidence complements focused unit tests; no browser-testing dependency is required by this plan.

## Risks / Trade-offs

- [Full-list payload grows with content] → Bound this implementation to the current demo scale; do not imply pagination or search scalability. Revisit public summary/search endpoints as a separate change if needed.
- [Planned windows can resemble executable auctions] → Explicit labels, disabled bid panel, no current-price/result claims, and no lifecycle persistence or game calls.
- [Server sample is approximate by network latency] → Use it only for planned discovery presentation, display absolute times, refresh on resume and defer transaction timing to Change 5.
- [Demo seed remains draft or dates expire] → Document explicit review/publication with a valid future schedule before publishing, or use a fresh disposable schema. Never rebase published schedules or expose drafts to fill empty screens.
- [Shared preview components accidentally mislabel real content] → Separate planned phase badges, retain authored alt text, check notices/footer, and include a non-demo acceptance record.
- [OpenSpec artifacts are ignored by the repository's general ignore rule] → Keep artifacts in the resolved planning home and report their paths; any later decision to commit planning artifacts must explicitly include them. Do not change repository ignore rules as part of discovery implementation.

## Migration Plan

1. Implement the additive public envelope and frontend changes together; no database migration or automatic content operation occurs.
2. Run `make test` with a provisioned disposable-test PostgreSQL service and `make build`, then accept both Vite-proxied and embedded Go behavior against isolated demo data. Treat missing test service as a failed prerequisite, not a skipped pass.
3. Deploy the rebuilt embedded application with existing PostgreSQL configuration/media; public pages read whatever is already published. A development frontend must use a matching backend that supplies server-time metadata.
4. Rollback to the previous application build if required. The prior client ignores additional metadata, persisted catalog content is unchanged, and no reverse migration, deletion, SQLite fallback or legacy-data conversion is necessary. Restoring the older build restores its older public preview behavior.
