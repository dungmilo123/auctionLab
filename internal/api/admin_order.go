package api

import (
	"auctionlab/internal/order"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var orderIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func orderFilters(raw string) (order.Filters, bool) {
	f := order.Filters{Status: "all", Method: "all"}
	q, e := url.ParseQuery(raw)
	if e != nil {
		return f, false
	}
	for k, values := range q {
		if len(values) != 1 {
			return f, false
		}
		switch k {
		case "status":
			f.Status = values[0]
		case "method":
			f.Method = values[0]
		default:
			return f, false
		}
	}
	return f, f.Valid()
}
func decodeTransition(w http.ResponseWriter, r *http.Request) (order.Transition, bool) {
	var t order.Transition
	ok := accountDecode(w, r, &t)
	return t, ok
}
func RegisterOrders(mux *http.ServeMux, key string, s *order.Service) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		send := func(v any, e error) {
			if e != nil {
				orderError(w, e)
			} else {
				catalogJSON(w, 200, map[string]any{"data": v})
			}
		}
		if r.URL.Path == "/api/admin/orders" && r.Method == "GET" {
			f, ok := orderFilters(r.URL.RawQuery)
			if !ok {
				catalogError(w, 422, "validation", nil)
				return
			}
			v, e := s.Repo.AdminList(r.Context(), f)
			send(v, e)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/admin/orders/"), "/")
		if len(parts) > 2 || len(parts) < 1 || !orderIDPattern.MatchString(parts[0]) {
			catalogError(w, 404, "not_found", nil)
			return
		}
		if r.URL.RawQuery != "" {
			catalogError(w, 422, "validation", nil)
			return
		}
		if len(parts) == 1 && r.Method == "GET" {
			v, e := s.Repo.AdminGet(r.Context(), parts[0])
			send(v, e)
			return
		}
		if len(parts) == 2 && parts[1] == "transition" && r.Method == "POST" {
			t, ok := decodeTransition(w, r)
			if !ok {
				return
			}
			v, e := s.Repo.Transition(r.Context(), parts[0], t, s.Now)
			send(v, e)
			return
		}
		catalogError(w, 404, "not_found", nil)
	})
	guard := CatalogAdminGuard(key, handler)
	mux.Handle("/api/admin/orders", guard)
	mux.Handle("/api/admin/orders/", guard)
}
