# Design

## Context

See `proposal.md` for motivation. Source inspection, rather than a live visual audit, established the following baseline:

- `web/src/layouts/SiteLayout.tsx` owns three shells, plain-text navigation, EN/VI buttons, persistent capability notices and a progress-report footer. Account/admin navigation wraps and becomes a sidebar at 1024 px.
- `HomePage.tsx` uses a static camera illustration. `DiscoveryPresentation.tsx` owns public cards, demo badges, schedule metadata, basic loading states and thumbnail selection. `LotDetailPage.tsx` places disclosures, watch controls and the execution panel in a long vertical flow.
- `AuctionPanel.tsx` already separates server execution, eligibility and owner participation. Checkout has form/review/confirmation state, lifecycle cancellation and unknown-outcome recovery. Those behaviors are foundations to preserve.
- Website copy is split across site/discovery/account/auction/order/fulfillment/catalog/insights dictionaries. Recommendation sections expose algorithm/progress language; the admin analytics dictionary also owns legitimate operator provenance definitions.
- Existing tokens are ivory/charcoal/burgundy, local Noto Serif/Noto Sans with Vietnamese glyphs, 4-px spacing units and 44-px targets. `.website` scopes much of the CSS, with later overrides and an existing reduced-motion rule.
- `web/package.json` already includes `lucide-react`, `radix-ui`, `tw-animate-css` and React 19. Motion is not installed. Links and GET forms navigate native documents; there is no client routing layer to animate between pages.
- All 39 bundled catalog SVGs are project-authored illustrations with baked-in demo lettering. `internal/catalog/seed.go` also stores demonstration language in theme, description, condition, story, alt text, specifications and fulfillment instructions. These values can already be immutable published content or order snapshots.
- `internal/api/admin_gate.go` is a separate, bilingual, self-contained HTML page before React is authorized. It must retain its existing capability/key/cookie behavior.
- Current tests cover query/money rules, account lifecycle, auction feeds, API boundaries, checkout identity cancellation and inert order rendering. Existing `ROADMAP.md` records outstanding actual-zoom/reduced-motion coverage from Changes 4–7.

## Goals / Non-Goals

**Goals:**

- Translate the accepted boutique vintage direction into a concrete visual and interaction system for every implemented route, with deliberate imagery and a memorable homepage.
- Share navigation, icon, form, card, feedback, formatting and motion patterns without coupling them to private commerce state machines.
- Make the presentation independent of historical demonstration labels while retaining raw provenance, immutable purchase terms, exact USD values and operator truth.
- Make design quality reviewable through comparable images and interaction recordings, including real zoom and motion settings.

**Non-Goals:**

- Rebranding AuctionLab, adding commerce routes or changing its single-seller auction model.
- A payment gateway, financial inputs, carrier tracking, live currency conversion, trained recommendations, new database migrations or changes to the retained game.
- Converting native document navigation to a client router for full-page/shared-card transitions.
- Rewriting published catalog/order rows, stripping arbitrary user-authored words or retroactively completing archived acceptance records.

## Decisions

### 1. Art direction: a collector's viewing room

The visual focal point is the object: believable material, generous image space, legible condition information and carefully typeset titles. Keep the existing palette because it was chosen for Collected Stories, rather than replacing it merely to introduce novelty.

| Token | Value | Use |
| --- | --- | --- |
| Ivory | `#F6F1E8` | Quiet page canvas |
| Charcoal | `#24211E` | Body, headings, precise amounts |
| Burgundy | `#722F37` | Primary action, selection, brand emphasis |
| Olive | `#62664D` | Supporting success/status accents with text |
| Brass | `#A38355` | Small decorative/material accents, not low-contrast body text |
| Paper | `#FFFFFF` | Image mats and selectively elevated functional surfaces |

