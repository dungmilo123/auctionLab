# Tasks

All tasks are implementation/acceptance work for a later apply request. Current artifact existence is not implementation completion. Keep evidence in this change's `verification/`; record missing capabilities or checks explicitly. Preserve the existing unrelated `skills-lock.json` and archived evidence.

## 1. Baseline and visual direction

- [x] 1.1 Prepare a disposable real-PostgreSQL fixture using existing maintenance tooling, with published/empty/long-title/missing-image states, two bidders, shipping/pickup orders and admin access; verify isolation from operational schemas and record sanitized fixture identities and setup in `verification/README.md`.
- [x] 1.2 Capture baseline home, event/lot indexes and details, account/auth/watchlist/bids/wins, checkout/history/detail, admin overview/catalog/orders/analytics, gate and 404 at desktop/mobile; verify the route/state inventory and record viewport, language and fixture state for comparable before/after views.
- [x] 1.3 Record baseline build bytes, image transfer, navigation/commerce request counts and a reproducible local throttled interaction trace; verify measurements include methodology and contain no credentials or personal form values.
- [x] 1.4 Prepare annotated home/lot/mobile wireframes and the design's palette/type/spacing reference for the implementation pass; verify the delivered reference identifies image/action hierarchy and subject-specific choices, with any departures from `design.md` documented.

## 2. Shared visual and interaction foundations

- [x] 2.1 Consolidate website typography, spacing, surfaces, responsive rules and state styles in `web/src/index.css`, replacing conflicting selectors rather than layering overrides; verify key state contrast, long bilingual labels and 44-px targets, and keep dormant game styles isolated.
- [x] 2.2 Introduce shared Lucide icon sizing/alignment and refine website buttons, fields, badges and designed loading/empty/error feedback; verify decorative icon semantics, localized icon-only names, associated field errors and visible text on consequential actions by keyboard/DOM inspection.
- [x] 2.3 Add the `motion` dependency/lockfile and shared timing/easing/reduced-motion configuration, including website portal roots; verify typecheck/build and a focused runtime-preference test cover both CSS and scripted/portal motion without disabling essential status text.
- [x] 2.4 Document the reusable visual/icon/motion conventions and state vocabulary in `web/README.md`; verify the guidance matches the actual tokens/components and does not introduce production/gateway/ML claims.

## 3. Product copy, seed compatibility and local media

- [x] 3.1 Replace mounted capability inventories, global preview notices and buyer demo/simulation/no-charge/future-ML copy across website dictionaries/components, including profile, recommendations and receipts; verify rendered EN/VI/accessibility text uses natural product vocabulary while dedicated operator analytics definitions remain intact.
- [x] 3.2 Implement the exact-field legacy seed display adapter with provenance plus media identity checks for event/lot content, specifications, alt text and fulfillment terms, including historical owner snapshots; verify focused tests cover stock, edited, non-demo, unrelated and snapshot cases without changing prices, terms or raw records.
- [x] 3.3 Update future catalog seed defaults to equivalent natural wording and truthful unknown condition/provenance, preserving flags/identities/schedules; verify real-PostgreSQL seed/repeat/collision tests and publication tests pass without overwriting edits or existing published content.
- [x] 3.4 Upgrade all 18 object plates at existing local paths with cohesive object-specific editorial art; verify visual comparison, neutral object descriptions, compatible URLs, reserved dimensions and absence of baked-in demonstration lettering.
- [x] 3.5 Upgrade all 18 detail plates and three event covers with coherent art and recognizable object/collection relationships; verify gallery order, existing path compatibility and successful local fallback/rendering in Vite and embedded builds, including raster-in-SVG rendering if used.
- [x] 3.6 Update media source/license/authored-or-generated manifests and document the bounded seed adapter plus presentation-only change in active README/frontend docs; verify all 39 assets are accounted for, source descriptions remain truthful and offline requests use only the application origin.

## 4. Header, language and responsive navigation

