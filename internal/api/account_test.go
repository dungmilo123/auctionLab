package api

import (
	"auctionlab/internal/account"
	"auctionlab/internal/catalog"
	"auctionlab/internal/store"
	"auctionlab/internal/testpg"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func accountAPI(t *testing.T) (*http.ServeMux, *account.Service, *store.Store) {
	t.Helper()
	st, e := store.Open(testpg.NewURL(t))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	if e = st.Migrate(); e != nil {
		t.Fatal(e)
	}
	s := account.New(store.AccountRepository{Store: st})
	mux := http.NewServeMux()
	RegisterAccount(mux, s)
	return mux, s, st
}
func accountRequest(h http.Handler, method, path, body string, c *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://example.test"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-AuctionLab-Account", "1")
	r.Header.Set("Origin", "http://example.test")
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func registerTestAccount(t *testing.T, h http.Handler, email string) *http.Cookie {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email, "password": "fixture unicode 密碼", "display_name": "Fixture"})
	w := accountRequest(h, "POST", "/api/auth/register", string(b), nil)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
}
func TestAccountHTTPBoundaries(t *testing.T) {
	mux, _, _ := accountAPI(t)
	cookie := registerTestAccount(t, mux, "http@example.test")
	if cookie.Name != AccountCookieName || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Domain != "" || cookie.Path != "/" || cookie.Secure || cookie.MaxAge != 2592000 {
		t.Fatal("cookie policy")
	}
	cases := []struct {
		method, path, body string
		c                  *http.Cookie
		status             int
	}{
		{"GET", "/api/account", "", nil, 401}, {"GET", "/api/account/unknown", "", nil, 401}, {"GET", "/api/account/unknown", "", cookie, 404},
		{"GET", "/api/account", "", cookie, 200}, {"PATCH", "/api/account", `{"display_name":"Changed"}`, cookie, 200},
		{"PATCH", "/api/account", `{"display_name":"Name","id":"other"}`, cookie, 400}, {"PATCH", "/api/account", `{"email":"other@example.test"}`, cookie, 400}, {"PATCH", "/api/account", `{"role":"admin"}`, cookie, 400},
		{"PATCH", "/api/account", `null`, cookie, 400}, {"PATCH", "/api/account", `[]`, cookie, 400}, {"PATCH", "/api/account", ``, cookie, 400}, {"PATCH", "/api/account", `{} {}`, cookie, 400}, {"PATCH", "/api/account", `{"display_name":null}`, cookie, 400}, {"PATCH", "/api/account", `{"display_name":"` + strings.Repeat("a", 8192) + `"}`, cookie, 400},
		{"POST", "/api/auth/logout", `{}`, cookie, 200}, {"GET", "/api/account", "", cookie, 401}, {"POST", "/api/auth/logout", `{}`, cookie, 200}, {"POST", "/api/auth/logout", `{}`, nil, 200},
	}
	for _, c := range cases {
		w := accountRequest(mux, c.method, c.path, c.body, c.c)
		if w.Code != c.status || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
			t.Fatal(c.method, c.path, w.Code, w.Body.String())
		}
	}
	for _, tc := range []struct {
		header, value string
		status        int
	}{{"Origin", "http://evil.test", 403}, {"Origin", "null", 403}, {"X-AuctionLab-Account", "", 403}, {"Content-Type", "text/plain", 415}, {"Content-Type", "application/x-www-form-urlencoded", 415}} {
		r := httptest.NewRequest("POST", "http://example.test/api/auth/logout", strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-AuctionLab-Account", "1")
		r.Header.Set(tc.header, tc.value)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(tc.header, w.Code)
		}
	}
}
func TestAccountCookieScheme(t *testing.T) {
	for _, tc := range []struct {
		tls           bool
		remote, proto string
		secure        bool
	}{{false, "127.0.0.1:1", "", false}, {true, "192.0.2.1:1", "", true}, {false, "127.0.0.1:1", "https", true}, {false, "192.0.2.1:1", "https", false}, {true, "127.0.0.1:1", "http", false}} {
		r := httptest.NewRequest("POST", "http://example.test", nil)
		r.RemoteAddr = tc.remote
		r.Header.Set("X-Forwarded-Proto", tc.proto)
		if tc.tls {
			r.TLS = &tls.ConnectionState{}
		}
		for _, token := range []string{"opaque", ""} {
			w := httptest.NewRecorder()
			accountCookie(w, r, token, time.Now())
			c := w.Result().Cookies()[0]
			if c.Secure != tc.secure || c.Path != "/" || c.Domain != "" || (token == "" && c.MaxAge != -1) {
				t.Fatal("cookie scheme")
			}
		}
	}
}

