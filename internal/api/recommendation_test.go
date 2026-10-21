package api

import (
	"auctionlab/internal/store"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestRecommendationPrivacyQueriesAndOwners(t *testing.T) {
	_, svc, s := accountAPI(t)
	mux := http.NewServeMux()
	svc.Now = func() time.Time { return time.UnixMilli(100) }
	a := RegisterAccount(mux, svc)
	a.Recommendations = store.RecommendationRepository{Store: s}
	RegisterRecommendations(mux, a.Recommendations, func() time.Time { return time.UnixMilli(100) })
	ca := registerTestAccount(t, mux, "one@example.test")
	cb := registerTestAccount(t, mux, "two@example.test")
	_, e := s.DB.Exec(`INSERT INTO catalog_lots(id,slug,title,state,starting_price_minor,bid_increment_minor,created_at,updated_at) VALUES('l','lot','Lot','published',0,5,0,0),('other','other','Other','published',0,5,0,0),('d','draft','Draft','draft',0,5,0,0);INSERT INTO catalog_events(id,slug,title,state,opens_at,first_closes_at,close_interval_ms,created_at,updated_at) VALUES('e','event','Event','published',200,300,50,0,0);INSERT INTO catalog_event_lots VALUES('e','l',0),('e','other',1),('e','d',2)`)
	if e != nil {
		t.Fatal(e)
	}
	if w := accountRequest(mux, "PUT", "/api/account/watchlist/l", "{}", ca); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, c := range []struct {
		path        string
		cookie      *http.Cookie
		want, count int
	}{{"/api/catalog/recommendations", nil, 200, 2}, {"/api/catalog/recommendations", ca, 200, 2}, {"/api/catalog/recommendations?source=lot", nil, 200, 1}, {"/api/catalog/recommendations?source=draft", nil, 404, 0}, {"/api/catalog/recommendations?source=missing", nil, 404, 0}, {"/api/catalog/recommendations?source=", nil, 422, 0}, {"/api/catalog/recommendations?source=lot&source=other", nil, 422, 0}, {"/api/catalog/recommendations?account_id=one", nil, 422, 0}, {"/api/account/recommendations", nil, 401, 0}, {"/api/account/recommendations", &http.Cookie{Name: AdminCookieName, Value: "capability-only"}, 401, 0}, {"/api/account/recommendations", ca, 200, 1}, {"/api/account/recommendations", cb, 200, 2}} {
		w := accountRequest(mux, "GET", c.path, "", c.cookie)
		if w.Code != c.want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(c.path, w.Code, w.Body.String())
		}
		if c.want == 200 {
			var b struct{ Data []json.RawMessage }
			if e = json.Unmarshal(w.Body.Bytes(), &b); e != nil || len(b.Data) != c.count {
				t.Fatal(c.path, w.Body.String(), e)
			}
		}
	}
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM commerce_lot_auctions`).Scan(&n)
	if n != 0 {
		t.Fatal("recommendation initialized commerce")
	}
}
