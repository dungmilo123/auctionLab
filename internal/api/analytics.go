package api

import (
	"auctionlab/internal/analytics"
	"auctionlab/internal/catalog"
	"errors"
	"net/http"
	"net/url"
)

func analyticsFilters(raw string) (analytics.Filters, bool) {
	f := analytics.Filters{Event: "all", Dataset: "all"}
	q, e := url.ParseQuery(raw)
	if e != nil {
		return f, false
	}
	for k, v := range q {
		if len(v) != 1 || v[0] == "" {
			return f, false
		}
		switch k {
		case "event":
			f.Event = v[0]
		case "dataset":
			f.Dataset = v[0]
		default:
			return f, false
		}
	}
	return f, (f.Event == "all" || (len(f.Event) <= 120 && discoverySlug.MatchString(f.Event))) && (f.Dataset == "all" || f.Dataset == "demo" || f.Dataset == "non-demo")
}
func RegisterAnalytics(mux *http.ServeMux, key string, repo analytics.Repository) {
	h := CatalogAdminGuard(key, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/analytics" || r.Method != "GET" {
			catalogError(w, 404, "not_found", nil)
			return
		}
		f, ok := analyticsFilters(r.URL.RawQuery)
		if !ok {
			catalogError(w, 422, "validation", nil)
			return
		}
		out, e := repo.Report(r.Context(), f)
		if e != nil {
			var ce *catalog.Error
			if errors.As(e, &ce) && ce.Code == "not_found" {
				catalogError(w, 404, "not_found", nil)
			} else {
				catalogError(w, 503, "unavailable", nil)
			}
			return
		}
		catalogJSON(w, 200, out)
	}))
	mux.Handle("/api/admin/analytics", h)
	mux.Handle("/api/admin/analytics/", h)
}
