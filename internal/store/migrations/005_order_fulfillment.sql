ALTER TABLE commerce_demo_payments ADD CONSTRAINT commerce_payment_order_key UNIQUE(order_id,id);
CREATE TABLE commerce_order_fulfillment (
 order_id TEXT COLLATE "C" PRIMARY KEY REFERENCES commerce_orders(id),
 receipt_id TEXT COLLATE "C" NOT NULL UNIQUE,
 method TEXT NOT NULL CHECK(method IN ('shipping','pickup')),
 status TEXT NOT NULL,
 version BIGINT NOT NULL CHECK(version BETWEEN 1 AND 4),
 updated_at BIGINT NOT NULL CHECK(updated_at BETWEEN 0 AND 8640000000000000),
 carrier TEXT,
 tracking_reference TEXT,
 FOREIGN KEY(order_id,receipt_id) REFERENCES commerce_demo_payments(order_id,id),
 CHECK((method='shipping' AND ((status='paid' AND version=1) OR (status='preparing' AND version=2) OR (status='shipped' AND version=3) OR (status='completed' AND version=4))) OR
       (method='pickup' AND ((status='paid' AND version=1) OR (status='ready_for_pickup' AND version=2) OR (status='completed' AND version=3)))),
 CHECK((method='shipping' AND status IN ('shipped','completed') AND carrier IS NOT NULL AND tracking_reference IS NOT NULL AND char_length(carrier) BETWEEN 1 AND 120 AND char_length(tracking_reference) BETWEEN 1 AND 200 AND carrier=btrim(carrier) AND tracking_reference=btrim(tracking_reference) AND carrier !~ '[[:cntrl:]]' AND tracking_reference !~ '[[:cntrl:]]') OR
       (status NOT IN ('shipped') AND NOT(method='shipping' AND status='completed') AND carrier IS NULL AND tracking_reference IS NULL))
);
CREATE TABLE commerce_order_fulfillment_events (
 order_id TEXT COLLATE "C" NOT NULL REFERENCES commerce_order_fulfillment(order_id),
 version BIGINT NOT NULL CHECK(version BETWEEN 1 AND 4),
 from_status TEXT NOT NULL,
 to_status TEXT NOT NULL,
 occurred_at BIGINT NOT NULL CHECK(occurred_at BETWEEN 0 AND 8640000000000000),
 source TEXT NOT NULL CHECK(source IN ('payment','operator')),
 carrier TEXT,
 tracking_reference TEXT,
 PRIMARY KEY(order_id,version),
 CHECK((version=1 AND from_status='awaiting_payment' AND to_status='paid' AND source='payment') OR
       (version=2 AND from_status='paid' AND to_status IN ('preparing','ready_for_pickup') AND source='operator') OR
       (version=3 AND ((from_status='preparing' AND to_status='shipped') OR (from_status='ready_for_pickup' AND to_status='completed')) AND source='operator') OR
       (version=4 AND from_status='shipped' AND to_status='completed' AND source='operator')),
 CHECK((to_status='shipped' AND carrier IS NOT NULL AND tracking_reference IS NOT NULL AND char_length(carrier) BETWEEN 1 AND 120 AND char_length(tracking_reference) BETWEEN 1 AND 200 AND carrier=btrim(carrier) AND tracking_reference=btrim(tracking_reference) AND carrier !~ '[[:cntrl:]]' AND tracking_reference !~ '[[:cntrl:]]') OR (to_status<>'shipped' AND carrier IS NULL AND tracking_reference IS NULL))
);
CREATE INDEX commerce_orders_history ON commerce_orders(owner_id,recorded_at DESC,id);
CREATE INDEX commerce_orders_recorded ON commerce_orders(recorded_at DESC,id);
CREATE INDEX commerce_fulfillment_filter ON commerce_order_fulfillment(status,method,order_id);
CREATE TABLE demo_environments (
 id INTEGER PRIMARY KEY CHECK(id=1),
 schema_name TEXT NOT NULL CHECK(schema_name ~ '^auctionlab_demo_[a-z0-9_]+$'),
 format_version INTEGER NOT NULL CHECK(format_version=1),
 generation TEXT NOT NULL CHECK(generation ~ '^[0-9a-f]{32}$'),
 seed_reference BIGINT NOT NULL CHECK(seed_reference BETWEEN 0 AND 8639999654400000),
 prepared_at BIGINT NOT NULL CHECK(prepared_at BETWEEN 0 AND 8640000000000000)
);
-- Fail the entire upgrade on inconsistent historical purchases, never omit them.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM commerce_orders o LEFT JOIN commerce_demo_payments p ON p.order_id=o.id WHERE
 (o.state='paid' AND (p.id IS NULL OR p.owner_id<>o.owner_id OR p.total_minor<>o.total_minor OR p.paid_at<>o.paid_at OR p.payment_method IS DISTINCT FROM o.selection->>'payment_method' OR o.selection->>'fulfillment_method' IS NULL OR o.selection->>'fulfillment_method' NOT IN ('shipping','pickup') OR
 o.fulfillment_fee_minor IS DISTINCT FROM CASE WHEN o.selection->>'fulfillment_method'='pickup' THEN 0 ELSE (o.snapshot->'fulfillment'->>'shipping_fee_minor')::bigint END)) OR (o.state='awaiting_payment' AND p.id IS NOT NULL)) THEN
 RAISE EXCEPTION 'inconsistent historical purchase';
 END IF;
END $$;
INSERT INTO commerce_order_fulfillment(order_id,receipt_id,method,status,version,updated_at)
 SELECT o.id,p.id,o.selection->>'fulfillment_method','paid',1,p.paid_at FROM commerce_orders o JOIN commerce_demo_payments p ON p.order_id=o.id WHERE o.state='paid';
INSERT INTO commerce_order_fulfillment_events(order_id,version,from_status,to_status,occurred_at,source)
 SELECT order_id,1,'awaiting_payment','paid',updated_at,'payment' FROM commerce_order_fulfillment;
