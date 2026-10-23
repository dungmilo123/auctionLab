# Spec Delta

## MODIFIED Requirements

### Requirement: Recoverable bilingual recommendation sections
Home and lot detail SHALL present recommendations as independently loaded EN/VI sections using shared refined lot cards, concise product-language reasons and native lot links, without demo markers or rule-engine/future-ML announcements. New visitors SHALL get public fallback. Failures SHALL provide explicit retry and preserve the main catalog and bid/checkout controls. Authenticated users SHALL use private suggestions only after current-session validation.

#### Scenario: Failure and accessibility
- **WHEN** recommendations fail or the visitor browses by keyboard, at 320/375 CSS pixels, actual 200 percent browser zoom, or with reduced motion active
- **THEN** published content stays usable and the suggestion section remains readable with distinct loading, empty and error states
- **AND** changing language changes interface/reason copy without changing authored lot content or submitting a bid/payment

#### Scenario: Rule-based disclosure
- **WHEN** a visitor follows a suggested lot
- **THEN** its stored public URL resolves the selected published record and the section gives a truthful concise reason such as shared category, era or upcoming close
- **AND** the interface does not describe implementation algorithms, announce future work or claim a trained model, proven personalization lift or ML

#### Scenario: Personalized and public presentation
- **WHEN** owner preferences are not validated or private recommendations fail
- **THEN** public suggestions remain identifiable with neutral discovery copy and account recovery stays separate
- **AND** exit motion removes obsolete owner suggestions immediately and public cards are not falsely described as personal matches
