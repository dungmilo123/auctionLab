package api

import (
	"auctionlab/internal/account"
	"auctionlab/internal/game"
	"auctionlab/internal/store"
	"auctionlab/internal/testpg"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAccountTwoOwnerRehearsal(t *testing.T) {
	raw := testpg.NewURL(t)
	st, e := store.Open(raw)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { st.Close() }()
	if e = st.Migrate(); e != nil {
		t.Fatal(e)
	}
	engine, e := game.New(game.Config{Store: st, Now: time.Now, Fatal: func(error) { t.Error("game commit failed") }})
	if e != nil {
		t.Fatal(e)
	}
	legacy, e := engine.Join(game.JoinRequest{Role: game.RolePlayer, Nickname: "Retained player"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	before := engine.Rev()
	s := account.New(store.AccountRepository{Store: st})
	mux := http.NewServeMux()
	RegisterAccount(mux, s)
	a := registerTestAccount(t, mux, "rehearse-a@example.test")
	b := registerTestAccount(t, mux, "rehearse-b@example.test")
	_, e = st.DB.Exec(`INSERT INTO catalog_lots(id,slug,title,state,created_at,updated_at) VALUES('lot','lot','Lot','published',1,1); INSERT INTO catalog_events(id,slug,title,state,created_at,updated_at) VALUES('event','event','Event','published',1,1); INSERT INTO catalog_event_lots VALUES('event','lot',0);`)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range []*http.Cookie{a, b} {
		if w := accountRequest(mux, "PUT", "/api/account/watchlist/lot", `{}`, c); w.Code != 200 {
			t.Fatal("save", w.Code)
		}
	}
	if w := accountRequest(mux, "DELETE", "/api/account/watchlist/lot", `{}`, b); w.Code != 200 {
		t.Fatal("remove")
	}
	if w := accountRequest(mux, "PATCH", "/api/account", `{"display_name":"Confirmed"}`, a); w.Code != 200 {
		t.Fatal("profile")
	}
	if w := accountRequest(mux, "POST", "/api/auth/logout", `{}`, a); w.Code != 200 {
		t.Fatal("logout")
	}
	if w := accountRequest(mux, "GET", "/api/account", "", a); w.Code != 401 {
		t.Fatal("cookie replay")
	}
	w := accountRequest(mux, "POST", "/api/auth/login", `{"email":"rehearse-a@example.test","password":"fixture unicode 密碼"}`, nil)
	if w.Code != 200 {
		t.Fatal("later login")
	}
	a = w.Result().Cookies()[0]
	if _, e = s.Provision(context.Background(), [3]string{"seed one fixture password", "seed two fixture password", "seed admin fixture password"}); e != nil {
		t.Fatal(e)
	}
	if done, e := s.Provision(context.Background(), [3]string{}); e != nil || !done {
		t.Fatal("seed repeat")
	}
	current, e := st.LoadGame()
	if e != nil || current.Revision != before || engine.Rev() != before {
		t.Fatal("accounts mutated game")
	}
	players, e := st.LoadPlayers()
	if e != nil || len(players) != 1 || players[0].ID != legacy.PlayerID || players[0].Token != legacy.Token {
		t.Fatal("legacy identity changed")
	}
	assertLists := func() {
		t.Helper()
		for _, c := range []struct {
			cookie *http.Cookie
			count  int
		}{{a, 1}, {b, 0}} {
			w := accountRequest(mux, "GET", "/api/account/watchlist", "", c.cookie)
			var body struct{ Data []json.RawMessage }
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != 200 || len(body.Data) != c.count {
				t.Fatal("membership isolation/durability", w.Code)
			}
		}
	}
	assertLists()
	for _, mode := range []game.ResetMode{game.ResetKeepPlayers, game.ResetClearPlayers} {
		if _, e = engine.Reset(mode); e != nil {
			t.Fatal(e)
		}
		assertLists()
	}
	if e = st.Close(); e != nil {
		t.Fatal(e)
	}
	st, e = store.Open(raw)
	if e != nil {
		t.Fatal(e)
	}
	if e = st.Migrate(); e != nil {
		t.Fatal(e)
	}
	s = account.New(store.AccountRepository{Store: st})
	mux = http.NewServeMux()
	RegisterAccount(mux, s)
	assertLists()
	w = accountRequest(mux, "GET", "/api/account", "", a)
	if !strings.Contains(w.Body.String(), "Confirmed") {
		t.Fatal("profile lost")
	}
	var published bool
	st.DB.QueryRow(`SELECT state='published' FROM catalog_events WHERE id='event'`).Scan(&published)
	if !published {
		t.Fatal("publication changed")
	}
	s.Now = func() time.Time { return time.Now().Add(account.SessionLifetime + time.Second) }
	if w = accountRequest(mux, "GET", "/api/account", "", a); w.Code != 401 {
		t.Fatal("fixed expiry")
	}
}
