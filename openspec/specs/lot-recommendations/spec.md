# lot-recommendations Specification

## Purpose

Help visitors discover eligible published lots through explainable rules and private owner-watchlist preferences, with a deterministic fallback for new visitors.

## Requirements

### Requirement: Eligible recommendation candidates
Recommendations SHALL return up to six distinct lots whose lot and event are published, whose planned close is in the future at response time, and which have no committed terminal result. A contextual source lot and, for personalized requests, all currently saved owner lots SHALL be excluded. Empty eligible catalogs SHALL return a successful empty array.

#### Scenario: Excluded and future candidates
- **WHEN** candidates include drafts, past-close lots, sold/unsold lots, the source lot, saved lots and upcoming published lots
- **THEN** only eligible unsaved future-close published lots appear in owner recommendations
- **AND** source inspection can still be based on a published historical lot; a missing/draft source receives identical 404

### Requirement: Explainable deterministic similarity
Contextual recommendations SHALL rank by descending score: four points for the same category and two for the same nonempty era. Era comparison SHALL trim surrounding whitespace and lowercase text without inferring dates or overlapping periods. Ties SHALL use planned close ascending then slug ascending. Zero-score candidates SHALL provide the public fallback after matching candidates.

#### Scenario: Related and fallback order
- **WHEN** source and candidate share category and era, category only, era only, or neither
- **THEN** their scores are six, four, two and zero respectively before deterministic tie breaks
- **AND** reasons distinguish category-and-era, category, era and closing-soon fallback without claiming ML or authenticity

### Requirement: Owner-watchlist preference rules
Personalized recommendations SHALL use only the current session owner's saved published lots. Category and nonempty normalized-era membership in that set SHALL contribute four and two points per candidate, each at most once, rather than increasing with duplicate preferences. A contextual source SHALL take ranking precedence; owner preferences SHALL rank candidates only without a source.

#### Scenario: Preferences and cold start
- **WHEN** an authenticated owner's watchlist has two cameras from the same era
- **THEN** an unsaved candidate camera from that era scores six, rather than twelve
- **AND** an empty usable watchlist falls back to planned close then slug without consulting other users or requiring interaction history

#### Scenario: Context precedence
- **WHEN** an owner with camera preferences requests suggestions from a published clock source
- **THEN** clock-source similarity determines scores and owner-saved candidates remain excluded
- **AND** the response identifies contextual rules rather than falsely describing them as watchlist matches

### Requirement: Separate public and private recommendation APIs
`GET /api/catalog/recommendations` SHALL provide anonymous contextual or fallback suggestions without consuming account credentials. `GET /api/account/recommendations` SHALL require a live owner session. Both SHALL accept only optional `source=<published-lot-slug>`, return public lot projections plus reason codes and server time, and use no-store. Unknown, duplicate or empty query parameters SHALL return 422.

#### Scenario: No cross-account disclosure
- **WHEN** an anonymous visitor, another account or a catalog-capability holder calls recommendations
- **THEN** public responses depend only on public context and time, and private requests require the caller's own live session
- **AND** no response reveals source watchlist IDs, actor identifiers, private scores/history, contact data, draft content or game secrets

#### Scenario: Session change during loading
- **WHEN** logout, expiry, account switch or a newer source request occurs before a private response arrives
- **THEN** obsolete personalized state is cleared and the old response cannot render for the new identity/source
- **AND** unavailable account validation shows its recovery state; it is never treated as a confirmed signed-out session

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
