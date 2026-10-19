# Spec Delta

## ADDED Requirements

### Requirement: Public catalog response time
Successful public category, event and lot list/detail responses SHALL include `meta.server_time` as integer UTC Unix milliseconds sampled from the server clock for that response. Existing `data` fields, published-only visibility, error envelopes and `Cache-Control: no-store` SHALL remain compatible. Response time SHALL be metadata, separate from immutable editorial records and auction execution state.

#### Scenario: Anonymous list or detail response
- **WHEN** an anonymous client successfully requests any public category, event or lot list/detail endpoint
- **THEN** it receives `{data: ..., meta: {server_time: ...}}` with a valid server instant and the existing public data shape
- **AND** clients that read only `data` continue to work without admin credentials

#### Scenario: Time changes without record mutation
- **WHEN** the same published record is read at two different server instants
- **THEN** its response metadata reflects the respective instants while its stored content, editorial state, prices and planned dates remain unchanged
- **AND** no current bid, winner, execution status or game revision is created or changed by the reads

#### Scenario: Existing privacy and error policy
- **WHEN** public catalog reads encounter a missing/draft slug, storage failure, or records containing private notes
- **THEN** draft and missing slugs retain identical not-found responses, failures retain sanitized error envelopes, and public data exclude private fields and game secrets
- **AND** response time does not authorize private reads or alter the existing admin capability checks
