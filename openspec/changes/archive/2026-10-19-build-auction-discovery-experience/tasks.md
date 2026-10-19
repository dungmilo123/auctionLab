# Tasks

## 1. Public catalog read contract

- [x] 1.1 Add the successful public-response envelope with `meta.server_time` in `internal/api/catalog.go`, sampling the injected catalog clock and retaining existing data/error/cache contracts; verify all five public list/detail endpoints with focused HTTP tests using the existing disposable PostgreSQL fixture.
- [x] 1.2 Extend public HTTP regression coverage for published-only visibility, identical draft/missing 404s, private-note exclusion, sanitized backend failures and changed metadata with unchanged record data/game revision; verify the catalog/API tests pass with `TEST_DATABASE_URL` and no operational schema access.
- [x] 1.3 Add a GET-only anonymous public catalog client with encoded slugs, cancellation, envelope/shape/time validation, and stable error mapping; verify frontend tests cover successful empty/detail data, 404/503, transport/HTML/malformed responses and absence of credentials/admin headers/private API paths.
- [x] 1.4 Document the additive public envelope and unchanged private API boundary in `web/README.md`; verify examples match HTTP tests and existing `data`-only consumers remain compatible.

## 2. Discovery queries and schedule presentation

- [x] 2.1 Implement discovery query parsing for lot `q/category/phase/sort` and event `phase` with category-slug resolution, documented defaults and first-value duplicate handling; verify tests cover valid shared URLs, invalid/duplicate/unknown parameters and plain-text query content.
- [x] 2.2 Implement combined text/category/phase filtering, result counts and deterministic lot sorting, including future-before-elapsed closing order, cents-based prices, lowercase title order, null-last values and slug tie breaks; verify representative mixed-lot tests cover all sort modes, optional text, zero amounts and no matches.
- [x] 2.3 Implement server-sample/monotonic-elapsed clock presentation and planned-phase classification using each lot close or event planned end, including resume refresh and cleanup; verify exact opening/closing boundaries, staggered lots, missing/invalid dates, incorrect device wall time and resume-failure recovery without negative countdowns.
- [x] 2.4 Implement deterministic event ordering/featured selection and homepage current-window nearest-close selection from one event aggregate; verify tests cover overlapping events, phase ordering, opening/slug ties, maximum three/six selections and successful empty sections.
- [x] 2.5 Document query values, search limitations, sort rules, timezone presentation and planned-only status semantics in `web/README.md`; verify documented examples match query/schedule helper tests.

## 3. Public presentation and data-state foundation

- [x] 3.1 Pass the resolved route/slug through `App.tsx` and split storefront route rendering into focused public pages with a shared cancellable loader and retry states; verify route regressions plus browser loading/retry behavior, including ignored obsolete results and distinct empty/error/detail-404 views.
- [x] 3.2 Add compatible record-backed lot/event cards, explicit planned-phase badges and demo markers using existing design tokens and money helpers; verify representative demo/non-demo/zero-cent cards and long titles render correctly without live/sold claims or hardcoded drawing alt text.
- [x] 3.3 Add reusable loading, empty, no-results, unavailable, general-error and record-not-found presentations with retry/index/clear recovery actions; verify EN/VI copy completeness and observable recovery from each applicable state without fixture substitution or raw internal errors.
- [x] 3.4 Update storefront notices, footer and discovery labels while preserving account/order previews and catalog-management notices; verify EN/VI regression checks and inspect non-demo published content for obsolete generic-preview/illustration claims.
- [x] 3.5 Update `web/README.md` component and route-state guidance to describe working discovery and remaining previews; verify it no longer claims public routes always render fixtures or arbitrary generic detail records.

## 4. Homepage and event exploration

- [x] 4.1 Build the homepage brand introduction, featured published events, nearest planned-close lots and all category links from public reads; verify stored-slug navigation, selection limits, empty-section recovery and category-filter URLs in the browser.
- [x] 4.2 Build `/auctions` with GET phase controls and public event cards, preserving native submission/history behavior; verify all/window/upcoming/elapsed selections, no-results recovery, direct load, refresh and Back/Forward against published data.
- [x] 4.3 Build `/auctions/:slug` with matching content, authored cover/alt, planned opening/end and the complete ordered lot list with individual times and links; verify staggered/overlapping event display and identical missing/draft recovery using stored fixtures.
- [x] 4.4 Update active frontend route documentation for homepage/event discovery and explicit publication prerequisites; verify the documented links work in Vite and the embedded Go build without a prior homepage visit.

## 5. Lot discovery and inspection

- [x] 5.1 Build `/lots` with labeled GET search/category/phase/sort controls, result count and clear filters using the shared helpers; verify combined matching, all sorts, no-results versus empty/backend failure, category entry links and query restoration across refresh/new tabs/history.
- [x] 5.2 Build the lot gallery with stored image order/alt text and named keyboard-selectable thumbnail controls using the existing local fallback; verify multiple images, visible selected/focus states, a missing image and continued access to all details.
- [x] 5.3 Build `/lots/:slug` from the matching public record with category, descriptions, condition, optional brand/era/story, ordered specifications and parent-event link; verify custom and demo records, missing optional history, safe text rendering and identical draft/missing recovery.
- [x] 5.4 Add opening price/increment, absolute planned times with timezone, enabled shipping/pickup terms and fees before the explained disabled bid action in reading order; verify exact cents, zero price/free shipping, pickup-only content and no current-bid/history/result fabrication or mutation requests.
- [x] 5.5 Finish responsive discovery/gallery/disclosure styling and bilingual labels using the established tokens; verify 320/375px and desktop, keyboard, 200 percent text zoom, reduced motion and language changes preserving loaded content and selections.
- [x] 5.6 Update frontend/operator documentation for functioning public search/details, planned phases and unavailable bidding/accounts/checkout; verify obsolete public-preview instructions are removed and no roadmap milestone is marked implemented before acceptance.

## 6. Integrated acceptance

- [x] 6.1 Create a disposable-schema acceptance dataset via explicit seed/review/publication, plus one non-demo multi-image exact-cent/free-shipping/pickup record and an unpublished record; verify and record published visibility, empty-before-publication behavior, stable slugs/content after application restart and unchanged game state.
- [x] 6.2 Run the five-route discovery journey in Vite-proxied and embedded builds, including phase transitions, GET filters/history, unknown/draft slugs, loading, empty, no matches, API unavailable/malformed data, retry and image failure; record results and focused desktop/mobile EN/VI screenshots in `verification/` with requirement-to-evidence mapping.
- [x] 6.3 Inspect public network activity and page output while testing a browser with an existing game token; verify only public GET reads/local assets, no private data or legacy secrets, no game socket/session restoration, disabled bidding and no commerce mutation.
- [x] 6.4 Run `make test` with the required real test PostgreSQL service and `make build`; verify frontend/type/lint, Go regressions and frontend-first embedded no-CGO build pass, and record exact outcomes and any remaining acceptance limitations in `verification/`.
- [x] 6.5 Reconcile `README.md`, `web/README.md` and the Change 3 status in `ROADMAP.md` with measured acceptance results; verify documents distinguish working discovery from Changes 4–8 and the ML extension, and do not claim executable auction states or unperformed checks.
