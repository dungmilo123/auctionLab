package game

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// This file is the permanent leak lock (D-26). It is deliberately written
// against the *marshaled JSON* rather than against the Go types: the type
// boundary is the mechanism, but the wire is what a player can actually read, so
// the assertions walk the JSON the socket would carry.
//
// The fixture is populated with the secret fields on purpose — a leak test over
// an empty state proves nothing, which is exactly what D-26's positive control
// exists to prevent.

const (
	// fixtureTruthValue is the item's secret common value: it must appear in the
	// admin view and in none of the player/screen views.
	fixtureTruthValue = int64(350_000)
	// fixtureSealedAmount is one sealed bid's amount: same rule.
	fixtureSealedAmount = int64(654_321)
	// fixtureToken is one player's session token: admin only, and the only
	// message allowed to carry a token is that player's own hello (D-29).
	fixtureToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

// forbiddenKey is D-26's key rule: the common value, any sealed-bid key, and any
// token key. `sealed_bids` (the collection) matches the sealed_bid prefix, which
// is intended — the collection is a secret too.
func forbiddenKey(key string) bool {
	return key == "truth_value" || key == "token" || strings.HasPrefix(key, "sealed_bid")
}

// walkKeysAt visits every key in a decoded JSON document together with the path
// that reaches it. The path is what lets the settle reveal be allowed only
// UNDER the settle object while a top-level truth_value stays a leak (D-67):
// the boundary is the presence of the object, not of the field.
func walkKeysAt(value any, path []string, visit func(path []string, key string)) {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			next := append(append([]string(nil), path...), key)
			visit(next, key)
			walkKeysAt(child, next, visit)
		}
	case []any:
		for _, child := range node {
			walkKeysAt(child, path, visit)
		}
	}
}

// insideSettle reports whether a key path passes through the settle object.
func insideSettle(path []string) bool {
	for _, p := range path {
		if p == "settle" {
			return true
		}
	}
	return false
}

// forbiddenKeyAt is D-26 extended by D-67: every key forbiddenKey names, except
// that `truth_value` is legal INSIDE the settle object — and only there.
func forbiddenKeyAt(path []string, key string) bool {
	if key == "truth_value" {
		return !insideSettle(path)
	}
	return forbiddenKey(key)
}

// assertNoSecrets fails on every forbidden key in one decoded payload.
func assertNoSecrets(t *testing.T, name string, doc map[string]any) {
	t.Helper()

	walkKeysAt(doc, nil, func(path []string, key string) {
		if forbiddenKeyAt(path, key) {
			t.Errorf("%s carries the forbidden key %q at %v: a player/screen payload must not be able to express a secret (D-26)", name, key, path)
		}
	})
}

// auctionFixture is populatedState() wired to one Phase 3 scenario: the item is
// the current one, the run carries two accepted bids, and the phase decides
// which fields the projection must (and must not) carry. For SETTLED the reveal
// and the winner's already-updated economics are present, because that is what
// the real post-settle state looks like — an empty fixture would prove nothing
// (D-26's positive control).
func auctionFixture(phase Phase) *state {
	s := populatedState()
	it := s.items["it-1"]
	it.minIncrement = 10_000
	it.durationMS = 30_000
	it.antiSnipeMode = "extend"
	it.antiSnipeWindowMS = 10_000
	it.maxExtensions = 2

	startedAt := testStart.UnixMilli()
	run := &auctionRun{itemID: "it-1", startedAt: startedAt, deadlineAt: startedAt + 30_000}
	s.bids = []bid{
		{itemID: "it-1", playerID: "p-an", amount: 100_000, createdAt: startedAt + 1_000},
		{itemID: "it-1", playerID: "p-binh", amount: 110_000, createdAt: startedAt + 2_000},
	}

	switch phase {
	case PhaseItemIntro:
		s.phase, it.status = PhaseItemIntro, "intro"
	case PhaseLive:
		s.phase, it.status = PhaseLive, "live"
		run.extensionsUsed = 0
		s.currentAuction = run
	case PhaseSettled:
		s.phase, it.status = PhaseSettled, "settled"
		run.settle = &SettleInfo{
			Outcome: "sold", Winner: &Bidder{Nickname: "Bình"},
			Price: 110_000, TruthValue: fixtureTruthValue, BudgetDelta: fixtureTruthValue - 110_000,
		}
		run.settledAt = run.deadlineAt
		s.currentAuction = run
		s.players["p-binh"].budget = 750_000 - 110_000 + fixtureTruthValue
		s.players["p-binh"].totalValue = fixtureTruthValue
	}
	return s
}

