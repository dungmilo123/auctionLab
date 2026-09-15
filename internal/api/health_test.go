package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// healthBody is the D-19 contract, byte for byte. It is written as a literal so
// a renamed or added field fails the test rather than silently widening the
// payload every smoke check and the verifier will curl.
const healthBody = `{"status":"ok","wal":true}`

// TestHealth pins the /api/health payload: exact bytes, exact key set, JSON
// content type — and that `wal` reflects the boot assertion the handler was
// constructed with (it is never re-probed per request).
func TestHealth(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewHealth(true)(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("Content-Type = %q, want it to contain application/json", ct)
		}
		if body := rec.Body.String(); body != healthBody {
			t.Errorf("body = %q, want %q", body, healthBody)
		}

		var decoded map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("body is not valid JSON: %v", err)
		}
		if len(decoded) != 2 {
			t.Fatalf("body carries %d fields (%v), want exactly status and wal", len(decoded), decoded)
		}
		if decoded["status"] != "ok" {
			t.Errorf("status = %v, want \"ok\"", decoded["status"])
		}
		wal, ok := decoded["wal"].(bool)
		if !ok {
			t.Fatalf("wal = %v (%T), want a JSON boolean", decoded["wal"], decoded["wal"])
		}
		if !wal {
			t.Error("wal = false, want true for a handler built from a successful boot assertion")
		}
	})

	t.Run("false is passed through", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewHealth(false)(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

		if body := rec.Body.String(); body != `{"status":"ok","wal":false}` {
			t.Errorf("body = %q, want the boot assertion value passed through", body)
		}
	})
}

// spaMarker identifies the fallback document, so any response that reaches it
// can be told apart from a real API response.
const spaMarker = `<!doctype html><html lang="vi">auctionlab-health-test-spa-document</html>`

// wiredMux builds the same route table cmd/server/main.go uses: health →
// "/api/" JSON 404 → "/ws" → "/" SPA fallback. The order is a readability
// convention, not the protection: Go 1.22+ ServeMux matches by specificity, so
// the "/" fallback cannot swallow the others from any position.
//
// The test exercises the *wiring*, not the handlers in isolation: Pitfall 11 is
// a catch-all answering a path the API should have owned, which no per-handler
// test can catch.
func wiredMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", NewHealth(true))
	mux.HandleFunc("/api/", NotFoundJSON)
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUpgradeRequired)
		_, _ = w.Write([]byte("ws-upgrade-required"))
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(spaMarker))
	}))
	return mux
}

// TestAPIBoundaryOnTheWiredMux pins the boundary in both directions: unknown or
// wrong-method /api/* paths get a JSON 404 and never the SPA document, while
// non-API paths and /ws still reach their own handlers.
func TestAPIBoundaryOnTheWiredMux(t *testing.T) {
	mux := wiredMux()

	cases := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
		wantSPA    bool
	}{
		{
			name: "health still answers on the wired mux", method: http.MethodGet, path: "/api/health",
			wantStatus: http.StatusOK, wantBody: healthBody,
		},
		{
			name: "unknown GET api path is a JSON 404", method: http.MethodGet, path: "/api/nope",
			wantStatus: http.StatusNotFound, wantBody: `{"error":"not found"}`,
		},
		{
			name: "unknown POST api path is a JSON 404", method: http.MethodPost, path: "/api/nope",
			wantStatus: http.StatusNotFound, wantBody: `{"error":"not found"}`,
		},
		{
			name:   "wrong method on a real endpoint falls to the API 404, not the SPA",
			method: http.MethodPost, path: "/api/health",
			wantStatus: http.StatusNotFound, wantBody: `{"error":"not found"}`,
		},
		{
			name: "the SPA fallback still serves a deep link", method: http.MethodGet, path: "/admin",
			wantStatus: http.StatusOK, wantBody: spaMarker, wantSPA: true,
		},
		{
			name: "the WS path is not swallowed by the SPA fallback", method: http.MethodGet, path: "/ws",
			wantStatus: http.StatusUpgradeRequired, wantBody: "ws-upgrade-required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, rec.Code, tc.wantStatus)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tc.wantBody) {
				t.Errorf("%s %s: body = %q, want it to contain %q", tc.method, tc.path, body, tc.wantBody)
			}
			if !tc.wantSPA && strings.Contains(body, "auctionlab-health-test-spa-document") {
				t.Errorf("%s %s: got the SPA document instead of an API/WS response (Pitfall 11)", tc.method, tc.path)
			}
			if !tc.wantSPA && tc.wantStatus != http.StatusUpgradeRequired {
				if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
					t.Errorf("%s %s: Content-Type = %q, want application/json", tc.method, tc.path, ct)
				}
			}
		})
	}
}
