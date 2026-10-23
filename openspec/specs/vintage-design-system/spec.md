# vintage-design-system Specification

## Purpose

Provide a coherent, accessible vintage presentation language and reusable interface elements for AuctionLab's storefront, account, and administration experiences.

## Requirements

### Requirement: Shared vintage visual language
The website SHALL use a shared light theme with ivory backgrounds, charcoal text, burgundy primary actions, serif headings, and sans-serif body text. It SHALL define consistent typography, spacing, borders, interaction states, and responsive rules. Displayed prices and time values SHALL use tabular numerals. Olive and brass SHALL be secondary accents. Storefront, account and administration SHALL use this language as implemented product surfaces with deliberate image, content and action hierarchy.

#### Scenario: Moving between layouts
- **WHEN** a visitor moves between storefront, account, and admin pages
- **THEN** shared controls, typography roles, icons and state labels use the same visual conventions
- **AND** the former orange game theme and hard offset shadows do not appear on these pages

#### Scenario: Visual emphasis matches the task
- **WHEN** a visitor views home, lot detail, checkout or an operational workspace
- **THEN** the collection image, current auction amount/action, purchase review or operational selection respectively has clear primary emphasis
- **AND** decorative labels, repeated frames and uniform card treatments do not obscure that hierarchy

### Requirement: Readable text and identifiable states
Normal text SHALL have contrast of at least 4.5:1 and large text at least 3:1 against its rendered background. Focus indicators and required control boundaries SHALL have at least 3:1 contrast against adjacent colors. Status and validation meaning SHALL be conveyed by readable text as well as any color treatment.

#### Scenario: Inspecting state variants
- **WHEN** a visitor views upcoming, live, ended, sold, unavailable, or invalid states in the component previews
- **THEN** each state has an explicit text label or explanation
- **AND** enabled default, hover, focus, and error variants meet the applicable contrast threshold

### Requirement: Accessible shared controls
Navigation, buttons, form fields, and status badges SHALL expose appropriate semantic roles and accessible names. Fields SHALL associate labels, help, and error text with their controls. Disabled actions SHALL not submit or mutate data and SHALL have an adjacent explanation when their unavailability would otherwise be unclear. Interactive targets SHALL be at least 44 by 44 CSS pixels.

#### Scenario: Keyboard traversal
- **WHEN** a visitor uses Tab and keyboard activation through a page
- **THEN** enabled controls are reachable in a logical order with visible focus
- **AND** links navigate, buttons perform only their stated UI action, and disabled actions do not execute

#### Scenario: Form preview validation
- **WHEN** a demonstration field is shown with an error
- **THEN** its label remains visible, its invalid state is exposed to assistive technology, and the error is associated with that field
- **AND** the preview form does not collect credentials or submit business data

### Requirement: Responsive and motion-aware layouts
Website shells and shared components SHALL remain usable from 320 CSS pixels wide through desktop layouts without page-level horizontal overflow. They SHALL preserve access to navigation at narrow widths and support 200 percent text zoom. Nonessential transitions SHALL be suppressed when reduced motion is requested.

#### Scenario: Narrow viewport and long copy
- **WHEN** the page is viewed at 320 or 375 CSS pixels with long lot titles and navigation labels
- **THEN** content wraps without clipping controls or hiding navigation destinations
- **AND** cards and form fields fit the available width

#### Scenario: Enlarged text and reduced motion
- **WHEN** a visitor enlarges text to 200 percent or enables reduced motion
- **THEN** controls and content remain usable
- **AND** nonessential motion does not run under the reduced-motion preference

### Requirement: Reusable lot presentation
A lot card SHALL present an image or local fallback, a title, descriptive metadata, a textual schedule status, an explicitly labeled opening price and a native detail link. It SHALL support long titles and missing images without breaking its layout. Shared cards SHALL use consistent image framing and concise metadata, omit demonstration badges and show a saved-lot control with its own accessible action, separate from navigation.

#### Scenario: Preview lot with unavailable image
- **WHEN** a published lot image fails to load
- **THEN** a local fallback maintains the image region's dimensions and communicates the image's unavailability
- **AND** the title and detail link remain readable and usable

#### Scenario: Preview commercial information
- **WHEN** a card presents price or timing
- **THEN** values come from the matching published catalog and absent amounts remain unavailable
- **AND** it does not derive commercial values from a player's game budget, secret value, or game result

