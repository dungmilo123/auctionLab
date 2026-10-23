# Spec Delta

## MODIFIED Requirements

### Requirement: Local catalog media
Catalog image references SHALL use application-origin paths under `/images/` or `/site-assets/catalog/`, without traversal, query strings, fragments, or external URLs. Publication SHALL require readable image files and nonempty alternative text for each referenced image, including the event cover. Seed media SHALL be bundled and have recorded source/license and authored/generated status. Bundled catalog media SHALL omit visible demo/preview lettering. Missing images after publication SHALL retain an accessible local fallback.

#### Scenario: Local gallery and cover
- **WHEN** an operator publishes an event whose cover and lot gallery paths resolve to local supported image files
- **THEN** the media load from the application origin with external internet unavailable
- **AND** gallery order and alternative text are retained

#### Scenario: Unsafe or unavailable media
- **WHEN** an image reference is external, escapes an allowed namespace, or names a missing file at publication
- **THEN** the invalid reference is rejected or publication reports the missing image
- **AND** the server does not fetch remote media or publish a partially valid event

#### Scenario: Presentation asset upgrade
- **WHEN** bundled catalog art is upgraded for the polished website
- **THEN** existing application-origin references continue to resolve, visible art has no demo/illustration banner and the object identity matches its catalog content
- **AND** asset manifests retain source, rights and authored/generated status without rewriting published records or purchase snapshots

### Requirement: Explicit repeat-safe demo content
An explicit demo seed operation SHALL create 18 provenance-marked demonstration lots across three events named Analog Memories, Objects of Time, and The Listening Room. Provenance SHALL remain stored metadata, not mandatory storefront labeling. It SHALL use all five categories, local media, and valid content/pricing/fulfillment. It SHALL establish dates once from a supplied reference time, remain repeat-safe without overwriting edits, and never run on ordinary startup.

#### Scenario: Seed a demo database
- **WHEN** the seed operation runs against an empty catalog with a reference time
- **THEN** it creates three complete draft events with six distinct lots each and persists their schedules and provenance flags
- **AND** an operator can review and publish an event using the normal validation flow without requiring a buyer-facing demo badge

#### Scenario: Repeat seed or restart
- **WHEN** an operator edits a seed record, reruns the seed operation, or restarts the server
- **THEN** rerunning reports the seed already applied and creates no duplicates
- **AND** neither seed replay nor startup overwrites edits, changes publication state, or rebases stored dates

#### Scenario: Seed failure or identity collision
- **WHEN** seeding fails or a non-seed record already occupies an intended seed identity/slug
- **THEN** seeding leaves no partially created dataset and reports the problem
- **AND** existing user content is preserved

## ADDED Requirements

### Requirement: Bounded legacy seed presentation
Known project seed boilerplate SHALL have a presentation-only product-language equivalent when provenance, media identity and exact original field value match. Equivalent wording SHALL preserve unknown condition/provenance and fulfillment meaning. Custom/authored fields, prices, schedules and stored purchase terms SHALL remain unchanged. New seed content SHALL use the same natural wording while retaining provenance metadata.

#### Scenario: Existing seed and owner snapshot
- **WHEN** a recognized historical seed lot or its owned purchase snapshot contains the original stock description, condition, image alt or delivery instructions
- **THEN** the interface presents its defined neutral equivalent without demonstration wording or an invented history, assessment or collection address
- **AND** raw API/storage values, fees, eligibility and the immutable purchase snapshot remain unchanged

#### Scenario: Edited or unrelated content
- **WHEN** a seeded field was edited, provenance is absent, media identity is unrecognized or an ordinary listing happens to contain a matching word
- **THEN** the authored field is rendered as supplied without generic search-and-replace or invented translation
- **AND** seeding again does not overwrite that content or create duplicates