Reuse neutral border/error/focus tokens and recheck contrast in actual states. Noto Serif carries headings and object names; Noto Sans carries controls and body; numbers remain tabular, without a new monospace data style. Use a deliberate scale approximately 14/16/18/24/32/48/64 px with fluid headings and wrapping, body line lengths around 60–75 characters and comfortable line height. Keep most content left aligned and numeric table columns aligned to their values.

Spacing uses the existing 4-px scale: 8/12 for related controls, 16/24 for groups, 32/48/64 for sections. Avoid one universal card frame: imagery gets quiet mats, the bid/review area gets functional emphasis, editorial stories remain open text. Corners and shadows are restrained and assigned by purpose. Remove repeated uppercase eyebrows and generic diamond empty-state ornaments from mounted pages.

**Alternatives considered:** a generic rounded-card dashboard, full dark luxury theme or changing fonts everywhere. They add churn and weaken the existing subject-specific identity. Retaining cream and serif is intentional here; quality must come from object imagery, composition and hierarchy.

### 2. Compose the primary screens before extending their patterns

These are the chosen layout targets; implementation captures baseline screenshots first and critiques the first rendered home/lot/mobile pass against them before propagating the patterns.

```text
Desktop home
+------------------------------------------------------------------+
| AuctionLab    Auctions   Collection     Search  Saved  Account Lang |
+------------------------------------------------------------------+
| Collection story + heading       | Large leading event image       |
| Explore the collection           | One visual focal point          |
+------------------------------------------------------------------+
| Featured events: distinct collection compositions                  |
| Closing by schedule: concise object cards                          |
| Suggestions and category discovery                                 |
+------------------------------------------------------------------+
| Short brand statement + working navigation                         |
+------------------------------------------------------------------+

Desktop lot
+------------------------------------+-----------------------------+
| Object title + parent collection                                  |
+------------------------------------+-----------------------------+
| Large image + selectable thumbnails| Current price / no bids      |
| Enlarged inspection on demand      | Close time and bid minimum   |
|                                    | Condition + fulfillment      |
|                                    | Save + eligible bid action   |
+------------------------------------+-----------------------------+
| Object story + specifications      | Bid history                  |
| Related objects                                                   |
+------------------------------------------------------------------+

Mobile lot
+----------------------------------+
| Brand        Language    Menu     |
| Title + parent collection        |
| Image + thumbnails               |
| Current price + close time       |
| Condition + delivery/pickup terms |
| Save + bid action                |
| Story / specs / history          |
+----------------------------------+
```

Use the first featured published event selected by the existing `homeSelection` order for the hero; preserve the existing featured, nearest-close, category and independently loaded recommendation contracts. Missing content uses an intentional empty brand composition, never fake auctions, awards, quotes or bidder counts. Event index/detail use their stored cover, narrative and schedule rather than identical boxes.

Desktop lot uses a wider gallery and a narrower functional column. Keep required condition and full enabled fulfillment disclosures before the actual submit control in DOM/reading order. Secondary prose can follow. A desktop sticky functional column is allowed only within its section and if long content/200% zoom remains usable; mobile keeps controls in flow and introduces no fixed bottom bid control that bypasses disclosures.

Lot/event indexes use a visible search/count/sort row and an expandable secondary filter section on mobile, still submitting native GET forms. Account pages use an icon-supported sidebar on desktop and compact section navigation on mobile. Login/profile forms receive grouped fields and focused error summaries. Watchlist, bids and wins share refined cards/rows with clear owned status. Checkout shows contact/fulfillment/method groups, then a stable review summary and explicit confirmation. Order history uses image/title/amount/status summaries; detail uses a committed-event timeline. Admin catalog/order/analytics retain their denser task layout, real table headers, filter URLs and correct next actions while sharing the same primitives.

**Design self-review:** applying the same three-column card grid, uppercase label and entrance animation to every section would reproduce a template. Revise that baseline into one large collection focal image, smaller object cards, open editorial content and a single clearly prioritized commerce panel. Concentrate visual boldness in the collection imagery; utility surfaces stay quiet.

