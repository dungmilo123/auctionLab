# Winner checkout contract

All purchases are single-lot orders created from sold commerce results. Snapshot terms and provenance are copied explicitly; private catalog notes and other participants' identities are excluded. Reads never create orders. Publication creates neither orders nor payments.

The existing AccountAPI owns these no-store routes:

| Route | Request / result |
| --- | --- |
| GET /api/account/wins | Owner's awaiting-payment orders, result time descending then ID ascending; empty is a successful array |
| GET /api/account/orders/:id | Owner snapshot/options, or immutable stored paid selection and receipt |
| POST /api/account/orders/:id/review | Selection; stateless normalized review, or existing paid confirmation |
| POST /api/account/orders/:id/confirm | Selection plus lowercase UUID `request_id`; confirmed paid order, `replay`, `already_paid` |

There is no order-history collection, admin order API, draft update or deletion endpoint. Ownership comes only from a live commerce session. Commerce admin roles, capability cookies and game tokens grant no additional purchase access. Missing and foreign orders both return 404; invalid sessions return 401.

Mutations require JSON, `X-AuctionLab-Account: 1`, and the existing same-origin check. The body is bounded at 8 KiB, requires valid UTF-8 and rejects unknown fields, including financial credentials, amounts, fees, currency and owner/status fields. Responses use `{data: ...}` or `{error:{code,fields?}}`. Malformed JSON is 400, boundary rejection 403/415, field validation 422, accepted-request conflict 409, unavailable/unknown commit 503. Errors never echo raw inputs or database details.

Selection contains `contact: {name,email,phone}`, `fulfillment_method` (`shipping` or `pickup`), optional `shipping_address: {line1,line2,locality,region,postal_code,country}`, and `payment_method` (`demo-card` or `demo-bank-transfer`). Name/email/phone are required. Trim surrounding whitespace and reject controls. Limits in Unicode code points: name 120, email 254, phone 40, address lines 200 each, locality/region 120 each, postal code 32. Email is normalized lowercase and syntax checked; country is normalized uppercase, exactly two ASCII letters. Shipping requires line1/locality/country; other address fields are optional. Pickup discards the whole address. No profile update is implied.

Totals are integer USD cents: the winning amount and published flat shipping fee each allow 0–1,000,000,000,000, their sum allows 0–2,000,000,000,000. Pickup fee is zero; absent shipping fee is unavailable, never free. No tax, premium, discount or external quotation exists.

Confirmation takes the leased writer, locks the order, and revalidates the session after waiting. One transaction saves normalized contact/selection, one simulation receipt, exact amounts and paid time. A deterministic hash includes the order and normalized selection. Accepted account/request IDs replay only matching intents; changed payload/order conflicts. Validation failures reserve no ID. A fresh ID on a paid order returns its original confirmation with `already_paid`, without reserving the ID or updating the purchase.

Every payment is a local simulation, including non-demo lots. There is no external payment call, card/bank input, charge or credential storage. No checkout body/contact/address is logged. Success requires confirmed commit. Transport failure or `commit_unknown` means the outcome is unknown: authenticate and GET the same order, then either display the stored receipt or deliberately retry the same in-memory intent/ID. Never automatically confirm on login, reconnect or reload. Reload clears unsent personal data and reads durable state first.
