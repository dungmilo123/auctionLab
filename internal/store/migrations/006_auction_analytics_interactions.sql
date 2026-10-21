CREATE TABLE commerce_interactions (
 id TEXT COLLATE "C" PRIMARY KEY CHECK(id ~ '^[0-9a-f]{32}$'),
 kind TEXT NOT NULL CHECK(kind IN ('lot_view','watchlist_add','bid_accepted','payment_confirmed')),
 lot_id TEXT NOT NULL REFERENCES catalog_lots(id),
 event_id TEXT NOT NULL REFERENCES catalog_events(id),
 actor_id TEXT COLLATE "C" REFERENCES commerce_accounts(id),
 occurred_at BIGINT NOT NULL CHECK(occurred_at BETWEEN 0 AND 8640000000000000),
 recorded_at BIGINT NOT NULL CHECK(recorded_at BETWEEN 0 AND 8640000000000000),
 lot_demo BOOLEAN NOT NULL,
 event_demo BOOLEAN NOT NULL,
 actor_demo BOOLEAN,
 provenance TEXT NOT NULL CHECK(provenance IN ('observed','backfilled','snapshot')),
 source_key TEXT COLLATE "C" NOT NULL CHECK(char_length(source_key) BETWEEN 1 AND 120),
 UNIQUE(kind,source_key),
 CHECK((kind='lot_view' AND actor_id IS NULL AND actor_demo IS NULL AND provenance='observed' AND source_key ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$') OR
 (kind='watchlist_add' AND actor_id IS NOT NULL AND actor_demo IS NOT NULL AND provenance IN ('observed','snapshot')) OR
 (kind IN ('bid_accepted','payment_confirmed') AND actor_id IS NOT NULL AND actor_demo IS NOT NULL AND provenance IN ('observed','backfilled')))
);
CREATE INDEX commerce_interactions_event_kind_time ON commerce_interactions(event_id,kind,occurred_at);
CREATE INDEX commerce_interactions_actor_lot ON commerce_interactions(actor_id,lot_id);
CREATE TABLE commerce_interaction_coverage (
 id INTEGER PRIMARY KEY CHECK(id=1),
 collection_started_at BIGINT NOT NULL CHECK(collection_started_at BETWEEN 0 AND 8640000000000000)
);
INSERT INTO commerce_interactions
 SELECT md5('bid:'||b.id::text),'bid_accepted',b.lot_id,x.event_id,b.account_id,b.accepted_at,(extract(epoch FROM transaction_timestamp())*1000)::bigint,l.demo,e.demo,a.demo,'backfilled',b.id::text
 FROM commerce_bids b JOIN commerce_lot_auctions x ON x.lot_id=b.lot_id JOIN catalog_lots l ON l.id=b.lot_id JOIN catalog_events e ON e.id=x.event_id JOIN commerce_accounts a ON a.id=b.account_id;
INSERT INTO commerce_interactions
 SELECT md5('payment:'||p.id),'payment_confirmed',o.lot_id,x.event_id,p.owner_id,p.paid_at,(extract(epoch FROM transaction_timestamp())*1000)::bigint,l.demo,e.demo,a.demo,'backfilled',p.id
 FROM commerce_demo_payments p JOIN commerce_orders o ON o.id=p.order_id JOIN commerce_lot_auctions x ON x.lot_id=o.lot_id JOIN catalog_lots l ON l.id=o.lot_id JOIN catalog_events e ON e.id=x.event_id JOIN commerce_accounts a ON a.id=p.owner_id;
INSERT INTO commerce_interactions
 SELECT md5('watch:'||w.account_id||':'||w.lot_id),'watchlist_add',w.lot_id,m.event_id,w.account_id,w.saved_at,(extract(epoch FROM transaction_timestamp())*1000)::bigint,l.demo,e.demo,a.demo,'snapshot',md5(w.account_id||':'||w.lot_id)
 FROM commerce_watchlist w JOIN catalog_event_lots m ON m.lot_id=w.lot_id JOIN catalog_lots l ON l.id=w.lot_id JOIN catalog_events e ON e.id=m.event_id JOIN commerce_accounts a ON a.id=w.account_id;
INSERT INTO commerce_interaction_coverage
 SELECT 1,(extract(epoch FROM transaction_timestamp())*1000)::bigint
 WHERE EXISTS(SELECT 1 FROM commerce_accounts) OR EXISTS(SELECT 1 FROM catalog_lots) OR EXISTS(SELECT 1 FROM catalog_events) OR EXISTS(SELECT 1 FROM demo_environments);
