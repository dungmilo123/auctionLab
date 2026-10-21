ALTER TABLE commerce_auction_results ADD CONSTRAINT commerce_sold_result_key UNIQUE(lot_id,winning_bid_id,winning_account_id,winning_amount_minor);
CREATE TABLE commerce_orders (
 id TEXT COLLATE "C" PRIMARY KEY CHECK(id ~ '^[0-9a-f]{32}$'),
 lot_id TEXT NOT NULL UNIQUE,
 winning_bid_id BIGINT NOT NULL,
 owner_id TEXT COLLATE "C" NOT NULL,
 winning_amount_minor BIGINT NOT NULL CHECK(winning_amount_minor BETWEEN 0 AND 1000000000000),
 currency TEXT NOT NULL CHECK(currency='USD'),
 snapshot JSONB NOT NULL CHECK(jsonb_typeof(snapshot)='object'),
 closes_at BIGINT NOT NULL CHECK(closes_at BETWEEN 0 AND 8640000000000000),
 recorded_at BIGINT NOT NULL CHECK(recorded_at BETWEEN closes_at AND 8640000000000000),
 created_at BIGINT NOT NULL CHECK(created_at BETWEEN recorded_at AND 8640000000000000),
 state TEXT NOT NULL DEFAULT 'awaiting_payment' CHECK(state IN ('awaiting_payment','paid')),
 selection JSONB,
 fulfillment_fee_minor BIGINT CHECK(fulfillment_fee_minor BETWEEN 0 AND 1000000000000),
 total_minor BIGINT CHECK(total_minor BETWEEN 0 AND 2000000000000),
 paid_at BIGINT,
 UNIQUE(id,owner_id),
 FOREIGN KEY(lot_id,winning_bid_id,owner_id,winning_amount_minor) REFERENCES commerce_auction_results(lot_id,winning_bid_id,winning_account_id,winning_amount_minor),
 CHECK((state='awaiting_payment' AND selection IS NULL AND fulfillment_fee_minor IS NULL AND total_minor IS NULL AND paid_at IS NULL) OR
       (state='paid' AND selection IS NOT NULL AND jsonb_typeof(selection)='object' AND fulfillment_fee_minor IS NOT NULL AND total_minor IS NOT NULL AND total_minor=winning_amount_minor+fulfillment_fee_minor AND paid_at IS NOT NULL AND paid_at BETWEEN created_at AND 8640000000000000))
);
CREATE INDEX commerce_orders_unpaid ON commerce_orders(owner_id,recorded_at DESC,id) WHERE state='awaiting_payment';
CREATE TABLE commerce_demo_payments (
 id TEXT COLLATE "C" PRIMARY KEY CHECK(id ~ '^[0-9a-f]{32}$'),
 order_id TEXT COLLATE "C" NOT NULL UNIQUE,
 owner_id TEXT COLLATE "C" NOT NULL,
 request_id TEXT COLLATE "C" NOT NULL CHECK(request_id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'),
 intent_digest TEXT NOT NULL CHECK(intent_digest ~ '^[0-9a-f]{64}$'),
 payment_method TEXT NOT NULL CHECK(payment_method IN ('demo-card','demo-bank-transfer')),
 currency TEXT NOT NULL CHECK(currency='USD'),
 total_minor BIGINT NOT NULL CHECK(total_minor BETWEEN 0 AND 2000000000000),
 paid_at BIGINT NOT NULL CHECK(paid_at BETWEEN 0 AND 8640000000000000),
 simulated BOOLEAN NOT NULL CHECK(simulated),
 UNIQUE(owner_id,request_id),
 FOREIGN KEY(order_id,owner_id) REFERENCES commerce_orders(id,owner_id)
);