// decodeObject marshals a projection and decodes it as the JSON object the wire
// would carry. A nil projection (or a null document) is a failure here: the role
// has no view, and every assertion below would otherwise pass vacuously.
func decodeObject(t *testing.T, payload any) map[string]any {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshaling the projection: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the projection is not a JSON object: %v\n%s", err, raw)
	}
	if doc == nil {
		t.Fatalf("the projection marshaled to %s: the role has no view", raw)
	}
	return doc
}

// asObject asserts one field is a JSON object.
func asObject(t *testing.T, value any, what string) map[string]any {
	t.Helper()

	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want a JSON object", what, value)
	}
	return obj
}

// asArray asserts one field is a JSON array.
func asArray(t *testing.T, value any, what string) []any {
	t.Helper()

	arr, ok := value.([]any)
	if !ok {
		t.Fatalf("%s = %#v, want a JSON array", what, value)
	}
	return arr
}

// populatedState is the synthetic fixture: two players (one connected), one live
// item carrying a truth value and a hint range, and one sealed bid. It is
// installed directly onto the engine because Phase 2 never loads items — the
// item flow is Phase 3 — and the leak boundary must still be provable today.
func populatedState() *state {
	return &state{
		phase:         PhaseLobby,
		revision:      7,
		currentItemID: "it-1",
		config:        engineConfig{StartingBudget: 1_000_000, Currency: "VND"},
		players: map[string]*player{
			"p-an": {
				id: "p-an", nickname: "An", nicknameKey: "an", token: fixtureToken,
				budget: 1_000_000, totalValue: 250_000,
				joinedAt: testStart.UnixMilli(), lastSeenAt: testStart.UnixMilli(),
			},
			"p-binh": {
				id: "p-binh", nickname: "Bình", nicknameKey: "bình", token: strings.Repeat("b", 64),
				budget: 750_000, totalValue: 0,
				joinedAt: testStart.Add(3_000).UnixMilli(), lastSeenAt: testStart.Add(3_000).UnixMilli(),
			},
		},
		order:     []string{"p-an", "p-binh"},
		connected: map[string]bool{"p-an": true, "p-binh": false},
		items: map[string]*item{
			"it-1": {
				id: "it-1", orderIndex: 1, name: "Bức tranh", description: "Sơn dầu",
				auctionType: "english", truthValue: fixtureTruthValue,
				hintMin: 200_000, hintMax: 500_000, startPrice: 100_000, status: "live",
			},
		},
		itemOrder: []string{"it-1"},
		sealedBids: []sealedBid{{
			itemID: "it-1", playerID: "p-an", amount: fixtureSealedAmount,
			submittedAt: testStart.UnixMilli(), updatedAt: testStart.UnixMilli(),
		}},
	}
}

// installState swaps the engine's state for a synthetic fixture.
func (env *testEnv) installState(next *state) {
	env.engine.mu.Lock()
	defer env.engine.mu.Unlock()
	env.engine.state = next
	env.engine.revision.Store(next.revision)
}