### 3. Upgrade imagery without changing published paths

Replace the 18 object plates, 18 detail plates and three event covers under existing `/site-assets/catalog/*.svg` paths with cohesive, detailed editorial artwork. Use consistent neutral lighting/backgrounds, object-specific compositions and details compatible with the supplied title/category. Retain path compatibility for old publications and snapshots. High-quality native vector work can stay SVG; if raster imagery materially improves the result, use the imagegen skill for generated assets and embed the optimized raster within the existing SVG wrapper so old URLs still resolve without a database rewrite or nested remote fetch.

Record original/generated status, source, rights and any generation provenance in the asset manifest; keep the fallback and manifests truthful. Remove demo/illustration lettering from the artwork, including accessible SVG titles where applicable. Do not invent branded models, serials, certified authenticity or photographic evidence. Reserve image dimensions, keep the focal image eager and secondary images lazy where appropriate, and verify the image wrappers actually render offline in both builds. The hero takes its media from the leading published event; the static camera illustration becomes a fallback/development asset rather than the homepage focal point.

**Alternative:** changing seed media URLs or publishing replacements would either leave historical snapshots visually inconsistent or require data maintenance. Path-preserving local artwork is reversible and applies to existing references.

### 4. Separate product copy from operational facts

Audit mounted copy and accessible text across all eight dictionaries and the components that render them. Remove global `PreviewNotice`/published-management capability inventories, consumer `Demo` markers, simulation qualifiers, no-charge banners and algorithm/future-ML announcements. Use sentence case and consistent verbs: Save/Remove, Place bid, Review order, Confirm payment, Payment confirmed, View order, Reload. Keep localized pending, stale, conflict and unknown states explicit. Method display maps `demo-card` to Card/Thẻ and `demo-bank-transfer` to Bank transfer/Chuyển khoản; wire values do not change and there are still no financial-credential fields or redirects.

Dedicated admin analytics retains demo cohort filters, actor separation, simulated-value definitions, snapshot/coverage details and exact formulas. These are useful data interpretation, not repeated global marketing or progress copy. Active README/operator docs continue to explain simulation and maintenance. Raw developer identifiers remain in technical documentation, not buyer status messages.

Legacy seed text requires a bounded compatibility layer rather than a database edit or regex over arbitrary prose. Define explicit replacements for the known stock fields in `internal/catalog/seed.go`, applied only when demo provenance, recognized catalog media identity and the entire original field match. Work field by field so an edited field remains authored. This includes event theme/description/alt, lot description/story/condition/alt, stock representation specifications and delivery/pickup instructions, including matching fields in purchase snapshots. Preserve unknown condition and absent provenance, e.g. “Condition has not been assessed” and “Provenance information is unavailable”; do not promise a pickup address or verified history. Keep title, money, schedules, eligibility and stored terms intact. New seed defaults use equivalent natural wording while preserving demo flags and repeat-safety. Raw APIs remain unchanged, and unfamiliar/edited content renders untouched. Fixture/editorial copy remains as authored except for these explicit legacy stock fields.

**Alternative:** replacing every occurrence of “demo” would corrupt authored content and potentially change fulfillment meaning. Mutating old published rows or snapshots would violate immutability. The exact-match display adapter is a limited compatibility measure with focused tests and a documented scope.

### 5. Use existing icons and accessible primitives

Use installed Lucide SVGs consistently at 18–20 px in controls, 20–24 px in navigation and appropriately larger in designed state illustrations. Share icon sizing/stroke/alignment through a small website wrapper. Use Globe/Chevron for language, Search for search, Heart for saving, Sliders for filters, User for account, Clock for timing and Package/Truck/Check for the order journey. Status meaning remains in text; decorative icons use `aria-hidden`. Icons never become sole labels for bid/payment/publish/fulfillment confirmation.

