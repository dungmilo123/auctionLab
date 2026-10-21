package api

import (
	"auctionlab/internal/order"
	"errors"
	"net/http"
	"strings"
)

func orderError(w http.ResponseWriter, e error) {
	var oe *order.Error
	if !errors.As(e, &oe) {
		accountError(w, e)
		return
	}
	status := 503
	switch oe.Code {
	case "not_found":
		status = 404
	case "validation":
		status = 422
	case "request_conflict":
		status = 409
	}
	catalogError(w, status, oe.Code, oe.Fields)
}
func (a *AccountAPI) checkout(w http.ResponseWriter, r *http.Request, digest string) {
	if a.Orders == nil {
		catalogError(w, 503, "unavailable", nil)
		return
	}
	s := a.Orders
	ctx := r.Context()
	send := func(v any, e error) {
		if e != nil {
			orderError(w, e)
		} else {
			catalogJSON(w, 200, map[string]any{"data": v})
		}
	}
	if r.URL.Path == "/api/account/wins" {
		if r.Method != "GET" {
			catalogError(w, 404, "not_found", nil)
			return
		}
		v, e := s.Repo.Wins(ctx, digest, s.Now)
		send(v, e)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/account/orders/"), "/")
	if len(parts) < 1 || parts[0] == "" || len(parts) > 2 {
		catalogError(w, 404, "not_found", nil)
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == "GET" {
		v, e := s.Repo.Get(ctx, digest, id, s.Now)
		send(v, e)
		return
	}
	if len(parts) == 2 && r.Method == "POST" {
		switch parts[1] {
		case "review":
			var body order.Selection
			if !accountDecode(w, r, &body) {
				return
			}
			o, e := s.Repo.Get(ctx, digest, id, s.Now)
			if e != nil {
				orderError(w, e)
				return
			}
			v, e := order.MakeReview(o, body)
			send(v, e)
			return
		case "confirm":
			var body order.Intent
			if !accountDecode(w, r, &body) {
				return
			}
			v, e := s.Repo.Confirm(ctx, digest, id, body, s.Now)
			send(v, e)
			return
		}
	}
	catalogError(w, 404, "not_found", nil)
}
