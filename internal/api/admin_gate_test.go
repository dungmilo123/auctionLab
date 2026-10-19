package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// This file is D-75/D-77's contract: the admin gate is the project's only
// server-side authorization, it renders its own offline page, and the protected
// document is unreachable without the capability cookie. The tests drive the
// WIRED route table (the health_test precedent) so the registration — GET /admin
// plus the /admin/ subtree, ahead of the SPA fallback — is exercised too.

// gateKey is the fixture capability: the same 64 lowercase hex shape
// game.NewToken mints.
const gateKey = "f0e1d2c3b4a5968778695a4b3c2d1e0ff0e1d2c3b4a5968778695a4b3c2d1e0f"

// gateFormMarker is a fragment only the gate page carries, so a response that
// reached the protected document can be told apart from one that did not.
const gateFormMarker = `<form method="get" action="/admin">`

// gateSPA stands in for the embedded SPA handler the gate wraps. The marker is
// the one health_test.go defines: it is the package's "this reached the SPA
// fallback" signal.
func gateSPA() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", indexCacheControl)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(spaMarker))
	})
}

// gateMux builds the route table cmd/server/main.go registers for the protected
// paths — GET /admin, the /admin/ subtree, then the SPA fallback — so the
// registration itself is what the tests drive (Pitfall 11: a catch-all
// answering a path the gate should own is invisible to a handler-only test).
func gateMux(key string) http.Handler {
	spa := gateSPA()
	gate := AdminGate(key, spa)

	mux := http.NewServeMux()
	mux.Handle("GET /admin", gate)
	mux.Handle("/admin/", gate)
	mux.Handle("/", spa)
	return mux
}

// gateGet runs one GET through the table, optionally carrying cookies.
func gateGet(t *testing.T, h http.Handler, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// adminCookie is the capability cookie as a browser would carry it.
func adminCookie(value string) *http.Cookie {
	return &http.Cookie{Name: "auction_admin", Value: value}
}

// assertGateDocument pins the gate page's hard constraints (UI-SPEC §Admin Gate
// Page): the headers, both languages inline with the VI run marked, the form the
// presenter types into — and, the constraint that matters when the venue has no
// internet, no external request and no JavaScript of any kind.
func assertGateDocument(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	body := rec.Body.String()
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != indexCacheControl {
		t.Errorf("Cache-Control = %q, want %q (a cached gate page after a rotation is a dead end)", cc, indexCacheControl)
	}
	if x := rec.Header().Get("X-Content-Type-Options"); x != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", x)
	}

	if !strings.Contains(body, `lang="en"`) {
		t.Error(`the document does not declare lang="en" (EN is the default)`)
	}
	if !strings.Contains(body, `lang="vi"`) {
		t.Error(`the Vietnamese runs do not carry lang="vi"`)
	}
	if !strings.Contains(body, " · ") {
		t.Error("the two languages are not separated by ` · ` (the page's whole i18n mechanism)")
	}
	if !strings.Contains(body, gateFormMarker) {
		t.Errorf("the document does not carry the plain GET form %q", gateFormMarker)
	}
	if !strings.Contains(body, `name="k"`) || !strings.Contains(body, `for="k"`) {
		t.Error("the form does not label and name the key field k")
	}
	if !strings.Contains(body, "<style>") {
		t.Error("the document carries no inline <style> block")
	}

	for _, token := range []string{"#F6F1E8", "#24211E", "#722F37", "#8D2E35", "min-height: 44px", ":focus-visible", "prefers-reduced-motion"} {
		if !strings.Contains(body, token) {
			t.Errorf("gate missing vintage/accessibility token %q", token)
		}
	}
	if strings.Contains(body, "box-shadow: 4px") || strings.Contains(body, "#FF6B00") {
		t.Error("gate retains the old game presentation")
	}

	// Zero external requests, zero JavaScript (UI-SPEC hard constraints): the
	// page must be perfect with the venue's internet off.
	for _, banned := range []string{"<script", "src=", "href=", "http://", "https://"} {
		if strings.Contains(body, banned) {
			t.Errorf("the gate document contains %q — it must make no external request and run no script", banned)
		}
	}
}

