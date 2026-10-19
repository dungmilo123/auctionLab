package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"

	"auctionlab/internal/catalog"
	"auctionlab/internal/store"
)

func catalogJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func catalogError(w http.ResponseWriter, status int, code string, fields map[string]string) {
	catalogJSON(w, status, map[string]any{"error": catalog.Error{Code: code, Fields: fields}})
}
func CatalogAdminGuard(key string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(AdminCookieName)
		if err != nil || !Equal(cookie.Value, key) {
			catalogError(w, 401, "access_required", nil)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if r.Header.Get("X-AuctionLab-Admin") != "1" || !catalogOrigin(r) {
				catalogError(w, 403, "request_rejected", nil)
				return
			}
			typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || typ != "application/json" {
				catalogError(w, 415, "json_required", nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func catalogOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded == "http" || forwarded == "https" {
				scheme = forwarded
			}
		}
	}
	expected, err := url.Parse(scheme + "://" + r.Host)
	if err != nil || expected.Host == "" {
		return false
	}
	normalize := func(u *url.URL) string {
		port := u.Port()
		if port == "" {
			if u.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		return strings.ToLower(u.Scheme + "://" + net.JoinHostPort(u.Hostname(), port))
	}
	return normalize(parsed) == normalize(expected)
}
func decodeCatalog(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 512<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		var typed *json.UnmarshalTypeError
		if errors.As(err, &typed) {
			catalogError(w, 422, "validation", map[string]string{typed.Field: "invalid"})
		} else {
			catalogError(w, 400, "malformed_json", nil)
		}
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		catalogError(w, 400, "malformed_json", nil)
		return false
	}
	return true
}
func respondCatalog(w http.ResponseWriter, data any, err error, status int) {
	if err == nil {
		catalogJSON(w, status, map[string]any{"data": data})
		return
	}
	var validation *catalog.Error
	if errors.As(err, &validation) {
		code := validation.Code
		status := 409
		switch code {
		case "validation":
			status = 422
		case "not_found":
			status = 404
		case "entropy", "storage":
			status = 500
		}
		catalogError(w, status, code, validation.Fields)
		return
	}
	var uncertain interface{ CommitUncertain() bool }
	if errors.As(err, &uncertain) && uncertain.CommitUncertain() {
		catalogError(w, 503, "commit_unknown", nil)
		return
	}
	switch store.SQLState(err) {
	case "23505":
		catalogError(w, 409, "duplicate_slug", nil)
		return
	case "23503", "23514":
		catalogError(w, 422, "validation", nil)
		return
	case "40001", "40P01":
		catalogError(w, 409, "transaction_conflict", nil)
		return
	case "57P01", "57P02", "57P03", "08000", "08003", "08006":
		catalogError(w, 503, "unavailable", nil)
		return
	}
	var unavailable interface{ Unavailable() bool }
	if errors.As(err, &unavailable) && unavailable.Unavailable() {
		catalogError(w, 503, "unavailable", nil)
		return
	}
	var network net.Error
	if errors.As(err, &network) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		catalogError(w, 503, "unavailable", nil)
		return
	}
	catalogError(w, 500, "storage_error", nil)
}
func RegisterCatalog(mux *http.ServeMux, key string, service *catalog.Service) {
	private := http.NewServeMux()
	private.HandleFunc("GET /api/admin/catalog/categories", func(w http.ResponseWriter, r *http.Request) {
		data, err := service.Snapshot(r.Context())
		respondCatalog(w, data.Categories, err, 200)
	})
	private.HandleFunc("GET /api/admin/catalog/lots", func(w http.ResponseWriter, r *http.Request) {
		data, err := service.Snapshot(r.Context())
		respondCatalog(w, data.Lots, err, 200)
	})
	private.HandleFunc("GET /api/admin/catalog/events", func(w http.ResponseWriter, r *http.Request) {
		data, err := service.Snapshot(r.Context())
		respondCatalog(w, data.Events, err, 200)
	})
	private.HandleFunc("GET /api/admin/catalog/lots/{id}", func(w http.ResponseWriter, r *http.Request) {
		data, err := service.Lot(r.Context(), r.PathValue("id"))
		respondCatalog(w, data, err, 200)
	})
	private.HandleFunc("GET /api/admin/catalog/events/{id}", func(w http.ResponseWriter, r *http.Request) {
		data, err := service.Event(r.Context(), r.PathValue("id"))
		respondCatalog(w, data, err, 200)
	})
	saveLot := func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			catalog.LotContent
			Version int64 `json:"version"`
		}
		if !decodeCatalog(w, r, &request) {
			return
		}
		id := r.PathValue("id")
		data, err := service.SaveLot(r.Context(), id, request.Version, request.LotContent)
		status := 200
		if id == "" {
			status = 201
		}
		respondCatalog(w, data, err, status)
	}
	saveEvent := func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			catalog.EventContent
			Version int64 `json:"version"`
		}
		if !decodeCatalog(w, r, &request) {
			return
		}
		id := r.PathValue("id")
		data, err := service.SaveEvent(r.Context(), id, request.Version, request.EventContent)
		status := 200
		if id == "" {
			status = 201
		}
		respondCatalog(w, data, err, status)
	}
	private.HandleFunc("POST /api/admin/catalog/lots", saveLot)
	private.HandleFunc("PUT /api/admin/catalog/lots/{id}", saveLot)
	private.HandleFunc("POST /api/admin/catalog/events", saveEvent)
	private.HandleFunc("PUT /api/admin/catalog/events/{id}", saveEvent)
	private.HandleFunc("PUT /api/admin/catalog/events/{id}/lots", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Version int64    `json:"version"`
			LotIDs  []string `json:"lot_ids"`
		}
		if !decodeCatalog(w, r, &request) {
			return
		}
		data, err := service.Members(r.Context(), r.PathValue("id"), request.Version, request.LotIDs)
		respondCatalog(w, data, err, 200)
	})
	private.HandleFunc("POST /api/admin/catalog/events/{id}/publish", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Version int64 `json:"version"`
		}
		if !decodeCatalog(w, r, &request) {
			return
		}
		data, err := service.Publish(r.Context(), r.PathValue("id"), request.Version)
		respondCatalog(w, data, err, 200)
	})
	private.HandleFunc("GET /api/admin/catalog/events/{id}/preview", func(w http.ResponseWriter, r *http.Request) {
		data, err := service.Preview(r.Context(), r.PathValue("id"))
		if err != nil {
			respondCatalog(w, nil, err, 200)
			return
		}
		catalogJSON(w, 200, map[string]any{"preview": true, "data": data})
	})
	private.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { catalogError(w, 404, "not_found", nil) })
	guarded := CatalogAdminGuard(key, private)
	mux.Handle("/api/admin/catalog", guarded)
	mux.Handle("/api/admin/catalog/", guarded)
	mux.HandleFunc("GET /api/catalog/categories", func(w http.ResponseWriter, r *http.Request) {
		data, err := service.Snapshot(r.Context())
		respondCatalog(w, data.Categories, err, 200)
	})
	mux.HandleFunc("GET /api/catalog/events", func(w http.ResponseWriter, r *http.Request) {
		data, err := service.PublicEvents(r.Context())
		respondCatalog(w, data, err, 200)
	})
	mux.HandleFunc("GET /api/catalog/events/{slug}", func(w http.ResponseWriter, r *http.Request) {
		data, err := service.PublicEvent(r.Context(), r.PathValue("slug"))
		respondCatalog(w, data, err, 200)
	})
	mux.HandleFunc("GET /api/catalog/lots", func(w http.ResponseWriter, r *http.Request) {
		data, err := service.PublicLots(r.Context())
		respondCatalog(w, data, err, 200)
	})
	mux.HandleFunc("GET /api/catalog/lots/{slug}", func(w http.ResponseWriter, r *http.Request) {
		data, err := service.PublicLot(r.Context(), r.PathValue("slug"))
		respondCatalog(w, data, err, 200)
	})
}
