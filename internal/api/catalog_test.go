package api

import (
	"auctionlab/internal/catalog"
	"auctionlab/internal/store"
	"auctionlab/internal/testpg"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func catalogHTTP(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	db, err := store.Open(testpg.NewURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	s := catalog.New(store.CatalogRepository{Store: db}, func() time.Time { return time.UnixMilli(1000) }, nil)
	mux := http.NewServeMux()
	RegisterCatalog(mux, "private-test-key", s)
	mux.HandleFunc("/api/", NotFoundJSON)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, db
}
func call(t *testing.T, server *httptest.Server, method, path, body string, cookie bool, headers map[string]string) (int, string) {
	t.Helper()
	r, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if cookie {
		r.AddCookie(&http.Cookie{Name: AdminCookieName, Value: "private-test-key"})
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Header.Get("Cache-Control") != "no-store" || !strings.Contains(response.Header.Get("Content-Type"), "application/json") {
		t.Fatal("catalog response policy", response.Header)
	}
	return response.StatusCode, string(raw)
}
func TestCatalogAuthorizationAndOrigin(t *testing.T) {
	server, db := catalogHTTP(t)
	_ = db
	for _, path := range []string{"/api/admin/catalog", "/api/admin/catalog/categories", "/api/admin/catalog/lots?k=private-test-key", "/api/admin/catalog/events/missing/preview", "/api/admin/catalog/nope"} {
		status, raw := call(t, server, "GET", path, "", false, nil)
		if status != 401 || strings.Contains(raw, "private-test-key") || strings.Contains(raw, "<html") {
			t.Fatal(status, raw)
		}
	}
	headers := map[string]string{"Content-Type": "application/json", "X-AuctionLab-Admin": "1", "Origin": server.URL}
	status, raw := call(t, server, "POST", "/api/admin/catalog/lots", `{"title":"Draft"}`, true, headers)
	if status != 201 {
		t.Fatal(status, raw)
	}
	for _, h := range []map[string]string{{"Content-Type": "application/json"}, {"Content-Type": "application/x-www-form-urlencoded", "X-AuctionLab-Admin": "1"}, {"Content-Type": "application/json", "X-AuctionLab-Admin": "1", "Origin": "https://foreign.example"}, {"Content-Type": "application/json", "X-AuctionLab-Admin": "1", "Origin": "null"}} {
		status, _ := call(t, server, "POST", "/api/admin/catalog/lots", `{"title":"Rejected"}`, true, h)
		if status != 403 && status != 415 {
			t.Fatal(status)
		}
	}
	var n int
	db.DB.QueryRow("SELECT count(*) FROM catalog_lots").Scan(&n)
	if n != 1 {
		t.Fatal("rejected mutation wrote", n)
	}
	for _, tc := range []struct {
		host, remote, origin, forwarded string
		tls, ok                         bool
	}{{"192.168.1.2:8080", "192.168.1.3:1000", "http://192.168.1.2:8080", "", false, true}, {"localhost:5173", "127.0.0.1:1000", "http://localhost:5173", "", false, true}, {"demo.example", "127.0.0.1:1000", "https://demo.example", "https", false, true}, {"demo.example", "192.168.1.3:1000", "https://demo.example", "https", false, false}, {"demo.example", "127.0.0.1:1000", "https://demo.example", "https,http", false, false}, {"demo.example", "127.0.0.1:1000", "http://user:pass@demo.example", "", false, false}} {
		r := httptest.NewRequest("POST", "http://"+tc.host+"/api/admin/catalog/lots", nil)
		r.Host = tc.host
		r.RemoteAddr = tc.remote
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-Forwarded-Proto", tc.forwarded)
		if got := catalogOrigin(r); got != tc.ok {
			t.Fatal(tc, got)
		}
	}
}
func TestCatalogHTTPEnvelopesAndDraftVisibility(t *testing.T) {
	server, db := catalogHTTP(t)
	headers := map[string]string{"Content-Type": "application/json", "X-AuctionLab-Admin": "1"}
	status, raw := call(t, server, "POST", "/api/admin/catalog/lots", `{"title":"Camera"}`, true, headers)
	if status != 201 {
		t.Fatal(status, raw)
	}
	var envelope struct {
		Data catalog.Lot `json:"data"`
	}
	json.Unmarshal([]byte(raw), &envelope)
	lot := envelope.Data
	status, _ = call(t, server, "GET", "/api/catalog/lots/"+lot.Slug, "", false, nil)
	if status != 404 {
		t.Fatal("draft visible")
	}
	missing, missingBody := call(t, server, "GET", "/api/catalog/lots/no-such-slug", "", false, nil)
	_, draftBody := call(t, server, "GET", "/api/catalog/lots/"+lot.Slug, "", false, nil)
	if missing != 404 || draftBody != missingBody {
		t.Fatal("draft existence disclosed")
	}
	for _, tc := range []struct {
		body string
		want int
	}{{`{"title":"bad","unknown":1}`, 400}, {`{"title":"x"} {}`, 400}, {`{"title":"x","starting_price_minor":1.2}`, 422}, {`{"title":"x","starting_price_minor":-1}`, 422}, {`{"title":"x","currency":"VND"}`, 422}, {strings.Repeat(" ", 513<<10) + `{}`, 400}} {
		status, raw = call(t, server, "POST", "/api/admin/catalog/lots", tc.body, true, headers)
		if status != tc.want {
			t.Fatal(status, raw, tc.want)
		}
	}
	request := map[string]any{"title": "Edited", "slug": lot.Slug, "currency": "USD", "version": lot.Version}
	encoded, _ := json.Marshal(request)
	status, raw = call(t, server, "PUT", "/api/admin/catalog/lots/"+lot.ID, string(encoded), true, headers)
	if status != 200 {
		t.Fatal(status, raw)
	}
	status, _ = call(t, server, "PUT", "/api/admin/catalog/lots/"+lot.ID, string(encoded), true, headers)
	if status != 409 {
		t.Fatal("stale update succeeded")
	}
	if err := db.Bootstrap(time.Now()); err != nil {
		t.Fatal(err)
	}
	db.Close()
	status, raw = call(t, server, "POST", "/api/admin/catalog/lots", `{"title":"No false success"}`, true, headers)
	if status < 500 || strings.Contains(raw, "private-test-key") {
		t.Fatal(status, raw)
	}
}

type uncertainUnavailable struct{}

func (uncertainUnavailable) Error() string         { return "connection failed during commit" }
func (uncertainUnavailable) CommitUncertain() bool { return true }
func (uncertainUnavailable) Unavailable() bool     { return true }
func TestUncertainCommitRequiresReloadEvenWhenUnavailable(t *testing.T) {
	response := httptest.NewRecorder()
	respondCatalog(response, nil, uncertainUnavailable{}, 200)
	if response.Code != 503 || !strings.Contains(response.Body.String(), "commit_unknown") {
		t.Fatal("uncertain commit was downgraded to a retryable unavailable response", response.Code, response.Body.String())
	}
}
