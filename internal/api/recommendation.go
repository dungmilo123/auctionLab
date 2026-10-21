package api

import (
	"auctionlab/internal/catalog"
	"auctionlab/internal/recommendation"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

var discoverySlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func sourceFilter(raw string) (string, bool) {
	q, e := url.ParseQuery(raw)
	if e != nil {
		return "", false
	}
	source := ""
	for k, v := range q {
		if k != "source" || len(v) != 1 || len(v[0]) > 120 || !discoverySlug.MatchString(v[0]) {
			return "", false
		}
		source = v[0]
	}
	return source, true
}
func serveRecommendations(w http.ResponseWriter, r *http.Request, repo recommendation.Repository, digest string, now time.Time) {
	if r.Method != "GET" {
		catalogError(w, 405, "method_not_allowed", nil)
		return
	}
	source, ok := sourceFilter(r.URL.RawQuery)
	if !ok {
		catalogError(w, 422, "validation", nil)
		return
	}
	if repo == nil {
		catalogError(w, 503, "unavailable", nil)
		return
	}
	v, e := repo.Recommend(r.Context(), digest, source, now.UnixMilli())
	if e != nil {
		var ce *catalog.Error
		if errors.As(e, &ce) && ce.Code == "not_found" {
			catalogError(w, 404, "not_found", nil)
		} else {
			accountError(w, e)
		}
		return
	}
	catalogJSON(w, 200, map[string]any{"data": v, "meta": map[string]any{"server_time": now.UnixMilli(), "strategy": "rules-v1"}})
}
func RegisterRecommendations(mux *http.ServeMux, repo recommendation.Repository, now func() time.Time) {
	mux.HandleFunc("/api/catalog/recommendations", func(w http.ResponseWriter, r *http.Request) { serveRecommendations(w, r, repo, "", now()) })
}