// assertAdminCookie pins D-75's cookie exactly: the fixed name, the compared
// value, Path=/, 12 hours, HttpOnly, SameSite=Lax — and deliberately NO Secure,
// because the venue LAN is plain http and a Secure cookie would never be sent.
func assertAdminCookie(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("response carries %d cookies, want exactly the one capability cookie", len(cookies))
	}
	c := cookies[0]
	if c.Name != "auction_admin" {
		t.Errorf("cookie name = %q, want auction_admin", c.Name)
	}
	if c.Value != gateKey {
		t.Errorf("cookie value = %q, want the key that was compared", c.Value)
	}
	if c.Path != "/" {
		t.Errorf("cookie Path = %q, want /", c.Path)
	}
	if c.MaxAge != 43200 {
		t.Errorf("cookie Max-Age = %d, want 43200 (12h)", c.MaxAge)
	}
	if !c.HttpOnly {
		t.Error("cookie is not HttpOnly — a script could read the capability (T-04-02)")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v, want Lax", c.SameSite)
	}
	if c.Secure {
		t.Error("cookie is Secure — it would never be sent over the venue's plain http (D-75)")
	}
}

// TestAdminGate drives the wired table through the whole D-75 decision: no key,
// an empty key, a wrong key, the correct key, a valid cookie, and the deep links
// under /admin/.
func TestAdminGate(t *testing.T) {
	t.Run("no cookie renders the gate page with 200", func(t *testing.T) {
		rec := gateGet(t, gateMux(gateKey), "/admin")

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200 for a request that supplied no key", rec.Code)
		}
		assertGateDocument(t, rec)
		body := rec.Body.String()
		if strings.Contains(body, spaMarker) {
			t.Error("the request reached the protected document without a key")
		}
		if strings.Contains(body, `role="alert"`) {
			t.Error("the error line rendered although no key was supplied")
		}
	})

	t.Run("an empty k is the no-key state, not a wrong key", func(t *testing.T) {
		rec := gateGet(t, gateMux(gateKey), "/admin?k=")

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200 — an empty ?k= is \"no key supplied\" (D-75)", rec.Code)
		}
		assertGateDocument(t, rec)
		if strings.Contains(rec.Body.String(), `role="alert"`) {
			t.Error("an empty ?k= rendered the wrong-key alert")
		}
	})

	t.Run("a wrong k is 401 and never echoes the supplied value", func(t *testing.T) {
		// The value carries markup on purpose: it proves nothing is echoed and
		// nothing reaches the document unescaped.
		supplied := `<script>alert(1)</script>`
		rec := gateGet(t, gateMux(gateKey), "/admin?k="+url.QueryEscape(supplied))

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 for a wrong key", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `role="alert"`) || !strings.Contains(body, `id="gate-error"`) {
			t.Error("the 401 document does not carry the role=\"alert\" error line")
		}
		if !strings.Contains(body, `aria-invalid="true"`) || !strings.Contains(body, `aria-describedby="gate-error"`) {
			t.Error("the key field is not marked aria-invalid with the error line described by it")
		}
		if strings.Contains(body, supplied) {
			t.Error("the supplied key is echoed back into the response")
		}
		if strings.Contains(body, "<script") {
			t.Error("the supplied value reached the markup as a script tag")
		}
	})

	t.Run("a correct k sets the cookie and redirects only to /admin", func(t *testing.T) {
		// A query parameter the gate must ignore: the redirect target is a
		// literal, never anything the caller can steer (T-04-06).
		rec := gateGet(t, gateMux(gateKey), "/admin?k="+gateKey+"&next=https://evil.example/steal")

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303 (D-75)", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/admin" {
			t.Errorf("Location = %q, want the literal /admin", loc)
		}
		assertAdminCookie(t, rec)

		body := rec.Body.String()
		if strings.Contains(body, gateKey) {
			t.Error("the response body carries the key — only the Set-Cookie header may")
		}
		if strings.Contains(body, gateFormMarker) {
			t.Error("the correct-key path served the gate page instead of redirecting")
		}
	})

	t.Run("a valid cookie reaches the protected document", func(t *testing.T) {
		rec := gateGet(t, gateMux(gateKey), "/admin", adminCookie(gateKey))

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), spaMarker) {
			t.Error("a valid cookie did not reach the protected document")
		}
		if strings.Contains(rec.Body.String(), gateFormMarker) {
			t.Error("the gate page answered a request that carried a valid cookie")
		}
	})

	t.Run("a wrong cookie is gated", func(t *testing.T) {
		rec := gateGet(t, gateMux(gateKey), "/admin", adminCookie(strings.Repeat("0", 64)))

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200 (no ?k= was supplied)", rec.Code)
		}
		assertGateDocument(t, rec)
		if strings.Contains(rec.Body.String(), spaMarker) {
			t.Error("a wrong cookie reached the protected document")
		}
	})

	t.Run("a stale k never locks out a valid cookie", func(t *testing.T) {
		rec := gateGet(t, gateMux(gateKey), "/admin?k="+strings.Repeat("0", 64), adminCookie(gateKey))

		if !strings.Contains(rec.Body.String(), spaMarker) {
			t.Error("a stale ?k= locked out a valid cookie; the capability is the cookie, not the query")
		}
		if strings.Contains(rec.Body.String(), gateFormMarker) {
			t.Error("the gate page answered a request that carried a valid cookie")
		}
	})

	t.Run("deep links under /admin/ are gated too", func(t *testing.T) {
		gated := gateGet(t, gateMux(gateKey), "/admin/players")
		if gated.Code != http.StatusOK {
			t.Errorf("status = %d, want 200 for an unauthenticated deep link", gated.Code)
		}
		assertGateDocument(t, gated)
		if strings.Contains(gated.Body.String(), spaMarker) {
			t.Error("a deep link under /admin/ reached the protected document without a cookie")
		}

		allowed := gateGet(t, gateMux(gateKey), "/admin/players", adminCookie(gateKey))
		if !strings.Contains(allowed.Body.String(), spaMarker) {
			t.Error("a valid cookie did not reach the protected document on a deep link")
		}
	})

	t.Run("an empty expected key authorizes nothing", func(t *testing.T) {
		mux := gateMux("")

		cases := []struct {
			name   string
			target string
			cookie *http.Cookie
		}{
			{name: "no input", target: "/admin"},
			{name: "an empty cookie value", target: "/admin", cookie: adminCookie("")},
			{name: "a non-empty cookie value", target: "/admin", cookie: adminCookie("x")},
			{name: "an empty k", target: "/admin?k="},
			{name: "a non-empty k", target: "/admin?k=x"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rec := gateGet(t, mux, tc.target, tc.cookie)
				if strings.Contains(rec.Body.String(), spaMarker) {
					t.Error("an empty expected key authorized a request — Equal(\"\", \"\") must never open the gate")
				}
			})
		}
	})
}

