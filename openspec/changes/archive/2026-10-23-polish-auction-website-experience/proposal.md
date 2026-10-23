# Proposal

## Why

Changes 1–8 provide the auction journey, but the website still reads and looks like an implementation demonstration: repeated capability notices, demo badges, labeled geometric artwork, flat information hierarchy and mostly static interactions. Change 9 should make AuctionLab's Collected Stories experience a distinctive, polished vintage auction house that is beautiful to browse and clear to transact with across desktop and mobile.

## What Changes

- Redesign the implemented storefront, account and administration surfaces as one coherent product: image-led editorial home/events, refined cards and search/filter controls, an inspectable gallery, a prominent bidding area, focused account forms, checkout and order timelines, and legible operational workspaces.
- Upgrade the locally served hero/catalog imagery, including demo lettering baked into SVGs, and present recognized legacy seed boilerplate in natural product language; retain asset provenance and existing published identities, URLs and purchase records.
- Replace progress-report copy with concise EN/VI product language. Remove demo/preview badges, payment/fulfillment simulation wording and no-charge disclaimers from buyer-facing UI, including confirmation and receipts. Keep underlying simulation, provenance flags and operator documentation; dedicated analytics provenance/filter definitions remain available to operators.
- Replace EN/VI buttons with a keyboard-operable language dropdown showing native language names, a globe and chevron; refine mobile navigation, search access and contextual navigation.
- Use the installed Lucide icon family consistently for meaningful controls, statuses and timelines; preserve visible labels for consequential actions and confirmed state for saves/bids/payments.
- Add a deliberate motion system using Motion for React and CSS: one coordinated homepage entrance, responsive menus/gallery/layout changes, and feedback for confirmed auction/purchase updates. Preserve immediate interaction, native document navigation, privacy cleanup and reduced-motion behavior.
- Require route/state coverage, before/after visual critique and recorded interaction evidence, including actual browser 200% zoom and runtime reduced motion. Record new evidence for the outstanding Change 4–7 acceptance areas without rewriting archived checklists.

## Capabilities

### New Capabilities

- `website-motion`: Shared motion behavior, bounded timings, lifecycle-safe feedback and reduced-motion alternatives across implemented website surfaces.

### Modified Capabilities

- `vintage-design-system`: Product-wide visual hierarchy, coherent icons, local editorial media, bilingual presentation and accessible interaction patterns.
- `website-navigation`: Refined responsive shells, native-name language selection and product-facing presentation replacing historical preview notices.
- `auction-discovery`: Image-led home/events, compact cards/filters, enlarged gallery inspection, focused lot/bid hierarchy and removal of consumer demo labels.
- `vintage-catalog`: Demo provenance remains seed metadata; bundled catalog artwork and recognized seed boilerplate support the polished presentation without visible demonstration lettering.
- `bidder-accounts`: Provisioned-account provenance remains metadata rather than a required profile badge.
- `lot-recommendations`: Natural discovery reasons and refined cards without demo markers or implementation explanations.
- `winner-checkout`: Clear form/review/confirmation presentation without simulation notices, retaining owned, explicit, server-reviewed checkout.
- `demo-payments`: Buyer-friendly method/receipt labels replace mandatory simulation/no-charge copy; internal method identifiers and local confirmation semantics stay unchanged.
- `order-history`: Readable purchase summaries and progress timelines without simulation badges.
- `demo-fulfillment`: Refined operational controls and buyer shipment presentation without mandatory simulated/manual badges.

## Impact

Primary work is in `web/src/layouts`, `components/site`, implemented `routes/storefront`, `routes/account`, `routes/admin`, website copy dictionaries, `index.css`, local media and asset manifests, plus future text defaults in `internal/catalog/seed.go`. Add `motion` to frontend dependencies/lockfile; reuse installed `lucide-react` and Radix primitives. The separate Go admin gate receives presentation-only alignment. Update active operator/frontend documentation and ROADMAP during implementation to explain the presentation change and its evidence.

This deliberately supersedes earlier visible-demo and explicit-simulation presentation requirements. It does not add a payment gateway, financial-credential fields, carrier integration, ML, currency conversion, new commerce routes, database migrations or changes to bidding, payment/fulfillment authorization, idempotency, timing or analytics formulas. Archived evidence and the unrelated existing `skills-lock.json` remain outside this change's edits.
