# Tasks

All tasks are implementation work for a later apply request. Refer to `design.md` for the route/token tables and the two delta specs for acceptance behavior. Do not count a preview page as a completed commerce feature.

## 1. Visual tokens and local assets

- [x] 1.1 Implement the light-theme colors, type roles, spacing, borders, focus, breakpoints, and reduced-motion rules in `web/src/index.css`; verify foreground/background contrast for each enabled state against the design thresholds and record the checked pairs in `verification/visual-checks.md`.
- [x] 1.2 Vendor the selected Noto Serif/Noto Sans font files and license text under the frontend asset tree, covering the specified weights and English/Vietnamese glyphs; verify the files render Vietnamese diacritics locally and record exact sources, releases, and licenses in `web/src/assets/README.md`.
- [x] 1.3 Add a small set of local vintage preview illustrations and a shared missing-image fallback using the imported asset and `/site-assets/` locations from the design; verify a frontend build includes the files and their source/license records identify each asset.

## 2. Shared components and website copy

- [x] 2.1 Adapt the existing button, input, label, and card primitives to the vintage tokens, removing old hard shadows and thick-outline recipes from website use; verify default, hover, focus, disabled, and invalid states in browser previews with at least 44px targets and correctly associated field help/errors.
- [x] 2.2 Create reusable lot cards, status badges, preview notices, and empty/unavailable states with presentation-only props; verify long titles, textual statuses, image failure, usable detail links, and disabled actions without business requests or game-state imports.
- [x] 2.3 Add typed English/Vietnamese website copy integrated with the existing language provider without replacing the game dictionary; add focused copy-completeness/interpolation tests and verify both languages render shared labels, notices, recovery text, and document titles while existing language tests pass.
- [x] 2.4 Document component usage, states, local assets, and typography conventions in `web/README.md`; verify the examples correspond to the actual component interfaces and identify illustrative content as preview data.

## 3. Explicit routes and responsive shells

- [x] 3.1 Replace the player/admin/screen prefix selector with the explicit page/layout resolver from the design; update `web/src/lib/route.test.ts` to cover every route class, one trailing slash, query/fragment handling, case sensitivity, prefix near-misses, extra segments, and retired screen paths, and verify the focused tests pass.
- [x] 3.2 Build shared header, footer, and contextual navigation plus storefront/account/admin layouts using native links; verify all displayed links have implemented destinations, active links are marked accessibly, skip links reach main content, and narrow layouts retain navigation access.
- [x] 3.3 Add route-specific preview pages and isolated illustrative fixtures, including generic auction/lot detail previews and the recoverable 404; verify every route-table entry has the correct heading/layout and each future business action is absent or disabled with an explanation.
- [x] 3.4 Switch `App.tsx` to the new shells, set route-aware document titles/language, and update brand metadata/favicon as needed; verify `/`, `/admin`, and `/screen` no longer mount game views and a browser with saved game identity opens no game WebSocket or business request.
- [x] 3.5 Document the route table, native navigation behavior, public account previews, retired game URLs, and frontend-404 HTTP status distinction in `web/README.md`; verify the documented paths by direct load and refresh in the development frontend, and verify Back/Forward and new-tab navigation.

## 4. Embedded hosting and admin entry

- [x] 4.1 Extend static hosting to return real 404 responses for missing `/site-assets/*` while preserving document fallback and `/assets/*` caching behavior; extend `internal/api/static_test.go` with new nested routes, font/image files, namespace misses, and cache assertions, then verify the focused Go tests pass.
- [x] 4.2 Check missing asset behavior in the development frontend and add a narrowly scoped handler in `web/vite.config.ts` only if necessary; verify missing font/image requests in both reserved namespaces return 404 without SPA HTML, while existing assets and public nested pages still load.
- [x] 4.3 Restyle the standalone admin gate's inline HTML/CSS using the new palette and system font fallbacks; preserve key/cookie/redirect semantics, retain or adjust presentation assertions in `internal/api/admin_gate_test.go`, and verify those tests pass plus keyboard/error-state usability in the Go-served gate.
- [x] 4.4 Document the Vite visual-preview versus Go authorization boundary and offline build behavior in `web/README.md`; verify the instructions distinguish a public empty dev shell from the protected Go document and do not imply account authentication is implemented.

## 5. Integrated acceptance

- [x] 5.1 Run `make test` and `make build`; verify frontend tests/lint/typecheck, Go tests, and the CGO-disabled embedded build succeed, the frontend is built before Go, `.gitkeep` remains present, and retained game tests were not removed to hide regressions.
- [x] 5.2 Start the built binary with a temporary database and inspect every route class, direct refresh, Back/Forward, new-tab navigation, unknown routes, and `/screen`; verify production HTML caching and asset/API error behavior, recording observations in `verification/acceptance.md` without credentials.
- [x] 5.3 Exercise `/admin` and `/admin/lots` through the Go server with no key, a wrong key, and a valid capability; verify the gate blocks unauthorized document access, correct-key entry removes the key via the fixed redirect, and authorized nested navigation/refresh works.
- [x] 5.4 Inspect storefront, account, admin, 404, and gate layouts at 320, 375, 768, and 1280 CSS pixels, 200 percent text zoom, keyboard-only navigation, and reduced motion in both languages; verify no horizontal overflow or inaccessible controls, actual rendered contrast, readable fallback states, and save representative screenshots plus observations under `verification/`.
- [x] 5.5 Cold-load the built application with external origins blocked but the app server reachable; verify selected fonts/illustrations load locally, forced font/image failures degrade readably, language preference survives navigation, and saved player tokens neither restore game identity nor trigger game/business network activity.
- [x] 5.6 Reconcile `verification/acceptance.md` with both spec files and `web/README.md`; verify each requirement has automated or explicitly recorded browser evidence, list any remaining failures, and confirm the delivered functionality is the website foundation with no commerce completion claims or database migration/deletion.
