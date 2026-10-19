package store

import (
	"auctionlab/internal/account"
	"auctionlab/internal/auction"
	"auctionlab/internal/catalog"
	"auctionlab/internal/testpg"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func auctionFixture(t *testing.T) (*Store, AuctionRepository) {
	t.Helper()
	s, e := Open(testpg.NewURL(t))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	if e = s.Migrate(); e != nil {
		t.Fatal(e)
	}
	_, e = s.DB.Exec(`INSERT INTO game VALUES(1,'LOBBY',NULL,42,'{}',0);
 INSERT INTO server_config VALUES('admin_key','secret-capability');
 INSERT INTO commerce_accounts VALUES('a','secret-a@example.test','Secret name A','secret-password','bidder',false,0,0),('b','secret-b@example.test','Secret name B','secret-password','admin',false,0,0);
 INSERT INTO commerce_sessions VALUES('a','a',0,10000,NULL),('b','b',0,10000,NULL);
 INSERT INTO catalog_lots(id,slug,title,state,starting_price_minor,bid_increment_minor,admin_notes,created_at,updated_at) VALUES('l','lot','Lot','published',0,5,'secret-notes',0,0),('other','other','Other','published',12550,500,'secret-notes',0,0),('draft','draft','Draft','draft',0,1,'secret-notes',0,0),('expired','expired','Expired','published',0,1,'secret-notes',0,0);
 INSERT INTO catalog_events(id,slug,title,state,opens_at,first_closes_at,close_interval_ms,created_at,updated_at) VALUES('e','event','Event','published',100,200,50,0,0),('overlap','overlap','Overlap','published',100,180,50,0,0),('past','past','Past','published',10,20,1,0,0);
 INSERT INTO catalog_event_lots VALUES('e','l',0),('overlap','other',0),('e','draft',1),('past','expired',0);`)
	if e != nil {
		t.Fatal(e)
	}
	return s, AuctionRepository{s}
}
func at(ms int64) func() time.Time { return func() time.Time { return time.UnixMilli(ms) } }
func bid(id string, amount int64, n int) auction.Intent {
	return auction.Intent{LotID: id, AmountMinor: &amount, RequestID: fmt.Sprintf("00000000-0000-0000-0000-%012d", n)}
}
func wantAuctionCode(t *testing.T, e error, code string) {
	t.Helper()
	var ae *auction.Error
	var ac *account.Error
	if errors.As(e, &ae) && ae.Code == code {
		return
	}
	if errors.As(e, &ac) && ac.Code == code {
		return
	}
	t.Fatalf("want %s got %v", code, e)
}
func snapshot(t *testing.T, r AuctionRepository, id string, ms int64) auction.Snapshot {
	t.Helper()
	s, e := r.Snapshot(context.Background(), id, at(ms))
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func reconcile(t *testing.T, r AuctionRepository, ms int64) {
	t.Helper()
	if e := auction.New(r, at(ms)).Reconcile(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func TestAuctionLifecycleAndIndependentPublication(t *testing.T) {
	s, r := auctionFixture(t)
	ctx := context.Background()
	if snap := snapshot(t, r, "l", 100); snap.State != "pending" || snap.CurrentPriceMinor != nil {
		t.Fatal(snap)
	}
	if _, e := r.Snapshot(ctx, "draft", at(100)); e == nil {
		t.Fatal("draft exposed")
	}
	reconcile(t, r, 99)
	if snapshot(t, r, "l", 99).State != "scheduled" {
		t.Fatal("early opening")
	}
	if snapshot(t, r, "l", 100).State != "opening" {
		t.Fatal("opening presentation")
	}
	reconcile(t, r, 100)
	if snapshot(t, r, "l", 100).State != "open" || snapshot(t, r, "other", 100).State != "open" {
		t.Fatal("overlap")
	}
	expired := snapshot(t, r, "expired", 100)
	if expired.State != "unsold" || expired.Result.ClosesAt != 20 || expired.Result.RecordedAt != 99 {
		t.Fatal(expired)
	}
	if _, e := r.Place(ctx, "a", bid("l", 0, 1), at(100)); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Place(ctx, "b", bid("l", 11, 2), at(199)); e != nil {
		t.Fatal(e)
	}
	before := snapshot(t, r, "l", 200)
	if before.State != "closing" || before.Result != nil {
		t.Fatal(before)
	}
	reconcile(t, r, 200)
	after := snapshot(t, r, "l", 200)
	if after.State != "sold" || after.Result.Winner.AmountMinor != 11 || after.BidCount != 2 {
		t.Fatal(after)
	}
	reconcile(t, r, 500)
	if snapshot(t, r, "l", 500).Revision != after.Revision {
		t.Fatal("repeated close changed result")
	}
	other := snapshot(t, r, "other", 500)
	if other.State != "unsold" {
		t.Fatal(other)
	}
	var ok bool
	e := s.DB.QueryRow(`SELECT (SELECT revision=42 FROM game) AND (SELECT count(*)=0 FROM bids) AND (SELECT count(*)=0 FROM commerce_lot_auctions WHERE lot_id='draft') AND (SELECT version=1 AND state='published' AND opens_at=100 FROM catalog_events WHERE id='e')`).Scan(&ok)
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	// A later publication is discovered without changing its dates.
	_, e = s.DB.Exec(`INSERT INTO catalog_lots(id,slug,title,state,starting_price_minor,bid_increment_minor,created_at,updated_at) VALUES('new','new','New','published',0,1,0,0); INSERT INTO catalog_event_lots VALUES('e','new',2)`)
	if e != nil {
		t.Fatal(e)
	}
	reconcile(t, r, 210)
	if snapshot(t, r, "new", 210).ClosesAt != 300 || snapshot(t, r, "new", 210).State != "open" {
		t.Fatal("new publication")
	}
}
func TestAuctionBidLadderReplayAndPrivateReads(t *testing.T) {
	_, r := auctionFixture(t)
	ctx := context.Background()
	_, e := r.Place(ctx, "a", bid("other", 12550, 1), at(99))
	wantAuctionCode(t, e, "not_open")
	receipt, e := r.Place(ctx, "a", bid("other", 12550, 1), at(100))
	if e != nil || receipt.Replay {
		t.Fatal(receipt, e)
	}
	_, e = r.Place(ctx, "a", bid("other", 20000, 2), at(101))
	wantAuctionCode(t, e, "already_leading")
	_, e = r.Place(ctx, "b", bid("other", 13049, 3), at(102))
	wantAuctionCode(t, e, "minimum_not_met")
	receipt, e = r.Place(ctx, "b", bid("other", 14001, 3), at(103))
	if e != nil || receipt.Bid.Sequence != 2 {
		t.Fatal(receipt, e)
	}
	snap := snapshot(t, r, "other", 103)
	if *snap.NextMinimumMinor != 14501 || snap.BidCount != 2 || snap.History[1].Alias != "Bidder 1" || snap.Leader.Alias != "Bidder 2" {
		t.Fatal(snap)
	}
	if _, e = r.Place(ctx, "a", bid("l", 0, 1), at(104)); e == nil {
		t.Fatal("cross-lot id reuse")
	}
	for _, v := range []struct{ digest, status string }{{"a", "outbid"}, {"b", "leading"}} {
		p, e := r.Participation(ctx, v.digest, "other", at(104))
		if e != nil || len(p) != 1 || p[0].Status != v.status || len(p[0].Bids) != 1 {
			t.Fatal(p, e)
		}
	}
	p, e := r.Participation(ctx, "a", "l", at(104))
	if e != nil || p[0].Status != "not_participating" {
		t.Fatal(p, e)
	}
	_, e = r.Place(ctx, "a", bid("other", 14501, 4), at(180))
	wantAuctionCode(t, e, "closed")
	reconcile(t, r, 180)
	replay, e := r.Place(ctx, "a", bid("other", 12550, 1), at(190))
	if e != nil || !replay.Replay || replay.Bid.Sequence != 1 {
		t.Fatal(replay, e)
	}
	for _, v := range []struct{ digest, status string }{{"a", "lost"}, {"b", "won"}} {
		p, e := r.Participation(ctx, v.digest, "other", at(190))
		if e != nil || p[0].Status != v.status || p[0].CheckoutAvailable {
			t.Fatal(p, e)
		}
	}
	_, e = r.Place(ctx, "a", bid("other", 12550, 1), at(10000))
	wantAuctionCode(t, e, "access_required")
}
func TestAuctionRacesAndMaximum(t *testing.T) {
	s, r := auctionFixture(t)
	ctx := context.Background()
	reconcile(t, r, 100)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for n, digest := range []string{"a", "b"} {
		wg.Add(1)
		go func(n int, d string) { defer wg.Done(); _, e := r.Place(ctx, d, bid("l", 0, n+1), at(150)); errs <- e }(n, digest)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			wantAuctionCode(t, e, "minimum_not_met")
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
	// Identical overlapping requests both return the single durable receipt.
	errs = make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := r.Place(ctx, "a", bid("other", catalog.MaxMoney, 100), at(150))
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	snap := snapshot(t, r, "other", 150)
	if snap.BidCount != 1 || !snap.AmountLimitReached || snap.NextMinimumMinor != nil {
		t.Fatal(snap)
	}
	_, e := r.Place(ctx, "b", bid("other", catalog.MaxMoney, 101), at(150))
	wantAuctionCode(t, e, "amount_limit_reached")
	errs = make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); _, e := r.Place(ctx, "b", bid("l", 10, 102), at(199)); errs <- e }()
	go func() { defer wg.Done(); _, e := r.Reconcile(ctx, at(200)); errs <- e }()
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			var ae *auction.Error
			if !errors.As(e, &ae) || ae.Code != "closed" {
				t.Fatal(e)
			}
		}
	}
	reconcile(t, r, 200)
	snap = snapshot(t, r, "l", 200)
	if snap.State != "sold" || snap.Result.Winner.AmountMinor != *snap.CurrentPriceMinor {
		t.Fatal(snap)
	}
	var n int
	if e = s.DB.QueryRow(`SELECT count(*) FROM commerce_auction_results WHERE lot_id='l'`).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
}
func TestAuctionLockWaitRevalidatesDeadlineAndSession(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(fmt.Sprint(expire), func(t *testing.T) {
			s, r := auctionFixture(t)
			ctx := context.Background()
			reconcile(t, r, 100)
			lock, e := s.DB.Begin()
			if e != nil {
				t.Fatal(e)
			}
			defer lock.Rollback()
			if _, e = lock.Exec(`SELECT lot_id FROM commerce_lot_auctions WHERE lot_id='l' FOR UPDATE`); e != nil {
				t.Fatal(e)
			}
			var mu sync.Mutex
			ms := int64(150)
			clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return time.UnixMilli(ms) }
			done := make(chan error, 1)
			go func() { _, e := r.Place(ctx, "a", bid("l", 0, 1), clock); done <- e }()
			deadline := time.Now().Add(time.Second)
			for {
				var waits bool
				if e = s.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%FOR UPDATE OF a%')`).Scan(&waits); e != nil {
					t.Fatal(e)
				}
				if waits {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("bid never waited")
				}
				time.Sleep(time.Millisecond)
			}
			mu.Lock()
			if expire {
				ms = 10000
			} else {
				ms = 200
			}
			mu.Unlock()
			if e = lock.Commit(); e != nil {
				t.Fatal(e)
			}
			e = <-done
			if expire {
				wantAuctionCode(t, e, "access_required")
			} else {
				wantAuctionCode(t, e, "closed")
			}
			if snapshot(t, r, "l", ms).BidCount != 0 {
				t.Fatal("late bid accepted")
			}
		})
	}
}
func TestAuctionHistoryPaginationAliasesAndRecovery(t *testing.T) {
	s, r := auctionFixture(t)
	ctx := context.Background()
	for n := 1; n <= 55; n++ {
		digest := "a"
		if n%2 == 0 {
			digest = "b"
		}
		if _, e := r.Place(ctx, digest, bid("l", int64(n*5), n), at(150)); e != nil {
			t.Fatal(e)
		}
	}
	snap := snapshot(t, r, "l", 150)
	if len(snap.History) != 50 || !snap.HistoryHasMore || snap.History[0].Sequence != 55 || snap.History[49].Sequence != 6 {
		t.Fatal(snap)
	}
	h, e := r.History(ctx, "l", 6)
	if e != nil || h.HasMore || len(h.Bids) != 5 || h.Bids[0].Sequence != 5 {
		t.Fatal(h, e)
	}
	p, e := r.Participation(ctx, "a", "", at(150))
	if e != nil || len(p) != 1 || len(p[0].Bids) != 28 {
		t.Fatal(p, e)
	}
	cfg := Config{URL: os.Getenv("TEST_DATABASE_URL"), Schema: s.SchemaName}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := OpenConfig(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	r = AuctionRepository{reopened}
	reconcile(t, r, 500)
	snap = snapshot(t, r, "l", 500)
	if snap.State != "sold" || snap.Result.Winner.Sequence != 55 {
		t.Fatal(snap)
	}
	replay, e := r.Place(ctx, "a", bid("l", 275, 55), at(500))
	if e != nil || !replay.Replay {
		t.Fatal(replay, e)
	}
}

func TestAuctionRollbackCancellationAndOwnershipLoss(t *testing.T) {
	s, r := auctionFixture(t)
	ctx := context.Background()
	reconcile(t, r, 100)
	_, e := s.DB.Exec(`ALTER TABLE commerce_lot_auctions ADD CONSTRAINT reject_bid CHECK(bid_count=0)`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Place(ctx, "a", bid("l", 0, 1), at(150)); e == nil {
		t.Fatal("injected failure accepted")
	}
	snap := snapshot(t, r, "l", 150)
	if snap.BidCount != 0 || len(snap.History) != 0 || snap.Leader != nil {
		t.Fatal("partial bid", snap)
	}
	_, e = s.DB.Exec(`ALTER TABLE commerce_lot_auctions DROP CONSTRAINT reject_bid`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Place(ctx, "a", bid("l", 0, 1), at(150)); e != nil {
		t.Fatal(e)
	}
	_, e = s.DB.Exec(`ALTER TABLE commerce_auction_results ADD CONSTRAINT reject_close CHECK(lot_id<>'l' OR recorded_at=0)`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Reconcile(ctx, at(200)); e == nil {
		t.Fatal("injected close accepted")
	}
	snap = snapshot(t, r, "l", 200)
	if snap.State != "closing" || snap.Result != nil || snap.BidCount != 1 {
		t.Fatal("partial close", snap)
	}
	if _, e = s.DB.Exec(`ALTER TABLE commerce_auction_results DROP CONSTRAINT reject_close`); e != nil {
		t.Fatal(e)
	}
	s.writerMu.Lock()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, e = r.Place(cancelled, "b", bid("l", 5, 2), at(150))
	s.writerMu.Unlock()
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	// Lost leases are never replaced. Terminate only this fixture's own backend.
	var pid int
	if e = s.writer.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(`SELECT pg_terminate_backend($1)`, pid); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if _, e = r.Place(ctx, "b", bid("l", 5, 2), at(150)); e == nil {
			t.Fatal("lost writer accepted bid")
		}
	}
	if _, e = s.Ready(ctx); e == nil {
		t.Fatal("lost writer ready")
	}
	if !sanitize("auction commit", errors.New("uncertain")).(safeError).CommitUncertain() {
		t.Fatal("unknown commit not classified")
	}
}
func TestAuctionParticipationOrderAndWorker(t *testing.T) {
	s, r := auctionFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, e := r.Place(ctx, "a", bid("l", 0, 1), at(100)); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Place(ctx, "a", bid("other", 12550, 2), at(150)); e != nil {
		t.Fatal(e)
	}
	p, e := r.Participation(ctx, "a", "", at(150))
	if e != nil || len(p) != 2 || p[0].LotID != "other" {
		t.Fatal(p, e)
	}
	p, e = r.Participation(ctx, "a", "l", at(200))
	if e != nil || p[0].Status != "closing" {
		t.Fatal(p, e)
	}
	done := make(chan struct{})
	go func() { defer close(done); auction.New(r, at(250)).Run(ctx, func(e error) { t.Error(e) }) }()
	deadline := time.Now().Add(time.Second)
	for snapshot(t, r, "l", 250).State != "sold" {
		if time.Now().After(deadline) {
			t.Fatal("worker failed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not join")
	}
	// Resetting retained game storage cannot remove commerce bids or results.
	if _, e = s.DB.Exec(`DELETE FROM game; DELETE FROM bids; DELETE FROM players`); e != nil {
		t.Fatal(e)
	}
	if snapshot(t, r, "l", 250).Result.Winner.AmountMinor != 0 {
		t.Fatal("game reset changed auction")
	}
}
