package store

import (
	"auctionlab/internal/account"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func watchCatalog(t *testing.T, st *Store) {
	t.Helper()
	_, e := st.DB.Exec(`INSERT INTO catalog_lots(id,slug,title,state,admin_notes,created_at,updated_at,starting_price_minor) VALUES('a','a','A','published','secret-notes',1,1,100),('b','b','B','published','secret-notes',1,1,200),('c','c','C','published','secret-notes',1,1,300),('draft','draft','Draft','draft','secret-notes',1,1,400);
 INSERT INTO catalog_events(id,slug,title,state,opens_at,first_closes_at,close_interval_ms,created_at,updated_at) VALUES('e','e','Event','published',1,1000,100,1,1);
 INSERT INTO catalog_event_lots VALUES('e','a',0),('e','b',1),('e','c',2);`)
	if e != nil {
		t.Fatal(e)
	}
}
func TestWatchlistMembership(t *testing.T) {
	st, s, _ := accountFixture(t)
	watchCatalog(t, st)
	ctx := context.Background()
	_, tok, e := s.Register(ctx, "watch@example.test", "watch password long", "Watch", "")
	if e != nil {
		t.Fatal(e)
	}
	r := AccountRepository{st}
	d := account.Digest(tok)
	yes, no := true, false
	for _, id := range []string{"draft", "missing"} {
		if _, e = r.Membership(ctx, d, id, &yes, 100); account.Code(e) != "not_found" {
			t.Fatal(e)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			if v, e := r.Membership(ctx, d, "a", &yes, 200); e != nil || !v {
				t.Error("save", e)
			}
		})
	}
	wg.Wait()
	if _, e = r.Membership(ctx, d, "a", &yes, 300); e != nil {
		t.Fatal(e)
	}
	var n int
	var saved int64
	st.DB.QueryRow(`SELECT count(*),min(saved_at) FROM commerce_watchlist`).Scan(&n, &saved)
	if n != 1 || saved != 200 {
		t.Fatal("repeat changed saved time")
	}
	for i := 0; i < 2; i++ {
		if v, e := r.Membership(ctx, d, "a", &no, 400); e != nil || v {
			t.Fatal(e)
		}
	}
	if _, e = r.Membership(ctx, d, "draft", &no, 400); e != nil {
		t.Fatal("draft removal leaked", e)
	}
	if e = r.Logout(ctx, d, 500); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Membership(ctx, d, "a", &yes, 600); account.Code(e) != "access_required" {
		t.Fatal("revoked write")
	}
}
func TestWatchlistSnapshotProjectionAndIsolation(t *testing.T) {
	st, s, _ := accountFixture(t)
	watchCatalog(t, st)
	ctx := context.Background()
	p, a, e := s.Register(ctx, "a@example.test", "account one password", "One", "")
	if e != nil {
		t.Fatal(e)
	}
	_, b, e := s.Register(ctx, "b@example.test", "account two password", "Two", "")
	if e != nil {
		t.Fatal(e)
	}
	r := AccountRepository{st}
	yes := true
	now := time.Now().UnixMilli()
	for _, v := range []struct {
		token, id string
		saved     int64
	}{{a, "a", now}, {a, "c", now + 1}, {a, "b", now + 1}, {b, "a", now}} {
		if _, e = r.Membership(ctx, account.Digest(v.token), v.id, &yes, v.saved); e != nil {
			t.Fatal(e)
		}
	}
	lots, e := r.Watchlist(ctx, account.Digest(a), now+2)
	if e != nil || len(lots) != 3 || lots[0].ID != "b" || lots[1].ID != "c" || lots[2].ID != "a" {
		t.Fatal("order", e)
	}
	raw, _ := json.Marshal(lots)
	for _, secret := range []string{"admin_notes", "secret-notes", "version", "password", "token_digest", p.ID, "saved_at"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("projection leak", secret)
		}
	}
	if lots[0].ClosesAt == nil || *lots[0].ClosesAt != 1100 {
		t.Fatal("elapsed lot not retained")
	}
	other, e := r.Watchlist(ctx, account.Digest(b), now)
	if e != nil || len(other) != 1 {
		t.Fatal("owner isolation")
	}
	st.DB.Exec(`UPDATE catalog_lots SET state='draft' WHERE id='b'`)
	lots, e = r.Watchlist(ctx, account.Digest(a), now)
	if e != nil || len(lots) != 2 {
		t.Fatal("draft included")
	}
	st.DB.Exec(`UPDATE catalog_events SET state='draft'`)
	lots, e = r.Watchlist(ctx, account.Digest(a), now)
	if e != nil || len(lots) != 0 {
		t.Fatal("draft event included")
	}
}