#### Scenario: Card save action
- **WHEN** a visitor activates a card's save control by pointer or keyboard
- **THEN** it follows existing account validation and confirmed watchlist semantics without following the card link
- **AND** its accessible name identifies the action and lot, and an unknown membership is not displayed as confirmed unsaved

### Requirement: Locally available fonts and illustrations
The built application SHALL include its selected fonts, English/Vietnamese glyphs, editorial media and local image fallback. Rendering SHALL not require third-party font, image, stylesheet or script services. Asset failure SHALL preserve readable content. Distributed media SHALL have recorded source, rights and authored/generated status; visible catalog media SHALL omit demonstration lettering and match the represented object without adding authentication claims.

#### Scenario: External internet is unavailable
- **WHEN** a visitor loads the built application while the app server is reachable but external internet access is blocked
- **THEN** all required styles, fonts, icons and editorial media load from the application origin
- **AND** the layout and navigation remain usable without external requests

#### Scenario: Font cannot be loaded
- **WHEN** a bundled font request fails
- **THEN** a suitable system fallback displays the text without hiding it or blocking navigation

#### Scenario: Editorial image provenance
- **WHEN** bundled hero, event or lot artwork is replaced or refined
- **THEN** its source/license and generated or authored status are recorded with the assets and accessible descriptions match the image
- **AND** the image has no baked-in demo/preview text and introduces no unsupported maker, certification or condition claim

### Requirement: Consistent language presentation
The website SHALL retain English and Vietnamese selection and stored preference. Shared navigation, states, recovery copy and accessible names SHALL be available in both languages, with the document language reflecting the active choice. Interface copy SHALL use concise product language; editorial content SHALL remain as authored except for defined bounded legacy seed presentation. USD values SHALL remain exact without implicit currency conversion.

#### Scenario: Language survives navigation
- **WHEN** a visitor selects Vietnamese and follows a link to another website page
- **THEN** shared interface content remains in Vietnamese
- **AND** the document language is Vietnamese and its text renders without missing glyphs

#### Scenario: Consistent vocabulary and numeric meaning
- **WHEN** a visitor changes language while viewing an amount, timestamp or transaction action
- **THEN** interface formatting and labels follow that language while currency, instant, stored values and authored content retain their meaning
- **AND** pending, unknown and confirmed outcomes remain distinct in both languages

### Requirement: Purposeful coherent iconography
Website navigation, save/search/filter controls, gallery controls and order timelines SHALL use a consistent vector icon family and sizing. Decorative icons SHALL be hidden from assistive technology. Icon-only controls SHALL have localized accessible names and state where applicable. Bid, payment, publication and fulfillment confirmation SHALL retain visible action text.

#### Scenario: Understand and operate icon controls
- **WHEN** a visitor uses an icon control with pointer, keyboard or assistive technology
- **THEN** its action, selected state, focus and at least 44-by-44-CSS-pixel target remain identifiable
- **AND** the icon does not create a nested interactive element, replace the sole textual status or receive a duplicate screen-reader announcement

### Requirement: Designed product states and forms
Implemented pages SHALL present distinct loading, empty, no-match, access-required, missing, validation, unavailable, pending and unknown states with contextual guidance. Forms SHALL group related fields, preserve associated labels/errors and distinguish primary actions. Success feedback SHALL reflect confirmed results; layouts SHALL remain usable with long bilingual content and missing media.

#### Scenario: Empty watchlist and failed checkout
- **WHEN** the owner has no saved lots or a checkout review fails validation
- **THEN** the former offers catalog discovery and the latter identifies invalid fields while retaining correct entered values
- **AND** neither screen substitutes decorative sample records or a fabricated successful result

### Requirement: Visual and interaction acceptance evidence
Website acceptance SHALL cover implemented routes and important states with comparable before/after images and recorded interactions. It SHALL include EN/VI, 320/375-CSS-pixel and desktop layouts, actual 200-percent browser zoom, active reduced motion, keyboard navigation, long content and missing-media/error states. Missing evidence SHALL remain explicitly incomplete.

#### Scenario: Review the polished journey
- **WHEN** the change is verified
- **THEN** evidence shows home, event/lot discovery, gallery/bidding, account/watchlist, checkout/history and administration using the same design language
- **AND** interaction recordings demonstrate motion, focus and confirmed feedback; viewport resizing alone is not accepted as browser-zoom evidence
