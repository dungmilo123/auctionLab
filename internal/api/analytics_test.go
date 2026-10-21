package api

import (
	"auctionlab/internal/analytics"
	"auctionlab/internal/store"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnalyticsCapabilityAndQueryBoundaries(t *testing.T) {
	_, _, s := accountAPI(t)
	mux := http.NewServeMux()
	RegisterAnalytics(mux, "operator-capability", store.AnalyticsRepository{Store: s})
	for _, c := range []struct {
		path, key string
		want      int
	}{{"/api/admin/analytics", "", 401}, {"/api/admin/analytics/unknown", "", 401}, {"/api/admin/analytics", "wrong", 401}, {"/api/admin/analytics", "operator-capability", 200}, {"/api/admin/analytics/unknown", "operator-capability", 404}, {"/api/admin/analytics?dataset=demo&dataset=all", "operator-capability", 422}, {"/api/admin/analytics?event=", "operator-capability", 422}, {"/api/admin/analytics?dataset=", "operator-capability", 422}, {"/api/admin/analytics?unknown=1", "operator-capability", 422}, {"/api/admin/analytics?event=%zz", "operator-capability", 422}, {"/api/admin/analytics?event=missing", "operator-capability", 404}} {
		r := httptest.NewRequest("GET", c.path, nil)
		if c.key != "" {
			r.AddCookie(&http.Cookie{Name: AdminCookieName, Value: c.key})
		}
		r.AddCookie(&http.Cookie{Name: AccountCookieName, Value: "commerce-admin-only"})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != c.want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(c.path, w.Code, w.Body.String())
		}
		for _, secret := range []string{"owner_id", "receipt_id", "password", "token", "address", "contact", "watchlist_ids", "operator-capability"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("private aggregate projection", secret)
			}
		}
	}
}

type brokenAnalytics struct{}

func (brokenAnalytics) Report(context.Context, analytics.Filters) (analytics.Response, error) {
	return analytics.Response{}, context.Canceled
}
func TestAnalyticsUnavailableNoFalseZeros(t *testing.T) {
	mux := http.NewServeMux()
	RegisterAnalytics(mux, "key", brokenAnalytics{})
	r := httptest.NewRequest("GET", "/api/admin/analytics", nil)
	r.AddCookie(&http.Cookie{Name: AdminCookieName, Value: "key"})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 503 || strings.Contains(w.Body.String(), "summary") {
		t.Fatal(w.Code, w.Body.String())
	}
}