// TestPlayerAndScreenPayloadsLeakNothing is RT-01's contract: every player and
// screen wire candidate is recursively inspected for the keys D-26 forbids, and
// — the other half of the assertion — the snapshots must actually carry the data
// the UI needs, so an accidentally empty projection cannot pass by being empty.
//
// The table covers every message a player or the screen can receive in Phase 2:
// the two snapshots and `hello`. `hello` is the one message allowed to carry a
// token, and only the token of the player it is addressed to (D-29) — which is
// why the exception is a column here rather than a comment.
func TestPlayerAndScreenPayloadsLeakNothing(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))

	t.Run("player hello carries only the caller's own token", func(t *testing.T) {
		doc := decodeObject(t, Hello{
			Role:     RolePlayer,
			PlayerID: "p-an",
			Nickname: "An",
			Token:    fixtureToken,
			Rev:      7,
		})

		walkKeysAt(doc, nil, func(path []string, key string) {
			if key == "token" {
				return // D-29's single exception: the caller's own token in hello
			}
			if forbiddenKeyAt(path, key) {
				t.Errorf("hello carries the forbidden key %q at %v", key, path)
			}
		})
		if doc["token"] != fixtureToken {
			t.Errorf("hello carries %#v for token, want the caller's own token — it is the one place a token travels", doc["token"])
		}
	})

	t.Run("the populated LOBBY fixture stays secret-free and self-sufficient", func(t *testing.T) {
		env.installState(populatedState())

		for _, tc := range []struct {
			name     string
			role     Role
			playerID string
			wantMe   bool
		}{
			{name: "player self view", role: RolePlayer, playerID: "p-an", wantMe: true},
			{name: "screen view", role: RoleScreen},
		} {
			t.Run(tc.name, func(t *testing.T) {
				doc := decodeObject(t, env.engine.Snapshot(tc.role, tc.playerID))
				assertNoSecrets(t, tc.name, doc)

				// D-25: the snapshot must be self-sufficient, not merely secret-free.
				if doc["phase"] != string(PhaseLobby) {
					t.Errorf("phase = %#v, want %q", doc["phase"], PhaseLobby)
				}
				if doc["revision"] != float64(7) {
					t.Errorf("revision = %#v, want 7", doc["revision"])
				}
				if doc["server_ts"] != float64(testStart.UnixMilli()) {
					t.Errorf("server_ts = %#v, want the injected clock %d", doc["server_ts"], testStart.UnixMilli())
				}

				lobby := asArray(t, doc["lobby"], "lobby")
				if len(lobby) != 2 {
					t.Fatalf("lobby has %d entries, want 2", len(lobby))
				}
				first := asObject(t, lobby[0], "lobby[0]")
				if first["nickname"] != "An" || first["connected"] != true {
					t.Errorf("lobby[0] = %#v, want An connected", first)
				}
				if _, hasBudget := first["budget"]; hasBudget {
					t.Errorf("lobby[0] carries another player's budget: %#v", first)
				}

				if got := len(asArray(t, doc["leaderboard"], "leaderboard")); got != 2 {
					t.Errorf("leaderboard has %d entries, want 2", got)
				}

				// The current item carries its public parameters and the hint range.
				item := asObject(t, doc["item"], "item")
				if item["name"] != "Bức tranh" || item["start_price"] != float64(100_000) {
					t.Errorf("item = %#v, want the public item parameters", item)
				}
				if item["hint_min"] != float64(200_000) || item["hint_max"] != float64(500_000) {
					t.Errorf("item hint range = %#v..%#v, want 200000..500000", item["hint_min"], item["hint_max"])
				}

				if !tc.wantMe {
					if _, hasMe := doc["me"]; hasMe {
						t.Errorf("the screen view carries a `me` section: the projector has no identity")
					}
					return
				}

				me := asObject(t, doc["me"], "me")
				if me["id"] != "p-an" || me["nickname"] != "An" {
					t.Errorf("me = %#v, want the joined identity {id: p-an, nickname: An}", me)
				}
				if me["budget"] != float64(1_000_000) || me["total_value"] != float64(250_000) {
					t.Errorf("me budget/total_value = %#v/%#v, want 1000000/250000", me["budget"], me["total_value"])
				}
				if me["connected"] != true {
					t.Errorf("me.connected = %#v, want true", me["connected"])
				}
			})
		}
	})

	// Phase 3's half of the lock (D-67/D-68): the live fields must be present
	// where the UI reads them, and the settle object must exist ONLY at SETTLED —
	// its presence is the boundary, so an absent-before / present-after pair of
	// assertions is the whole test.
	scenarios := []struct {
		name       string
		phase      Phase
		role       Role
		playerID   string
		wantSettle bool
	}{
		{name: "ITEM_INTRO player view", phase: PhaseItemIntro, role: RolePlayer, playerID: "p-an"},
		{name: "LIVE player view", phase: PhaseLive, role: RolePlayer, playerID: "p-an"},
		{name: "LIVE screen view", phase: PhaseLive, role: RoleScreen},
		{name: "SETTLED player view", phase: PhaseSettled, role: RolePlayer, playerID: "p-binh", wantSettle: true},
		{name: "SETTLED screen view", phase: PhaseSettled, role: RoleScreen, wantSettle: true},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			env.installState(auctionFixture(sc.phase))
			doc := decodeObject(t, env.engine.Snapshot(sc.role, sc.playerID))
			assertNoSecrets(t, sc.name, doc)

			item := asObject(t, doc["item"], "item")
			settle, hasSettle := item["settle"]
			if hasSettle != sc.wantSettle {
				t.Fatalf("item.settle present = %t, want %t (D-67: the leak boundary is the object's presence)", hasSettle, sc.wantSettle)
			}
			if !sc.wantSettle {
				walkKeysAt(doc, nil, func(path []string, key string) {
					if key == "settle" {
						t.Errorf("%s carries a settle key at %v before SETTLED", sc.name, path)
					}
				})
			}

			if sc.role == RoleScreen {
				if _, hasMe := doc["me"]; hasMe {
					t.Errorf("the screen view carries a `me` section: the projector has no identity")
				}
			}

			switch sc.phase {
			case PhaseItemIntro:
				if item["status"] != "intro" {
					t.Errorf("ITEM_INTRO item.status = %#v, want intro", item["status"])
				}
			case PhaseLive:
				for _, key := range []string{"current_price", "min_next_bid", "deadline_at", "top_bidder"} {
					if _, ok := item[key]; !ok {
						t.Errorf("a LIVE item must carry %q so a late joiner renders without replaying events (ENGL-01/SESS-04)", key)
					}
				}
				if item["current_price"] != float64(110_000) {
					t.Errorf("current_price = %#v, want the top bid 110000", item["current_price"])
				}
				if item["min_next_bid"] != float64(120_000) {
					t.Errorf("min_next_bid = %#v, want current + increment = 120000 (server-computed, D-54)", item["min_next_bid"])
				}
				top := asObject(t, item["top_bidder"], "item.top_bidder")
				if top["nickname"] != "Bình" {
					t.Errorf("top_bidder = %#v, want Bình", top)
				}
				if sc.role == RolePlayer {
					me := asObject(t, doc["me"], "me")
					if me["last_bid"] != float64(100_000) || me["is_top_bidder"] != false {
						t.Errorf("me = %#v, want last_bid 100000 and is_top_bidder false (Bình leads)", me)
					}
				}
			case PhaseSettled:
				reveal := asObject(t, settle, "item.settle")
				if reveal["outcome"] != "sold" || reveal["price"] != float64(110_000) {
					t.Errorf("settle = %#v, want sold at 110000", reveal)
				}
				if reveal["truth_value"] != float64(fixtureTruthValue) {
					t.Errorf("settle.truth_value = %#v, want the reveal %d", reveal["truth_value"], fixtureTruthValue)
				}
				if reveal["budget_delta"] != float64(fixtureTruthValue-110_000) {
					t.Errorf("settle.budget_delta = %#v, want truth_value - price", reveal["budget_delta"])
				}
				winner := asObject(t, reveal["winner"], "settle.winner")
				if winner["nickname"] != "Bình" {
					t.Errorf("settle.winner = %#v, want Bình", winner)
				}
				if sc.role == RolePlayer {
					me := asObject(t, doc["me"], "me")
					if me["last_bid"] != float64(110_000) || me["is_top_bidder"] != true {
						t.Errorf("the winner's me = %#v, want last_bid 110000 and is_top_bidder true", me)
					}
				}
			}
		})
	}
}

