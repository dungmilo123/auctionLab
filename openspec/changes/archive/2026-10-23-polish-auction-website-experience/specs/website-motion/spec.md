# Spec Delta

## Purpose

Provide a coherent motion language that makes vintage discovery memorable and product interactions clear without delaying commerce or retaining private data.

## ADDED Requirements

### Requirement: Coordinated motion hierarchy
The website SHALL use shared easing and timing roles: 120–180 ms for micro-feedback, 200–300 ms for panels and image transitions, and 500–800 ms for one coordinated homepage entrance. Other pages SHALL prioritize interaction-triggered transitions. Content and controls SHALL remain available independently of entrance completion.

#### Scenario: First homepage presentation
- **WHEN** published homepage content becomes available with ordinary motion enabled
- **THEN** its focal image, heading and exploration actions enter as one coordinated sequence completed within 800 ms
- **AND** visiting lower sections does not trigger repeated decorative entrances or require waiting to follow a link

#### Scenario: Menu and gallery interaction
- **WHEN** a visitor opens or closes navigation, filters, language selection or enlarged imagery
- **THEN** transitions use the shared panel timing and preserve visible focus and a stable surrounding layout
- **AND** keyboard input and rapid reversal remain responsive during the transition

### Requirement: Confirmed state feedback
Watchlist, bid, payment and fulfillment success feedback SHALL follow server-confirmed state. Motion SHALL NOT invent intermediate prices, imply success while pending, extend an auction window or trigger mutations. Countdown changes SHALL not flash or announce every tick.

#### Scenario: Price changes and auction close
- **WHEN** a newer committed auction revision changes the current price or closes bidding
- **THEN** the exact new amount or terminal state appears immediately, with at most one brief visual emphasis for the changed value
- **AND** submission eligibility changes immediately without waiting for an animation or presenting an interpolated amount

#### Scenario: Pending or unknown payment
- **WHEN** a confirmation request is pending or its commit outcome is unknown
- **THEN** only processing or recovery feedback appears and competing confirmation remains disabled
- **AND** a success treatment appears only after the existing durable recovery or confirmation establishes payment

### Requirement: Reduced motion across all surfaces
The website SHALL honor the active reduced-motion preference for CSS and scripted animations, including overlays rendered outside the page container. It SHALL suppress nonessential transform, layout, entrance, looping and scroll motion while preserving immediate state changes, focus, text and recovery controls.

#### Scenario: Runtime preference change
- **WHEN** reduced motion is enabled before loading or changed while the website or an overlay is open
- **THEN** nonessential motion stops or is omitted without requiring navigation
- **AND** gallery, menus, status changes and pending feedback remain fully understandable without animation

### Requirement: Lifecycle-safe animated surfaces
Private data SHALL disappear immediately on logout, access loss, account change, order/source change or page restoration requiring revalidation. Exit animations SHALL NOT retain obsolete private DOM. Animation lifecycle events SHALL NOT resubmit actions, re-open streams or record additional lot views.

#### Scenario: Identity changes during exit
- **WHEN** logout or account switching occurs during a private order, receipt or recommendation transition
- **THEN** the prior identity's content is removed immediately, including any overlay
- **AND** delayed responses or exit completion cannot restore it or submit a previous intent

### Requirement: Bounded motion cost
Motion SHALL reserve image/layout space, avoid continuous decorative animation and keep native document navigation. Resting pages SHALL NOT run decorative frame loops or animation-triggered network polling. Missing animation code or media SHALL leave essential content and actions usable.

#### Scenario: Slow device and missing imagery
- **WHEN** the website is used on a narrow or throttled device and a focal image fails
- **THEN** the local fallback preserves layout, text and controls remain available, and rendering does not depend on an animation completing
- **AND** native links, reload, Back/Forward and commerce request counts retain their existing behavior
