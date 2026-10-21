package api

import (
	"auctionlab/internal/account"
	"auctionlab/internal/order"
	"auctionlab/internal/store"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheckoutHTTPBoundariesPrivacyAndRecovery(t *testing.T) {
	mux, svc, st, _, clock := commerceAPI(t)
	winner := registerTestAccount(t, mux, "buyer@example.test")
	loser := registerTestAccount(t, mux, "loser@example.test")
	if w := accountRequest(mux, "POST", "/api/account/bids", `{"lot_id":"l","amount_minor":12550,"request_id":"00000000-0000-0000-0000-000000000001"}`, winner); w.Code != 201 {
		t.Fatal(w.Code)
	}
	clock.Store(200)
	if e := svc.Reconcile(context.Background()); e != nil {
		t.Fatal(e)
	}
	w := accountRequest(mux, "GET", "/api/account/wins", "", winner)
	var wins struct{ Data []order.Order }
	if e := json.Unmarshal(w.Body.Bytes(), &wins); e != nil || len(wins.Data) != 1 {
		t.Fatal(w.Code, e)
	}
	id := wins.Data[0].ID
	path := "/api/account/orders/" + id
	selection := `{"contact":{"name":"PII-MARKER","email":"private@example.test","phone":"123"},"fulfillment_method":"shipping","shipping_address":{"line1":"ADDRESS-MARKER","line2":"","locality":"City","region":"","postal_code":"","country":"VN"},"payment_method":"demo-card"}`
	confirm := strings.TrimSuffix(selection, "}") + `,"request_id":"00000000-0000-0000-0000-000000000001"}`
	for _, tc := range []struct {
		method, path, body string
		cookie             *http.Cookie
		status             int
	}{
		{"GET", path, "", nil, 401}, {"GET", path, "", loser, 404}, {"GET", "/api/account/orders/missing", "", loser, 404}, {"GET", "/api/account/orders", "", winner, 404},
		{"POST", path + "/review", selection, winner, 200}, {"POST", path + "/review", strings.Replace(selection, `"name":"PII-MARKER"`, `"name":""`, 1), winner, 422},
		{"POST", path + "/confirm", strings.TrimSuffix(confirm, "}") + `,"total_minor":0}`, winner, 400}, {"POST", path + "/confirm", strings.TrimSuffix(confirm, "}") + `,"owner_id":"other"}`, winner, 400},
		{"POST", path + "/confirm", strings.TrimSuffix(confirm, "}") + `,"card_number":"123"}`, winner, 400}, {"POST", path + "/confirm", "null", winner, 400}, {"POST", path + "/review", `{"contact":null}`, winner, 400}, {"POST", path + "/confirm", strings.Repeat(" ", 8193) + confirm, winner, 400},
	} {
		w := accountRequest(mux, tc.method, tc.path, tc.body, tc.cookie)
		if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(tc.path, w.Code, w.Body.String())
		}
	}
	var n int
	st.DB.QueryRow(`SELECT count(*) FROM commerce_demo_payments`).Scan(&n)
	if n != 0 {
		t.Fatal("review mutated")
	}
	for _, header := range []string{"Origin", "X-AuctionLab-Account"} {
		req := httptest.NewRequest("POST", "http://example.test"+path+"/confirm", strings.NewReader(confirm))
		req.AddCookie(winner)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-AuctionLab-Account", "1")
		if header == "Origin" {
			req.Header.Set(header, "http://foreign.test")
		} else {
			req.Header.Del(header)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	for _, cookie := range []*http.Cookie{{Name: "auction_admin", Value: "DISTINCT-CAPABILITY"}, {Name: "token", Value: "game-token"}} {
		if w := accountRequest(mux, "GET", path, "", cookie); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	// The commerce admin role is still the winning owner, with no special purchase access.
	if _, e := st.DB.Exec(`UPDATE commerce_accounts SET role='admin' WHERE email='buyer@example.test'`); e != nil {
		t.Fatal(e)
	}
	w = accountRequest(mux, "POST", path+"/confirm", confirm, winner)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var first struct{ Data order.Confirmation }
	json.Unmarshal(w.Body.Bytes(), &first)
	if first.Data.Order.Receipt == nil || first.Data.Order.Receipt.TotalMinor != 14050 {
		t.Fatal("wrong receipt")
	}
	w = accountRequest(mux, "POST", path+"/confirm", confirm, winner)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"replay":true`) {
		t.Fatal(w.Code)
	}
	w = accountRequest(mux, "POST", path+"/confirm", strings.Replace(confirm, "demo-card", "demo-bank-transfer", 1), winner)
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
	w = accountRequest(mux, "POST", path+"/review", selection, winner)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"paid"`) {
		t.Fatal(w.Code)
	}
	w = accountRequest(mux, "GET", "/api/account/wins", "", winner)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatal(w.Code)
	}
	for _, p := range []string{"/api/auctions/lots/l", "/api/auctions/lots/l/bids"} {
		w := accountRequest(mux, "GET", p, "", nil)
		for _, marker := range []string{"PII-MARKER", "ADDRESS-MARKER", "private@example.test", id, first.Data.Order.Receipt.ID} {
			if strings.Contains(w.Body.String(), marker) {
				t.Fatal("public purchase leak")
			}
		}
	}
	w = accountRequest(mux, "GET", path, "", winner)
	if w.Code != 200 || !strings.Contains(w.Body.String(), first.Data.Order.Receipt.ID) {
		t.Fatal("lost-response readback")
	}
}

// Return an uncertain outcome even after a real durable PostgreSQL commit.
// This simulates a dropped commit response without weakening production storage.
type lostCheckoutResponse struct{ store.OrderRepository }

func (r lostCheckoutResponse) Confirm(ctx context.Context, digest, id string, i order.Intent, now func() time.Time) (order.Confirmation, error) {
	if _, e := r.OrderRepository.Confirm(ctx, digest, id, i, now); e != nil {
		return order.Confirmation{}, e
	}
	return order.Confirmation{}, unknownCommit{}
}
func TestCheckoutUnknownCommitAndDurableRecovery(t *testing.T) {
	mux, auctions, st, _, clock := commerceAPI(t)
	cookie := registerTestAccount(t, mux, "unknown-buyer@example.test")
	w := accountRequest(mux, "POST", "/api/account/bids", `{"lot_id":"l","amount_minor":0,"request_id":"00000000-0000-0000-0000-000000000001"}`, cookie)
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
	clock.Store(200)
	if e := auctions.Reconcile(context.Background()); e != nil {
		t.Fatal(e)
	}
	var id string
	if e := st.DB.QueryRow(`SELECT id FROM commerce_orders`).Scan(&id); e != nil {
		t.Fatal(e)
	}
	accounts := account.New(store.AccountRepository{Store: st})
	accounts.Now = auctions.Now
	h := &AccountAPI{Service: accounts, Orders: order.New(lostCheckoutResponse{store.OrderRepository{Store: st}}, auctions.Now)}
	payload := `{"contact":{"name":"Buyer","email":"fixture@example.test","phone":"123"},"fulfillment_method":"pickup","payment_method":"demo-bank-transfer","request_id":"00000000-0000-0000-0000-000000000001"}`
	path := "/api/account/orders/" + id
	w = accountRequest(h, "POST", path+"/confirm", payload, cookie)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "commit_unknown") || strings.Contains(w.Body.String(), "sentinel") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = accountRequest(mux, "GET", path, "", cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"paid"`) {
		t.Fatal(w.Code)
	}
	w = accountRequest(mux, "POST", path+"/confirm", payload, cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"replay":true`) {
		t.Fatal(w.Code)
	}
	var n int
	st.DB.QueryRow(`SELECT count(*) FROM commerce_demo_payments`).Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
}
