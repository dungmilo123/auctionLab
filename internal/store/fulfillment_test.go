package store

import (
	"auctionlab/internal/order"
	"auctionlab/internal/sqliteimport"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func mustSQL(t *testing.T, s *Store, q string, args ...any) {
	t.Helper()
	if _, e := s.DB.Exec(q, args...); e != nil {
		t.Fatal(e)
	}
}
func TestFulfillmentPathsAndPaymentRollback(t *testing.T) {
	for _, method := range []string{"shipping", "pickup"} {
		t.Run(method, func(t *testing.T) {
			s, r, id := soldOrder(t)
			ctx := context.Background()
			mustSQL(t, s, `ALTER TABLE commerce_order_fulfillment_events ADD CONSTRAINT reject_event CHECK(version<>1)`)
			intent := checkoutIntent(1)
			intent.FulfillmentMethod = method
			if _, e := r.Confirm(ctx, "a", id, intent, at(300)); e == nil {
				t.Fatal("initial event failure accepted")
			}
			o, e := r.Get(ctx, "a", id, at(301))
			if e != nil || o.State != "awaiting_payment" || o.Selection != nil || len(o.Fulfillment.Timeline) != 1 {
				t.Fatal(o, e)
			}
			var n int
			s.DB.QueryRow(`SELECT count(*) FROM commerce_demo_payments`).Scan(&n)
			if n != 0 {
				t.Fatal("partial receipt")
			}
			if _, e = r.Transition(ctx, id, order.Transition{ExpectedVersion: 1, NextStatus: "preparing"}, at(301)); e == nil {
				t.Fatal("unpaid fulfillment")
			}
			mustSQL(t, s, `ALTER TABLE commerce_order_fulfillment_events DROP CONSTRAINT reject_event`)
			confirmation, e := r.Confirm(ctx, "a", id, intent, at(302))
			if e != nil {
				t.Fatal(e)
			}
			original := confirmation.Order
			o = original
			for next := order.Next(method, o.Fulfillment.Status); next != ""; next = order.Next(method, o.Fulfillment.Status) {
				transition := order.Transition{ExpectedVersion: o.Fulfillment.Version, NextStatus: next}
				if next == "shipped" {
					transition.Shipment = &order.Shipment{Carrier: "  " + strings.Repeat("航", 120) + "  ", TrackingReference: strings.Repeat("参", 200)}
				}
				// One accepted event despite simultaneous duplicate clicks; roll clock back.
				var wg sync.WaitGroup
				errs := make(chan error, 2)
				for j := 0; j < 2; j++ {
					wg.Add(1)
					go func() { defer wg.Done(); _, e := r.Transition(ctx, id, transition, at(200)); errs <- e }()
				}
				wg.Wait()
				close(errs)
				successes := 0
				for e := range errs {
					if e == nil {
						successes++
					} else {
						var typed *order.Error
						if !errors.As(e, &typed) || typed.Code != "transition_conflict" {
							t.Fatal(e)
						}
					}
				}
				if successes != 1 {
					t.Fatal(successes)
				}
				o, e = r.Get(ctx, "a", id, at(303))
				if e != nil {
					t.Fatal(e)
				}
				admin, e := r.AdminGet(ctx, id)
				if e != nil || !reflect.DeepEqual(o, admin) {
					t.Fatal("projection mismatch", e)
				}
				staged, e := r.History(ctx, "a", at(303))
				if e != nil || len(staged) != 1 || staged[0].Status != o.Fulfillment.Status || staged[0].Version != o.Fulfillment.Version {
					t.Fatal("history stage", staged, e)
				}
				if !reflect.DeepEqual(original.Receipt, o.Receipt) || !reflect.DeepEqual(original.Snapshot, o.Snapshot) || !reflect.DeepEqual(original.Selection, o.Selection) || o.Fulfillment.UpdatedAt != 302 {
					t.Fatal("purchase rewritten")
				}
				replay, e := r.Confirm(ctx, "a", id, intent, at(304))
				if e != nil || !reflect.DeepEqual(replay.Order.Fulfillment, o.Fulfillment) {
					t.Fatal("replay reset progress", e)
				}
				fresh := intent
				fresh.RequestID = checkoutIntent(2).RequestID
				again, e := r.Confirm(ctx, "a", id, fresh, at(304))
				if e != nil || !again.AlreadyPaid || !reflect.DeepEqual(again.Order.Fulfillment, o.Fulfillment) {
					t.Fatal("fresh retry reset", e)
				}
			}
			if _, e = r.Transition(ctx, id, order.Transition{ExpectedVersion: o.Fulfillment.Version, NextStatus: "completed"}, at(305)); e == nil {
				t.Fatal("reopened completed")
			}
			summaries, e := r.History(ctx, "a", at(305))
			if e != nil || len(summaries) != 1 || summaries[0].Status != "completed" {
				t.Fatal(summaries, e)
			}
			raw, _ := json.Marshal(summaries)
			for _, secret := range []string{"Buyer", "Street", "demo-card", o.Receipt.ID} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("summary leaked", secret)
				}
			}
			foreign, e := r.History(ctx, "b", at(305))
			if e != nil || len(foreign) != 0 {
				t.Fatal("foreign history", e)
			}
			for _, filter := range []order.Filters{{Status: "completed", Method: method}, {Status: "paid", Method: method}, {Status: "all", Method: "unselected"}} {
				v, e := r.AdminList(ctx, filter)
				want := 0
				if filter.Status == "completed" {
					want = 1
				}
				if e != nil || len(v) != want {
					t.Fatal(filter, v, e)
				}
			}
			if e = s.Bootstrap(at(306)()); e != nil {
				t.Fatal(e)
			}
			if e = s.Commit(Mutation{Phase: "LOBBY", Revision: 1, UpdatedAt: 306, ClearBids: true, ClearSealedBids: true, DeleteRuntime: true}); e != nil {
				t.Fatal(e)
			}
			cfg := Config{URL: os.Getenv("TEST_DATABASE_URL"), Schema: s.SchemaName}
			s.Close()
			opened, e := OpenConfig(cfg)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { opened.Close() })
			s = opened
			r = OrderRepository{s}
			persisted, e := r.Get(ctx, "a", id, at(307))
			if e != nil || !reflect.DeepEqual(persisted, o) {
				t.Fatal("restart/game reset changed progress", e)
			}
			mustSQL(t, s, `DELETE FROM commerce_order_fulfillment_events WHERE order_id=$1 AND version=$2`, id, o.Fulfillment.Version)
			if _, e = r.AdminGet(ctx, id); e == nil {
				t.Fatal("corruption fabricated progress")
			}
		})
	}
}
func TestFulfillmentRollbackAndLostLease(t *testing.T) {
	s, r, id := soldOrder(t)
	ctx := context.Background()
	paid, e := r.Confirm(ctx, "a", id, checkoutIntent(1), at(300))
	if e != nil {
		t.Fatal(e)
	}
	mustSQL(t, s, `ALTER TABLE commerce_order_fulfillment_events ADD CONSTRAINT reject_event CHECK(version<>2)`)
	if _, e = r.Transition(ctx, id, order.Transition{ExpectedVersion: 1, NextStatus: "preparing"}, at(301)); e == nil {
		t.Fatal("partial transition")
	}
	o, e := r.Get(ctx, "a", id, at(302))
	if e != nil || !reflect.DeepEqual(o, paid.Order) {
		t.Fatal("partial event/update", e)
	}
	mustSQL(t, s, `ALTER TABLE commerce_order_fulfillment_events DROP CONSTRAINT reject_event`)
	for _, next := range []string{"completed", "ready_for_pickup", "shipped"} {
		if _, e = r.Transition(ctx, id, order.Transition{ExpectedVersion: 1, NextStatus: next, Shipment: func() *order.Shipment {
			if next == "shipped" {
				return &order.Shipment{Carrier: "C", TrackingReference: "R"}
			}
			return nil
		}()}, at(303)); e == nil {
			t.Fatal("skipped/method step")
		}
	}
	// Database connection drops before or after a commit both require readback, never success.
	if !sanitize("auction commit", fmt.Errorf("lost response")).(safeError).CommitUncertain() {
		t.Fatal("unknown classification")
	}
	var pid int
	s.writer.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid)
	mustSQL(t, s, `SELECT pg_terminate_backend($1)`, pid)
	for i := 0; i < 2; i++ {
		if _, e = r.Transition(ctx, id, order.Transition{ExpectedVersion: 1, NextStatus: "preparing"}, at(304)); e == nil {
			t.Fatal("stale ownership reacquired")
		}
	}
	if _, e = s.Ready(ctx); e == nil {
		t.Fatal("stale ready")
	}
}
func TestFulfillmentMigrationBackfillAndRollback(t *testing.T) {
	for _, method := range []string{"shipping", "pickup"} {
		for _, corrupt := range []bool{false, true} {
			t.Run(fmt.Sprint(method, corrupt), func(t *testing.T) {
				s, auctions := auctionFixture(t)
				r := OrderRepository{s}
				if _, e := auctions.Place(context.Background(), "a", bid("other", 12550, 1), at(100)); e != nil {
					t.Fatal(e)
				}
				if _, e := auctions.Place(context.Background(), "b", bid("l", 0, 2), at(100)); e != nil {
					t.Fatal(e)
				}
				reconcile(t, auctions, 200)
				var id string
				s.DB.QueryRow(`SELECT id FROM commerce_orders WHERE owner_id='a'`).Scan(&id)
				mustSQL(t, s, `UPDATE commerce_orders SET created_at=250 WHERE id=$1`, id)
				ctx := context.Background()
				intent := checkoutIntent(1)
				intent.FulfillmentMethod = method
				paid, e := r.Confirm(ctx, "a", id, intent, at(300))
				if e != nil {
					t.Fatal(e)
				}
				mustSQL(t, s, `DROP TABLE commerce_interactions,commerce_interaction_coverage;DROP INDEX commerce_orders_history;DROP INDEX commerce_orders_recorded;DROP TABLE commerce_order_fulfillment_events;DROP TABLE commerce_order_fulfillment;DROP TABLE demo_environments;ALTER TABLE commerce_demo_payments DROP CONSTRAINT commerce_payment_order_key;DELETE FROM schema_migrations WHERE version>=5`)
				var before string
				s.DB.QueryRow(`SELECT to_jsonb(o)::text||to_jsonb(p)::text FROM commerce_orders o JOIN commerce_demo_payments p ON p.order_id=o.id`).Scan(&before)
				if corrupt {
					mustSQL(t, s, `UPDATE commerce_demo_payments SET paid_at=301`)
					if s.Migrate() == nil {
						t.Fatal("inconsistent backfill accepted")
					}
					var ok bool
					s.DB.QueryRow(`SELECT max(version)=4 AND to_regclass('commerce_order_fulfillment') IS NULL FROM schema_migrations`).Scan(&ok)
					if !ok {
						t.Fatal("partial migration")
					}
					mustSQL(t, s, `UPDATE commerce_demo_payments SET paid_at=300`)
				}
				for j := 0; j < 2; j++ {
					if e = s.Migrate(); e != nil {
						t.Fatal(e)
					}
				}
				var after string
				s.DB.QueryRow(`SELECT to_jsonb(o)::text||to_jsonb(p)::text FROM commerce_orders o JOIN commerce_demo_payments p ON p.order_id=o.id`).Scan(&after)
				if before != after {
					t.Fatal("purchase migration rewrite")
				}
				var initialized bool
				s.DB.QueryRow(`SELECT (SELECT count(*)=1 FROM commerce_order_fulfillment) AND (SELECT count(*)=2 FROM commerce_orders)`).Scan(&initialized)
				if !initialized {
					t.Fatal("invented unpaid progress")
				}
				o, e := r.Get(ctx, "a", id, at(301))
				if e != nil || !reflect.DeepEqual(o, paid.Order) {
					t.Fatal(o, e)
				}
			})
		}
	}
}
func TestFulfillmentImportOccupied(t *testing.T) {
	s, _, _ := soldOrder(t)
	ctx := context.Background()
	sourcePath := sourceFixture(t)
	before, _ := os.ReadFile(sourcePath)
	source, e := sqliteimport.Read(ctx, sourcePath)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ImportSQLite(ctx, source); e == nil {
		t.Fatal("occupied imported")
	}
	after, _ := os.ReadFile(sourcePath)
	if string(before) != string(after) {
		t.Fatal("source changed")
	}
}

func TestOrderHistoryOrderingAndFailure(t *testing.T) {
	s, a := auctionFixture(t)
	ctx := context.Background()
	for i, id := range []string{"l", "other"} {
		amount := int64(0)
		if id == "other" {
			amount = 12550
		}
		if _, e := a.Place(ctx, "a", bid(id, amount, i+1), at(100)); e != nil {
			t.Fatal(e)
		}
	}
	reconcile(t, a, 200)
	r := OrderRepository{s}
	v, e := r.History(ctx, "a", at(201))
	if e != nil || len(v) != 2 || v[0].ID >= v[1].ID {
		t.Fatal(v, e)
	}
	for _, o := range v {
		if o.Method != nil || o.TotalMinor != nil || o.Status != "awaiting_payment" || o.Version != 0 {
			t.Fatal(o)
		}
	}
	s.Close()
	if empty, e := r.History(ctx, "a", at(201)); e == nil || empty != nil {
		t.Fatal("failure became empty success", e)
	}
}
