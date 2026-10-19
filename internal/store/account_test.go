package store

import (
	"auctionlab/internal/account"
	"auctionlab/internal/testpg"
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func accountFixture(t *testing.T) (*Store, *account.Service, string) {
	t.Helper()
	raw := testpg.NewURL(t)
	s, e := Open(raw)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	if e = s.Migrate(); e != nil {
		t.Fatal(e)
	}
	return s, account.New(AccountRepository{s}), raw
}
func TestAccountsDurableSessions(t *testing.T) {
	st, s, raw := accountFixture(t)
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	s.Now = func() time.Time { return now }
	p, token, e := s.Register(ctx, " A@example.test ", "unicode password 密碼", "A", "")
	if e != nil || p.Role != "bidder" || p.Demo {
		t.Fatal("registration failed", e)
	}
	repo := AccountRepository{st}
	get := func(tok string, want bool) {
		t.Helper()
		got, e := repo.Profile(ctx, account.Digest(tok), now.UnixMilli())
		if (e == nil) != want || (want && got.ID != p.ID) {
			t.Fatal("session result", e)
		}
	}
	get(token, true)
	var created, expires int64
	st.DB.QueryRow(`SELECT created_at,expires_at FROM commerce_sessions`).Scan(&created, &expires)
	if expires-created != account.SessionLifetime.Milliseconds() {
		t.Fatal("expiry")
	}
	_, other, e := s.Login(ctx, p.Email, "unicode password 密碼", "")
	if e != nil {
		t.Fatal(e)
	}
	_, next, e := s.Login(ctx, p.Email, "unicode password 密碼", token)
	if e != nil || next == token {
		t.Fatal(e)
	}
	get(token, false)
	get(other, true)
	get(next, true)
	if _, e = repo.Update(ctx, account.Digest(token), "No", now.UnixMilli()); account.Code(e) != "access_required" {
		t.Fatal(e)
	}
	if _, e = repo.Update(ctx, account.Digest(next), "Changed", now.UnixMilli()); e != nil {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, e = s.Register(cancelled, "cancel@example.test", "cancelled password", "Cancel", ""); e == nil {
		t.Fatal("cancel accepted")
	}
	if e = repo.Logout(ctx, account.Digest(next), now.UnixMilli()); e != nil {
		t.Fatal(e)
	}
	get(next, false)
	get(other, true)
	if e = repo.Logout(ctx, account.Digest(next), now.UnixMilli()); e != nil {
		t.Fatal(e)
	}
	if e = st.Close(); e != nil {
		t.Fatal(e)
	}
	fresh, e := Open(raw)
	if e != nil {
		t.Fatal(e)
	}
	defer fresh.Close()
	repo = AccountRepository{fresh}
	get(other, true)
	get(next, false)
	got, e := repo.Profile(ctx, account.Digest(other), now.UnixMilli())
	if e != nil || got.DisplayName != "Changed" {
		t.Fatal(e)
	}
	now = now.Add(account.SessionLifetime)
	get(other, false)
}
func TestAccountAtomicConcurrentRegistration(t *testing.T) {
	st, s, _ := accountFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			_, _, e := s.Register(ctx, "race@example.test", "concurrent password", "Race", "")
			results <- e
		})
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if account.Code(e) == "duplicate_email" {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	st.DB.Exec(`ALTER TABLE commerce_sessions ADD CONSTRAINT injected CHECK(false) NOT VALID`)
	if _, token, e := s.Register(ctx, "rollback@example.test", "transaction password", "Rollback", ""); e == nil || token != "" {
		t.Fatal("partial success")
	}
	var n int
	st.DB.QueryRow(`SELECT count(*) FROM commerce_accounts WHERE email='rollback@example.test'`).Scan(&n)
	if n != 0 {
		t.Fatal("partial account")
	}
}
func TestAccountLostWriterAndClassification(t *testing.T) {
	st, s, _ := accountFixture(t)
	ctx := context.Background()
	st.lost = true
	if _, token, e := s.Register(ctx, "lost@example.test", "lost writer password", "Lost", ""); e == nil || token != "" || !errors.Is(e, ErrWriterLost) {
		t.Fatal("ownership ignored")
	}
	for i := 0; i < 2; i++ {
		if e := (AccountRepository{st}).Logout(ctx, "digest", time.Now().UnixMilli()); !errors.Is(e, ErrWriterLost) {
			t.Fatal("reacquired")
		}
	}
	uncertain := sanitize("account write", sanitize("account commit", errors.New("private failure")))
	var c interface{ CommitUncertain() bool }
	if !errors.As(uncertain, &c) || !c.CommitUncertain() {
		t.Fatal("classification lost")
	}
	if uncertain.Error() != "store: account write failed" {
		t.Fatal("unsafe error")
	}
}

func TestAccountCancellationWhileWriterQueued(t *testing.T) {
	st, _, _ := accountFixture(t)
	st.writerMu.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- (AccountRepository{st}).Logout(ctx, "digest", time.Now().UnixMilli()) }()
	select {
	case e := <-done:
		if !errors.Is(e, context.DeadlineExceeded) {
			t.Error("cancel classification")
		}
	case <-time.After(time.Second):
		t.Error("cancelled request waited for writer")
	}
	st.writerMu.Unlock()
	if e := (AccountRepository{st}).Logout(context.Background(), "digest", time.Now().UnixMilli()); e != nil {
		t.Fatal("cancel damaged lease", e)
	}
}
func TestAccountPhysicalWriterLoss(t *testing.T) {
	st, s, _ := accountFixture(t)
	var pid int
	if e := st.writer.QueryRowContext(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
		t.Fatal(e)
	}
	var stopped bool
	if e := st.DB.QueryRow(`SELECT pg_terminate_backend($1)`, pid).Scan(&stopped); e != nil || !stopped {
		t.Fatal("could not terminate fixture writer")
	}
	for i := 0; i < 2; i++ {
		if _, tok, e := s.Register(context.Background(), "lease-loss@example.test", "fixture loss password", "Lost", ""); e == nil || tok != "" {
			t.Fatal("lost writer accepted mutation")
		}
	}
	var n int
	st.DB.QueryRow(`SELECT count(*) FROM commerce_accounts`).Scan(&n)
	if n != 0 {
		t.Fatal("writer reacquired")
	}
}
