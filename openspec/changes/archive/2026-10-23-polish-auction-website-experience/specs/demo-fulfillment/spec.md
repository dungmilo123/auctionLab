# Spec Delta

## MODIFIED Requirements

### Requirement: Manual simulated shipment information
The shipping transition to `shipped` SHALL require carrier and tracking-reference text, trimmed and control-free, limited to 120 and 200 Unicode code points respectively. These values SHALL commit with shipment, remain immutable and display as text to the owner/operator using ordinary shipment labels without required simulated/manual badges. Pickup SHALL reject shipping fields. No external tracking request or automatic delivery claim SHALL occur.

#### Scenario: Record shipment
- **WHEN** an operator submits valid carrier/reference while advancing preparing to shipped
- **THEN** the values appear with the committed shipment event and natural shipment labels without a simulation disclaimer
- **AND** text resembling markup or a URL is safely rendered without executable markup or an automatic external lookup

#### Scenario: Invalid shipping data
- **WHEN** shipment lacks a required value, exceeds a limit, contains controls, or shipping fields accompany another transition
- **THEN** field validation fails without changing state or timeline
- **AND** valid entered values remain available for correction while access remains valid

### Requirement: Accessible operator fulfillment controls
The workspace SHALL provide EN/VI, labeled keyboard controls, explicit pending/conflict/unknown/error states and usable narrow/desktop/actual-200%-zoom layouts with reduced motion. Navigation, access loss and page restoration SHALL clear sensitive cached detail and suppress late responses. Language changes SHALL retain authorized unsaved shipping text. The workspace SHALL group filters, summaries, selected-order details, timeline and next valid action with coherent iconography, natural action labels and visible confirmation text.

#### Scenario: Recover without private stale data
- **WHEN** capability access expires during a read or update, or the operator changes order selection while a response is pending
- **THEN** the workspace clears obsolete contact/detail immediately and shows access-required or the current selected-order recovery state
- **AND** returning through the Go gate never replays the previous transition automatically

#### Scenario: Fulfillment review and confirmation
- **WHEN** an operator reviews shipping values or advances an order
- **THEN** the current selected order, immutable purchase terms and next permitted transition remain clear, with pending controls preventing competing submissions
- **AND** labels omit simulation qualifiers while version checks, durable reload after conflict/unknown and one-step progression remain unchanged
