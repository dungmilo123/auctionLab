package api

import (
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

func operatorRequest(h http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://example.test"+path, strings.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if method != "GET" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-AuctionLab-Admin", "1")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
func TestAdminOrderHTTPBoundaryAndHistory(t *testing.T) {
	mux, auctions, st, _, clock := commerceAPI(t)
	svc := order.New(store.OrderRepository{Store: st}, auctions.Now)
	RegisterOrders(mux, "DISTINCT-CAPABILITY", svc)
	buyer := registerTestAccount(t, mux, "buyer-private@example.test")
	foreign := registerTestAccount(t, mux, "foreign@example.test")
	capability := &http.Cookie{Name: AdminCookieName, Value: "DISTINCT-CAPABILITY"}
	if w := accountRequest(mux, "POST", "/api/account/bids", `{"lot_id":"l","amount_minor":12550,"request_id":"00000000-0000-0000-0000-000000000001"}`, buyer); w.Code != 201 {
		t.Fatal(w.Code)
	}
	clock.Store(200)
	if e := auctions.Reconcile(context.Background()); e != nil {
		t.Fatal(e)
	}
	var id string
	st.DB.QueryRow(`SELECT id FROM commerce_orders`).Scan(&id)
	path := "/api/admin/orders/" + id
	for _, p := range []string{"/api/admin/orders", path, path + "/unknown/deep"} {
		for _, cookie := range []*http.Cookie{nil, buyer, foreign} {
			w := operatorRequest(mux, "GET", p, "", cookie)
			if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(p, w.Code)
			}
		}
	}
	if w := accountRequest(mux, "GET", "/api/account/orders", "", capability); w.Code != 401 {
		t.Fatal("capability impersonated", w.Code)
	}
	if w := accountRequest(mux, "GET", "/api/account/orders?owner_id=other", "", buyer); w.Code != 422 {
		t.Fatal(w.Code)
	}
	if w := accountRequest(mux, "GET", "/api/account/orders/"+id, "", foreign); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := operatorRequest(mux, "GET", path+"/unknown", "", capability); w.Code != 404 {
		t.Fatal(w.Code)
	}
	for _, q := range []string{"status=bad", "method=", "status=all&status=paid", "view=" + id, "owner_id=other", "status=%zz"} {
		if w := operatorRequest(mux, "GET", "/api/admin/orders?"+q, "", capability); w.Code != 422 {
			t.Fatal(q, w.Code)
		}
	}
	if w := operatorRequest(mux, "POST", path+"/transition", `{"expected_version":1,"next_status":"preparing"}`, capability); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	selection := `{"contact":{"name":"CONTACT-MARKER","email":"PII@example.test","phone":"123"},"fulfillment_method":"shipping","shipping_address":{"line1":"ADDRESS-MARKER","line2":"","locality":"City","region":"","postal_code":"","country":"VN"},"payment_method":"demo-card","request_id":"00000000-0000-0000-0000-000000000001"}`
	if w := accountRequest(mux, "POST", "/api/account/orders/"+id+"/confirm", selection, buyer); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	valid := `{"expected_version":1,"next_status":"preparing"}`
	for _, tc := range []struct {
		body   string
		status int
	}{{"null", 400}, {"[]", 400}, {strings.Repeat(" ", 8193) + valid, 400}, {`{"expected_version":1,"next_status":"preparing","total_minor":1}`, 400}, {`{"expected_version":1,"next_status":"preparing","owner_id":"x"}`, 400}, {`{"expected_version":1,"next_status":"preparing","contact":{}}`, 400}, {`{"expected_version":0,"next_status":"preparing"}`, 422}, {`{"expected_version":2,"next_status":"preparing"}`, 409}, {`{"expected_version":1,"next_status":"completed"}`, 409}, {`{"expected_version":1,"next_status":"preparing","shipment":{"carrier":"x","tracking_reference":"y"}}`, 422}} {
		w := operatorRequest(mux, "POST", path+"/transition", tc.body, capability)
		if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
	for _, kind := range []string{"origin", "header", "content"} {
		req := httptest.NewRequest("POST", "http://example.test"+path+"/transition", strings.NewReader(valid))
		req.AddCookie(capability)
		req.Header.Set("X-AuctionLab-Admin", "1")
		req.Header.Set("Content-Type", "application/json")
		want := 403
		switch kind {
		case "origin":
			req.Header.Set("Origin", "http://evil.test")
		case "header":
			req.Header.Del("X-AuctionLab-Admin")
		case "content":
			req.Header.Set("Content-Type", "text/plain")
			want = 415
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatal(kind, w.Code)
		}
	}
	for _, p := range []string{"/api/account/orders", "/api/admin/orders"} {
		cookie := buyer
		if p == "/api/admin/orders" {
			cookie = capability
		}
		w := operatorRequest(mux, "GET", p, "", cookie)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		for _, secret := range []string{"CONTACT-MARKER", "ADDRESS-MARKER", "pii@example.test", "receipt", "selection"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("summary privacy", secret)
			}
		}
	}
	w := operatorRequest(mux, "POST", path+"/transition", valid, capability)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var admin struct{ Data order.Order }
	json.Unmarshal(w.Body.Bytes(), &admin)
	owner := accountRequest(mux, "GET", "/api/account/orders/"+id, "", buyer)
	var owned struct{ Data order.Order }
	json.Unmarshal(owner.Body.Bytes(), &owned)
	if stringMustJSON(admin.Data) != stringMustJSON(owned.Data) {
		t.Fatal("owner/admin differ")
	}
	// Version never silently retries a duplicate request.
	if w := operatorRequest(mux, "POST", path+"/transition", valid, capability); w.Code != 409 {
		t.Fatal(w.Code)
	}
}
func stringMustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

type lostTransitionResponse struct{ store.OrderRepository }

func (r lostTransitionResponse) Transition(ctx context.Context, id string, t order.Transition, now func() time.Time) (order.Order, error) {
	if _, e := r.OrderRepository.Transition(ctx, id, t, now); e != nil {
		return order.Order{}, e
	}
	return order.Order{}, unknownCommit{}
}
func TestFulfillmentUnknownCommitHTTPRecovery(t *testing.T) {
	mux, auctions, st, _, clock := commerceAPI(t)
	buyer := registerTestAccount(t, mux, "unknown-fulfillment@example.test")
	accountRequest(mux, "POST", "/api/account/bids", `{"lot_id":"l","amount_minor":0,"request_id":"00000000-0000-0000-0000-000000000001"}`, buyer)
	clock.Store(200)
	if e := auctions.Reconcile(context.Background()); e != nil {
		t.Fatal(e)
	}
	var id string
	st.DB.QueryRow(`SELECT id FROM commerce_orders`).Scan(&id)
	selection := `{"contact":{"name":"Private","email":"private@example.test","phone":"123"},"fulfillment_method":"pickup","payment_method":"demo-card","request_id":"00000000-0000-0000-0000-000000000001"}`
	if w := accountRequest(mux, "POST", "/api/account/orders/"+id+"/confirm", selection, buyer); w.Code != 200 {
		t.Fatal(w.Code)
	}
	RegisterOrders(mux, "capability", order.New(lostTransitionResponse{store.OrderRepository{Store: st}}, auctions.Now))
	cookie := &http.Cookie{Name: AdminCookieName, Value: "capability"}
	path := "/api/admin/orders/" + id
	w := operatorRequest(mux, "POST", path+"/transition", `{"expected_version":1,"next_status":"ready_for_pickup"}`, cookie)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "commit_unknown") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = operatorRequest(mux, "GET", path, "", cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"ready_for_pickup"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = operatorRequest(mux, "POST", path+"/transition", `{"expected_version":1,"next_status":"ready_for_pickup"}`, cookie); w.Code != 409 {
		t.Fatal(w.Code)
	}
}