Use installed Radix dropdown/dialog primitives for language and image inspection, with website-styled portal containers. The header language trigger shows the active native name; choices are English and Tiếng Việt with a selected indicator. No flag-only control. Reuse the current language provider/storage key; opening/selecting cannot remount the page or private form. Mobile menu/filter controls support Escape, correct expanded state and focus restoration. Gallery selection is shared with enlarged inspection, whose modal contains focus and exposes previous/next, image position and close.

Header search navigates/submits to existing `/lots?q=...`; saved/account links use existing routes and safe login recovery. All normal navigation stays `<a href>`. The Go admin gate receives matching typography/spacing/focus treatment with inline CSS and no new JavaScript/external asset requirement; its intentionally bilingual entry form stays bilingual.

Card hearts must reuse account and watchlist semantics. Read the existing owner watchlist once per validated identity/document into shared membership state rather than fetching every card independently. Cards and detail/watchlist controls share confirmed membership and invalidate/reconcile it after writes. Checking/unavailable is not unsaved; pending writes prevent duplicates, failures offer existing membership recovery, logout invalidates the whole cache and no optimistic saved result is announced. This adds no endpoint, polling loop or persistent private cache.

### 6. Motion for stateful transitions, CSS for local feedback

Add the `motion` package using current Motion for React APIs and lock the resolved version in the frontend lockfile during implementation. Use CSS for color/border/hover/press effects and Motion for coordinated entrances, menu/dialog/gallery transitions and bounded layout changes. Do not add GSAP, a second icon package or a client router. References checked through Context7 in this conversation: [layout transitions](https://motion.dev/docs/react-layout-animations), [MotionConfig](https://motion.dev/docs/react-motion-config), [reduced motion](https://motion.dev/docs/react-accessibility).

| Role | Initial token | Application |
| --- | --- | --- |
| Micro | 150 ms; range 120–180 ms | Button/heart/selection feedback |
| Panel | 240 ms; range 200–300 ms | Dropdown, disclosure, dialog, selected image |
| Entrance | 650 ms total; range 500–800 ms | One coordinated homepage image/heading/action sequence |
| Easing | `cubic-bezier(0.22, 1, 0.36, 1)` | Calm entrances and panels |

Use opacity and restrained transforms, no constant parallax, cursor trails, autoplay carousel or bounce on every control. Card image hover scale is approximately 1.02 with a stable clipping region. A price update immediately renders the exact new cents with brief emphasis; never animate numerical interpolation for a bid/total. Countdown ticks do not animate or create live announcements. Bid/checkout/fulfillment success effects follow existing confirmed state and never trigger a request.

Put reduced-motion policy at the common website boundary and use the runtime preference for custom behavior. Existing CSS `.website *` does not cover portals or JS transforms: portal roots need the same website motion class/policy, CSS fallback and verification. Under reduce, omit nonessential entrance, transform, layout and looping motion; indicators remain understandable with static text. Content must render if an animation is interrupted or unavailable.

**Private lifecycle rule:** animated exits can retain DOM after logical unmount. Private order/contact/receipt, shipping edits and personalized cards therefore bypass exit retention immediately on identity/access/order/source changes. Key state by the existing identity generation, clear overlay content synchronously, abort/ignore late reads, and never depend on animation completion for auth cleanup. Animation wrappers must not remount auction feeds, replace request IDs, re-record views, reset pending form values or replay mutations.

**Alternatives:** CSS alone is adequate for small effects but cumbersome for coordinated enter/exit/layout changes; adding multiple engines increases cost without a needed capability. Native cross-document/shared-element page transitions are deferred because the current navigation/security model already works.

### 7. Verify aesthetics and behavior as separate evidence

Keep evidence under this change's `verification/` during apply. Capture before/after views with matching fixture, viewport, language and state; record an actual two-account journey and menu/gallery/confirmation motion. Supply a route/state matrix and critique composition, image coherence, title/price hierarchy, action clarity, icon consistency and mobile flow. Fix issues revealed by screenshots before declaring visual acceptance.

Required browser coverage spans Vite and the embedded binary, both languages, 320/375-CSS-pixel layouts and desktop (1280–1440 px), actual browser 200% zoom, keyboard-only interaction and active reduced motion. Include long titles, field errors, missing images, slow/unavailable services, unknown confirmations, signed-out/identity switch and dense admin comparison. Responsive viewport emulation is not actual zoom. Record runtime settings with evidence; if an exact setting is unavailable, its task stays open.

Compare built asset sizes and a fixed local interaction trace against baseline. Report JS/image transfer bytes, throttling settings and layout-shift observations; ensure primary controls are not animation-gated, focal media reserves space, secondary images defer loading, idle pages have no decorative frame loop, and animation adds no network polling or commerce requests. No universal 60-fps guarantee is inferred from one machine.

Run targeted new interaction/lifecycle tests plus existing frontend tests/lint/typecheck/build. Backend regressions run against a real isolated PostgreSQL target because seed defaults and Go gate presentation are in scope; use repository `make test`, `make test-race` and `make build` with required test service configuration. Do not create tests that merely mirror icon lists or arbitrary CSS values. Meaningful additions cover dropdown/dialog focus, card save reconciliation, exact legacy-text matching, immediate private removal during animation and preservation of payment/auction invariants.

Map new current-build evidence to Change 4 tasks 7.3/8.3, Change 5 task 8.4, Change 6 task 7.3 and Change 7 tasks 7.5/7.6. Cover both languages, long titles, field errors, checkout and history/fulfillment, runtime reduced motion and actual 200% zoom for those areas. Keep old archives unedited; the new report identifies precisely what is now verified and what remains open.

## Risks / Trade-offs

- [Polish becomes only icons and animation] → Require upgraded imagery, distinct compositions and before/after critique of primary screens before propagating patterns.
- [Motion hides useful content, hurts interaction or leaks private exits] → Short shared timings, immediate state rendering, stable DOM/lifecycle boundaries and identity-switch tests with transitions active.
- [Legacy stock text remains visible or custom content is overwritten] → Inventory every seed field and image title, use exact field/provenance/media matches, and test custom/edited/non-demo counterexamples and historical snapshots.
- [Raster artwork bloats local assets or wrappers fail] → Optimize per-image payload, test SVG rendering offline in Vite/embedded and record transfer bytes; retain high-quality vector rendering where appropriate.
- [New visual credibility implies unsupported authenticity or integrations] → Keep unknown condition/provenance meaningful, avoid invented trust claims and preserve technical processing definitions in operator docs.
- [Shared styles regress dormant game components or portal readability] → Scope website styles/overlays and replace conflicting existing website selectors deliberately rather than append more overrides; run retained tests.
- [Visual pass masks earlier acceptance gaps] → Separate planning, implementation and visual verification states; retain explicit incomplete checks and archived evidence.

## Migration Plan

1. During apply, capture baseline screenshots, motion/network observations and build-size data on an isolated fixture; retain existing worktree changes.
2. Implement shared tokens/components, copy compatibility and locally served imagery, then the primary layouts, remaining routes and motion. Update future seed text only through the existing explicit seed paths.
3. Run focused checks and real-PostgreSQL regressions, build frontend before Go embedding, then verify both serving modes and all route/state evidence.
4. Update active docs and ROADMAP with Change 9 scope/status and current acceptance evidence. Sync delta specs/archive only in a separately requested lifecycle step.
5. Deploy normal rebuilt assets/binary with no schema or data maintenance. Existing sessions, method IDs, published references and snapshots stay valid. Rollback restores the prior binary/assets and dependency lockfile; it does not reset data or revert committed user transactions. Newly seeded natural descriptions are valid existing-format content and need no reverse migration.