type unknownCommit struct{}

func (unknownCommit) Error() string         { return "private SQL credential sentinel" }
func (unknownCommit) CommitUncertain() bool { return true }

type uncertainAccountRepo struct{ account.Repository }

func (r uncertainAccountRepo) Register(ctx context.Context, row account.Record, digest, old string, now int64) (account.Profile, error) {
	_, e := r.Repository.Register(ctx, row, digest, old, now)
	if e != nil {
		return account.Profile{}, e
	}
	return account.Profile{}, unknownCommit{}
}
func TestAccountCommitFailureAndRecovery(t *testing.T) {
	_, s, st := accountAPI(t)
	s.Repo = uncertainAccountRepo{s.Repo}
	mux := http.NewServeMux()
	RegisterAccount(mux, s)
	w := accountRequest(mux, "POST", "/api/auth/register", `{"email":"recover@example.test","password":"recovery password","display_name":"Recover"}`, nil)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "commit_unknown") || len(w.Result().Cookies()) != 0 || strings.Contains(w.Body.String(), "sentinel") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = accountRequest(mux, "POST", "/api/auth/login", `{"email":"recover@example.test","password":"recovery password"}`, nil)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatal("login recovery", w.Code)
	}
	st.Close()
	w = accountRequest(mux, "POST", "/api/auth/login", `{"email":"recover@example.test","password":"recovery password"}`, nil)
	if w.Code != 503 || len(w.Result().Cookies()) != 0 {
		t.Fatal("outage false success")
	}
	w = accountRequest(mux, "POST", "/api/auth/logout", `{}`, nil)
	if w.Code != 503 || len(w.Result().Cookies()) != 0 {
		t.Fatal("false logout")
	}
}
func TestAccountInvalidLoginAndLimiter(t *testing.T) {
	mux, s, _ := accountAPI(t)
	cookie := registerTestAccount(t, mux, "login@example.test")
	_ = cookie
	calls := 0
	verify := s.Verify
	s.Verify = func(p, v string) bool { calls++; return verify(p, v) }
	for _, email := range []string{"login@example.test", "unknown@example.test"} {
		w := accountRequest(mux, "POST", "/api/auth/login", fmt.Sprintf(`{"email":%q,"password":"wrong password"}`, email), nil)
		if w.Code != 401 || !strings.Contains(w.Body.String(), "invalid_credentials") || len(w.Result().Cookies()) != 0 {
			t.Fatal(w.Code)
		}
	}
	if calls != 2 {
		t.Fatal("dummy work omitted")
	}
	// Shared direct peer remains limited even if each request claims a new forwarded IP.
	for i := 0; i < 8; i++ {
		r := httptest.NewRequest("POST", "http://example.test/api/auth/login", strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-AuctionLab-Account", "1")
		r.Header.Set("X-Forwarded-For", fmt.Sprint(i))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if i == 7 && (w.Code != 429 || w.Header().Get("Retry-After") == "") {
			t.Fatal("forwarded bypass", w.Code)
		}
	}
}

func TestAccountCapabilityIndependence(t *testing.T) {
	mux, _, st := accountAPI(t)
	RegisterCatalog(mux, "test-only-capability", catalog.New(store.CatalogRepository{Store: st}, time.Now, nil))
	c := registerTestAccount(t, mux, "boundary@example.test")
	for _, role := range []string{"bidder", "admin"} {
		if _, e := st.DB.Exec(`UPDATE commerce_accounts SET role=$1`, role); e != nil {
			t.Fatal(e)
		}
		for _, path := range []string{"/api/admin/catalog", "/api/admin/catalog/lots", "/api/admin/catalog/events/missing/preview", "/api/admin/catalog/nope"} {
			for _, method := range []string{"GET", "POST", "PUT"} {
				w := accountRequest(mux, method, path, `{}`, c)
				if w.Code != 401 {
					t.Fatal(role, path, method, w.Code)
				}
			}
		}
		w := accountRequest(mux, "GET", "/api/account", "", c)
		if !strings.Contains(w.Body.String(), `"role":"`+role+`"`) {
			t.Fatal("stored role ignored")
		}
	}
	cap := &http.Cookie{Name: AdminCookieName, Value: "test-only-capability"}
	if w := accountRequest(mux, "GET", "/api/admin/catalog/lots", "", cap); w.Code != 200 {
		t.Fatal("capability compatibility", w.Code)
	}
	for _, path := range []string{"/api/account", "/api/account/watchlist", "/api/account/nope"} {
		if w := accountRequest(mux, "GET", path, "", cap); w.Code != 401 {
			t.Fatal("capability chose account")
		}
	}
}

func TestWatchlistHTTP(t *testing.T) {
	mux, _, st := accountAPI(t)
	a := registerTestAccount(t, mux, "watch-a@example.test")
	b := registerTestAccount(t, mux, "watch-b@example.test")
	_, e := st.DB.Exec(`INSERT INTO catalog_lots(id,slug,title,state,created_at,updated_at) VALUES('lot','lot','Lot','published',1,1),('draft','draft','Private','draft',1,1); INSERT INTO catalog_events(id,slug,title,state,created_at,updated_at) VALUES('e','e','Event','published',1,1); INSERT INTO catalog_event_lots VALUES('e','lot',0);`)
	if e != nil {
		t.Fatal(e)
	}
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		if w := accountRequest(mux, method, "/api/account/watchlist/lot", `{}`, nil); w.Code != 401 {
			t.Fatal("anonymous watchlist")
		}
	}
	for _, id := range []string{"draft", "missing"} {
		w := accountRequest(mux, "PUT", "/api/account/watchlist/"+id, `{}`, a)
		if w.Code != 404 || !strings.Contains(w.Body.String(), "not_found") {
			t.Fatal("draft disclosure")
		}
	}
	for _, body := range []string{`{"account_id":"other"}`, `{"saved":true}`, `null`, `{} {}`} {
		w := accountRequest(mux, "PUT", "/api/account/watchlist/lot", body, a)
		if w.Code != 400 {
			t.Fatal("owner fields accepted", w.Code)
		}
	}
	for i := 0; i < 2; i++ {
		w := accountRequest(mux, "PUT", "/api/account/watchlist/lot", `{}`, a)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"saved":true`) {
			t.Fatal("save")
		}
	}
	w := accountRequest(mux, "GET", "/api/account/watchlist", "", a)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "server_time") || !strings.Contains(w.Body.String(), `"id":"lot"`) {
		t.Fatal("list", w.Code, w.Body.String())
	}
	w = accountRequest(mux, "GET", "/api/account/watchlist", "", b)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatal("foreign membership")
	}
	w = accountRequest(mux, "GET", "/api/account/watchlist/lot", "", b)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"saved":false`) {
		t.Fatal("foreign state")
	}
	r := httptest.NewRequest("PUT", "http://example.test/api/account/watchlist/lot", strings.NewReader(`{}`))
	r.AddCookie(a)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-AuctionLab-Account", "1")
	r.Header.Set("Origin", "http://foreign.test")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatal("cross origin")
	}
	for i := 0; i < 2; i++ {
		w = accountRequest(mux, "DELETE", "/api/account/watchlist/lot", `{}`, a)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"saved":false`) {
			t.Fatal("remove")
		}
	}
}
