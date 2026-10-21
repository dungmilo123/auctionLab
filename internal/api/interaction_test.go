package api

import (
	"auctionlab/internal/interaction"
	"auctionlab/internal/store"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestViewEndpointBoundaries(t *testing.T) {
	mux, _, s := accountAPI(t)
	RegisterInteractions(mux, interaction.Service{Repo: store.InteractionRepository{Store: s}, Now: func() time.Time { return time.UnixMilli(100) }})
	_, e := s.DB.Exec(`INSERT INTO catalog_lots(id,slug,title,state,created_at,updated_at) VALUES('l','lot','Lot','published',0,0),('d','draft','Draft','draft',0,0);INSERT INTO catalog_events(id,slug,title,state,created_at,updated_at) VALUES('e','event','Event','published',0,0);INSERT INTO catalog_event_lots VALUES('e','l',0),('e','d',1)`)
	if e != nil {
		t.Fatal(e)
	}
	body := `{"lot_id":"l","observation_id":"00000000-0000-0000-0000-000000000001"}`
	cases := []struct {
		body, header, origin, typ string
		want                      int
	}{{body, "1", "http://example.test", "application/json", 200}, {body, "1", "http://example.test", "application/json", 200}, {`{"lot_id":"d","observation_id":"00000000-0000-0000-0000-000000000001"}`, "1", "", "application/json", 404}, {`{"lot_id":"l","observation_id":"BAD"}`, "1", "", "application/json", 422}, {`{"lot_id":1,"observation_id":true}`, "1", "", "application/json", 422}, {body, "", "", "application/json", 403}, {body, "1", "http://evil.test", "application/json", 403}, {body, "1", "", "text/plain", 415}, {`{"lot_id":"\u0000","observation_id":"00000000-0000-0000-0000-000000000001"}`, "1", "", "application/json", 422}, {`null`, "1", "", "application/json", 400}, {`[]`, "1", "", "application/json", 400}, {`{} {}`, "1", "", "application/json", 400}, {`{"lot_id":"l","observation_id":"00000000-0000-0000-0000-000000000001","actor_id":"a"}`, "1", "", "application/json", 400}, {strings.Repeat(" ", 1025), "1", "", "application/json", 413}, {`{"lot_id":"l","lot_id":"l","observation_id":"00000000-0000-0000-0000-000000000001"}`, "1", "", "application/json", 400}}
	for i, c := range cases {
		r := httptest.NewRequest("POST", "http://example.test/api/catalog/lot-views", strings.NewReader(c.body))
		r.RemoteAddr = fmt.Sprintf("192.0.2.%d:1", i)
		r.Header.Set("X-AuctionLab-Interaction", c.header)
		r.Header.Set("Origin", c.origin)
		r.Header.Set("Content-Type", c.typ)
		r.Header.Set("X-Forwarded-For", "arbitrary")
		r.AddCookie(&http.Cookie{Name: AccountCookieName, Value: "ignored"})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != c.want || w.Header().Get("Cache-Control") != "no-store" || len(w.Result().Cookies()) != 0 {
			t.Fatal(i, w.Code, w.Body.String())
		}
	}
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM commerce_interactions WHERE actor_id IS NULL AND actor_demo IS NULL AND kind='lot_view'`).Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
	for i := 0; i < 31; i++ {
		r := httptest.NewRequest("POST", "http://example.test/api/catalog/lot-views", strings.NewReader(body))
		r.RemoteAddr = "192.0.2.240:1"
		r.Header.Set("X-AuctionLab-Interaction", "1")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Forwarded-For", fmt.Sprint(i))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if i == 30 && w.Code != 429 {
			t.Fatal("forwarding bypass", w.Code)
		}
	}
}

type failedObservation struct{}

func (failedObservation) Observe(context.Context, string, string, int64) error {
	return fmt.Errorf("private storage sentinel")
}
func TestViewFailureSanitized(t *testing.T) {
	mux := http.NewServeMux()
	RegisterInteractions(mux, interaction.Service{Repo: failedObservation{}, Now: time.Now})
	r := httptest.NewRequest("POST", "/api/catalog/lot-views", strings.NewReader(`{"lot_id":"l","observation_id":"00000000-0000-0000-0000-000000000001"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-AuctionLab-Interaction", "1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 503 || strings.Contains(w.Body.String(), "sentinel") {
		t.Fatal(w.Code, w.Body.String())
	}
}
