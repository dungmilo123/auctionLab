package api

import (
	"auctionlab/internal/auction"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AuctionAPI owns read-only public streams and cancels them before HTTP shutdown.
type AuctionAPI struct {
	Service        *auction.Service
	ctx            context.Context
	cancel         context.CancelFunc
	slots          chan struct{}
	once           sync.Once
	SampleInterval time.Duration
	WriteTimeout   time.Duration
}

func RegisterAuction(mux *http.ServeMux, s *auction.Service) *AuctionAPI {
	ctx, cancel := context.WithCancel(context.Background())
	a := &AuctionAPI{Service: s, ctx: ctx, cancel: cancel, slots: make(chan struct{}, 64), SampleInterval: time.Second, WriteTimeout: 5 * time.Second}
	mux.Handle("/api/auctions/", a)
	return a
}
func (a *AuctionAPI) Close() { a.once.Do(a.cancel) }
func auctionResponse(w http.ResponseWriter, status int, data any, now int64) {
	catalogJSON(w, status, map[string]any{"data": data, "meta": map[string]int64{"server_time": now}})
}
func auctionError(w http.ResponseWriter, e error, snapshot *auction.Snapshot) {
	code, status := "unavailable", 503
	var ae *auction.Error
	if errors.As(e, &ae) {
		code = ae.Code
		switch code {
		case "not_found":
			status = 404
		case "invalid_request":
			status = 400
		case "invalid_amount":
			status = 422
		default:
			status = 409
		}
	}
	var ac interface{ CommitUncertain() bool }
	if errors.As(e, &ac) && ac.CommitUncertain() {
		code = "commit_unknown"
		status = 503
	}
	// Session errors are mapped by the existing account error writer.
	if ae == nil && code != "commit_unknown" {
		accountError(w, e)
		return
	}
	body := map[string]any{"error": map[string]string{"code": code}}
	if snapshot != nil {
		body["data"] = map[string]any{"snapshot": snapshot}
		body["meta"] = map[string]int64{"server_time": snapshot.ServerTime}
	}
	catalogJSON(w, status, body)
}
func (a *AuctionAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		catalogError(w, 404, "not_found", nil)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/auctions/lots/")
	if path == r.URL.Path {
		catalogError(w, 404, "not_found", nil)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) > 2 || parts[0] == "" {
		catalogError(w, 404, "not_found", nil)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "stream" {
		a.stream(w, r, id)
		return
	}
	if len(parts) == 2 && parts[1] == "bids" {
		before := int64(0)
		if v := r.URL.Query().Get("before_sequence"); v != "" {
			var e error
			before, e = strconv.ParseInt(v, 10, 64)
			if e != nil || before <= 0 {
				catalogError(w, 400, "invalid_cursor", nil)
				return
			}
		}
		h, e := a.Service.Repo.History(r.Context(), id, before)
		if e != nil {
			auctionError(w, e, nil)
			return
		}
		auctionResponse(w, 200, h, a.Service.Now().UnixMilli())
		return
	}
	if len(parts) != 1 {
		catalogError(w, 404, "not_found", nil)
		return
	}
	s, e := a.Service.Repo.Snapshot(r.Context(), id, a.Service.Now)
	if e != nil {
		auctionError(w, e, nil)
		return
	}
	auctionResponse(w, 200, s, s.ServerTime)
}
func (a *AuctionAPI) stream(w http.ResponseWriter, r *http.Request, id string) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(a.ctx, cancel)
	defer stop()
	s, e := a.Service.Repo.Snapshot(ctx, id, a.Service.Now)
	if e != nil {
		auctionError(w, e, nil)
		return
	}
	select {
	case a.slots <- struct{}{}:
		defer func() { <-a.slots }()
	default:
		w.Header().Set("Retry-After", "2")
		catalogError(w, 503, "stream_capacity", nil)
		return
	}
	if _, ok := w.(http.Flusher); !ok {
		catalogError(w, 503, "unavailable", nil)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	send := func(event string, data any, revision int64) bool {
		if e := controller.SetWriteDeadline(time.Now().Add(a.WriteTimeout)); e != nil {
			return false
		}
		raw, e := json.Marshal(data)
		if e != nil {
			return false
		}
		if _, e = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", revision, event, raw); e != nil {
			return false
		}
		return controller.Flush() == nil
	}
	envelope := func(s auction.Snapshot) any {
		return map[string]any{"data": s, "meta": map[string]int64{"server_time": s.ServerTime}}
	}
	if !send("snapshot", envelope(s), s.Revision) {
		return
	}
	tick := time.NewTicker(a.SampleInterval)
	defer tick.Stop()
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			next, e := a.Service.Repo.Snapshot(ctx, id, a.Service.Now)
			if e != nil {
				return
			}
			if next.Revision != s.Revision || next.State != s.State {
				if !send("snapshot", envelope(next), next.Revision) {
					return
				}
				s = next
			}
		case <-heartbeat.C:
			if !send("heartbeat", map[string]any{"meta": map[string]int64{"server_time": a.Service.Now().UnixMilli()}}, s.Revision) {
				return
			}
		}
	}
}
func (a *AccountAPI) auction(w http.ResponseWriter, r *http.Request, digest string) {
	if a.Auctions == nil {
		catalogError(w, 503, "unavailable", nil)
		return
	}
	s := a.Auctions
	if r.Method == "POST" && r.URL.Path == "/api/account/bids" {
		var body struct {
			LotID     string          `json:"lot_id"`
			Amount    json.RawMessage `json:"amount_minor"`
			RequestID string          `json:"request_id"`
		}
		if !accountDecode(w, r, &body) {
			return
		}
		var amount int64
		if len(body.Amount) == 0 || json.Unmarshal(body.Amount, &amount) != nil {
			auctionError(w, auction.Fail("invalid_amount"), nil)
			return
		}
		receipt, e := s.Place(r.Context(), digest, auction.Intent{LotID: body.LotID, AmountMinor: &amount, RequestID: body.RequestID})
		var fresh *auction.Snapshot
		if body.LotID != "" {
			if snap, se := s.Repo.Snapshot(r.Context(), body.LotID, s.Now); se == nil {
				fresh = &snap
			}
		}
		if e != nil {
			auctionError(w, e, fresh)
			return
		}
		status := 201
		if receipt.Replay {
			status = 200
		}
		auctionResponse(w, status, map[string]any{"receipt": receipt, "snapshot": fresh}, s.Now().UnixMilli())
		return
	}
	if r.Method == "GET" {
		id := ""
		if r.URL.Path != "/api/account/bids" {
			id = strings.TrimPrefix(r.URL.Path, "/api/account/bids/")
			if id == "" || strings.Contains(id, "/") {
				catalogError(w, 404, "not_found", nil)
				return
			}
		}
		p, e := s.Repo.Participation(r.Context(), digest, id, s.Now)
		if e != nil {
			auctionError(w, e, nil)
			return
		}
		if id != "" {
			auctionResponse(w, 200, p[0], s.Now().UnixMilli())
		} else {
			auctionResponse(w, 200, p, s.Now().UnixMilli())
		}
		return
	}
	catalogError(w, 404, "not_found", nil)
}
