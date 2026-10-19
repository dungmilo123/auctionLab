# vintage-design-system Specification

## Purpose

Provide a coherent, accessible vintage presentation language and reusable interface elements for AuctionLab's storefront, account, and administration experiences.

## Requirements

### Requirement: Shared vintage visual language
The website SHALL use a shared light theme with ivory backgrounds, charcoal text, burgundy primary actions, serif headings, and sans-serif body text. It SHALL define consistent typography, spacing, borders, interaction states, and responsive rules. Displayed prices and time values SHALL use tabular numerals. Olive and brass SHALL be secondary accents.

#### Scenario: Moving between layouts
- **WHEN** a visitor moves between storefront, account, and admin preview pages
- **THEN** shared controls, typography roles, and state labels use the same visual conventions
- **AND** the former orange game theme and hard offset shadows do not appear on these pages

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
A lot card SHALL present an image or local fallback, a title, descriptive metadata, a textual status, and a detail link. It SHALL support long titles and missing images without breaking its layout. Illustration cards SHALL be labeled as previews and SHALL not imply that bids, sales, or authenticity have been verified.

#### Scenario: Preview lot with unavailable image
- **WHEN** an illustrative lot image fails to load
- **THEN** a local fallback maintains the image region's dimensions and communicates the image's unavailability
- **AND** the title and detail link remain readable and usable

#### Scenario: Preview commercial information
- **WHEN** a preview card illustrates price or timing placement
- **THEN** it uses an explicit unavailable value or labeled sample
- **AND** it does not derive commercial values from a player's game budget, secret value, or game result

### Requirement: Locally available fonts and illustrations
The built application SHALL include its selected heading and body fonts, required English and Vietnamese glyphs, and preview illustrations. Rendering SHALL not require third-party font, image, stylesheet, or script services. Font or image failure SHALL fall back to readable text or a local placeholder. Distributed assets SHALL have recorded source and license information.

#### Scenario: External internet is unavailable
- **WHEN** a visitor loads the built application while the app server is reachable but external internet access is blocked
- **THEN** all required styles, fonts, and illustrations load from the application origin
- **AND** the layout and navigation remain usable without external requests

#### Scenario: Font cannot be loaded
- **WHEN** a bundled font request fails
- **THEN** a suitable system fallback displays the text without hiding it or blocking navigation

### Requirement: Consistent language presentation
The website SHALL retain the existing English and Vietnamese language selection and stored preference. New shared navigation, preview notices, statuses, and recovery text SHALL be available in both languages, with the document language reflecting the active choice.

#### Scenario: Language survives navigation
- **WHEN** a visitor selects Vietnamese and follows a link to another website shell
- **THEN** the shared interface and preview notices remain in Vietnamese
- **AND** the document language is Vietnamese and its text renders without missing glyphs