- [x] 4.1 Refine `SiteLayout` header/footer and contextual navigation with meaningful icon-supported account/watchlist/search entries and collapsible mobile navigation; verify all links use existing destinations, active states/skip link work and native new-tab/Back/Forward semantics remain intact.
- [x] 4.2 Replace mounted EN/VI buttons with the native-name globe/chevron dropdown using the existing language provider; verify focused keyboard/focus-return tests and browser checks cover selection, Escape, persistence, `document.lang`, query/image/form preservation and zero automatic commerce or view-recording actions.
- [x] 4.3 Add header search through native `/lots?q=...` and accessible mobile menu/filter panel behavior; verify GET/query restoration, Escape/focus handling and clear recovery at 320/375 px and actual 200% zoom.
- [x] 4.4 Align the self-contained Go admin gate's visual presentation and update navigation documentation in `web/README.md`; verify existing gate tests, bilingual content, invalid-key behavior, no-cache response and capability-cookie/redirect contract remain unchanged with no external dependencies.

## 5. Discovery, cards and gallery

- [x] 5.1 Replace the static homepage focal illustration with the first featured published event's image/story and refine featured, nearest-close, category and recommendation sections; verify existing selection tests, empty-catalog behavior and before/after desktop/mobile screenshots show clear hierarchy without fixture substitution.
- [x] 5.2 Refine event index/detail and shared lot/event cards for image emphasis, compact schedule/price metadata and natural state/reason labels; verify ordered membership, stored-slug links, long-title wrapping, missing images and distinction between schedule classification and committed auction outcomes.
- [x] 5.3 Redesign lot/event index search/filter/count/sort controls with expandable mobile secondary filters; verify existing query/sort tests plus deliberate GET submission, reset/no-match recovery, direct links, refresh and Back/Forward preserve current URL semantics.
- [x] 5.4 Add enlarged gallery inspection with shared image selection, previous/next/position/close controls and accessible dialog focus; verify focused keyboard/focus tests, failed-media recovery, language preservation and no extra lot-view records or bid mutations.
- [x] 5.5 Recompose lot detail into gallery and focused auction/terms hierarchy, retaining full condition and fulfillment disclosure before submission in reading order; verify no-bids/current/open/closing/terminal/stale states, exact amounts and keyboard/zoom/mobile usability without an overlay hiding terms or controls.
- [x] 5.6 Critique the first rendered home/lot/mobile compositions before propagating their patterns to account/admin, then update discovery/gallery/query documentation in `web/README.md`; verify saved screenshots demonstrate the chosen hierarchy and documented interactions/native-navigation constraints match implementation.

## 6. Account and saved-lot experience

- [x] 6.1 Refine overview, login, registration, profile, bids and unpaid-wins layouts with grouped fields, clear primary actions and coherent state feedback; verify safe auth return, read-only email, confirmed profile saves, pending/error states and absence of unimplemented recovery controls in EN/VI.
- [x] 6.2 Add shared identity-scoped confirmed watchlist membership for card/detail/history-of-saves surfaces using existing APIs, with heart controls outside links; verify meaningful tests cover one shared list read, duplicate/pending suppression, unknown recovery, delayed responses and immediate clearing on logout/account change.
- [x] 6.3 Refine the owned watchlist page to share the card/action patterns and synchronize confirmed removals; verify anonymous login return, unavailable identity, empty list, edited membership and no stale saved state after account switching.
- [x] 6.4 Update account/watchlist documentation and run account/API/lifecycle regression tests; verify no persistent private cache, new endpoint, automatic bid or mutation replay was introduced.

## 7. Checkout, receipt and order history

- [x] 7.1 Rework checkout into grouped contact/fulfillment/payment form, server review and confirmation with a legible exact-total summary and product-language method names; verify existing checkout/session/money tests plus field correction and changed-review invalidation preserve internal method IDs and deliberate submission.
- [x] 7.2 Refine receipt and unpaid/paid order navigation, removing demo/simulation/no-charge UI while keeping paid detail read-only; verify refresh and unknown-result reconciliation retrieve the existing receipt with no extra confirmation or financial-credential input.
- [x] 7.3 Redesign history summaries/detail and icon-supported shipping/pickup timelines; verify existing inert-rendering/privacy tests and visual checks retain exact amounts/timezones, omit contact/receipt from summaries and mark only committed events complete.
- [x] 7.4 Add focused lifecycle coverage for account/logout/order changes during animated checkout/receipt/timeline presentation and document the new presentation in active README/frontend docs; verify obsolete private DOM/responses/form values disappear immediately and original payment/fulfillment behavior remains documented.

