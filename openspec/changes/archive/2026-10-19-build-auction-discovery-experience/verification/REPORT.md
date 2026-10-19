# Auction discovery acceptance — 2026-10-19

Change: `build-auction-discovery-experience`. All five public destinations now read published PostgreSQL catalog records. Bidding remains disabled; planned phases do not assert execution or auction results. Accounts/watchlists, bidding, checkout, orders, analytics/recommendations (Changes 4–8) and the ML extension remain future work.

## Build and automated checks

- `TEST_DATABASE_URL=<dedicated disposable PostgreSQL 18 test service> make test`: **passed**. All Go package checks passed; **23 frontend test files / 502 tests passed**, oxlint passed without warnings. See [make-test.txt](make-test.txt).
- `make build`: **passed**, including TypeScript, Vite and `CGO_ENABLED=0 go build -o bin/auctionlab ./cmd/server`. See [make-build.txt](make-build.txt).
- `openspec validate build-auction-discovery-experience --strict`: passed.
- `git diff --check`: passed.

Public HTTP regression tests cover all five successful endpoint envelopes, injected response time, unchanged `data`, no-store policy, empty-before-publication, published-only visibility, identical draft/missing responses, private-field exclusion, sanitized database failures and unchanged game revision. Client/helper tests cover anonymous GET options, slug encoding, aborts/obsolete resolution, shape/time validation, error mapping, query composition/defaults, sort modes/ties/null/zero values, exact schedule boundaries and homepage limits/order.

## Isolated data and restart

A new loopback-only PostgreSQL **18.6** container and generated `test_load_*` schemas were used. No operational schema or SQLite source was accessed. The app was explicitly seeded, saved previews were reviewed through the authorized API, future schedules were saved, and three six-lot demo events were explicitly published. Seeded drafts initially produced an empty public event list.

A fourth, non-demo event publishes a custom camera with two ordered authored image alternatives, a long title, plain-text markup, ordered specifications, no era/story, an opening price of **$125.50 USD**, **$1.01 USD** increment, **$0.00 USD** shipping and pickup instructions. Private note sentinels are excluded from public output. A lot draft and event draft remain unpublished. The second camera view uses isolated runtime media; the first and event covers use bundled local assets.

Public aggregates before/after an application restart compare identically: **4 events / 19 lots**, including slugs, editorial content, prices and schedules. After browsing, the retained game remains `LOBBY`, revision **0**, with **0 players / 0 bids**. See [fixture-results.json](fixture-results.json). Fixture identifiers in [fixture.json](fixture.json) contain no credentials.

## Browser evidence and requirement mapping

Chromium exercised both Vite-proxied `:5173` and the rebuilt embedded Go application `:8080`. [browser-results.json](browser-results.json) records **16 passed journey groups**, [extra-results.json](extra-results.json) records **10 passed disclosure/ordered-event checks**, and [final-browser-results.json](final-browser-results.json) records additional checks for both builds.

| Requirement | Evidence |
|---|---|
| Published homepage | Home/event/lot journey: three featured event cards, published stored-slug links and category links. Selection helper tests cover six current-window lots, ordering, overlaps and successful empty sections. `vite-home-en.png`, `embedded-home-en.png`. |
| Event browsing/detail | GET `all/window/upcoming/elapsed` selections; direct details; all six demo members checked against API position order for every demo event; overlapping custom/demo schedules and individual staggered closes. `*-event-en.png`, extra-results.json. |
| Searchable lot index | Combined text/category/phase query, each sort control, result counts, no matches, clearing and restored inputs. Helper tests independently verify actual ordering, cents/null/tie behavior and AND semantics. `*-320-index-en.png`. |
| Shareable URLs | Native GET submission, refresh, new tab, Back/Forward; query parser tests verify invalid/unknown/duplicate values and plain-text markup. |
| Content/gallery | Authored title/description/condition/specifications; absent era/story omitted; markup displayed as text, no script execution; named image buttons selected with keyboard, visible selection, authored alt and local missing-image fallback. `*-desktop-lot-en.png`, `*-320-lot-vi.png`, `*-375-lot-vi.png`. |
| Price/fulfillment | Stored exact cents/free shipping/pickup disclosed before disabled bid. Additional intercepted public responses verify zero opening price and pickup-only presentation without a shipping fee or invented totals. extra-results.json. |
| Planned phases | Unit boundary/staggered/end tests plus browser response samples crossing opening/closing; an incorrect `Date.now()` does not affect classification. Synthetic visible-tab `visibilitychange` triggers refresh, a 503 replaces stale content, and retry recovers. No countdown is displayed. |
| Recoverable loading | Delayed response visibly loads; successful empty collection differs from no matches; stored drafts/missing lot and event slugs share identical not-found recovery; 503 and malformed HTML success show retry, then recover while retaining URL selection. |
| Bilingual/accessibility | EN/VI controls/document language, selections and loaded content retained; 320/375 CSS-pixel mobile and desktop screenshots; no page-level overflow on checked pages, including 200% root text enlargement; keyboard gallery operation/focus; reduced-motion browser preference. `*-320-text200-en.png`. |
| Anonymous/no transactions | Browser starts with an existing game-token sentinel. Catalog requests are public GETs only; no `/api/admin/`, game sockets, session restoration or commerce mutation; bidding remains a native disabled button. `vite-network.json`, `embedded-network.json`. Vite additionally uses its expected development HMR WebSocket; it is not a game connection. |
| Additive API contract | HTTP tests check response metadata independently of unchanged record data/game revision. Existing `data` consumers remain compatible; private capability checks/envelopes are unchanged. |
| Honest notices/navigation | Non-demo custom content has no demo/illustrative-preview label, generic footer illustration claims removed, local hero illustration loads, account/order previews remain separate. Missing routes remain website 404 views. |

## Reproduction and limits

The three `*-check.mjs` scripts record browser assertions and screenshots. They need an explicitly prepared isolated fixture matching `fixture.json`, the corresponding Go/Vite servers, and a locally installed Playwright Chromium; set `PLAYWRIGHT_MODULE` to its module path if it is outside the normal module search path. No browser dependency was added to the application. The fixture has short-lived planned windows; repeat acceptance with freshly reviewed future schedules in a new disposable schema rather than altering immutable published schedules.

Stored-record, publication and restart checks used the real PostgreSQL-backed API. Delayed/empty/unavailable/malformed responses, exact accelerated phase transitions, missing image responses, zero opening and pickup-only disclosures were additionally exercised through browser interception; they are UI recovery/disclosure evidence, not real server outages or new stored records. Visibility resume was dispatched synthetically; OS suspension was not tested. Text enlargement used 200% root font size, not physical device scaling. No screen-reader audit or exhaustive browser/device matrix is claimed. The server sample has network latency uncertainty and is only a presentation reference.

OpenSpec planning/evidence paths remain covered by the repository's existing ignore rule; it was not changed. No archive or commit was performed.

After acceptance, both owned disposable schemas, the isolated PostgreSQL container and the fixture Go/Vite servers were removed/stopped. No operational service was stopped. Screenshots, sanitized request paths, test/build logs and fixture-result summaries remain in this directory.
