package store

import (
	"auctionlab/internal/analytics"
	"context"
	"strconv"
	"testing"
)

func analyticsFixture(t *testing.T) *Store {
	s, a := auctionFixture(t)
	ctx := context.Background()
	mustSQL(t, s, `UPDATE catalog_lots SET state='published' WHERE id='draft';INSERT INTO catalog_lots(id,slug,title,state,starting_price_minor,bid_increment_minor,condition,fulfillment,created_at,updated_at) SELECT 'pending','pending','Pending','published',0,5,condition,fulfillment,0,0 FROM catalog_lots WHERE id='l';INSERT INTO catalog_event_lots VALUES('e','pending',2);UPDATE catalog_events SET first_closes_at=200,close_interval_ms=50 WHERE id='overlap';UPDATE catalog_lots SET starting_price_minor=0 WHERE id='other';UPDATE catalog_events SET demo=true WHERE id='e';UPDATE commerce_accounts SET demo=true WHERE id='a';DELETE FROM catalog_event_lots WHERE event_id='past';UPDATE catalog_events SET state='draft' WHERE id='past'`)
	for i, b := range []struct {
		lot, actor string
		amount     int64
	}{{"l", "b", 5000}, {"l", "a", 10000}, {"draft", "a", 10000}, {"draft", "b", 20000}, {"other", "b", 1000}, {"other", "a", 5000}} {
		if _, e := a.Place(ctx, b.actor, bid(b.lot, b.amount, i+1), at(100)); e != nil {
			t.Fatal(e)
		}
	}
	// The fourth terminal lot is a no-bid published slot. Pending remains future.
	mustSQL(t, s, `UPDATE catalog_event_lots SET position=3 WHERE lot_id='pending';INSERT INTO catalog_event_lots VALUES('e','expired',2)`)
	// Positions must be unique, so move pending first in a single validated update.
	reconcile(t, a, 300)
	o := OrderRepository{s}
	wins, e := o.Wins(ctx, "a", at(301))
	if e != nil || len(wins) != 2 {
		t.Fatal(wins, e)
	}
	for i, w := range wins {
		intent := checkoutIntent(i + 1)
		if w.Snapshot.LotID == "other" {
			intent.FulfillmentMethod = "pickup"
			intent.ShippingAddress = nil
		}
		if _, e = o.Confirm(ctx, "a", w.ID, intent, at(400)); e != nil {
			t.Fatal(e)
		}
	}
	return s
}
func TestAnalyticsReferenceAndFilters(t *testing.T) {
	s := analyticsFixture(t)
	r := AnalyticsRepository{s}
	ctx := context.Background()
	out, e := r.Report(ctx, analytics.Filters{Event: "all", Dataset: "all"})
	if e != nil {
		t.Fatal(e)
	}
	m := out.Data.Summary
	if m.Lots != "5" || m.Bids != "6" || m.Bidders != "2" || m.Sold != "3" || m.Unsold != "1" || m.Pending != "1" || m.Terminal != "4" || m.Winning != "35000" || m.PaidWinning != "15000" || m.PaidFees != "1500" || m.PaidTotal != "16500" || m.PaidOrders != "2" || m.Unpaid != "1" || m.SellThrough.Sold != "3" || m.SellThrough.Terminal != "4" {
		t.Fatalf("reference: %+v", m)
	}
	if len(out.Data.Events) != 2 || out.Data.Events[0].Slug != "event" || out.Data.Events[0].Metrics.Bidders != "2" || out.Data.Events[1].Metrics.Bidders != "2" {
		t.Fatal(out.Data.Events)
	}
	// Independent direct SQL over this fixture, without the report cohort/query.
	var bids, bidders, winning, paid string
	e = s.DB.QueryRow(`SELECT (SELECT count(*)::text FROM commerce_bids),(SELECT count(DISTINCT account_id)::text FROM commerce_bids),(SELECT sum(winning_amount_minor)::text FROM commerce_auction_results),(SELECT sum(total_minor)::text FROM commerce_demo_payments)`).Scan(&bids, &bidders, &winning, &paid)
	if e != nil || bids != m.Bids || bidders != m.Bidders || winning != m.Winning || paid != m.PaidTotal {
		t.Fatal("independent SQL", e)
	}
	for _, dataset := range []string{"demo", "non-demo"} {
		v, e := r.Report(ctx, analytics.Filters{Event: "all", Dataset: dataset})
		if e != nil {
			t.Fatal(e)
		}
		if len(v.Data.Events) != 2 {
			t.Fatal("zero-cohort event omitted")
		}
		if dataset == "non-demo" && (v.Data.Summary.Lots != "1" || v.Data.Summary.DemoBids != "1" || v.Data.Summary.OrdinaryBids != "1") {
			t.Fatal(v.Data.Summary)
		}
	}
	for _, event := range []string{"past", "missing"} {
		if _, e := r.Report(ctx, analytics.Filters{Event: event, Dataset: "all"}); e == nil {
			t.Fatal("draft/missing")
		}
	}
	mustSQL(t, s, `UPDATE commerce_demo_payments SET total_minor=total_minor+1 WHERE order_id=(SELECT id FROM commerce_orders WHERE lot_id='l')`)
	if _, e := r.Report(ctx, analytics.Filters{Event: "all", Dataset: "all"}); e == nil {
		t.Fatal("corruption silently omitted")
	}
}
func TestAnalyticsZeroAndUninitialized(t *testing.T) {
	s, _ := auctionFixture(t)
	out, e := (AnalyticsRepository{s}).Report(context.Background(), analytics.Filters{Event: "all", Dataset: "all"})
	if e != nil {
		t.Fatal(e)
	}
	m := out.Data.Summary
	if m.Lots != "3" || m.Pending != "3" || m.SellThrough != nil || m.Bids != "0" || m.PaidTotal != "0" {
		t.Fatal(m)
	}
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM commerce_lot_auctions`).Scan(&n)
	if n != 0 {
		t.Fatal("read initialized auctions")
	}
}

func TestAnalyticsExactLargeSum(t *testing.T) {
	s, _ := auctionFixture(t)
	mustSQL(t, s, `INSERT INTO catalog_lots(id,slug,title,state,starting_price_minor,bid_increment_minor,created_at,updated_at) SELECT 'large-'||n,'large-'||n,'Large','published',1000000000000,1,0,0 FROM generate_series(1,10000)n;
 INSERT INTO catalog_event_lots SELECT 'overlap','large-'||n,n FROM generate_series(1,10000)n;
 INSERT INTO commerce_lot_auctions(lot_id,event_id,opens_at,closes_at,starting_price_minor,bid_increment_minor) SELECT 'large-'||n,'overlap',100,180+n*50,1000000000000,1 FROM generate_series(1,10000)n;
 INSERT INTO commerce_bids(lot_id,account_id,sequence,amount_minor,accepted_at,request_id,bidder_number) SELECT 'large-'||n,'a',1,1000000000000,150,'00000000-0000-0000-0000-'||lpad(n::text,12,'0'),1 FROM generate_series(1,10000)n;
 UPDATE commerce_lot_auctions a SET leading_bid_id=b.id,bid_count=1,state='sold' FROM commerce_bids b WHERE b.lot_id=a.lot_id;
 INSERT INTO commerce_auction_results SELECT a.lot_id,'sold',a.closes_at,600000,b.id,b.account_id,b.amount_minor FROM commerce_lot_auctions a JOIN commerce_bids b ON b.id=a.leading_bid_id;
 INSERT INTO commerce_orders(id,lot_id,winning_bid_id,owner_id,winning_amount_minor,currency,snapshot,closes_at,recorded_at,created_at) SELECT md5(r.lot_id),r.lot_id,r.winning_bid_id,r.winning_account_id,r.winning_amount_minor,'USD','{}',r.closes_at,r.recorded_at,600000 FROM commerce_auction_results r;`)
	out, e := (AnalyticsRepository{s}).Report(context.Background(), analytics.Filters{Event: "all", Dataset: "all"})
	if e != nil {
		t.Fatal(e)
	}
	if out.Data.Summary.Winning != "10000000000000000" || out.Data.Summary.Bidders != "1" {
		t.Fatal(out.Data.Summary)
	}
}

func TestAnalyticsConcurrentSnapshot(t *testing.T) {
	s := analyticsFixture(t)
	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 100; i++ {
			if _, e := s.DB.Exec(`UPDATE commerce_accounts SET demo=NOT demo WHERE id='a'`); e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 12; i++ {
		out, e := (AnalyticsRepository{s}).Report(ctx, analytics.Filters{Event: "all", Dataset: "all"})
		if e != nil {
			t.Fatal(e)
		}
		var sum int
		for _, ev := range out.Data.Events {
			n, e := strconv.Atoi(ev.Metrics.DemoBids)
			if e != nil {
				t.Fatal(e)
			}
			sum += n
		}
		if strconv.Itoa(sum) != out.Data.Summary.DemoBids {
			t.Fatal("mixed snapshots")
		}
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}

func TestAnalyticsZeroPriceAndFee(t *testing.T) {
	s, a := auctionFixture(t)
	ctx := context.Background()
	if _, e := a.Place(ctx, "a", bid("l", 0, 1), at(100)); e != nil {
		t.Fatal(e)
	}
	reconcile(t, a, 200)
	r := OrderRepository{s}
	wins, e := r.Wins(ctx, "a", at(201))
	if e != nil || len(wins) != 1 {
		t.Fatal(wins, e)
	}
	intent := checkoutIntent(1)
	intent.FulfillmentMethod = "pickup"
	intent.ShippingAddress = nil
	if _, e = r.Confirm(ctx, "a", wins[0].ID, intent, at(300)); e != nil {
		t.Fatal(e)
	}
	out, e := (AnalyticsRepository{s}).Report(ctx, analytics.Filters{Event: "all", Dataset: "all"})
	if e != nil || out.Data.Summary.PaidOrders != "1" || out.Data.Summary.PaidTotal != "0" || out.Data.Summary.PaidWinning != "0" || out.Data.Summary.PaidFees != "0" || out.Data.Summary.Winning != "0" {
		t.Fatal(out, e)
	}
}