## 8. Administration presentation

- [x] 8.1 Refine admin overview and lot/event editors with consistent icons, grouped fields, list/editor hierarchy and publication feedback; verify draft save/validation/preview/publication and existing capability guards behave identically without progress-report chrome.
- [x] 8.2 Refine admin order list/detail/filter/next-action and shipping review controls; verify pending/conflict/unknown recovery, text-only carrier/reference, language-preserved shipping input and correct one-step shipping/pickup transitions with no pay-on-behalf action.
- [x] 8.3 Refine analytics metric hierarchy, filters and semantic comparisons using the shared visual system; verify snapshot/coverage definitions, demo/non-demo distinctions, simulated-value interpretation, exact large totals and current aggregate formulas remain accessible and unchanged.
- [x] 8.4 Update operational UI guidance and capture keyboard/narrow/zoom/error screenshots for all administration areas; verify direct Vite/embedded URLs, gate recovery, missing selections and sanitized access failures remain distinct.

## 9. Motion choreography and interaction critique

- [x] 9.1 Add the single coordinated homepage entrance and restrained image/card/heart feedback using shared timings; verify a recording shows entrance completed within 800 ms, stable image regions and immediate interaction without repeated section reveals or decorative idle loops.
- [x] 9.2 Apply panel/gallery/selection/layout transitions to menus, filters, language and image inspection; verify 200–300-ms roles, rapid reversal, keyboard focus/escape and a fully usable reduced-motion equivalent, including portal overlays.
- [x] 9.3 Add brief confirmed-price, bid, checkout and fulfillment feedback; verify exact values appear immediately, countdown ticks stay quiet and pending/unknown states never play success or delay eligibility changes.
- [x] 9.4 Verify exit transitions on private surfaces bypass retention during identity/access/order/source changes; use targeted tests and an active-animation browser recording to show no prior order/contact/receipt/recommendation remains and no duplicate stream/view/mutation is triggered.
- [x] 9.5 Critique rendered primary and utility screens against `design.md`, refine inconsistent spacing/imagery/icons/motion and update frontend motion guidance; verify comparable screenshots plus interaction recordings show a coherent object-led experience across desktop/mobile.

## 10. Integrated acceptance and handoff

- [x] 10.1 Run `make test`, `make test-race` and `make build` with an explicit dedicated `TEST_DATABASE_URL`, recording sanitized outputs; verify real-PostgreSQL, frontend, lint, race and frontend-before-embedding build checks pass without SQLite fallback.
- [x] 10.2 Exercise two bidders from public discovery through save/login, competing bids, sold result, owned review/payment, refresh and shipping/pickup completion on both Vite and embedded builds; verify authorization, exact values, one receipt, durable results and no legacy game sockets, credentials or external integrations in the recording/network evidence.
- [x] 10.3 Complete a route/state visual matrix in both languages at 320/375 CSS px and desktop, including loading/empty/missing/unavailable/access/validation/unknown states, long titles and failed media; verify before/after screenshots and focused keyboard flows cover every implemented storefront/account/admin destination plus gate/404.
- [x] 10.4 Verify actual browser 200% zoom and active reduced motion, including runtime preference changes, for shared chrome, gallery/bidding, account/auth/watchlist, checkout errors/receipt, order history and admin fulfillment/analytics in both builds; record settings and cross-reference outstanding Change 4–7 areas without editing archived checklists or substituting viewport emulation.
- [x] 10.5 Compare final build/image transfer and throttled interaction traces with baseline, and check offline media/font/icon loading; verify reserved media space, responsive primary controls, deferred secondary images, no animation-triggered extra requests, no decorative idle frame loops and documented size/layout/performance results.
- [x] 10.6 Publish `verification/REPORT.md` with visual critique, recordings, command evidence, spec/task coverage and explicit unresolved checks; update active README/frontend docs and ROADMAP to record Change 9 status without overstating earlier acceptance, then verify strict change validation and a clean scoped diff.

## Workflow follow-up

- Review completed implementation evidence before deciding whether acceptance is complete.
- Sync main specs and archive only when separately requested; preserve all previous archive limitations and unrelated working-tree files.
