package store

import (
	"auctionlab/internal/account"
	"context"
	"testing"
	"time"
)

func TestAccountSeedAtomicRepeat(t *testing.T) {
	st, s, _ := accountFixture(t)
	ctx := context.Background()
	passwords := [3]string{"seed bidder one password", "seed bidder two password", "seed operator password"}
	if _, e := s.Provision(ctx, [3]string{}); account.Code(e) != "seed_credentials_required" {
		t.Fatal("missing credentials")
	}
	if done, e := s.Provision(ctx, passwords); e != nil || done {
		t.Fatal("first seed", e)
	}
	ids := []string{}
	for i, email := range []string{"bidder-one@demo.auctionlab.test", "bidder-two@demo.auctionlab.test", "operator@demo.auctionlab.test"} {
		p, _, e := s.Login(ctx, email, passwords[i], "")
		role := "bidder"
		if i == 2 {
			role = "admin"
		}
		if e != nil || p.Role != role || !p.Demo {
			t.Fatal("seed login", e)
		}
		ids = append(ids, p.ID)
	}
	watchCatalog(t, st)
	_, token, e := s.Login(ctx, "bidder-one@demo.auctionlab.test", passwords[0], "")
	if e != nil {
		t.Fatal(e)
	}
	r := AccountRepository{st}
	yes := true
	r.Membership(ctx, account.Digest(token), "a", &yes, time.Now().UnixMilli())
	if done, e := s.Provision(ctx, [3]string{}); e != nil || !done {
		t.Fatal("seed repeat", e)
	}
	lots, e := r.Watchlist(ctx, account.Digest(token), time.Now().UnixMilli())
	if e != nil || len(lots) != 1 {
		t.Fatal("seed changed session/watchlist")
	}
	for i, email := range []string{"bidder-one@demo.auctionlab.test", "bidder-two@demo.auctionlab.test", "operator@demo.auctionlab.test"} {
		p, _, e := s.Login(ctx, email, passwords[i], "")
		if e != nil || p.ID != ids[i] {
			t.Fatal("seed changed identity/password")
		}
	}
}
func TestAccountSeedCollisionRollback(t *testing.T) {
	st, s, _ := accountFixture(t)
	ctx := context.Background()
	if _, _, e := s.Register(ctx, "bidder-two@demo.auctionlab.test", "preexisting password", "Existing", ""); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Provision(ctx, [3]string{"seed one password", "seed two password", "seed admin password"}); account.Code(e) != "duplicate_email" {
		t.Fatal("collision", e)
	}
	var n int
	st.DB.QueryRow(`SELECT count(*) FROM commerce_accounts`).Scan(&n)
	if n != 1 {
		t.Fatal("partial seed")
	}
	st.DB.QueryRow(`SELECT count(*) FROM commerce_account_seed_runs`).Scan(&n)
	if n != 0 {
		t.Fatal("partial marker")
	}
}
