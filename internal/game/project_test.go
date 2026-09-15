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

// walkKeys visits every key in a decoded JSON document, at every depth.
func walkKeys(value any, visit func(key string)) {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			visit(key)
			walkKeys(child, visit)
		}
	case []any:
		for _, child := range node {
			walkKeys(child, visit)
		}
	}
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
// screen projection is a self-sufficient snapshot that contains no secret key at
// any depth, and — the other half of the assertion — actually contains the data
// the UI needs, so an accidentally empty projection cannot pass.
func TestPlayerAndScreenPayloadsLeakNothing(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	env.installState(populatedState())

	t.Run("player self view", func(t *testing.T) {
		doc := decodeObject(t, env.engine.Snapshot(RolePlayer, "p-an"))

		assertNoForbiddenKeys(t, doc, "player view")

		if got := doc["phase"]; got != string(PhaseLobby) {
			t.Errorf("phase = %#v, want %q", got, PhaseLobby)
		}
		if got := doc["revision"]; got != float64(7) {
			t.Errorf("revision = %#v, want 7", got)
		}
		if got := doc["server_ts"]; got != float64(testStart.UnixMilli()) {
			t.Errorf("server_ts = %#v, want the injected clock %d", got, testStart.UnixMilli())
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

		leaderboard := asArray(t, doc["leaderboard"], "leaderboard")
		if len(leaderboard) != 2 {
			t.Errorf("leaderboard has %d entries, want 2", len(leaderboard))
		}

		// The public item carries the hint range and the start price, and no
		// truth value (D-25/D-26).
		item := asObject(t, doc["item"], "item")
		if item["name"] != "Bức tranh" || item["start_price"] != float64(100_000) {
			t.Errorf("item = %#v, want the public item parameters", item)
		}
		if item["hint_min"] != float64(200_000) || item["hint_max"] != float64(500_000) {
			t.Errorf("item hint range = %#v..%#v, want 200000..500000", item["hint_min"], item["hint_max"])
		}
	})

	t.Run("screen view", func(t *testing.T) {
		doc := decodeObject(t, env.engine.Snapshot(RoleScreen, ""))

		assertNoForbiddenKeys(t, doc, "screen view")

		if _, hasMe := doc["me"]; hasMe {
			t.Error("the screen view carries a `me` section: the projector has no identity")
		}
		if len(asArray(t, doc["lobby"], "lobby")) != 2 {
			t.Errorf("lobby has %d entries, want 2", len(asArray(t, doc["lobby"], "lobby")))
		}
		item := asObject(t, doc["item"], "item")
		if _, hasTruth := item["truth_value"]; hasTruth {
			t.Error("the public item carries truth_value before SETTLED")
		}
	})
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

// assertNoForbiddenKeys walks the whole document and fails on any key D-26
// forbids, naming the key and the path it was found at so a regression is
// actionable.
func assertNoForbiddenKeys(t *testing.T, doc map[string]any, what string) {
	t.Helper()

	walkKeys(doc, func(key string) {
		if forbiddenKey(key) {
			t.Errorf("%s carries the forbidden key %q: the role projection must not be able to express a secret (D-26)", what, key)
		}
	})
}