// TestAdminViewPositiveControl proves the fixture really is populated: the admin
// projection must carry the truth value, the sealed bid and the token, otherwise
// the leak assertions above are vacuous (D-26).
func TestAdminViewPositiveControl(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	env.installState(populatedState())

	payload := env.engine.Snapshot(RoleAdmin, "")
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshaling the admin view: %v", err)
	}
	doc := decodeObject(t, payload)

	// Positive control: the secrets are present. Asserting the *values*, not just
	// the keys, is what proves the fixture and the projection are connected.
	if !strings.Contains(string(raw), `"truth_value":350000`) {
		t.Errorf("the admin view does not carry the item's truth value:\n%s", raw)
	}
	if !strings.Contains(string(raw), `654321`) {
		t.Errorf("the admin view does not carry the sealed bid amount:\n%s", raw)
	}
	if !strings.Contains(string(raw), fixtureToken) {
		t.Errorf("the admin view does not carry the player token:\n%s", raw)
	}

	players := asArray(t, doc["players"], "players")
	if len(players) != 2 {
		t.Fatalf("the admin view lists %d players, want 2", len(players))
	}
	items := asArray(t, doc["items"], "items")
	if len(items) != 1 {
		t.Fatalf("the admin view lists %d items, want 1", len(items))
	}
	bids := asArray(t, doc["sealed_bids"], "sealed_bids")
	if len(bids) != 1 {
		t.Fatalf("the admin view lists %d sealed bids, want 1", len(bids))
	}
	bid := asObject(t, bids[0], "sealed_bids[0]")
	if bid["amount"] != float64(fixtureSealedAmount) {
		t.Errorf("sealed bid amount = %#v, want %d", bid["amount"], fixtureSealedAmount)
	}
}

