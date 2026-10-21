# Winner checkout contract

All purchases are single-lot orders created from sold commerce results. Snapshot terms and provenance are copied explicitly; private catalog notes and other participants' identities are excluded. Reads never create orders. Publication creates neither orders nor payments.

The existing AccountAPI owns these no-store routes:

| Route | Request / result |
| --- | --- |
| GET /api/account/wins | Owner's awaiting-payment orders, result time descending then ID ascending; empty is a successful array |
| GET /api/account/orders/:id | Owner snapshot/options, or immutable stored paid selection and receipt |
| POST /api/account/orders/:id/review | Selection; stateless normalized review, or existing paid confirmation |
| POST /api/account/orders/:id/confirm | Selection plus lowercase UUID `request_id`; confirmed paid order, `replay`, `already_paid` |

History and dedicated admin fulfillment APIs are described below; arbitrary draft updates/deletion are unavailable. Account namespace ownership comes only from a live commerce session. Commerce admin roles, capability cookies and game tokens grant no additional access in that namespace. Missing and foreign orders both return 404; invalid sessions return 401.

Mutations require JSON, `X-AuctionLab-Account: 1`, and the existing same-origin check. The body is bounded at 8 KiB, requires valid UTF-8 and rejects unknown fields, including financial credentials, amounts, fees, currency and owner/status fields. Responses use `{data: ...}` or `{error:{code,fields?}}`. Malformed JSON is 400, boundary rejection 403/415, field validation 422, accepted-request conflict 409, unavailable/unknown commit 503. Errors never echo raw inputs or database details.

Selection contains `contact: {name,email,phone}`, `fulfillment_method` (`shipping` or `pickup`), optional `shipping_address: {line1,line2,locality,region,postal_code,country}`, and `payment_method` (`demo-card` or `demo-bank-transfer`). Name/email/phone are required. Trim surrounding whitespace and reject controls. Limits in Unicode code points: name 120, email 254, phone 40, address lines 200 each, locality/region 120 each, postal code 32. Email is normalized lowercase and syntax checked; country is normalized uppercase, exactly two ASCII letters. Shipping requires line1/locality/country; other address fields are optional. Pickup discards the whole address. No profile update is implied.

Totals are integer USD cents: the winning amount and published flat shipping fee each allow 0–1,000,000,000,000, their sum allows 0–2,000,000,000,000. Pickup fee is zero; absent shipping fee is unavailable, never free. No tax, premium, discount or external quotation exists.

Confirmation takes the leased writer, locks the order, and revalidates the session after waiting. One transaction saves normalized contact/selection, one simulation receipt, exact amounts and paid time. A deterministic hash includes the order and normalized selection. Accepted account/request IDs replay only matching intents; changed payload/order conflicts. Validation failures reserve no ID. A fresh ID on a paid order returns its original confirmation with `already_paid`, without reserving the ID or updating the purchase.

Every payment is a local simulation, including non-demo lots. There is no external payment call, card/bank input, charge or credential storage. No checkout body/contact/address is logged. Success requires confirmed commit. Transport failure or `commit_unknown` means the outcome is unknown: authenticate and GET the same order, then either display the stored receipt or deliberately retry the same in-memory intent/ID. Never automatically confirm on login, reconnect or reload. Reload clears unsent personal data and reads durable state first.

## History and simulated fulfillment (schema 5)

Payment `state` remains `awaiting_payment|paid` permanently. Separate `fulfillment` contains `status`, positive paid `version`, `updated_at`, nullable `{carrier,tracking_reference}` shipment and ordered `timeline`. Unpaid version 0 derives its sole system creation event from `created_at`; paid version 1 stores the payment event at the original receipt time. Original auction close/result-recording times remain separate, including historical adoption. Each operator action appends exactly one event. Shared capability actions identify an operator, never a named human.

Shipping is paid → preparing → shipped → completed (versions 1–4); pickup is paid → ready_for_pickup → completed (1–3). Unpaid and completed orders have no operator action. Shipment is required only for preparing → shipped: trim Unicode text, reject controls, require 1–120 carrier and 1–200 reference code points. It stays immutable afterward and is rendered as text without external tracking. There is no pay-on-behalf, method/contact/financial editing or cancellation endpoint.

Every detail read validates the stored receipt/progress/events in one repeatable-read snapshot. Missing or inconsistent required progress is unavailable, never synthesized as an earlier stage. Payment confirmation initializes progress/event in its receipt transaction; failed initialization rolls back contact, payment and state. Paid confirmation replays/fresh IDs preserve later progress.

`GET /api/account/orders` uses the current session and accepts no query parameters. Summaries include IDs/owner binding, snapshot title/image/slug/event/demo flags, USD winning amount, original close/result/creation times, payment state, progress status/version, selected method and paid total (both null while unpaid). They omit contacts/addresses and receipt IDs/details. Order is recorded_at descending, ID ascending with C ordering. Empty is a successful empty array; failures are service errors.

Dedicated `GET /api/admin/orders?status=all&method=all`, `GET /api/admin/orders/:id`, and `POST /api/admin/orders/:id/transition` require the capability, independently of commerce roles/sessions. Account routes remain strictly owner-only. Detail is the same bounded purchase projection under the separate authorization boundary. Neither capability nor commerce admin role impersonates an owner.

POST accepts only `{"expected_version":1,"next_status":"preparing"}` or shipment on the shipped step. It requires `Content-Type: application/json`, `X-AuctionLab-Admin: 1`, matching supplied Origin, and at most 8 KiB of strict object JSON. All responses/errors are no-store. Status filters: all, awaiting_payment, paid, preparing, shipped, ready_for_pickup, completed. Method filters: all, shipping, pickup, unselected. They combine with AND; unknown/duplicate/empty-invalid filters return 422. `view` belongs only to the document URL `/admin/orders?view=<id>`.

Errors: 400 malformed/unsupported body, 401 access, 403 request boundary, 404 missing/unsupported path, 409 stale or invalid progression, 415 non-JSON, 422 field/query validation, 503 unavailable/unknown commit. Errors never echo submitted values. Lost responses/commit_unknown require a durable detail reload before another deliberate versioned action; no automatic transition replay. An uncertain reload remains unknown. Transactions serialize through the leased writer, clamp event time to its predecessor, and never reacquire lost ownership.
