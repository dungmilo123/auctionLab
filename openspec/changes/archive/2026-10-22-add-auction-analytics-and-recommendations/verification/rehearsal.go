// Run only against TEST_DATABASE_URL. Creates a fresh marked owned demo schema.
package main

import (
	"auctionlab"
	"auctionlab/internal/account"
	"auctionlab/internal/auction"
	"auctionlab/internal/catalog"
	"auctionlab/internal/order"
	"auctionlab/internal/store"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgconn"
	"os"
	"time"
)

func must(e error) {
	if e != nil {
		var pe *pgconn.PgError
		if errors.As(e, &pe) {
			panic(pe.Code + ":" + pe.ConstraintName)
		}
		for errors.Unwrap(e) != nil {
			e = errors.Unwrap(e)
		}
		panic(e)
	}
}
func main() {
	ctx := context.Background()
	var b [8]byte
	_, e := rand.Read(b[:])
	must(e)
	schema := "auctionlab_demo_change8_" + hex.EncodeToString(b[:])
	reset := len(os.Args) > 1 && os.Args[1] == "--reset"
	audit := len(os.Args) > 1 && os.Args[1] == "--audit"
	if reset || audit {
		raw, e := os.ReadFile("/tmp/auctionlab-change8-rehearsal.json")
		must(e)
		var m map[string]string
		must(json.Unmarshal(raw, &m))
		schema = m["schema"]
		if !store.ValidDemoSchema(schema) {
			panic("invalid owned schema")
		}
	}
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		panic("TEST_DATABASE_URL required")
	}
	s, e := store.OpenConfig(store.Config{URL: raw, Schema: schema, ReadOnly: audit})
	must(e)
	defer s.Close()
	if audit {
		var bids, bidders, sold, unsold, winning, paid, coverage, observations string
		must(s.DB.QueryRow(`SELECT (SELECT count(*)::text FROM commerce_bids),(SELECT count(DISTINCT account_id)::text FROM commerce_bids),(SELECT count(*)::text FROM commerce_auction_results WHERE outcome='sold'),(SELECT count(*)::text FROM commerce_auction_results WHERE outcome='unsold'),(SELECT coalesce(sum(winning_amount_minor),0)::text FROM commerce_auction_results),(SELECT coalesce(sum(total_minor),0)::text FROM commerce_demo_payments),(SELECT collection_started_at::text FROM commerce_interaction_coverage),(SELECT count(*)::text FROM commerce_interactions WHERE kind='lot_view')`).Scan(&bids, &bidders, &sold, &unsold, &winning, &paid, &coverage, &observations))
		must(json.NewEncoder(os.Stdout).Encode(map[string]string{"bids": bids, "bidders": bidders, "sold": sold, "unsold": unsold, "winning_minor": winning, "simulated_paid_minor": paid, "coverage": coverage, "views": observations}))
		return
	}
	must(s.Migrate())
	now := time.Now()
	base := now.Add(-4 * time.Hour)
	password, e := account.Random(24)
	must(e)
	verifier, e := account.HashPassword(password)
	must(e)
	rows := []account.Record{}
	for _, id := range []string{"a", "b", "c"} {
		rows = append(rows, account.Record{Profile: account.Profile{ID: id, Email: id + "@rehearsal.test", DisplayName: "Rehearsal " + id, Role: "bidder", Demo: id == "a"}, Verifier: verifier, CreatedAt: base.UnixMilli()})
	}
	spa, e := auctionlab.SPA()
	must(e)
	generation, e := s.SeedDemo(ctx, reset, schema, base, rows, catalog.LocalMedia{Embedded: spa})
	must(e)
	if reset {
		var published, interactions, sessions int
		var start int64
		must(s.DB.QueryRow(`SELECT (SELECT count(*) FROM catalog_lots WHERE state='published'),(SELECT count(*) FROM commerce_interactions),(SELECT count(*) FROM commerce_sessions),(SELECT collection_started_at FROM commerce_interaction_coverage)`).Scan(&published, &interactions, &sessions, &start))
		if published != 0 || interactions != 0 || sessions != 0 || start != generation.PreparedAt {
			panic("reset scope mismatch")
		}
		raw, e := json.Marshal(map[string]any{"generation": generation.Generation, "published_lots": published, "interactions": interactions, "old_sessions": sessions, "collection_started_at": start})
		must(e)
		must(os.WriteFile("openspec/changes/add-auction-analytics-and-recommendations/verification/reset-drafts.json", raw, 0644))
	}
	sql := func(q string, args ...any) { _, e := s.DB.Exec(q, args...); must(e) }
	key, e := account.Random(32)
	must(e)
	sql(`INSERT INTO server_config VALUES('admin_key',$1) ON CONFLICT DO NOTHING`, key)
	must(s.DB.QueryRow(`SELECT value FROM server_config WHERE key='admin_key'`).Scan(&key))
	sql(`DELETE FROM catalog_event_lots;UPDATE catalog_lots SET demo=false,starting_price_minor=0,bid_increment_minor=500,fulfillment=jsonb_set(fulfillment,'{shipping_fee_minor}','1500');UPDATE catalog_events SET demo=false;`)
	sql(`UPDATE catalog_events SET state='published',opens_at=$1,first_closes_at=$2,close_interval_ms=3600000,demo=(id='demo-event-1') WHERE id IN ('demo-event-1','demo-event-2')`, base.UnixMilli(), now.Add(-150*time.Minute).UnixMilli())
	sql(`UPDATE catalog_lots SET state='published' WHERE id IN ('demo-lot-01','demo-lot-02','demo-lot-03','demo-lot-04','demo-lot-05');INSERT INTO catalog_event_lots VALUES('demo-event-1','demo-lot-01',0),('demo-event-1','demo-lot-02',1),('demo-event-1','demo-lot-03',2),('demo-event-1','demo-lot-04',3),('demo-event-2','demo-lot-05',0)`)
	sessionA, e := account.Random(32)
	must(e)
	sessionB, e := account.Random(32)
	must(e)
	tokens := map[string]string{"a": sessionA, "b": sessionB}
	sql(`INSERT INTO commerce_sessions VALUES($1,'a',$3,$4,NULL),($2,'b',$3,$4,NULL)`, account.Digest(tokens["a"]), account.Digest(tokens["b"]), base.UnixMilli(), now.Add(24*time.Hour).UnixMilli())
	a := store.AuctionRepository{Store: s}
	at := func(t time.Time) func() time.Time { return func() time.Time { return t } }
	for n, bid := range []struct {
		lot, actor string
		amount     int64
	}{{"demo-lot-01", "b", 5000}, {"demo-lot-01", "a", 10000}, {"demo-lot-02", "a", 10000}, {"demo-lot-02", "b", 20000}, {"demo-lot-05", "b", 1000}, {"demo-lot-05", "a", 5000}} {
		_, e = a.Place(ctx, account.Digest(tokens[bid.actor]), auction.Intent{LotID: bid.lot, AmountMinor: &bid.amount, RequestID: fmt.Sprintf("00000000-0000-0000-0000-%012d", n+1)}, at(now.Add(-3*time.Hour)))
		must(e)
	}
	_, e = a.Reconcile(ctx, at(now))
	must(e)
	r := store.OrderRepository{Store: s}
	wins, e := r.Wins(ctx, account.Digest(tokens["a"]), at(now))
	must(e)
	for n, w := range wins {
		i := order.Intent{Selection: order.Selection{Contact: order.Contact{Name: "Rehearsal", Email: "a@rehearsal.test", Phone: "000"}, FulfillmentMethod: "pickup", PaymentMethod: "demo-card"}, RequestID: fmt.Sprintf("10000000-0000-0000-0000-%012d", n+1)}
		if w.Snapshot.LotID == "demo-lot-01" {
			i.FulfillmentMethod = "shipping"
			i.ShippingAddress = &order.Address{Line1: "Fixture", Locality: "Fixture", Country: "VN"}
		}
		_, e = r.Confirm(ctx, account.Digest(tokens["a"]), w.ID, i, at(now))
		must(e)
	}
	yes := true
	_, e = (store.AccountRepository{Store: s}).Membership(ctx, account.Digest(tokens["a"]), "demo-lot-01", &yes, now.UnixMilli())
	must(e)
	manifest := map[string]any{"schema": schema, "password": password, "capability": key, "source": "folding-travel-camera", "pending": "pocket-point-and-shoot"}
	out, e := json.Marshal(manifest)
	must(e)
	must(os.WriteFile("/tmp/auctionlab-change8-rehearsal.json", out, 0600))
	fmt.Println(schema)
}
