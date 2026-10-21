package store

import (
	"auctionlab/internal/order"
	"auctionlab/internal/testpg"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func soldOrder(t *testing.T) (*Store, OrderRepository, string) {
	t.Helper()
	s, a := auctionFixture(t)
	if _, e := a.Place(context.Background(), "a", bid("other", 12550, 1), at(100)); e != nil {
		t.Fatal(e)
	}
	reconcile(t, a, 200)
	r := OrderRepository{s}
	wins, e := r.Wins(context.Background(), "a", at(201))
	if e != nil || len(wins) != 1 {
		t.Fatal(wins, e)
	}
	return s, r, wins[0].ID
}
func checkoutIntent(n int) order.Intent {
	return order.Intent{Selection: order.Selection{Contact: order.Contact{Name: " Buyer ", Email: "BUYER@example.test", Phone: "123"}, FulfillmentMethod: "shipping", ShippingAddress: &order.Address{Line1: "Street", Locality: "City", Country: "vn"}, PaymentMethod: "demo-card"}, RequestID: fmt.Sprintf("00000000-0000-0000-0000-%012d", n)}
}
func TestWinnerCheckoutSnapshotIsolationAndPayment(t *testing.T) {
	s, r, id := soldOrder(t)
	ctx := context.Background()
	o, e := r.Get(ctx, "a", id, at(201))
	if e != nil || o.Snapshot.Title != "Other" || o.WinningAmountMinor != 12550 || o.State != "awaiting_payment" || o.Selection != nil {
		t.Fatal(o, e)
	}
	b, _ := json.Marshal(o)
	if strings.Contains(string(b), "secret") {
		t.Fatal("private catalog data in snapshot")
	}
	if _, e = r.Get(ctx, "b", id, at(201)); e == nil {
		t.Fatal("foreign access")
	}
	if _, e = r.Get(ctx, "invalid", id, at(201)); e == nil {
		t.Fatal("invalid session")
	}
	review, e := order.MakeReview(o, checkoutIntent(1).Selection)
	if e != nil || review.Total.TotalMinor != 14050 {
		t.Fatal(review, e)
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM commerce_demo_payments`).Scan(&count)
	if count != 0 {
		t.Fatal(count)
	}
	before, _ := r.Get(ctx, "a", id, at(202))
	if before.Selection != nil || before.State != "awaiting_payment" {
		t.Fatal(before)
	}
	invalid := checkoutIntent(1)
	invalid.Contact.Name = ""
	if _, e = r.Confirm(ctx, "a", id, invalid, at(202)); e == nil {
		t.Fatal("invalid payment accepted")
	}
	paid, e := r.Confirm(ctx, "a", id, checkoutIntent(1), at(203))
	if e != nil || paid.Order.Receipt == nil || paid.Order.Receipt.TotalMinor != 14050 || !paid.Order.Receipt.Simulated {
		t.Fatal(paid, e)
	}
	if wins, e := r.Wins(ctx, "a", at(204)); e != nil || len(wins) != 0 {
		t.Fatal(wins, e)
	}
	repeat, e := r.Confirm(ctx, "a", id, checkoutIntent(1), at(205))
	if e != nil || !repeat.Replay || repeat.Order.Receipt.ID != paid.Order.Receipt.ID {
		t.Fatal(repeat, e)
	}
	changed := checkoutIntent(1)
	changed.PaymentMethod = "demo-bank-transfer"
	if _, e = r.Confirm(ctx, "a", id, changed, at(206)); e == nil {
		t.Fatal("altered replay")
	}
	fresh := checkoutIntent(2)
	fresh.FulfillmentMethod = "pickup"
	fresh.PaymentMethod = "demo-bank-transfer"
	again, e := r.Confirm(ctx, "a", id, fresh, at(207))
	if e != nil || !again.AlreadyPaid || again.Order.Selection.FulfillmentMethod != "shipping" || again.Order.Receipt.PaidAt != 203 {
		t.Fatal(again, e)
	}
	if _, e = s.DB.Exec(`UPDATE commerce_accounts SET display_name='Changed' WHERE id='a'; DELETE FROM game;DELETE FROM bids;DELETE FROM players`); e != nil {
		t.Fatal(e)
	}
	after, e := r.Get(ctx, "a", id, at(208))
	if e != nil || after.Selection.Contact.Name != "Buyer" || !reflect.DeepEqual(after.Snapshot, o.Snapshot) {
		t.Fatal("snapshot changed", e)
	}
}
func TestWinnerOrderAtomicClosureAndAdoption(t *testing.T) {
	s, a := auctionFixture(t)
	ctx := context.Background()
	if _, e := a.Place(ctx, "a", bid("l", 0, 1), at(100)); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(`ALTER TABLE commerce_orders ADD CONSTRAINT reject_order CHECK(lot_id<>'l')`); e != nil {
		t.Fatal(e)
	}
	if _, e := a.Reconcile(ctx, at(200)); e == nil {
		t.Fatal("injected failure")
	}
	if snap := snapshot(t, a, "l", 200); snap.Result != nil || snap.State != "closing" {
		t.Fatal("partial closure", snap)
	}
	if _, e := s.DB.Exec(`ALTER TABLE commerce_orders DROP CONSTRAINT reject_order`); e != nil {
		t.Fatal(e)
	}
	reconcile(t, a, 210)
	var original string
	s.DB.QueryRow(`SELECT id FROM commerce_orders WHERE lot_id='l'`).Scan(&original)
	reconcile(t, a, 220)
	var id string
	s.DB.QueryRow(`SELECT id FROM commerce_orders WHERE lot_id='l'`).Scan(&id)
	if id != original {
		t.Fatal("rewritten order")
	}
	if _, e := s.DB.Exec(`DELETE FROM commerce_orders`); e != nil {
		t.Fatal(e)
	}
	n, e := a.Reconcile(ctx, at(500))
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
	var ok bool
	e = s.DB.QueryRow(`SELECT closes_at=200 AND recorded_at=210 AND created_at=500 FROM commerce_orders`).Scan(&ok)
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
}
func TestWinnerConfirmationConcurrencyAndRollback(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(fmt.Sprint(same), func(t *testing.T) {
			s, r, id := soldOrder(t)
			ctx := context.Background()
			if _, e := s.DB.Exec(`ALTER TABLE commerce_orders ADD CONSTRAINT reject_paid CHECK(state='awaiting_payment')`); e != nil {
				t.Fatal(e)
			}
			if _, e := r.Confirm(ctx, "a", id, checkoutIntent(1), at(250)); e == nil {
				t.Fatal("partial payment accepted")
			}
			var n int
			s.DB.QueryRow(`SELECT count(*) FROM commerce_demo_payments`).Scan(&n)
			if n != 0 {
				t.Fatal("partial receipt")
			}
			if _, e := s.DB.Exec(`ALTER TABLE commerce_orders DROP CONSTRAINT reject_paid`); e != nil {
				t.Fatal(e)
			}
			var wg sync.WaitGroup
			errs := make(chan error, 2)
			for j := 1; j <= 2; j++ {
				wg.Add(1)
				go func(j int) {
					defer wg.Done()
					if same {
						j = 1
					}
					_, e := r.Confirm(ctx, "a", id, checkoutIntent(j), at(251))
					errs <- e
				}(j)
			}
			wg.Wait()
			close(errs)
			for e := range errs {
				if e != nil {
					t.Fatal(e)
				}
			}
			s.DB.QueryRow(`SELECT count(*) FROM commerce_demo_payments`).Scan(&n)
			if n != 1 {
				t.Fatal(n)
			}
		})
	}
}
func TestWinnerMigrationFourUpgradeAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			s, e := Open(testpg.NewURL(t))
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			_, e = s.DB.Exec(initialDDL + accountsDDL + auctionsDDL + `; CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at BIGINT NOT NULL);INSERT INTO schema_migrations VALUES(1,1),(2,2),(3,3);INSERT INTO game VALUES(1,'LOBBY',NULL,42,'{}',1);INSERT INTO server_config VALUES('admin_key','fixture');`)
			if e != nil {
				t.Fatal(e)
			}
			if fail {
				if _, e = s.DB.Exec(`CREATE TABLE commerce_demo_payments(blocker int)`); e != nil {
					t.Fatal(e)
				}
				if s.Migrate() == nil {
					t.Fatal("failed migration accepted")
				}
				var ok bool
				e = s.DB.QueryRow(`SELECT (SELECT max(version)=3 FROM schema_migrations) AND to_regclass('commerce_orders') IS NULL`).Scan(&ok)
				if e != nil || !ok {
					t.Fatal(ok, e)
				}
				s.DB.Exec(`DROP TABLE commerce_demo_payments`)
			}
			for j := 0; j < 2; j++ {
				if e = s.Migrate(); e != nil {
					t.Fatal(e)
				}
			}
			var ok bool
			e = s.DB.QueryRow(`SELECT (SELECT max(version)=5 FROM schema_migrations) AND (SELECT revision=42 FROM game) AND (SELECT value='fixture' FROM server_config) AND (SELECT count(*)=0 FROM commerce_orders) AND (SELECT count(*)=0 FROM commerce_demo_payments)`).Scan(&ok)
			if e != nil || !ok {
				t.Fatal(ok, e)
			}
		})
	}
}
func TestWinnerSchemaConstraints(t *testing.T) {
	s, _, id := soldOrder(t)
	for _, q := range []string{`UPDATE commerce_orders SET owner_id='b'`, `UPDATE commerce_orders SET lot_id='expired'`, `UPDATE commerce_orders SET winning_amount_minor=-1`, `UPDATE commerce_orders SET winning_amount_minor=1000000000001`, `UPDATE commerce_orders SET state='paid'`, `UPDATE commerce_orders SET currency='VND'`, `INSERT INTO commerce_orders SELECT repeat('f',32),lot_id,winning_bid_id,owner_id,winning_amount_minor,currency,snapshot,closes_at,recorded_at,created_at,state,selection,fulfillment_fee_minor,total_minor,paid_at FROM commerce_orders`} {
		if _, e := s.DB.Exec(q); e == nil {
			t.Fatal("invalid order accepted")
		}
	}
	r := OrderRepository{s}
	if _, e := r.Confirm(context.Background(), "a", id, checkoutIntent(1), at(300)); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{`UPDATE commerce_demo_payments SET owner_id='b'`, `UPDATE commerce_demo_payments SET simulated=false`, `UPDATE commerce_demo_payments SET total_minor=-1`, `UPDATE commerce_demo_payments SET total_minor=2000000000001`, `UPDATE commerce_demo_payments SET payment_method='real-card'`, `INSERT INTO commerce_demo_payments SELECT repeat('f',32),order_id,owner_id,request_id,intent_digest,payment_method,currency,total_minor,paid_at,simulated FROM commerce_demo_payments`} {
		if _, e := s.DB.Exec(q); e == nil {
			t.Fatal("invalid payment accepted")
		}
	}
	var oe *order.Error
	if _, e := r.Get(context.Background(), "a", "missing", at(301)); !errors.As(e, &oe) || oe.Code != "not_found" {
		t.Fatal(e)
	}
}

func TestWinnerAdoptionMultipleBatchesAndRestart(t *testing.T) {
	s, a := auctionFixture(t)
	ctx := context.Background()
	// Emulate committed schema-3 winners, without running the new closure helper.
	_, e := s.DB.Exec(`INSERT INTO catalog_lots(id,slug,title,condition,fulfillment,state,starting_price_minor,bid_increment_minor,created_at,updated_at)
 SELECT 'history-'||n,'history-'||n,'Historical lot','Good','{"pickup_enabled":true,"pickup_instructions":"Published pickup"}','published',0,1,0,0 FROM generate_series(1,101)n;
 INSERT INTO catalog_lot_images SELECT 'history-'||n,0,'/site-assets/camera.svg','Historical image' FROM generate_series(1,101)n;
 INSERT INTO catalog_event_lots SELECT 'e','history-'||n,n+1 FROM generate_series(1,101)n;
 INSERT INTO commerce_lot_auctions(lot_id,event_id,opens_at,closes_at,starting_price_minor,bid_increment_minor) SELECT 'history-'||n,'e',100,200,0,1 FROM generate_series(1,101)n;
 INSERT INTO commerce_bids(lot_id,account_id,sequence,amount_minor,accepted_at,request_id,bidder_number) SELECT 'history-'||n,'a',1,0,150,'11111111-0000-0000-0000-'||lpad(n::text,12,'0'),1 FROM generate_series(1,101)n;
 UPDATE commerce_lot_auctions a SET state='sold',bid_count=1,leading_bid_id=b.id FROM commerce_bids b WHERE b.lot_id=a.lot_id;
 INSERT INTO commerce_auction_results SELECT a.lot_id,'sold',a.closes_at,210,b.id,b.account_id,b.amount_minor FROM commerce_lot_auctions a JOIN commerce_bids b ON b.id=a.leading_bid_id;`)
	if e != nil {
		t.Fatal(e)
	}
	n, e := a.Reconcile(ctx, at(500))
	if e != nil || n < 100 {
		t.Fatal(n, e)
	}
	var count int
	if e = s.DB.QueryRow(`SELECT count(*) FROM commerce_orders`).Scan(&count); e != nil || count != 100 {
		t.Fatal(count, e)
	}
	reconcile(t, a, 500)
	if e = s.DB.QueryRow(`SELECT count(*) FROM commerce_orders`).Scan(&count); e != nil || count != 101 {
		t.Fatal(count, e)
	}
	r := OrderRepository{s}
	wins, e := r.Wins(ctx, "a", at(501))
	if e != nil || len(wins) != 101 {
		t.Fatal(len(wins), e)
	}
	for j, o := range wins {
		if o.RecordedAt != 210 || o.ClosesAt != 200 || o.CreatedAt != 500 || j > 0 && wins[j-1].ID > o.ID {
			t.Fatal("history/order changed")
		}
	}
	i := checkoutIntent(500)
	i.FulfillmentMethod = "pickup"
	i.PaymentMethod = "demo-bank-transfer"
	paid, e := r.Confirm(ctx, "a", wins[0].ID, i, at(510))
	if e != nil || paid.Order.Selection.ShippingAddress != nil || paid.Order.Total.TotalMinor != 0 {
		t.Fatal(paid, e)
	}
	if _, e = r.Confirm(ctx, "a", wins[1].ID, i, at(511)); e == nil {
		t.Fatal("accepted ID reused across orders")
	}
	cfg := Config{URL: os.Getenv("TEST_DATABASE_URL"), Schema: s.SchemaName}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	opened, e := OpenConfig(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer opened.Close()
	r = OrderRepository{opened}
	a = AuctionRepository{opened}
	reconcile(t, a, 600)
	again, e := r.Confirm(ctx, "a", wins[0].ID, i, at(601))
	if e != nil || !again.Replay || !reflect.DeepEqual(again.Order, paid.Order) {
		t.Fatal("restart replay changed purchase", e)
	}
}
func TestWinnerConfirmationWaitCancellationAndWriterLoss(t *testing.T) {
	s, r, id := soldOrder(t)
	ctx := context.Background()
	lock, e := s.DB.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Rollback()
	if _, e = lock.Exec(`SELECT id FROM commerce_orders WHERE id=$1 FOR UPDATE`, id); e != nil {
		t.Fatal(e)
	}
	var ms atomic.Int64
	ms.Store(300)
	now := func() time.Time { return time.UnixMilli(ms.Load()) }
	done := make(chan error, 1)
	go func() { _, e := r.Confirm(ctx, "a", id, checkoutIntent(1), now); done <- e }()
	deadline := time.Now().Add(time.Second)
	for {
		var waiting bool
		e = s.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%FROM commerce_orders%FOR UPDATE%')`).Scan(&waiting)
		if e != nil {
			t.Fatal(e)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("confirmation did not wait")
		}
		time.Sleep(time.Millisecond)
	}
	ms.Store(10000)
	if e = lock.Commit(); e != nil {
		t.Fatal(e)
	}
	wantAuctionCode(t, <-done, "access_required")
	o, e := r.Get(ctx, "a", id, at(301))
	if e != nil || o.State != "awaiting_payment" {
		t.Fatal(o, e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = r.Confirm(cancelled, "a", id, checkoutIntent(1), at(302)); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	var pid int
	if e = s.writer.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(`SELECT pg_terminate_backend($1)`, pid); e != nil {
		t.Fatal(e)
	}
	for j := 0; j < 2; j++ {
		if _, e = r.Confirm(ctx, "a", id, checkoutIntent(1), at(303)); e == nil {
			t.Fatal("lost writer paid")
		}
	}
	if _, e = s.Ready(ctx); e == nil {
		t.Fatal("lost writer ready")
	}
	if !sanitize("auction commit", errors.New("connection lost after commit")).(safeError).CommitUncertain() {
		t.Fatal("commit classification")
	}
}

func TestWinnerUpgradePreservesPopulatedVersionThree(t *testing.T) {
	s, e := Open(testpg.NewURL(t))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	_, e = s.DB.Exec(initialDDL + accountsDDL + auctionsDDL + `;CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at BIGINT NOT NULL);INSERT INTO schema_migrations VALUES(1,1),(2,2),(3,3);
 INSERT INTO game VALUES(1,'LOBBY',NULL,42,'{}',1);INSERT INTO server_config VALUES('admin_key','fixture');
 INSERT INTO players VALUES('p','Player','player','fixture-token',123,7,1,2);
 INSERT INTO commerce_accounts VALUES('a','a@example.test','Original','fixture-verifier','bidder',true,1,1);
 INSERT INTO commerce_sessions VALUES('a','a',1,10000,NULL);
 INSERT INTO catalog_lots(id,slug,title,state,condition,fulfillment,starting_price_minor,bid_increment_minor,created_at,updated_at,demo) VALUES('l','historic-lot','Historic lot','published','Good','{"pickup_enabled":true,"pickup_instructions":"Published pickup"}',0,5,1,1,true);
 INSERT INTO catalog_lot_images VALUES('l',0,'/site-assets/catalog/lot-01.svg','Published image');
 INSERT INTO catalog_events(id,slug,title,state,opens_at,first_closes_at,close_interval_ms,created_at,updated_at,demo) VALUES('e','historic-event','Historic event','published',100,200,1,1,1,true);
 INSERT INTO catalog_event_lots VALUES('e','l',0);INSERT INTO commerce_watchlist VALUES('a','l',2);`)
	if e != nil {
		t.Fatal(e)
	}
	a := AuctionRepository{s}
	if _, e = a.Place(ctx, "a", bid("l", 12550, 1), at(150)); e != nil {
		t.Fatal(e)
	}
	_, e = s.DB.Exec(`INSERT INTO commerce_auction_results SELECT a.lot_id,'sold',a.closes_at,210,b.id,b.account_id,b.amount_minor FROM commerce_lot_auctions a JOIN commerce_bids b ON b.id=a.leading_bid_id;UPDATE commerce_lot_auctions SET state='sold',revision=revision+1`)
	if e != nil {
		t.Fatal(e)
	}
	var before string
	if e = s.DB.QueryRow(`SELECT json_build_object('bid',to_jsonb(b),'result',to_jsonb(r),'auction',to_jsonb(a))::text FROM commerce_bids b JOIN commerce_auction_results r ON r.winning_bid_id=b.id JOIN commerce_lot_auctions a ON a.lot_id=b.lot_id`).Scan(&before); e != nil {
		t.Fatal(e)
	}
	for j := 0; j < 2; j++ {
		if e = s.Migrate(); e != nil {
			t.Fatal(e)
		}
	}
	var after string
	if e = s.DB.QueryRow(`SELECT json_build_object('bid',to_jsonb(b),'result',to_jsonb(r),'auction',to_jsonb(a))::text FROM commerce_bids b JOIN commerce_auction_results r ON r.winning_bid_id=b.id JOIN commerce_lot_auctions a ON a.lot_id=b.lot_id`).Scan(&after); e != nil || after != before {
		t.Fatal("version-three execution changed", e)
	}
	var ok bool
	e = s.DB.QueryRow(`SELECT (SELECT count(*)=0 FROM commerce_orders) AND (SELECT count(*)=0 FROM commerce_demo_payments) AND (SELECT budget=123 AND token='fixture-token' FROM players) AND (SELECT revision=42 FROM game) AND (SELECT value='fixture' FROM server_config) AND (SELECT password_verifier='fixture-verifier' AND display_name='Original' FROM commerce_accounts) AND (SELECT token_digest='a' AND revoked_at IS NULL FROM commerce_sessions) AND (SELECT saved_at=2 FROM commerce_watchlist) AND (SELECT opens_at=100 AND first_closes_at=200 AND state='published' FROM catalog_events)`).Scan(&ok)
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	reconcile(t, a, 500)
	wins, e := (OrderRepository{s}).Wins(ctx, "a", at(501))
	if e != nil || len(wins) != 1 || !wins[0].Snapshot.AccountDemo || !wins[0].Snapshot.LotDemo || !wins[0].Snapshot.EventDemo || wins[0].CreatedAt != 500 || wins[0].RecordedAt != 210 {
		t.Fatal(wins, e)
	}
}
