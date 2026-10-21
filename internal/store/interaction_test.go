package store

import (
	"auctionlab/internal/interaction"
	"auctionlab/internal/order"
	"auctionlab/internal/sqliteimport"
	"auctionlab/internal/testpg"
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func interactionCount(t *testing.T, s *Store, kind string) int {
	t.Helper()
	var n int
	if e := s.DB.QueryRow(`SELECT count(*) FROM commerce_interactions WHERE kind=$1`, kind).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func TestInteractionAtomicActionsAndProvenance(t *testing.T) {
	s, a := auctionFixture(t)
	ctx := context.Background()
	r := AccountRepository{s}
	yes, no := true, false
	mustSQL(t, s, `ALTER TABLE commerce_interactions ADD CONSTRAINT reject_capture CHECK(kind<>'watchlist_add')`)
	if _, e := r.Membership(ctx, "a", "l", &yes, 100); e == nil {
		t.Fatal("capture failure accepted")
	}
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM commerce_watchlist`).Scan(&n)
	if n != 0 {
		t.Fatal("partial watch")
	}
	mustSQL(t, s, `ALTER TABLE commerce_interactions DROP CONSTRAINT reject_capture`)
	for _, v := range []*bool{&yes, &yes, &no, &yes} {
		if _, e := r.Membership(ctx, "a", "l", v, 100); e != nil {
			t.Fatal(e)
		}
	}
	if interactionCount(t, s, "watchlist_add") != 2 {
		t.Fatal("watch transitions")
	}
	mustSQL(t, s, `ALTER TABLE commerce_interactions ADD CONSTRAINT reject_capture CHECK(kind<>'bid_accepted')`)
	if _, e := a.Place(ctx, "a", bid("l", 0, 1), at(100)); e == nil {
		t.Fatal("partial bid")
	}
	s.DB.QueryRow(`SELECT count(*) FROM commerce_bids`).Scan(&n)
	if n != 0 {
		t.Fatal("bid survived capture failure")
	}
	mustSQL(t, s, `ALTER TABLE commerce_interactions DROP CONSTRAINT reject_capture`)
	for i := 0; i < 2; i++ {
		if _, e := a.Place(ctx, "a", bid("l", 0, 1), at(100)); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := a.Place(ctx, "b", bid("l", 0, 2), at(101)); e == nil {
		t.Fatal("rejected bid accepted")
	}
	if interactionCount(t, s, "bid_accepted") != 1 {
		t.Fatal("bid replay captured")
	}
	reconcile(t, a, 200)
	o := OrderRepository{s}
	wins, e := o.Wins(ctx, "a", at(201))
	if e != nil || len(wins) != 1 {
		t.Fatal(wins, e)
	}
	mustSQL(t, s, `ALTER TABLE commerce_interactions ADD CONSTRAINT reject_capture CHECK(kind<>'payment_confirmed')`)
	if _, e = o.Confirm(ctx, "a", wins[0].ID, checkoutIntent(2), at(300)); e == nil {
		t.Fatal("payment capture failure")
	}
	s.DB.QueryRow(`SELECT count(*) FROM commerce_demo_payments`).Scan(&n)
	if n != 0 {
		t.Fatal("partial payment")
	}
	mustSQL(t, s, `ALTER TABLE commerce_interactions DROP CONSTRAINT reject_capture`)
	for _, id := range []int{2, 2, 3} {
		if _, e = o.Confirm(ctx, "a", wins[0].ID, checkoutIntent(id), at(300)); e != nil {
			t.Fatal(e)
		}
	}
	if interactionCount(t, s, "payment_confirmed") != 1 {
		t.Fatal("payment replay captured")
	}
	var ok bool
	s.DB.QueryRow(`SELECT bool_and(provenance='observed' AND actor_id='a' AND NOT lot_demo AND NOT event_demo AND NOT actor_demo) FROM commerce_interactions`).Scan(&ok)
	if !ok {
		t.Fatal("derived flags")
	}
}
func TestInteractionViewsCoverageAndImport(t *testing.T) {
	s, _ := auctionFixture(t)
	ctx := context.Background()
	r := InteractionRepository{s}
	id := "00000000-0000-0000-0000-000000000001"
	for i := 0; i < 2; i++ {
		if e := r.Observe(ctx, "l", id, 100); e != nil {
			t.Fatal(e)
		}
	}
	if interactionCount(t, s, "lot_view") != 1 {
		t.Fatal("view duplicate")
	}
	if e := r.Observe(ctx, "other", id, 101); !errors.Is(e, interaction.ErrConflict) {
		t.Fatal(e)
	}
	if e := r.Observe(ctx, "draft", "00000000-0000-0000-0000-000000000002", 101); !errors.Is(e, interaction.ErrNotFound) {
		t.Fatal(e)
	}
	for _, ms := range []int64{100, 200} {
		if e := s.InitializeInteractionCoverage(ctx, time.UnixMilli(ms)); e != nil {
			t.Fatal(e)
		}
	}
	var start int64
	s.DB.QueryRow(`SELECT collection_started_at FROM commerce_interaction_coverage`).Scan(&start)
	if start != 100 {
		t.Fatal(start)
	}
	path := sourceFixture(t)
	before, _ := os.ReadFile(path)
	source, e := sqliteimport.Read(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	target, e := Open(testpg.NewURL(t))
	if e != nil {
		t.Fatal(e)
	}
	defer target.Close()
	if e = target.Migrate(); e != nil {
		t.Fatal(e)
	}
	if e = target.InitializeInteractionCoverage(ctx, time.UnixMilli(100)); e != nil {
		t.Fatal(e)
	}
	if _, e = target.ImportSQLite(ctx, source); e == nil {
		t.Fatal("coverage-only occupied target accepted")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("source changed")
	}
}
func TestInteractionUpgradeBackfill(t *testing.T) {
	s, o, id := soldOrder(t)
	ctx := context.Background()
	if _, e := o.Confirm(ctx, "a", id, checkoutIntent(1), at(300)); e != nil {
		t.Fatal(e)
	}
	yes := true
	if _, e := (AccountRepository{s}).Membership(ctx, "b", "l", &yes, 150); e != nil {
		t.Fatal(e)
	}
	mustSQL(t, s, `DROP TABLE commerce_interactions,commerce_interaction_coverage;DELETE FROM schema_migrations WHERE version=6`)
	mustSQL(t, s, `CREATE TABLE commerce_interactions(block INTEGER)`)
	if e := s.Migrate(); e == nil {
		t.Fatal("collision migration succeeded")
	}
	var v int
	s.DB.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&v)
	if v != 5 {
		t.Fatal(v)
	}
	mustSQL(t, s, `DROP TABLE commerce_interactions`)
	for i := 0; i < 2; i++ {
		if e := s.Migrate(); e != nil {
			t.Fatal(e)
		}
	}
	var ok bool
	e := s.DB.QueryRow(`SELECT (SELECT count(*)=1 FROM commerce_interactions WHERE kind='bid_accepted' AND provenance='backfilled' AND occurred_at=100) AND (SELECT count(*)=1 FROM commerce_interactions WHERE kind='payment_confirmed' AND provenance='backfilled' AND occurred_at=300) AND (SELECT count(*)=1 FROM commerce_interactions WHERE kind='watchlist_add' AND provenance='snapshot' AND occurred_at=150) AND (SELECT count(*)=0 FROM commerce_interactions WHERE kind='lot_view') AND (SELECT count(*)=1 FROM commerce_interaction_coverage)`).Scan(&ok)
	if e != nil || !ok {
		t.Fatal("backfill", e)
	}
}

func TestInteractionBackfillFailureRollsBack(t *testing.T) {
	s, _ := auctionFixture(t)
	mustSQL(t, s, `INSERT INTO commerce_watchlist VALUES('a','l',-1);DROP TABLE commerce_interactions,commerce_interaction_coverage;DELETE FROM schema_migrations WHERE version=6`)
	if e := s.Migrate(); e == nil {
		t.Fatal("invalid historical timestamp accepted")
	}
	var ok bool
	e := s.DB.QueryRow(`SELECT (SELECT max(version)=5 FROM schema_migrations) AND to_regclass('commerce_interactions') IS NULL AND to_regclass('commerce_interaction_coverage') IS NULL AND (SELECT saved_at=-1 FROM commerce_watchlist)`).Scan(&ok)
	if e != nil || !ok {
		t.Fatal("partial backfill", e)
	}
	mustSQL(t, s, `UPDATE commerce_watchlist SET saved_at=100`)
	if e = s.Migrate(); e != nil {
		t.Fatal(e)
	}
}
func TestInteractionCoverageRestartAndProjection(t *testing.T) {
	raw := testpg.NewURL(t)
	s, e := Open(raw)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Migrate(); e != nil {
		t.Fatal(e)
	}
	if e = s.InitializeInteractionCoverage(context.Background(), time.UnixMilli(100)); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(raw)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Migrate(); e != nil {
		t.Fatal(e)
	}
	if e = s.InitializeInteractionCoverage(context.Background(), time.UnixMilli(200)); e != nil {
		t.Fatal(e)
	}
	var start int64
	if e = s.DB.QueryRow(`SELECT collection_started_at FROM commerce_interaction_coverage`).Scan(&start); e != nil || start != 100 {
		t.Fatal(start, e)
	}
	rows, e := s.DB.Query(`SELECT column_name FROM information_schema.columns WHERE table_schema=$1 AND table_name='commerce_interactions' ORDER BY ordinal_position`, s.SchemaName)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	allow := map[string]bool{"id": true, "kind": true, "lot_id": true, "event_id": true, "actor_id": true, "occurred_at": true, "recorded_at": true, "lot_demo": true, "event_demo": true, "actor_demo": true, "provenance": true, "source_key": true}
	n := 0
	for rows.Next() {
		var name string
		if e = rows.Scan(&name); e != nil {
			t.Fatal(e)
		}
		if !allow[name] {
			t.Fatal("non-allowlisted interaction column", name)
		}
		n++
	}
	if e = rows.Err(); e != nil || n != len(allow) {
		t.Fatal(n, e)
	}
}
func TestInteractionConstraintCombinations(t *testing.T) {
	s, _ := auctionFixture(t)
	for _, q := range []string{
		`INSERT INTO commerce_interactions VALUES('00000000000000000000000000000001','lot_view','l','e','a',100,100,false,false,false,'observed','00000000-0000-0000-0000-000000000001')`,
		`INSERT INTO commerce_interactions VALUES('00000000000000000000000000000001','lot_view','l','e',NULL,100,100,false,false,NULL,'backfilled','00000000-0000-0000-0000-000000000001')`,
		`INSERT INTO commerce_interactions VALUES('00000000000000000000000000000001','bid_accepted','l','e','a',100,100,false,false,false,'snapshot','1')`,
		`INSERT INTO commerce_interactions VALUES('00000000000000000000000000000001','watchlist_add','l','e',NULL,100,100,false,false,NULL,'observed','1')`,
	} {
		if _, e := s.DB.Exec(q); e == nil {
			t.Fatal("invalid combination accepted")
		}
	}
}

func TestCaptureFulfillmentAndReadDoNotAddEvents(t *testing.T) {
	s, r, id := soldOrder(t)
	ctx := context.Background()
	o, e := r.Get(ctx, "a", id, at(201))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = order.MakeReview(o, checkoutIntent(1).Selection); e != nil {
		t.Fatal(e)
	}
	if interactionCount(t, s, "payment_confirmed") != 0 {
		t.Fatal("review captured")
	}
	if _, e = r.Confirm(ctx, "a", id, checkoutIntent(1), at(300)); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Transition(ctx, id, order.Transition{ExpectedVersion: 1, NextStatus: "preparing"}, at(301)); e != nil {
		t.Fatal(e)
	}
	if interactionCount(t, s, "payment_confirmed") != 1 || interactionCount(t, s, "bid_accepted") != 1 {
		t.Fatal("fulfillment captured")
	}
}

func TestCaptureUnknownCommitAndLostLease(t *testing.T) {
	for _, kind := range []string{"watchlist_add", "bid_accepted", "payment_confirmed"} {
		t.Run(kind, func(t *testing.T) {
			var s *Store
			var orderID string
			if kind == "payment_confirmed" {
				s, _, orderID = soldOrder(t)
			} else {
				s, _ = auctionFixture(t)
			}
			ctx := context.Background()
			yes := true
			perform := func() error {
				switch kind {
				case "watchlist_add":
					_, e := (AccountRepository{s}).Membership(ctx, "a", "l", &yes, 150)
					return e
				case "bid_accepted":
					_, e := (AuctionRepository{s}).Place(ctx, "a", bid("l", 0, 1), at(150))
					return e
				default:
					_, e := (OrderRepository{s}).Confirm(ctx, "a", orderID, checkoutIntent(1), at(300))
					return e
				}
			}
			before := interactionCount(t, s, kind)
			// Deferred trigger kills only this fixture writer during COMMIT. The caller
			// cannot claim commit or reuse its stale leased writer after this outcome.
			mustSQL(t, s, `CREATE FUNCTION reject_capture_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_terminate_backend(pg_backend_pid()); RETURN NEW; END $$;CREATE CONSTRAINT TRIGGER reject_capture_commit AFTER INSERT ON commerce_interactions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_capture_commit()`)
			e := perform()
			var uncertain interface{ CommitUncertain() bool }
			if e == nil || !errors.As(e, &uncertain) || !uncertain.CommitUncertain() {
				t.Fatal("not classified unknown", e)
			}
			if interactionCount(t, s, kind) != before {
				t.Fatal("partial commit")
			}
			if e = perform(); e == nil {
				t.Fatal("stale writer reused")
			}
			mustSQL(t, s, `DROP TRIGGER reject_capture_commit ON commerce_interactions;DROP FUNCTION reject_capture_commit()`)
			schema := s.SchemaName
			s.Close()
			s, e = OpenConfig(Config{URL: os.Getenv("TEST_DATABASE_URL"), Schema: schema})
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			for i := 0; i < 2; i++ {
				if e = perform(); e != nil {
					t.Fatal("durable retry", e)
				}
			}
			if interactionCount(t, s, kind) != before+1 {
				t.Fatal("duplicate recovery")
			}
			var unchanged bool
			if e = s.DB.QueryRow(`SELECT (SELECT revision=42 FROM game) AND (SELECT count(*)=0 FROM bids) AND (SELECT version=1 FROM catalog_lots WHERE id='l')`).Scan(&unchanged); e != nil || !unchanged {
				t.Fatal("unrelated facts changed", e)
			}
			var pid int
			if e = s.writer.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
				t.Fatal(e)
			}
			mustSQL(t, s, `SELECT pg_terminate_backend($1)`, pid)
			for i := 0; i < 2; i++ {
				if e = perform(); e == nil {
					t.Fatal("lost writer reacquired")
				}
			}
			if interactionCount(t, s, kind) != before+1 {
				t.Fatal("lost writer mutated")
			}
		})
	}
}

func TestImportInteractionOccupancyUnchanged(t *testing.T) {
	s, _ := auctionFixture(t)
	ctx := context.Background()
	r := InteractionRepository{s}
	if e := r.Observe(ctx, "l", "00000000-0000-0000-0000-000000000001", 100); e != nil {
		t.Fatal(e)
	}
	sourcePath := sourceFixture(t)
	before, e := os.ReadFile(sourcePath)
	if e != nil {
		t.Fatal(e)
	}
	source, e := sqliteimport.Read(ctx, sourcePath)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ImportSQLite(ctx, source); e == nil {
		t.Fatal("interaction occupied target accepted")
	}
	if interactionCount(t, s, "lot_view") != 1 {
		t.Fatal("destination changed")
	}
	after, e := os.ReadFile(sourcePath)
	if e != nil || string(before) != string(after) {
		t.Fatal("source changed", e)
	}
}
