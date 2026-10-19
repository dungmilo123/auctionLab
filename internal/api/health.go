package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Readiness interface {
	Ready(context.Context) (int, error)
}

func NewHealth(db Readiness) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		version, err := db.Ready(ctx)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "unavailable", "database": "postgresql", "ready": false})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "database": "postgresql", "ready": true, "schema_version": version})
	}
}

// NotFoundJSON answers every unknown /api/* path with a JSON 404.
//
// The "/api/" pattern is what keeps a typo'd endpoint from reaching the SPA
// fallback: without it, "/api/nope" would receive the SPA document with HTTP
// 200 and a broken API would look like a working one. (Registration order is
// not the protection — Go 1.22+ ServeMux matches by specificity.)
func NotFoundJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"error":"not found"}`))
}