// TestEqual pins the one comparison the phase has: both sides are hashed to a
// fixed width before crypto/subtle compares them, so an unequal LENGTH cannot be
// probed by an early return (T-04-01) — and an empty supplied value is never
// authorized, so a missing key can never match a missing expected key.
func TestEqual(t *testing.T) {
	t.Run("accepts equal values", func(t *testing.T) {
		if !Equal(gateKey, gateKey) {
			t.Error("Equal rejected two identical keys")
		}
		if !Equal("abc", "abc") {
			t.Error("Equal rejected two identical short values")
		}
	})

	t.Run("rejects unequal content", func(t *testing.T) {
		if Equal("abc", "abd") {
			t.Error("Equal accepted two different values")
		}
		if Equal(gateKey, strings.Repeat("0", 64)) {
			t.Error("Equal accepted a different key of the same length")
		}
	})

	t.Run("rejects unequal lengths", func(t *testing.T) {
		if Equal(gateKey[:63], gateKey) {
			t.Error("Equal accepted a one-character-short key")
		}
		if Equal(gateKey+"0", gateKey) {
			t.Error("Equal accepted a one-character-long key")
		}
		if Equal("", gateKey) {
			t.Error("Equal accepted an empty value against a real key")
		}
	})

	t.Run("a long common prefix is not enough", func(t *testing.T) {
		if Equal(gateKey[:32], gateKey) {
			t.Error("Equal accepted the key's first half")
		}
	})

	t.Run("an empty supplied value is never authorized", func(t *testing.T) {
		// Fail-closed: if the expected side were ever empty, an empty cookie must
		// not match it.
		if Equal("", "") {
			t.Error("Equal authorized the empty-vs-empty pair")
		}
	})
}
