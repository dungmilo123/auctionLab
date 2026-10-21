package api

import (
	"auctionlab/internal/account"
	"auctionlab/internal/auction"
	"auctionlab/internal/order"
	"auctionlab/internal/recommendation"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const AccountCookieName = "auction_account"

type AccountAPI struct {
	Service         *account.Service
	Limiter         *account.Limiter
	Auctions        *auction.Service
	Orders          *order.Service
	Recommendations recommendation.Repository
}

func RegisterAccount(mux *http.ServeMux, s *account.Service, auctions ...*auction.Service) *AccountAPI {
	a := &AccountAPI{Service: s, Limiter: account.NewLimiter()}
	if len(auctions) > 0 {
		a.Auctions = auctions[0]
	}
	mux.Handle("/api/auth/", a)
	mux.Handle("/api/account", a)
	mux.Handle("/api/account/", a)
	return a
}
func accountDigest(r *http.Request) string {
	c, e := r.Cookie(AccountCookieName)
	if e != nil {
		return account.Digest("")
	}
	return account.Digest(c.Value)
}
func accountToken(r *http.Request) string {
	c, e := r.Cookie(AccountCookieName)
	if e != nil {
		return ""
	}
	return c.Value
}
func effectiveHTTPS(r *http.Request) bool {
	scheme := r.TLS != nil
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			if p := r.Header.Get("X-Forwarded-Proto"); p == "http" || p == "https" {
				scheme = p == "https"
			}
		}
	}
	return scheme
}
func accountCookie(w http.ResponseWriter, r *http.Request, token string, now time.Time) {
	c := &http.Cookie{Name: AccountCookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: effectiveHTTPS(r), MaxAge: int(account.SessionLifetime.Seconds()), Expires: now.Add(account.SessionLifetime)}
	if token == "" {
		c.MaxAge = -1
		c.Expires = time.Unix(1, 0)
	}
	http.SetCookie(w, c)
}
func accountError(w http.ResponseWriter, e error) {
	status, code := 503, "unavailable"
	var fields map[string]string
	var ae *account.Error
	if errors.As(e, &ae) {
		code = ae.Code
		fields = ae.Fields
		switch code {
		case "validation":
			status = 422
		case "duplicate_email":
			status = 409
		case "access_required", "invalid_credentials":
			status = 401
		case "not_found":
			status = 404
		}
	}
	var uncertain interface{ CommitUncertain() bool }
	if errors.As(e, &uncertain) && uncertain.CommitUncertain() {
		status = 503
		code = "commit_unknown"
	}
	catalogError(w, status, code, fields)
}
func accountDecode(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	var raw json.RawMessage
	if e := decoder.Decode(&raw); e != nil {
		catalogError(w, 400, "malformed_json", nil)
		return false
	}
	var fields map[string]json.RawMessage
	if !utf8.Valid(raw) || len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &fields) != nil || fields == nil {
		catalogError(w, 400, "malformed_json", nil)
		return false
	}
	var tree any
	if json.Unmarshal(raw, &tree) != nil || containsJSONNull(tree) {
		catalogError(w, 400, "malformed_json", nil)
		return false
	}
	if decoder.Decode(new(any)) != io.EOF {
		catalogError(w, 400, "malformed_json", nil)
		return false
	}
	strict := json.NewDecoder(strings.NewReader(string(raw)))
	strict.DisallowUnknownFields()
	if strict.Decode(out) != nil {
		catalogError(w, 400, "malformed_json", nil)
		return false
	}
	return true
}
func accountWriteBoundary(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("X-AuctionLab-Account") != "1" || !catalogOrigin(r) {
		catalogError(w, 403, "request_rejected", nil)
		return false
	}
	typ, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || typ != "application/json" {
		catalogError(w, 415, "json_required", nil)
		return false
	}
	return true
}
func (a *AccountAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := a.Service.Now().UnixMilli()
	digest := accountDigest(r)
	// Guard the whole private namespace, including unknown paths and methods.
	if r.URL.Path == "/api/account" || strings.HasPrefix(r.URL.Path, "/api/account/") {
		if _, e := a.Service.Repo.Profile(ctx, digest, now); e != nil {
			accountError(w, e)
			return
		}
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		if !accountWriteBoundary(w, r) {
			return
		}
	}
	respond := func(data any, e error, status int) {
		if e != nil {
			accountError(w, e)
		} else {
			catalogJSON(w, status, map[string]any{"data": data})
		}
	}
	switch {
	case r.URL.Path == "/api/account/recommendations":
		serveRecommendations(w, r, a.Recommendations, digest, a.Service.Now())
	case r.URL.Path == "/api/account/orders" || r.URL.Path == "/api/account/wins" || strings.HasPrefix(r.URL.Path, "/api/account/orders/"):
		a.checkout(w, r, digest)
	case r.URL.Path == "/api/account/bids" || strings.HasPrefix(r.URL.Path, "/api/account/bids/"):
		a.auction(w, r, digest)
	case r.Method == "POST" && (r.URL.Path == "/api/auth/register" || r.URL.Path == "/api/auth/login"):
		done, ok := a.Limiter.Enter(r.RemoteAddr)
		if !ok {
			w.Header().Set("Retry-After", "60")
			catalogError(w, 429, "throttled", nil)
			return
		}
		defer done()
		var p account.Profile
		var token string
		var e error
		if r.URL.Path == "/api/auth/register" {
			var body struct {
				Email    string `json:"email"`
				Password string `json:"password"`
				Name     string `json:"display_name"`
			}
			if !accountDecode(w, r, &body) {
				return
			}
			p, token, e = a.Service.Register(ctx, body.Email, body.Password, body.Name, accountToken(r))
		} else {
			var body struct {
				Email    string `json:"email"`
				Password string `json:"password"`
			}
			if !accountDecode(w, r, &body) {
				return
			}
			p, token, e = a.Service.Login(ctx, body.Email, body.Password, accountToken(r))
		}
		status := 200
		if r.URL.Path == "/api/auth/register" {
			status = 201
		}
		if e == nil {
			accountCookie(w, r, token, a.Service.Now())
		}
		respond(p, e, status)
	case r.Method == "POST" && r.URL.Path == "/api/auth/logout":
		if !accountDecode(w, r, &struct{}{}) {
			return
		}
		e := a.Service.Repo.Logout(ctx, digest, now)
		if e == nil {
			accountCookie(w, r, "", a.Service.Now())
		}
		respond(map[string]bool{"signed_out": true}, e, 200)
	case r.URL.Path == "/api/account" && r.Method == "GET":
		p, e := a.Service.Repo.Profile(ctx, digest, now)
		respond(p, e, 200)
	case r.URL.Path == "/api/account" && r.Method == "PATCH":
		var body struct {
			Name string `json:"display_name"`
		}
		if !accountDecode(w, r, &body) {
			return
		}
		name, e := account.Name(body.Name)
		if e != nil {
			accountError(w, e)
			return
		}
		p, e := a.Service.Repo.Update(ctx, digest, name, now)
		respond(p, e, 200)
	case r.URL.Path == "/api/account/watchlist" && r.Method == "GET":
		lots, e := a.Service.Repo.Watchlist(ctx, digest, now)
		if e != nil {
			accountError(w, e)
			return
		}
		catalogJSON(w, 200, map[string]any{"data": lots, "meta": map[string]int64{"server_time": now}})
	case strings.HasPrefix(r.URL.Path, "/api/account/watchlist/"):
		id := strings.TrimPrefix(r.URL.Path, "/api/account/watchlist/")
		if id == "" || strings.Contains(id, "/") {
			catalogError(w, 404, "not_found", nil)
			return
		}
		var saved *bool
		switch r.Method {
		case "GET":
		case "PUT", "DELETE":
			if !accountDecode(w, r, &struct{}{}) {
				return
			}
			v := r.Method == "PUT"
			saved = &v
		default:
			catalogError(w, 404, "not_found", nil)
			return
		}
		value, e := a.Service.Repo.Membership(ctx, digest, id, saved, now)
		respond(map[string]bool{"saved": value}, e, 200)
	default:
		catalogError(w, 404, "not_found", nil)
	}
}

func containsJSONNull(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case map[string]any:
		for _, child := range x {
			if containsJSONNull(child) {
				return true
			}
		}
	case []any:
		for _, child := range x {
			if containsJSONNull(child) {
				return true
			}
		}
	}
	return false
}
