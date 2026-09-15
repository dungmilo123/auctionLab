package api

import (
	"encoding/json"
	"net/http"
)

type healthResponse struct {
	Status string `json:"status"`
	WAL    bool   `json:"wal"`
}

// NewHealth builds the GET /api/health handler (D-19).
//
// `wal` reports the mode the boot assertion observed on the live handle — the
// caller passes `store.Pragmas().JournalMode == "wal"`, not a literal. It is
// never re-probed per request, so a health 200 always means "this process is
// running on a WAL database".
func NewHealth(wal bool) http.HandlerFunc {
	body, err := json.Marshal(healthResponse{Status: "ok", WAL: wal})
	if err != nil {
		// json.Marshal cannot fail for this struct; keep the handler total anyway.
		body = []byte(`{"status":"ok","wal":false}`)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

// NotFoundJSON answers every unknown /api/* path with a JSON 404.
//
// It must be registered before the "/" SPA fallback: otherwise a typo'd
// endpoint would receive the SPA document with HTTP 200 and a broken API would
// look like a working one.
func NotFoundJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"error":"not found"}`))
}
