package api

import (
	"auctionlab/internal/interaction"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"unicode/utf8"
)

var observationLotID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,120}$`)

func RegisterInteractions(mux *http.ServeMux, s interaction.Service) {
	limiter := interaction.NewLimiter()
	mux.HandleFunc("/api/catalog/lot-views", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			catalogError(w, 405, "method_not_allowed", nil)
			return
		}
		if !limiter.Allow(r.RemoteAddr) {
			w.Header().Set("Retry-After", "60")
			catalogError(w, 429, "throttled", nil)
			return
		}
		if r.Header.Get("X-AuctionLab-Interaction") != "1" || !catalogOrigin(r) {
			catalogError(w, 403, "request_rejected", nil)
			return
		}
		typ, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if e != nil || typ != "application/json" {
			catalogError(w, 415, "json_required", nil)
			return
		}
		raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
		if e != nil {
			var size *http.MaxBytesError
			if errors.As(e, &size) {
				catalogError(w, 413, "too_large", nil)
			} else {
				catalogError(w, 400, "malformed_json", nil)
			}
			return
		}
		var fields map[string]json.RawMessage
		if !utf8.Valid(raw) || json.Unmarshal(raw, &fields) != nil || fields == nil {
			catalogError(w, 400, "malformed_json", nil)
			return
		}
		// Token decode also rejects duplicate field names, unlike map unmarshalling.
		dec := json.NewDecoder(bytes.NewReader(raw))
		if _, e = dec.Token(); e != nil {
			catalogError(w, 400, "malformed_json", nil)
			return
		}
		seen := map[string]bool{}
		for dec.More() {
			key, e := dec.Token()
			if e != nil {
				catalogError(w, 400, "malformed_json", nil)
				return
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				catalogError(w, 400, "malformed_json", nil)
				return
			}
			seen[name] = true
			var value any
			if dec.Decode(&value) != nil {
				catalogError(w, 400, "malformed_json", nil)
				return
			}
		}
		if len(fields) != 2 || fields["lot_id"] == nil || fields["observation_id"] == nil {
			catalogError(w, 400, "malformed_json", nil)
			return
		}
		var lot, id string
		if json.Unmarshal(fields["lot_id"], &lot) != nil || json.Unmarshal(fields["observation_id"], &id) != nil || !observationLotID.MatchString(lot) || !interaction.UUID.MatchString(id) {
			catalogError(w, 422, "validation", nil)
			return
		}
		e = s.Observe(r.Context(), lot, id)
		switch {
		case e == nil:
			catalogJSON(w, 200, map[string]any{"data": map[string]bool{"recorded": true}})
		case errors.Is(e, interaction.ErrNotFound):
			catalogError(w, 404, "not_found", nil)
		case errors.Is(e, interaction.ErrConflict):
			catalogError(w, 409, "observation_conflict", nil)
		default:
			catalogError(w, 503, "unavailable", nil)
		}
	})
}