// TestPlayerViewTracksPresence is D-37 seen from the client side: presence is
// in-memory state that rides the current revision, so the lobby's connected
// flags must change without the revision moving (research Pitfall 15).
func TestPlayerViewTracksPresence(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))

	an, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "An"}, nil)
	if err != nil {
		t.Fatalf("join An: %v", err)
	}
	if _, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "Bình"}, nil); err != nil {
		t.Fatalf("join Bình: %v", err)
	}
	revision := env.engine.Rev()

	lobbyOf := func(playerID string) []any {
		t.Helper()
		doc := decodeObject(t, env.engine.Snapshot(RolePlayer, playerID))
		return asArray(t, doc["lobby"], "lobby")
	}

	connectedOf := func(playerID string) map[string]bool {
		t.Helper()
		flags := make(map[string]bool, 2)
		for _, entry := range lobbyOf(playerID) {
			row := asObject(t, entry, "lobby entry")
			nickname, _ := row["nickname"].(string)
			connected, _ := row["connected"].(bool)
			flags[nickname] = connected
		}
		return flags
	}

	flags := connectedOf(an.PlayerID)
	if !flags["An"] || !flags["Bình"] {
		t.Errorf("after two joins the lobby shows %#v, want both connected", flags)
	}

	if err := env.engine.Disconnect(an.PlayerID); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if got := env.engine.Rev(); got != revision {
		t.Errorf("revision = %d after a disconnect, want %d", got, revision)
	}
	flags = connectedOf(an.PlayerID)
	if flags["An"] {
		t.Errorf("after a disconnect the lobby shows %#v, want An disconnected", flags)
	}
	if !flags["Bình"] {
		t.Errorf("the other player's presence changed too: %#v", flags)
	}
}
