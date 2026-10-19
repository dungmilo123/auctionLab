package api

import (
	"auctionlab/internal/account"
	"auctionlab/internal/auction"
	"auctionlab/internal/store"
	"auctionlab/internal/testpg"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func commerceAPI(t *testing.T) (*http.ServeMux, *auction.Service, *store.Store, *AuctionAPI, *atomic.Int64) {
	t.Helper()
	st, e := store.Open(testpg.NewURL(t))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	if e = st.Migrate(); e != nil {
		t.Fatal(e)
	}
	clock := &atomic.Int64{}
	clock.Store(150)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	as := account.New(store.AccountRepository{Store: st})
	as.Now = now
	svc := auction.New(store.AuctionRepository{Store: st}, now)
	mux := http.NewServeMux()
	RegisterAccount(mux, as, svc)
	streams := RegisterAuction(mux, svc)
	streams.SampleInterval = 10 * time.Millisecond
	t.Cleanup(streams.Close)
	_, e = st.DB.Exec(`INSERT INTO catalog_lots(id,slug,title,state,starting_price_minor,bid_increment_minor,admin_notes,created_at,updated_at) VALUES('l','lot','Public lot','published',0,5,'DISTINCT-PRIVATE-NOTE',0,0),('draft','draft','DRAFT-SECRET','draft',0,1,'DISTINCT-PRIVATE-NOTE',0,0);
 INSERT INTO catalog_events(id,slug,title,state,opens_at,first_closes_at,close_interval_ms,created_at,updated_at) VALUES('e','event','Event','published',100,200,1,0,0);
 INSERT INTO catalog_event_lots VALUES('e','l',0);
 INSERT INTO server_config VALUES('admin_key','DISTINCT-CAPABILITY');
 INSERT INTO items(id,order_index,name,auction_type,truth_value,start_price) VALUES('game-secret',0,'GAME-SECRET','english',987654321,1);`)
	if e != nil {
		t.Fatal(e)
	}
	return mux, svc, st, streams, clock
}
func TestCommerceHTTPBidBoundaryPrivacyAndIsolation(t *testing.T) {
	mux, svc, st, _, clock := commerceAPI(t)
	a := registerTestAccount(t, mux, "DISTINCT-EMAIL-A@example.test")
	b := registerTestAccount(t, mux, "DISTINCT-EMAIL-B@example.test")
	body := `{"lot_id":"l","amount_minor":0,"request_id":"00000000-0000-0000-0000-000000000001"}`
	for _, tc := range []struct {
		method, path, body string
		cookie             *http.Cookie
		status             int
	}{
		{"GET", "/api/account/bids", "", nil, 401}, {"GET", "/api/account/bids/unknown/deep", "", nil, 401},
		{"POST", "/api/account/bids", body, nil, 401},
		{"POST", "/api/account/bids", `{"lot_id":"l","amount_minor":0,"request_id":"00000000-0000-0000-0000-000000000001","account_id":"other"}`, a, 400},
		{"POST", "/api/account/bids", `{"lot_id":"l","request_id":"00000000-0000-0000-0000-000000000001"}`, a, 422},
		{"POST", "/api/account/bids", `{"lot_id":"l","amount_minor":0.5,"request_id":"00000000-0000-0000-0000-000000000001"}`, a, 422},
		{"POST", "/api/account/bids", strings.Replace(body, `"amount_minor":0`, `"amount_minor":-1`, 1), a, 422},
		{"POST", "/api/account/bids", strings.Replace(body, `"amount_minor":0`, `"amount_minor":1000000000001`, 1), a, 422},
		{"POST", "/api/account/bids", strings.Replace(body, `"l"`, `"draft"`, 1), a, 404},
		{"POST", "/api/account/bids", strings.Replace(body, `"l"`, `"missing"`, 1), a, 404},
		{"POST", "/api/account/bids", body, a, 201}, {"POST", "/api/account/bids", body, a, 200},
		{"POST", "/api/account/bids", strings.Replace(body, `"amount_minor":0`, `"amount_minor":5`, 1), a, 409},
		{"POST", "/api/account/bids", body, b, 409},
		{"POST", "/api/account/bids", strings.Replace(body, `"amount_minor":0`, `"amount_minor":11`, 1), b, 201},
		{"GET", "/api/account/bids", "", a, 200}, {"GET", "/api/account/bids/l", "", b, 200},
		{"GET", "/api/account/bids/unknown/deep", "", a, 404},
		{"GET", "/api/auctions/lots/draft", "", nil, 404}, {"GET", "/api/auctions/lots/missing", "", nil, 404},
		{"GET", "/api/auctions/lots/l/bids?before_sequence=invalid", "", nil, 400},
	} {
		w := accountRequest(mux, tc.method, tc.path, tc.body, tc.cookie)
		if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	for _, header := range []string{"Origin", "X-AuctionLab-Account"} {
		req := httptest.NewRequest("POST", "http://example.test/api/account/bids", strings.NewReader(body))
		req.AddCookie(a)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-AuctionLab-Account", "1")
		if header == "Origin" {
			req.Header.Set(header, "http://evil.test")
		} else {
			req.Header.Del(header)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatal(header, w.Code)
		}
	}
	for _, payload := range []string{`{}`, `null`, `[]`, body + body, `{"lot_id":null}`, strings.Repeat(" ", 8193) + body} {
		w := accountRequest(mux, "POST", "/api/account/bids", payload, a)
		if w.Code < 400 {
			t.Fatal(w.Code)
		}
	}
	for _, path := range []string{"/api/auctions/lots/l", "/api/auctions/lots/l/bids"} {
		w := accountRequest(mux, "GET", path, "", nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, secret := range []string{"DISTINCT-", "GAME-SECRET", "987654321", "request_id", "account_id", "email", "display_name", "admin_notes", "version", "token"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("public leak", secret, w.Body.String())
			}
		}
	}
	pa := accountRequest(mux, "GET", "/api/account/bids/l", "", a)
	if !strings.Contains(pa.Body.String(), `"status":"outbid"`) || !strings.Contains(pa.Body.String(), `"amount_minor":0`) {
		t.Fatal(pa.Body.String())
	}
	clock.Store(200)
	if e := svc.Reconcile(context.Background()); e != nil {
		t.Fatal(e)
	}
	won := accountRequest(mux, "GET", "/api/account/bids/l", "", b)
	if !strings.Contains(won.Body.String(), `"status":"won"`) {
		t.Fatal(won.Body.String())
	}
	if w := accountRequest(mux, "POST", "/api/account/bids", body, a); w.Code != 200 {
		t.Fatal("closed replay", w.Code)
	}
	if _, e := st.DB.Exec(`UPDATE commerce_sessions SET revoked_at=200 WHERE token_digest=$1`, account.Digest(a.Value)); e != nil {
		t.Fatal(e)
	}
	if w := accountRequest(mux, "POST", "/api/account/bids", body, a); w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestCommerceStreamFreshReconnectCapacityAndShutdown(t *testing.T) {
	mux, svc, _, streams, clock := commerceAPI(t)
	a := registerTestAccount(t, mux, "stream@example.test")
	server := httptest.NewServer(mux)
	defer func() { streams.Close(); server.Close() }()
	open := func() (*http.Response, *bufio.Reader) {
		t.Helper()
		res, e := http.Get(server.URL + "/api/auctions/lots/l/stream")
		if e != nil {
			t.Fatal(e)
		}
		if res.StatusCode != 200 {
			t.Fatal(res.StatusCode)
		}
		return res, bufio.NewReader(res.Body)
	}
	next := func(reader *bufio.Reader) auction.Snapshot {
		t.Helper()
		for {
			line, e := reader.ReadString('\n')
			if e != nil {
				t.Fatal(e)
			}
			if strings.HasPrefix(line, "data: ") {
				var b struct {
					Data auction.Snapshot `json:"data"`
				}
				if e = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &b); e != nil {
					t.Fatal(e)
				}
				return b.Data
			}
		}
	}
	res, reader := open()
	initial := next(reader)
	if initial.State != "pending" {
		t.Fatal(initial)
	}
	body := `{"lot_id":"l","amount_minor":0,"request_id":"00000000-0000-0000-0000-000000000001"}`
	if w := accountRequest(mux, "POST", "/api/account/bids", body, a); w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	changed := next(reader)
	if changed.BidCount != 1 || changed.Revision <= initial.Revision {
		t.Fatal(changed)
	}
	clock.Store(200)
	closing := next(reader)
	if closing.State != "closing" {
		t.Fatal(closing)
	}
	if e := svc.Reconcile(context.Background()); e != nil {
		t.Fatal(e)
	}
	sold := next(reader)
	if sold.State != "sold" {
		t.Fatal(sold)
	}
	res.Body.Close()
	again, rr := open()
	if next(rr).Revision != sold.Revision {
		t.Fatal("reconnect did not restore full latest snapshot")
	}
	again.Body.Close()
	// Wait for disconnected clients to release their capacity before reserving test slots.
	deadline := time.Now().Add(time.Second)
	for len(streams.slots) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("disconnect retained stream")
		}
		time.Sleep(time.Millisecond)
	}
	// Hold the configured process capacity without spawning 64 test clients.
	for len(streams.slots) < cap(streams.slots) {
		streams.slots <- struct{}{}
	}
	excess, e := http.Get(server.URL + "/api/auctions/lots/l/stream")
	if e != nil {
		t.Fatal(e)
	}
	if excess.StatusCode != 503 || excess.Header.Get("Retry-After") == "" {
		t.Fatal(excess.StatusCode)
	}
	excess.Body.Close()
	for len(streams.slots) > 0 {
		<-streams.slots
	}
	active, _ := open()
	streams.Close()
	done := make(chan error, 1)
	go func() { _, e := io.Copy(io.Discard, active.Body); done <- e }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream shutdown blocked")
	}
	active.Body.Close()
}

// Deadlines also apply to the first write, so a stalled transport cannot retain a slot.
type blockedStream struct {
	header   http.Header
	deadline time.Time
}

func (w *blockedStream) Header() http.Header                { return w.header }
func (w *blockedStream) WriteHeader(int)                    {}
func (w *blockedStream) Flush()                             {}
func (w *blockedStream) SetWriteDeadline(v time.Time) error { w.deadline = v; return nil }
func (w *blockedStream) Write([]byte) (int, error) {
	time.Sleep(time.Until(w.deadline))
	return 0, fmt.Errorf("write timeout")
}
func TestCommerceStalledStreamWriteBound(t *testing.T) {
	mux, _, _, streams, _ := commerceAPI(t)
	streams.WriteTimeout = 20 * time.Millisecond
	w := &blockedStream{header: http.Header{}}
	start := time.Now()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/auctions/lots/l/stream", nil))
	if time.Since(start) > time.Second || len(streams.slots) != 0 {
		t.Fatal("stalled stream retained resources")
	}
}
