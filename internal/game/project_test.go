package game

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		{itemID: "it-1", playerID: "p-an", amount: 100_000, createdAt: startedAt + 1_000, kind: trailBid},
		{itemID: "it-1", playerID: "p-binh", amount: 110_000, createdAt: startedAt + 2_000, kind: trailBid},
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

	// Phase 5's half of the same lock (D-94/D-98/D-105): a Dutch run carries the
	// falling price's clock and mechanics with no ladder, its acceptance never
	// reaches the projector's feed, and the reveal stays the only place a
	// truth_value may travel — before SETTLED the object does not exist at all.
	dutchScenarios := []struct {
		name       string
		phase      Phase
		role       Role
		playerID   string
		wantSettle bool
	}{
		{name: "Dutch LIVE player view", phase: PhaseLive, role: RolePlayer, playerID: "p-an"},
		{name: "Dutch LIVE screen view", phase: PhaseLive, role: RoleScreen},
		{name: "Dutch SETTLED player view", phase: PhaseSettled, role: RolePlayer, playerID: "p-binh", wantSettle: true},
		{name: "Dutch SETTLED screen view", phase: PhaseSettled, role: RoleScreen, wantSettle: true},
	}
	for _, sc := range dutchScenarios {
		t.Run(sc.name, func(t *testing.T) {
			env.installState(lateJoinDutchFixture(sc.phase))
			doc := decodeObject(t, env.engine.Snapshot(sc.role, sc.playerID))
			assertNoSecrets(t, sc.name, doc)

			item := asObject(t, doc["item"], "item")
			settle, hasSettle := item["settle"]
			if hasSettle != sc.wantSettle {
				t.Fatalf("item.settle present = %t, want %t for a Dutch %s item (D-67)", hasSettle, sc.wantSettle, sc.phase)
			}
			if !sc.wantSettle {
				walkKeysAt(doc, nil, func(path []string, key string) {
					if key == "settle" {
						t.Errorf("%s carries a settle key at %v before SETTLED", sc.name, path)
					}
				})
			}

			// D-94: the whole price clock travels, and nothing ladder-shaped does.
			for _, key := range []string{"started_at", "floor_price", "decrement_step", "decrement_interval_ms", "current_price"} {
				if _, ok := item[key]; !ok {
					t.Errorf("%s: a Dutch item must carry %q so a client can re-derive the price (D-94/D-63)", sc.name, key)
				}
			}
			for _, key := range []string{"min_next_bid", "deadline_at", "top_bidder"} {
				if _, ok := item[key]; ok {
					t.Errorf("%s: a Dutch item carries %q — it has no ladder and no deadline (D-93/D-98)", sc.name, key)
				}
			}

			if sc.wantSettle {
				// The settled Dutch item's number is the price that was CHARGED,
				// never a re-derivation long past the floor.
				reveal := asObject(t, settle, "item.settle")
				if reveal["outcome"] != "sold" || reveal["price"] != float64(70_000) {
					t.Errorf("settle = %#v, want sold at the accepted 70000", reveal)
				}
				if reveal["truth_value"] != float64(fixtureTruthValue) {
					t.Errorf("settle.truth_value = %#v, want the revealed %d", reveal["truth_value"], fixtureTruthValue)
				}
				winner := asObject(t, reveal["winner"], "settle.winner")
				if winner["nickname"] != "Bình" {
					t.Errorf("settle.winner = %#v, want Bình (the accept row's player)", winner)
				}
				if item["current_price"] != reveal["price"] {
					t.Errorf("a settled Dutch item's current_price = %#v, want the accepted %#v", item["current_price"], reveal["price"])
				}
			}

			if sc.role == RoleScreen {
				if _, hasMe := doc["me"]; hasMe {
					t.Errorf("the screen view carries a `me` section: the projector has no identity")
				}
				feed := asArray(t, doc["recent_bids"], "recent_bids")
				if len(feed) != 0 {
					t.Errorf("recent_bids = %+v, want an empty feed: the projector's feed is English-only (D-105)", feed)
				}
			}
		})
	}
}

// TestHintBoundsProjectIndependently is WR-02's regression: the item validator
// bounds `hint_min` and `hint_max` separately (only `hint_min > hint_max` with
// BOTH present is refused), so a row carrying one bound on its own is legal.
// Both projections used to gate the whole pair on hint_max, which made a
// configured hint_min invisible on every surface — and, because the admin editor
// round-trips exactly the projection it is shown, erased the value on the next
// unrelated save.
func TestHintBoundsProjectIndependently(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))

	for _, tc := range []struct {
		name    string
		hintMin int64
		hintMax int64
	}{
		{name: "min only", hintMin: 200_000},
		{name: "max only", hintMax: 500_000},
		{name: "both bounds", hintMin: 200_000, hintMax: 500_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := populatedState()
			s.items["it-1"].hintMin = tc.hintMin
			s.items["it-1"].hintMax = tc.hintMax
			env.installState(s)

			// One bound at a time: a set bound MUST be on the wire (the player and
			// the projector render from it) and an unset one must be ABSENT rather
			// than a zero (the wire's null, which the clients render as "no hint").
			checkBound := func(t *testing.T, what string, obj map[string]any, key string, want int64) {
				t.Helper()

				got, present := obj[key]
				if want > 0 {
					if !present || got != float64(want) {
						t.Errorf("%s.%s = %#v (present %t), want %d", what, key, got, present, want)
					}
					return
				}
				if present {
					t.Errorf("%s.%s = %#v, want the key absent: an unset bound is NULL, never 0", what, key, got)
				}
			}

			// The public projection: the player route and /screen share it.
			public := decodeObject(t, env.engine.Snapshot(RolePlayer, "p-an"))
			publicItem := asObject(t, public["item"], "item")
			checkBound(t, "public item", publicItem, "hint_min", tc.hintMin)
			checkBound(t, "public item", publicItem, "hint_max", tc.hintMax)

			// The admin projection: `draftFromItem` loads the editor from exactly
			// this object, so a bound missing here is a bound the next save sends
			// as null — the silent-erasure half of the defect.
			admin := decodeObject(t, env.engine.Snapshot(RoleAdmin, ""))
			rows := asArray(t, admin["items"], "items")
			if len(rows) != 1 {
				t.Fatalf("the admin projection carries %d item(s), want the one fixture item", len(rows))
			}
			adminRow := asObject(t, rows[0], "items[0]")
			checkBound(t, "admin item", adminRow, "hint_min", tc.hintMin)
			checkBound(t, "admin item", adminRow, "hint_max", tc.hintMax)
		})
	}
}

// assertKeySet fails when a decoded JSON object's keys are not exactly want:
// both a missing key (the body cannot render) and an unexpected one (a field
// nobody decided to ship) are failures.
func assertKeySet(t *testing.T, what string, obj map[string]any, want ...string) {
	t.Helper()

	allowed := make(map[string]bool, len(want))
	for _, key := range want {
		allowed[key] = true
	}
	for key := range obj {
		if !allowed[key] {
			t.Errorf("%s carries the unexpected key %q (allowed: %v)", what, key, want)
		}
	}
	for _, key := range want {
		if _, ok := obj[key]; !ok {
			t.Errorf("%s is missing the required key %q", what, key)
		}
	}
}

// lateJoinFixture is auctionFixture plus a genuine late joiner: a third player
// who joined after every bid and has never bid. LOBBY and FINALE additionally
// carry no current item, which is the invariant the boot enforces (D-62).
func lateJoinFixture(phase Phase) *state {
	s := auctionFixture(phase)
	switch phase {
	case PhaseLobby, PhaseFinale:
		s.phase = phase
		s.currentItemID = ""
		s.currentAuction = nil
	}
	s.players["p-cuong"] = &player{
		id: "p-cuong", nickname: "Cường", nicknameKey: "cường", token: strings.Repeat("c", 64),
		budget: 1_000_000, joinedAt: testStart.Add(6_000).UnixMilli(), lastSeenAt: testStart.Add(6_000).UnixMilli(),
	}
	s.order = append(s.order, "p-cuong")
	s.connected["p-cuong"] = true
	return s
}

// lateJoinDutchFixture is the Dutch arm of that table: the same three players,
// one Dutch item carrying the four public mechanics a client needs to re-derive
// the price (D-94), and a DEADLINE-LESS run (D-93) whose trail holds ONE
// kind=accept entry — the durable shape a real accept commits (D-98).
func lateJoinDutchFixture(phase Phase) *state {
	s := populatedState()
	it := s.items["it-1"]
	it.auctionType = "dutch"
	it.minIncrement, it.durationMS = 0, 0
	it.antiSnipeMode, it.antiSnipeWindowMS, it.maxExtensions = "", 0, 0
	it.floorPrice, it.decrementStep, it.decrementIntervalMS = 40_000, 10_000, 1_000
	it.status = "live"

	startedAt := testStart.UnixMilli()
	run := &auctionRun{itemID: "it-1", startedAt: startedAt} // no deadline (D-93)

	s.players["p-cuong"] = &player{
		id: "p-cuong", nickname: "Cường", nicknameKey: "cường", token: strings.Repeat("c", 64),
		budget: 1_000_000, joinedAt: testStart.Add(6_000).UnixMilli(), lastSeenAt: testStart.Add(6_000).UnixMilli(),
	}
	s.order = append(s.order, "p-cuong")
	s.connected["p-cuong"] = true

	switch phase {
	case PhaseLive:
		s.phase = PhaseLive
		s.currentAuction = run
	case PhaseSettled:
		s.phase, it.status = PhaseSettled, "settled"
		s.bids = []bid{{
			itemID: "it-1", playerID: "p-binh", amount: 70_000,
			createdAt: startedAt + 3_000, kind: trailAccept,
		}}
		run.settledAt = startedAt + 3_000
		run.settle = &SettleInfo{
			Outcome: "sold", Winner: &Bidder{Nickname: "Bình"},
			Price: 70_000, TruthValue: fixtureTruthValue, BudgetDelta: fixtureTruthValue - 70_000,
		}
		s.currentAuction = run
		s.players["p-binh"].budget = 750_000 - 70_000 + fixtureTruthValue
		s.players["p-binh"].totalValue = fixtureTruthValue
	}
	return s
}

// lateJoinSealedFixture is the sealed arm of the phase table: a sealed item
// carrying the TWO effective rows a live window holds (plus one row belonging to
// another item, which no sealed projection may show), and a run whose only live
// field is its deadline (D-116). The late joiner (p-cuong) has never submitted,
// so the own-amount key must be ABSENT for them (D-115).
func lateJoinSealedFixture(phase Phase) *state {
	s := populatedState()
	it := s.items["it-1"]
	it.auctionType = "sealed_first"
	it.minIncrement, it.durationMS = 0, 30_000
	it.antiSnipeMode, it.antiSnipeWindowMS, it.maxExtensions = "disabled", 0, 0
	it.status = "live"

	startedAt := testStart.UnixMilli()
	run := &auctionRun{itemID: "it-1", startedAt: startedAt, deadlineAt: startedAt + 30_000}
	s.sealedBids = []sealedBid{
		// Deliberately out of submission order, so the admin ledger's ordering
		// assertion is about the rule rather than about the slice.
		{itemID: "it-1", playerID: "p-binh", amount: 220_000, submittedAt: startedAt + 5_000, updatedAt: startedAt + 8_000},
		{itemID: "it-1", playerID: "p-an", amount: 150_000, submittedAt: startedAt + 1_000, updatedAt: startedAt + 1_000},
		{itemID: "it-prev", playerID: "p-an", amount: 999_000, submittedAt: startedAt - 60_000, updatedAt: startedAt - 60_000},
	}

	s.players["p-cuong"] = &player{
		id: "p-cuong", nickname: "Cường", nicknameKey: "cường", token: strings.Repeat("c", 64),
		budget: 1_000_000, joinedAt: testStart.Add(6_000).UnixMilli(), lastSeenAt: testStart.Add(6_000).UnixMilli(),
	}
	s.order = append(s.order, "p-cuong")
	s.connected["p-cuong"] = true

	switch phase {
	case PhaseLive:
		s.phase = PhaseLive
		s.currentAuction = run
	case PhaseSettled:
		s.phase, it.status = PhaseSettled, "settled"
		run.settledAt = startedAt + 30_000
		// The Vickrey charge: Bình wins at 220000 but pays the second highest
		// 150000 — the reveal publishes the PRICE, never the winner's bid
		// (D-114).
		run.settle = &SettleInfo{
			Outcome: "sold", Winner: &Bidder{Nickname: "Bình"},
			Price: 150_000, TruthValue: fixtureTruthValue, BudgetDelta: fixtureTruthValue - 150_000,
		}
		s.currentAuction = run
		s.players["p-binh"].budget = 750_000 - 150_000 + fixtureTruthValue
		s.players["p-binh"].totalValue = fixtureTruthValue
	}
	return s
}

// TestLateJoinSnapshotIsSelfSufficientAtEveryPhase is SESS-04/D-63's contract:
// one snapshot has to be enough for a client that just arrived. The table walks
// every phase — including LOBBY/FINALE with no item, and both a sold and an
// unsold SETTLED — and asserts each role's exact key set, so a body can only
// render from data the snapshot actually carries.
//
// The last two cases are the hard half: a process that restarts on a SETTLED
// file rebuilds the reveal from the trail (the runtime row persists timing, and
// the `settle` object is not a second source of truth), so a player who joins
// after the crash still sees the outcome, the winner and the truth value.
func TestLateJoinSnapshotIsSelfSufficientAtEveryPhase(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))

	// --- every phase, from a snapshot alone -----------------------------------
	introKeys := []string{"id", "name", "description", "auction_type", "hint_min", "hint_max", "start_price", "status"}
	runKeys := []string{"current_price", "top_bidder", "deadline_at", "extensions_used"}
	// D-94: a Dutch run swaps the ladder fields for the falling price's clock —
	// no top_bidder, no deadline_at, no min_next_bid, but the anchor and the
	// three mechanics that let a client re-derive the number itself.
	dutchRunKeys := []string{"current_price", "extensions_used", "started_at", "floor_price", "decrement_step", "decrement_interval_ms"}
	// D-116: a sealed run advertises exactly ONE live field. There is no ladder
	// to describe — no price, no minimum, no top bidder, no extension count —
	// and the counts live in sealed_progress, a separate projection field.
	sealedRunKeys := []string{"deadline_at"}

	cases := []struct {
		name   string
		phase  Phase
		status string // "" means: this phase has no current item
		run    bool   // the live fields a run carries
		next   bool   // min_next_bid is advertised only while LIVE
		settle bool
		dutch  bool // the Dutch arm: its own fixture and its own live key set
		sealed bool // the sealed arm: its own fixture and its own live key set
	}{
		{name: "LOBBY has no current item", phase: PhaseLobby},
		{name: "ITEM_INTRO renders the intro body", phase: PhaseItemIntro, status: "intro"},
		{name: "LIVE renders the English body", phase: PhaseLive, status: "live", run: true, next: true},
		{name: "LIVE renders the Dutch body", phase: PhaseLive, status: "live", run: true, dutch: true},
		{name: "LIVE renders the sealed body", phase: PhaseLive, status: "live", run: true, sealed: true},
		{name: "SETTLED renders the reveal", phase: PhaseSettled, status: "settled", run: true, settle: true},
		{name: "SETTLED renders the Dutch reveal", phase: PhaseSettled, status: "settled", run: true, settle: true, dutch: true},
		{name: "SETTLED renders the sealed reveal", phase: PhaseSettled, status: "settled", run: true, settle: true, sealed: true},
		{name: "FINALE has no current item", phase: PhaseFinale},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := lateJoinFixture(tc.phase)
			if tc.dutch {
				fixture = lateJoinDutchFixture(tc.phase)
			}
			if tc.sealed {
				fixture = lateJoinSealedFixture(tc.phase)
			}
			env.installState(fixture)

			for _, role := range []struct {
				name     string
				role     Role
				playerID string
			}{
				{name: "player late join", role: RolePlayer, playerID: "p-cuong"},
				{name: "screen late join", role: RoleScreen},
			} {
				t.Run(role.name, func(t *testing.T) {
					doc := decodeObject(t, env.engine.Snapshot(role.role, role.playerID))
					assertNoSecrets(t, role.name, doc)

					if doc["phase"] != string(tc.phase) {
						t.Fatalf("phase = %#v, want %q", doc["phase"], tc.phase)
					}

					item, hasItem := doc["item"]
					if tc.status == "" {
						if hasItem && item != nil {
							t.Fatalf("item = %#v, want an explicit null: %s has no current item", item, tc.phase)
						}
					} else {
						obj := asObject(t, item, "item")
						if obj["status"] != tc.status {
							t.Errorf("item.status = %#v, want %q", obj["status"], tc.status)
						}
						// Every key the phase body reads, and nothing else.
						want := append([]string(nil), introKeys...)
						switch {
						case tc.dutch:
							want = append(want, dutchRunKeys...)
						case tc.sealed:
							want = append(want, sealedRunKeys...)
						case tc.run:
							want = append(want, runKeys...)
						}
						if tc.next {
							want = append(want, "min_next_bid")
						}
						if tc.settle {
							want = append(want, "settle")
						}
						assertKeySet(t, "item", obj, want...)

						if tc.run && !tc.dutch && !tc.sealed {
							top := asObject(t, obj["top_bidder"], "item.top_bidder")
							assertKeySet(t, "item.top_bidder", top, "nickname")
						}
					}

					if tc.settle {
						reveal := asObject(t, asObject(t, item, "item")["settle"], "item.settle")
						assertKeySet(t, "item.settle", reveal, "outcome", "winner", "price", "truth_value", "budget_delta")
					}

					if role.role == RoleScreen {
						if _, present := doc["me"]; present {
							t.Errorf("the screen view carries a `me` section: the projector has no identity")
						}
					} else {
						me := asObject(t, doc["me"], "me")
						assertKeySet(t, "me", me, "id", "nickname", "budget", "total_value", "connected", "is_top_bidder", "rank")
						if me["is_top_bidder"] != false {
							t.Errorf("a never-bid late joiner shows is_top_bidder = %#v, want an explicit false", me["is_top_bidder"])
						}
						if me["id"] != "p-cuong" || me["budget"] != float64(1_000_000) {
							t.Errorf("me = %#v, want the late joiner's own identity and budget", me)
						}
					}

					// The public lists must not grow a private column: the lobby
					// is nickname + presence, the standings are the PLAN §6
					// public ranking.
					for _, entry := range asArray(t, doc["lobby"], "lobby") {
						assertKeySet(t, "lobby entry", asObject(t, entry, "lobby entry"), "nickname", "connected")
					}
					for _, entry := range asArray(t, doc["leaderboard"], "leaderboard") {
						assertKeySet(t, "leaderboard entry", asObject(t, entry, "leaderboard entry"), "rank", "nickname", "total_value", "budget")
					}

					// D-116: the count is the ONE sealed fact that crosses
					// between roles, and it is an explicit null for every other
					// item type — which is what lets a client tell "not a sealed
					// item" from "a field this build does not know".
					if tc.sealed {
						progress := asObject(t, doc["sealed_progress"], "sealed_progress")
						assertKeySet(t, "sealed_progress", progress, "submitted", "total")
						if progress["submitted"] != float64(2) || progress["total"] != float64(3) {
							t.Errorf("sealed_progress = %#v, want {submitted:2, total:3} — the item's two rows out of the room", progress)
						}
					} else if doc["sealed_progress"] != nil {
						t.Errorf("sealed_progress on a non-sealed %s = %#v, want an explicit null", tc.phase, doc["sealed_progress"])
					}
				})
			}
		})
	}

	// --- a restart at SETTLED: the reveal is rebuilt from the trail -----------
	t.Run("a restart at SETTLED keeps the sold reveal a late joiner renders", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "auction.db")
		base := testStart

		first := newTestEnvAt(t, path, &testClock{t: base})
		an := mustJoin(t, first, "An")
		binh := mustJoin(t, first, "Bình")
		itemID := mustUpsertItem(t, first, validEnglishInput())
		mustOpenLive(t, first, itemID)
		mustExtendOnce(t, first, base, an.PlayerID, binh.PlayerID)
		if err := first.store.Close(); err != nil {
			t.Fatalf("closing the store: %v", err)
		}

		// The process comes back after the deadline: the boot settles once.
		second := newTestEnvAt(t, path, &testClock{t: base.Add(40 * time.Second)})
		if got := second.engine.Phase(); got != PhaseSettled {
			t.Fatalf("phase after a past-deadline boot = %s, want SETTLED", got)
		}
		if err := second.store.Close(); err != nil {
			t.Fatalf("closing the settled store: %v", err)
		}

		// A THIRD process boots the settled file, and a player who never bid
		// joins it — a late join after a crash.
		third := newTestEnvAt(t, path, &testClock{t: base.Add(41 * time.Second)})
		late := mustJoin(t, third, "Cường")

		doc := decodeObject(t, third.engine.Snapshot(RolePlayer, late.PlayerID))
		assertNoSecrets(t, "restarted SETTLED late join", doc)
		item := asObject(t, doc["item"], "item")
		if item["status"] != "settled" {
			t.Fatalf("item.status after a restart at SETTLED = %#v, want settled", item["status"])
		}
		reveal := asObject(t, item["settle"], "item.settle after a restart at SETTLED")
		if reveal["outcome"] != "sold" || reveal["price"] != float64(110_000) {
			t.Errorf("rebuilt settle = %#v, want sold at 110000", reveal)
		}
		if reveal["truth_value"] != float64(fixtureTruthValue) {
			t.Errorf("rebuilt settle.truth_value = %#v, want the reveal %d — the only moment it crosses the boundary", reveal["truth_value"], fixtureTruthValue)
		}
		if want := float64(fixtureTruthValue - 110_000); reveal["budget_delta"] != want {
			t.Errorf("rebuilt settle.budget_delta = %#v, want truth_value − price = %v", reveal["budget_delta"], want)
		}
		winner := asObject(t, reveal["winner"], "settle.winner")
		if winner["nickname"] != "Bình" {
			t.Errorf("rebuilt settle.winner = %#v, want Bình", winner)
		}

		// The settlement's own effects are durable, so the late joiner reads
		// them from the same snapshot (D-63): the winner's budget is post-settle.
		secondSnapshot := decodeObject(t, third.engine.Snapshot(RolePlayer, binh.PlayerID))
		me := asObject(t, secondSnapshot["me"], "the winner's me")
		if want := float64(1_000_000 - 110_000 + fixtureTruthValue); me["budget"] != want {
			t.Errorf("the winner's budget after the restart = %#v, want %v", me["budget"], want)
		}
	})

	t.Run("a restart at SETTLED keeps an unsold reveal with a null winner", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "auction.db")
		base := testStart

		first := newTestEnvAt(t, path, &testClock{t: base})
		itemID := mustUpsertItem(t, first, validEnglishInput())
		mustOpenLive(t, first, itemID) // nobody bids
		if err := first.store.Close(); err != nil {
			t.Fatalf("closing the store: %v", err)
		}

		second := newTestEnvAt(t, path, &testClock{t: base.Add(40 * time.Second)})
		if got := second.engine.Phase(); got != PhaseSettled {
			t.Fatalf("phase after a past-deadline boot = %s, want SETTLED", got)
		}
		if err := second.store.Close(); err != nil {
			t.Fatalf("closing the settled store: %v", err)
		}

		third := newTestEnvAt(t, path, &testClock{t: base.Add(41 * time.Second)})
		late := mustJoin(t, third, "Cường")

		doc := decodeObject(t, third.engine.Snapshot(RoleScreen, ""))
		assertNoSecrets(t, "restarted unsold SETTLED screen", doc)
		item := asObject(t, doc["item"], "item")
		reveal := asObject(t, item["settle"], "item.settle after a restart at SETTLED")
		if reveal["outcome"] != "unsold" {
			t.Errorf("rebuilt settle.outcome = %#v, want unsold", reveal["outcome"])
		}
		if winner, present := reveal["winner"]; !present || winner != nil {
			t.Errorf("rebuilt settle.winner = %#v (present=%t), want an explicit null", winner, present)
		}
		if reveal["price"] != float64(0) || reveal["budget_delta"] != float64(0) {
			t.Errorf("rebuilt unsold settle = %#v, want price 0 and delta 0", reveal)
		}
		if reveal["truth_value"] != float64(fixtureTruthValue) {
			t.Errorf("rebuilt settle.truth_value = %#v, want %d", reveal["truth_value"], fixtureTruthValue)
		}
		if got := third.player(t, late.PlayerID); got.budget != 1_000_000 {
			t.Errorf("the late joiner's budget = %d, want the untouched starting budget: an unsold item moves nothing (ECON-03)", got.budget)
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

// TestProjectionsCarryJoinURLByRole is D-83/D-84/D-86's shape contract for the
// Join URLs panel, asserted on the marshaled wire rather than on the Go types:
// the admin panel receives the candidate list AND the selection (it is the only
// role that chooses), the projector receives the selection alone (a list on a
// projector is a list of wrong answers), and a player receives neither — nor any
// `config` object it could read a budget out of.
//
// It is also the leak assertion for the new surface: assertNoSecrets runs over
// every role's document, so a future field that puts a token or a truth value
// next to the URL fails here.
func TestProjectionsCarryJoinURLByRole(t *testing.T) {
	env := newConfigEnv(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart}, configCandidates)

	joined, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "An"}, nil)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}

	adminDoc := decodeObject(t, env.engine.Snapshot(RoleAdmin, ""))
	// The admin projection is the leak lock's POSITIVE control — it carries
	// tokens and sealed bids on purpose — so the assertion here is the
	// D-74 one instead: the capability that guards the panel never enters game
	// state, and therefore can never reach the panel.
	for _, key := range []string{"key", "admin_key"} {
		if _, present := adminDoc[key]; present {
			t.Errorf("the admin snapshot carries %q: the ADMIN_KEY is server_config's, never game state (D-74)", key)
		}
	}
	urls, ok := adminDoc["join_urls"].([]any)
	if !ok {
		t.Fatalf("admin join_urls = %#v, want the candidate array: the panel has nothing to offer without it", adminDoc["join_urls"])
	}
	if len(urls) != len(configCandidates) {
		t.Errorf("admin join_urls = %v, want all %d candidates", urls, len(configCandidates))
	}
	for i, want := range configCandidates {
		if i < len(urls) && urls[i] != want {
			t.Errorf("admin join_urls[%d] = %#v, want %q", i, urls[i], want)
		}
	}
	if got := adminDoc["selected_join_url"]; got != configCandidates[0] {
		t.Errorf("admin selected_join_url = %#v, want the first candidate %q before any selection", got, configCandidates[0])
	}

	screenDoc := decodeObject(t, env.engine.Snapshot(RoleScreen, ""))
	assertNoSecrets(t, "screen", screenDoc)
	if got := screenDoc["join_url"]; got != configCandidates[0] {
		t.Errorf("screen join_url = %#v, want %q", got, configCandidates[0])
	}
	for _, key := range []string{"join_urls", "selected_join_url", "config"} {
		if _, present := screenDoc[key]; present {
			t.Errorf("the screen snapshot carries %q: the projector has no choice to make", key)
		}
	}

	playerDoc := decodeObject(t, env.engine.Snapshot(RolePlayer, joined.PlayerID))
	assertNoSecrets(t, "player", playerDoc)
	for _, key := range []string{"join_url", "join_urls", "selected_join_url", "config"} {
		if _, present := playerDoc[key]; present {
			t.Errorf("the player snapshot carries %q: a player never needs the show's URLs or config", key)
		}
	}

	// Selecting a NON-first candidate is the whole point of the panel: the
	// projector follows the admin's choice, and the list itself does not move.
	selected := configCandidates[2]
	if err := env.engine.UpdateConfig(ConfigInput{JoinURL: &selected}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	adminDoc = decodeObject(t, env.engine.Snapshot(RoleAdmin, ""))
	if got := adminDoc["selected_join_url"]; got != selected {
		t.Errorf("admin selected_join_url = %#v, want the selected %q", got, selected)
	}
	if got, ok := adminDoc["join_urls"].([]any); !ok || len(got) != len(configCandidates) {
		t.Errorf("admin join_urls = %#v after a selection, want the unchanged candidate list", adminDoc["join_urls"])
	}

	screenDoc = decodeObject(t, env.engine.Snapshot(RoleScreen, ""))
	if got := screenDoc["join_url"]; got != selected {
		t.Errorf("screen join_url = %#v, want the selected %q: every projector renders the admin's choice", got, selected)
	}
}

// screenFixture is populatedState() wired to one LIVE English run with an empty
// bid trail: D-88's screen extras (the feed, the sealed counter) are then added
// by the case that needs them, so each assertion below is about one rule.
func screenFixture() *state {
	s := populatedState()
	it := s.items["it-1"]
	it.minIncrement = 10_000
	it.durationMS = 30_000
	it.status = "live"
	s.phase = PhaseLive
	startedAt := testStart.UnixMilli()
	s.currentAuction = &auctionRun{itemID: "it-1", startedAt: startedAt, deadlineAt: startedAt + 30_000}
	// D-83's enumeration is a BOOT input the engine holds, not a state row, so a
	// synthetic fixture has to carry it for join_url to mean anything.
	s.joinURLCandidates = configCandidates
	return s
}

// TestScreenLeak is D-88's contract on the marshaled wire (the file's rule: the
// type boundary is the mechanism, the bytes are what a projector can read):
// ScreenView keeps the 04-04 selected join URL, adds a NEWEST-FIRST feed capped
// at five entries with the sixth-oldest bid evicted and equal timestamps left in
// insertion order, reports the start price with no bidder when nobody has bid,
// carries a count-only sealed counter for a sealed type (and an explicit null
// otherwise), and — the half D-26 has owned since Phase 2 — still carries no
// truth_value and no settle object before SETTLED, while the admin projection
// keeps its positive control.
func TestScreenLeak(t *testing.T) {
	env := newConfigEnv(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart}, configCandidates)

	// Six accepted bids on the one live run, the fourth sharing the third's
	// timestamp: five must travel (newest first, the later insertion first among
	// equals) and the 100 000 bid — the sixth oldest — must not.
	startedAt := testStart.UnixMilli()
	live := screenFixture()
	live.bids = []bid{
		{itemID: "it-1", playerID: "p-an", amount: 100_000, createdAt: startedAt + 1_000, kind: trailBid},
		{itemID: "it-1", playerID: "p-binh", amount: 110_000, createdAt: startedAt + 2_000, kind: trailBid},
		{itemID: "it-1", playerID: "p-an", amount: 120_000, createdAt: startedAt + 3_000, kind: trailBid},
		{itemID: "it-1", playerID: "p-binh", amount: 130_000, createdAt: startedAt + 3_000, kind: trailBid},
		{itemID: "it-1", playerID: "p-an", amount: 140_000, createdAt: startedAt + 4_000, kind: trailBid},
		{itemID: "it-1", playerID: "p-binh", amount: 150_000, createdAt: startedAt + 5_000, kind: trailBid},
	}
	env.installState(live)

	screen := decodeObject(t, env.engine.Snapshot(RoleScreen, ""))
	assertNoSecrets(t, "screen", screen)

	t.Run("the selected join URL still travels with the projector's snapshot", func(t *testing.T) {
		if got := screen["join_url"]; got != configCandidates[0] {
			t.Errorf("screen join_url = %#v, want the selected %q (D-84 retained)", got, configCandidates[0])
		}
	})

	t.Run("the feed is the newest five bids, newest first", func(t *testing.T) {
		bids := asArray(t, screen["recent_bids"], "recent_bids")
		if len(bids) != 5 {
			t.Fatalf("recent_bids has %d entries, want the five newest (D-88)", len(bids))
		}
		want := []float64{150_000, 140_000, 130_000, 120_000, 110_000}
		wantTS := []float64{
			float64(startedAt + 5_000), float64(startedAt + 4_000), float64(startedAt + 3_000),
			float64(startedAt + 3_000), float64(startedAt + 2_000),
		}
		for i, entry := range bids {
			row := asObject(t, entry, "recent_bids entry")
			assertKeySet(t, "recent_bids entry", row, "nickname", "amount", "ts")
			if row["amount"] != want[i] {
				t.Errorf("recent_bids[%d].amount = %#v, want %v (newest first)", i, row["amount"], want[i])
			}
			if row["ts"] != wantTS[i] {
				t.Errorf("recent_bids[%d].ts = %#v, want %v (equal timestamps keep insertion order)", i, row["ts"], wantTS[i])
			}
		}
		// The eviction rule, stated as the absence it is: the sixth-oldest bid
		// never reaches the projector.
		for i, entry := range bids {
			if asObject(t, entry, "recent_bids entry")["amount"] == float64(100_000) {
				t.Errorf("recent_bids[%d] carries the sixth-oldest bid: the feed is capped at five", i)
			}
		}
	})

	t.Run("an empty trail shows the start price and no bidder", func(t *testing.T) {
		quiet := screenFixture()
		env.installState(quiet)
		doc := decodeObject(t, env.engine.Snapshot(RoleScreen, ""))
		assertNoSecrets(t, "screen with no bids", doc)

		if got := len(asArray(t, doc["recent_bids"], "recent_bids")); got != 0 {
			t.Errorf("recent_bids carries %d entries before any bid, want an empty feed", got)
		}
		item := asObject(t, doc["item"], "item")
		if item["current_price"] != float64(100_000) {
			t.Errorf("current_price = %#v, want the start price 100000 before any bid", item["current_price"])
		}
		if _, hasBidder := item["top_bidder"]; hasBidder {
			t.Errorf("item carries top_bidder before any bid: %#v", item)
		}
	})

	t.Run("sealed_progress is a count-only object for a sealed type and null otherwise", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			typ    string
			sealed bool
		}{
			{name: "sealed_first", typ: "sealed_first", sealed: true},
			{name: "vickrey", typ: "vickrey", sealed: true},
			{name: "english", typ: "english"},
			{name: "dutch", typ: "dutch"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				fixture := screenFixture()
				fixture.items["it-1"].auctionType = tc.typ
				env.installState(fixture)
				doc := decodeObject(t, env.engine.Snapshot(RoleScreen, ""))
				assertNoSecrets(t, "screen "+tc.typ, doc)

				raw, present := doc["sealed_progress"]
				if !present {
					t.Fatalf("the screen snapshot has no sealed_progress key: the UI shell cannot tell 'not sealed' from a missing field (D-88)")
				}
				if !tc.sealed {
					if raw != nil {
						t.Fatalf("sealed_progress = %#v for a %s item, want an explicit null", raw, tc.typ)
					}
					return
				}
				progress := asObject(t, raw, "sealed_progress")
				assertKeySet(t, "sealed_progress", progress, "submitted", "total")
				if progress["submitted"] != float64(1) {
					t.Errorf("sealed_progress.submitted = %#v, want the one sealed row on this item", progress["submitted"])
				}
				if progress["total"] != float64(2) {
					t.Errorf("sealed_progress.total = %#v, want the player count 2", progress["total"])
				}
			})
		}
	})

	t.Run("no truth_value and no settle before SETTLED, while the admin keeps its positive control", func(t *testing.T) {
		for _, phase := range []Phase{PhaseItemIntro, PhaseLive} {
			t.Run(string(phase), func(t *testing.T) {
				env.installState(auctionFixture(phase))
				screenPayload := env.engine.Snapshot(RoleScreen, "")
				doc := decodeObject(t, screenPayload)
				assertNoSecrets(t, "screen "+string(phase), doc)
				walkKeysAt(doc, nil, func(path []string, key string) {
					if key == "settle" {
						t.Errorf("the screen carries a settle key at %v before SETTLED (D-67)", path)
					}
				})

				raw, err := json.Marshal(screenPayload)
				if err != nil {
					t.Fatalf("marshaling the screen view: %v", err)
				}
				for _, forbidden := range []string{"truth_value", "350000"} {
					if strings.Contains(string(raw), forbidden) {
						t.Errorf("the %s screen payload carries %q: the boundary is the type, not the field\n%s", phase, forbidden, raw)
					}
				}

				adminRaw, err := json.Marshal(env.engine.Snapshot(RoleAdmin, ""))
				if err != nil {
					t.Fatalf("marshaling the admin view: %v", err)
				}
				if !strings.Contains(string(adminRaw), `"truth_value":350000`) {
					t.Errorf("the admin view lost its truth value at %s: the leak assertion above would be vacuous (D-26)", phase)
				}
			})
		}
	})
}

// rankFixture is six players whose keys walk ECON-05's whole sort chain, and
// whose ids are deliberately NOT in join order: the three players sharing budget
// AND total value AND joined_at can only be separated by the id rung, and a
// sort that fell back on the insertion order would put them in the wrong order.
func rankFixture() *state {
	base := testStart.UnixMilli()
	return &state{
		phase:    PhaseFinale,
		revision: 11,
		config:   engineConfig{StartingBudget: 1_000_000, Currency: "VND"},
		players: map[string]*player{
			"p-d": {id: "p-d", nickname: "Dung", budget: 1_000_000, joinedAt: base + 4_000, lastSeenAt: base + 4_000},
			"p-a": {id: "p-a", nickname: "An", budget: 900_000, totalValue: 500_000, joinedAt: base + 3_000, lastSeenAt: base + 3_000},
			"p-b": {id: "p-b", nickname: "Bình", budget: 900_000, totalValue: 500_000, joinedAt: base + 7_000, lastSeenAt: base + 7_000},
			"p-c": {id: "p-c", nickname: "Cường", budget: 900_000, totalValue: 400_000, joinedAt: base + 1_000, lastSeenAt: base + 1_000},
			"p-e": {id: "p-e", nickname: "Giang", budget: 900_000, totalValue: 400_000, joinedAt: base + 1_000, lastSeenAt: base + 1_000},
			"p-f": {id: "p-f", nickname: "Hạnh", budget: 900_000, totalValue: 400_000, joinedAt: base + 1_000, lastSeenAt: base + 1_000},
		},
		order:     []string{"p-f", "p-e", "p-c", "p-b", "p-a", "p-d"},
		connected: map[string]bool{"p-a": true, "p-d": true},
		items:     map[string]*item{},
	}
}

// TestLeaderboardTieBreak is ECON-05's ordering contract, asserted on the wire
// the projector and the player read: budget DESC, total_value DESC, joined_at
// ASC and finally player id ASC — the last rung is what makes the order total,
// so the podium a screen derives from the leaderboard can never depend on the
// order the rows happened to arrive in. It also pins the personal half D-89
// needs: `me.rank` is computed by the SERVER from that same order, so no client
// has to match an identity against a public list.
func TestLeaderboardTieBreak(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	env.installState(rankFixture())

	screen := decodeObject(t, env.engine.Snapshot(RoleScreen, ""))
	assertNoSecrets(t, "screen", screen)

	wantOrder := []string{"Dung", "An", "Bình", "Cường", "Giang", "Hạnh"}
	board := asArray(t, screen["leaderboard"], "leaderboard")
	if len(board) != len(wantOrder) {
		t.Fatalf("leaderboard has %d entries, want %d", len(board), len(wantOrder))
	}
	for i, entry := range board {
		row := asObject(t, entry, "leaderboard entry")
		if row["nickname"] != wantOrder[i] {
			t.Errorf("leaderboard[%d] = %#v, want %q (budget → total_value → joined_at → id)", i, row["nickname"], wantOrder[i])
		}
		if row["rank"] != float64(i+1) {
			t.Errorf("leaderboard[%d].rank = %#v, want %d", i, row["rank"], i+1)
		}
	}

	// Every player is ranked by the server, and the rank agrees with the
	// standing the projector shows.
	for i, id := range []string{"p-d", "p-a", "p-b", "p-c", "p-e", "p-f"} {
		doc := decodeObject(t, env.engine.Snapshot(RolePlayer, id))
		assertNoSecrets(t, "player "+id, doc)
		me := asObject(t, doc["me"], "me")
		if me["rank"] != float64(i+1) {
			t.Errorf("%s's me.rank = %#v, want %d (server-computed, ECON-05)", id, me["rank"], i+1)
		}
	}

	// An identity the engine does not know carries the zero rank: the finale body
	// omits the block instead of printing "rank 0" (D-89).
	unknown := asObject(t, decodeObject(t, env.engine.Snapshot(RolePlayer, "p-nobody"))["me"], "me")
	if unknown["rank"] != float64(0) {
		t.Errorf("an unknown player's me.rank = %#v, want 0", unknown["rank"])
	}

	// A real join through the mutation path lands in the same standing: the rank
	// is derived from committed state, never from a client's own bookkeeping.
	t.Run("a real join's rank comes from the committed standing", func(t *testing.T) {
		live := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, live, "An")
		live.clock.t = testStart.Add(2 * time.Second)
		binh := mustJoin(t, live, "Bình")

		for _, tc := range []struct {
			playerID string
			want     float64
		}{
			{playerID: an.PlayerID, want: 1},
			{playerID: binh.PlayerID, want: 2},
		} {
			me := asObject(t, decodeObject(t, live.engine.Snapshot(RolePlayer, tc.playerID))["me"], "me")
			if me["rank"] != tc.want {
				t.Errorf("%s's me.rank = %#v, want %v — equal budgets are separated by joined_at (ECON-05)", tc.playerID, me["rank"], tc.want)
			}
		}
	})
}

// finaleFixture drives one real engine to FINALE: players on the starting budget
// (nobody bids, so every economics key ties and the standing is decided by
// joined_at), one item that runs and settles, then the `admin.item.next` that
// finds nothing pending left. It returns the engine and the finale-transition
// event list, so a case can assert on what the room actually published. The path
// is the caller's, because the restart case reopens the SAME file.
func finaleFixture(t *testing.T, path string, clock *testClock) (*testEnv, []Event) {
	t.Helper()

	env := newTestEnvAt(t, path, clock)
	mustJoin(t, env, "An")
	env.clock.t = env.clock.t.Add(1 * time.Second)
	mustJoin(t, env, "Bình")

	itemID := mustUpsertItem(t, env, validEnglishInput())
	mustOpenLive(t, env, itemID)
	if err := env.engine.ForceEnd(); err != nil {
		t.Fatalf("ForceEnd: %v", err)
	}
	if err := env.engine.NextItem(); err != nil {
		t.Fatalf("NextItem into FINALE: %v", err)
	}
	if got := env.engine.Phase(); got != PhaseFinale {
		t.Fatalf("phase = %q after the last item, want FINALE", got)
	}
	return env, env.lastEvents()
}

// equalLeaderRow reports whether two decoded standing rows carry the same rank
// and the same public numbers. The podium is a SLICE of the standings, so any
// drift between them is a second ranking rule — which is exactly what ECON-05
// forbids.
func equalLeaderRow(a, b map[string]any) bool {
	for _, key := range []string{"rank", "nickname", "total_value", "budget"} {
		if a[key] != b[key] {
			return false
		}
	}
	return true
}

// TestFinaleProjection is D-89/ECON-05 at the engine: the FINALE transition
// publishes one `finale` payload — the top three of the ONE standing plus the
// complete standing — beside the phase frame and the snapshot, the snapshot a
// late joiner or a resync reads derives the same podium from its own
// leaderboard, and a zero-player finale is an empty podium rather than an
// invented placement. The wire literal "finale" is asserted instead of the Go
// constant on purpose: a rename of an identifier must not be able to move the
// protocol silently (the freeze file's rule).
func TestFinaleProjection(t *testing.T) {
	t.Run("the FINALE transition emits the podium, the standings and the snapshot", func(t *testing.T) {
		env, events := finaleFixture(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart})

		if len(events) != 3 || events[0].Type != EventPhaseChanged || events[1].Type != "finale" || events[2].Type != EventSnapshot {
			t.Fatalf("entering FINALE emitted %+v, want phase.changed, finale then snapshot (the snapshot stays last)", events)
		}
		doc := decodeObject(t, events[1].Data)
		assertNoSecrets(t, "finale", doc)

		standings := asArray(t, doc["standings"], "standings")
		podium := asArray(t, doc["podium"], "podium")
		if len(standings) != 2 {
			t.Fatalf("standings has %d entries, want every player (2)", len(standings))
		}
		if len(podium) != 2 {
			t.Fatalf("podium has %d entries, want the min(3, players) top of the standing", len(podium))
		}

		wantOrder := []string{"An", "Bình"}
		for i, entry := range standings {
			row := asObject(t, entry, "standings entry")
			assertKeySet(t, "standings entry", row, "rank", "nickname", "total_value", "budget")
			if row["nickname"] != wantOrder[i] || row["rank"] != float64(i+1) {
				t.Errorf("standings[%d] = %#v, want %q at rank %d (joined_at ASC when the economics tie)", i, row, wantOrder[i], i+1)
			}
		}
		for i, entry := range podium {
			if !equalLeaderRow(asObject(t, entry, "podium entry"), asObject(t, standings[i], "standings entry")) {
				t.Errorf("podium[%d] = %#v, want the first %d rows of the same standing", i, entry, len(podium))
			}
		}

		// The snapshot is the healing path: a projector that resyncs or arrives
		// after the finale derives the identical podium from its own leaderboard.
		snap := decodeObject(t, env.engine.Snapshot(RoleScreen, ""))
		assertNoSecrets(t, "finale screen", snap)
		board := asArray(t, snap["leaderboard"], "leaderboard")
		if len(board) != len(standings) {
			t.Fatalf("the FINALE snapshot carries %d standings, want %d", len(board), len(standings))
		}
		for i, entry := range podium {
			if !equalLeaderRow(asObject(t, entry, "podium entry"), asObject(t, board[i], "snapshot leaderboard entry")) {
				t.Errorf("podium[%d] = %#v, want the snapshot's own row %#v: one ranking rule, two delivery paths", i, entry, board[i])
			}
		}
	})

	t.Run("a third and fourth player exercise the three-row podium cap and the id rung", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		env.clock.t = testStart.Add(1 * time.Second)
		binh := mustJoin(t, env, "Bình")
		env.clock.t = testStart.Add(2 * time.Second)
		cuong := mustJoin(t, env, "Cường")
		dung := mustJoin(t, env, "Dung") // the SAME instant as Cường

		itemID := mustUpsertItem(t, env, validEnglishInput())
		mustOpenLive(t, env, itemID)
		if err := env.engine.ForceEnd(); err != nil {
			t.Fatalf("ForceEnd: %v", err)
		}
		if err := env.engine.NextItem(); err != nil {
			t.Fatalf("NextItem into FINALE: %v", err)
		}

		doc := decodeObject(t, env.lastEvents()[1].Data)
		standings := asArray(t, doc["standings"], "standings")
		podium := asArray(t, doc["podium"], "podium")
		if len(podium) != 3 {
			t.Fatalf("podium has %d entries with four players, want the cap at three", len(podium))
		}
		if len(standings) != 4 {
			t.Fatalf("standings has %d entries, want all four players", len(standings))
		}

		wantOrder := []string{"An", "Bình"}
		ranked := []string{an.PlayerID, binh.PlayerID}
		if cuong.PlayerID < dung.PlayerID {
			wantOrder = append(wantOrder, "Cường", "Dung")
			ranked = append(ranked, cuong.PlayerID, dung.PlayerID)
		} else {
			wantOrder = append(wantOrder, "Dung", "Cường")
			ranked = append(ranked, dung.PlayerID, cuong.PlayerID)
		}
		for i, entry := range standings {
			row := asObject(t, entry, "standings entry")
			if row["nickname"] != wantOrder[i] {
				t.Errorf("standings[%d] = %#v, want %q (equal joined_at falls through to the id rung)", i, row["nickname"], wantOrder[i])
			}
		}

		// The player half: every FINALE snapshot carries its own place, so the
		// body never matches an identity against a public list (D-89).
		for i, id := range ranked {
			me := asObject(t, decodeObject(t, env.engine.Snapshot(RolePlayer, id))["me"], "me")
			if me["rank"] != float64(i+1) {
				t.Errorf("%s's me.rank at FINALE = %#v, want %d", id, me["rank"], i+1)
			}
		}
	})

	t.Run("a restart at FINALE derives the same podium from its snapshot", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "auction.db")
		first, events := finaleFixture(t, path, &testClock{t: testStart})
		published := decodeObject(t, events[1].Data)
		podium := asArray(t, published["podium"], "podium")
		standings := asArray(t, published["standings"], "standings")
		if err := first.store.Close(); err != nil {
			t.Fatalf("closing the store: %v", err)
		}

		second := newTestEnvAt(t, path, &testClock{t: testStart.Add(time.Minute)})
		if got := second.engine.Phase(); got != PhaseFinale {
			t.Fatalf("phase after a restart on the FINALE file = %q, want FINALE", got)
		}

		snap := decodeObject(t, second.engine.Snapshot(RoleScreen, ""))
		assertNoSecrets(t, "restarted FINALE screen", snap)
		board := asArray(t, snap["leaderboard"], "leaderboard")
		if len(board) != len(standings) {
			t.Fatalf("the restarted snapshot carries %d standings, want %d", len(board), len(standings))
		}
		for i, entry := range podium {
			if !equalLeaderRow(asObject(t, entry, "podium entry"), asObject(t, board[i], "restarted leaderboard entry")) {
				t.Errorf("podium[%d] = %#v after a restart, want the snapshot's row %#v", i, entry, board[i])
			}
		}

		// A player joining the restarted finale reads the same order and its own
		// rank from that snapshot alone.
		late := mustJoin(t, second, "Khánh")
		me := asObject(t, decodeObject(t, second.engine.Snapshot(RolePlayer, late.PlayerID))["me"], "me")
		if me["rank"] != float64(len(board)+1) {
			t.Errorf("a late joiner's me.rank at the restarted FINALE = %#v, want %d", me["rank"], len(board)+1)
		}
	})

	t.Run("a zero-player finale publishes an empty podium and an empty standing", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		itemID := mustUpsertItem(t, env, validEnglishInput())
		mustOpenLive(t, env, itemID)
		if err := env.engine.ForceEnd(); err != nil {
			t.Fatalf("ForceEnd: %v", err)
		}
		if err := env.engine.NextItem(); err != nil {
			t.Fatalf("NextItem into FINALE: %v", err)
		}

		doc := decodeObject(t, env.lastEvents()[1].Data)
		if got := len(asArray(t, doc["podium"], "podium")); got != 0 {
			t.Errorf("a zero-player finale published %d podium rows, want none", got)
		}
		if got := len(asArray(t, doc["standings"], "standings")); got != 0 {
			t.Errorf("a zero-player finale published %d standings, want none", got)
		}

		// The snapshot agrees, and an empty standing is an empty LIST — the
		// finale body renders an empty block, never a null it has to guard.
		snap := decodeObject(t, env.engine.Snapshot(RoleScreen, ""))
		if got := len(asArray(t, snap["leaderboard"], "leaderboard")); got != 0 {
			t.Errorf("the zero-player FINALE snapshot carries %d standings, want none", got)
		}
		if got := len(asArray(t, snap["recent_bids"], "recent_bids")); got != 0 {
			t.Errorf("the zero-player FINALE snapshot carries %d feed rows, want none", got)
		}
	})
}

// numberPathsDeep collects the key path of every decoded number equal to want.
// It is how the sealed assertions prove an AMOUNT travelled (or did not) rather
// than merely that a key with a suspicious name exists.
func numberPathsDeep(value any, path []string, want int64, out *[][]string) {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			next := append(append([]string(nil), path...), key)
			if number, ok := child.(float64); ok && int64(number) == want {
				*out = append(*out, next)
			}
			numberPathsDeep(child, next, want, out)
		}
	case []any:
		for _, child := range node {
			numberPathsDeep(child, path, want, out)
		}
	}
}

// amountPaths returns every path in a decoded document whose value is want.
func amountPaths(doc map[string]any, want int64) [][]string {
	var paths [][]string
	numberPathsDeep(doc, nil, want, &paths)
	return paths
}

// samePath reports whether one collected path is exactly the keys in want.
func samePath(path []string, want ...string) bool {
	if len(path) != len(want) {
		return false
	}
	for i := range path {
		if path[i] != want[i] {
			return false
		}
	}
	return true
}

// TestSealedPlayerProjectionMyBidAndProgress is D-115/D-116's player half: the
// snapshot carries the player's OWN effective amount while a sealed run is live
// — never another player's, and only through the key `my_bid` (D-26's leak lock
// forbids every `sealed_bid*` spelling) — plus the count-only progress that is
// the one sealed fact shared with the projector.
func TestSealedPlayerProjectionMyBidAndProgress(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	const (
		anAmount   = int64(150_000)
		binhAmount = int64(220_000)
	)

	t.Run("LIVE: each owner reads its own amount and nothing of the other's", func(t *testing.T) {
		env.installState(lateJoinSealedFixture(PhaseLive))

		for _, tc := range []struct {
			playerID string
			own      int64
			other    int64
		}{
			{playerID: "p-an", own: anAmount, other: binhAmount},
			{playerID: "p-binh", own: binhAmount, other: anAmount},
		} {
			doc := decodeObject(t, env.engine.Snapshot(RolePlayer, tc.playerID))
			assertNoSecrets(t, "sealed owner "+tc.playerID, doc)

			me := asObject(t, doc["me"], "me")
			got, present := me["my_bid"]
			if !present || got != float64(tc.own) {
				t.Errorf("%s's my_bid = %#v (present=%t), want its own effective amount %d (D-115)", tc.playerID, got, present, tc.own)
			}

			// The other player's sheet is not an amount this payload may carry
			// under ANY key — the own-amount key is the only legal exit (D-118).
			if paths := amountPaths(doc, tc.other); len(paths) != 0 {
				t.Errorf("%s's payload carries the other player's amount %d at %v: the only legal exits are the owner and the admin (D-26/D-118)", tc.playerID, tc.other, paths)
			}
			if paths := amountPaths(doc, tc.own); len(paths) != 1 || !samePath(paths[0], "me", "my_bid") {
				t.Errorf("%s's payload carries its own amount at %v, want exactly me.my_bid", tc.playerID, paths)
			}

			progress := asObject(t, doc["sealed_progress"], "sealed_progress")
			if progress["submitted"] != float64(2) || progress["total"] != float64(3) {
				t.Errorf("%s's sealed_progress = %#v, want {submitted:2, total:3} (D-116)", tc.playerID, progress)
			}
		}

		// A player who has submitted NOTHING has no key at all: absence is the
		// wire's way of saying "not yet", exactly like last_bid (D-115).
		doc := decodeObject(t, env.engine.Snapshot(RolePlayer, "p-cuong"))
		me := asObject(t, doc["me"], "me")
		if value, present := me["my_bid"]; present {
			t.Errorf("a player with no effective row carries my_bid = %#v, want the key absent (D-115)", value)
		}
	})

	t.Run("SETTLED: the own amount is gone, the reveal is not", func(t *testing.T) {
		env.installState(lateJoinSealedFixture(PhaseSettled))

		for _, playerID := range []string{"p-an", "p-binh"} {
			doc := decodeObject(t, env.engine.Snapshot(RolePlayer, playerID))
			assertNoSecrets(t, "sealed settled "+playerID, doc)
			me := asObject(t, doc["me"], "me")
			if value, present := me["my_bid"]; present {
				t.Errorf("%s's my_bid = %#v at SETTLED, want the key absent: the own amount is emitted only while the sealed item runs (D-115)", playerID, value)
			}
			item := asObject(t, doc["item"], "item")
			reveal := asObject(t, item["settle"], "item.settle")
			if reveal["price"] != float64(anAmount) {
				t.Errorf("settled reveal price = %#v, want the CHARGED %d — for Vickrey that is the second highest, not the winner's bid (D-114)", reveal["price"], anAmount)
			}
			// The winner's own bid is never published: only the price they pay.
			if paths := amountPaths(doc, binhAmount); len(paths) != 0 {
				t.Errorf("%s's settled payload carries the winner's bid %d at %v: the reveal publishes the PRICE, never the bid (D-114)", playerID, binhAmount, paths)
			}
		}
	})
}

// TestSealedLiveFieldsAreDeadlineOnly is D-116's public-shape contract at the
// projection boundary: a LIVE sealed item carries its deadline and NOTHING from
// the ladder — no price, no minimum, no top bidder, no extension count. Without
// the type branch, currentPrice() would fall back to start_price and publish a
// plausible-looking number the client could act on (research Pitfall 4).
func TestSealedLiveFieldsAreDeadlineOnly(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))

	for _, tc := range []struct {
		name     string
		phase    Phase
		role     Role
		playerID string
	}{
		{name: "LIVE player", phase: PhaseLive, role: RolePlayer, playerID: "p-an"},
		{name: "LIVE screen", phase: PhaseLive, role: RoleScreen},
		{name: "SETTLED screen", phase: PhaseSettled, role: RoleScreen},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env.installState(lateJoinSealedFixture(tc.phase))

			doc := decodeObject(t, env.engine.Snapshot(tc.role, tc.playerID))
			assertNoSecrets(t, tc.name, doc)
			item := asObject(t, doc["item"], "item")

			for _, ladderField := range []string{"current_price", "min_next_bid", "top_bidder", "extensions_used"} {
				if value, present := item[ladderField]; present {
					t.Errorf("%s carries item.%s = %#v: a sealed run has no ladder to describe (D-116)", tc.name, ladderField, value)
				}
			}
			if got := item["deadline_at"]; got != float64(testStart.UnixMilli()+30_000) {
				t.Errorf("%s carries deadline_at = %#v, want the run's own %d: the countdown is the ONE live field a sealed run has", tc.name, got, testStart.UnixMilli()+30_000)
			}
			if _, present := item["settle"]; present != (tc.phase == PhaseSettled) {
				t.Errorf("%s carries settle presence = %t, want %t: the boundary is the object's presence (D-67)", tc.name, present, tc.phase == PhaseSettled)
			}
		})
	}
}

// TestSealedAdminLedgerProgress is D-117's presenter privilege: the admin
// snapshot carries the CURRENT item's effective rows — scoped, nickname
// included, in the tie-break's own order — plus the server's count. It is also
// the positive control for the player/screen assertions above: the amounts are
// present here, so their absence elsewhere is a boundary rather than an
// accident.
func TestSealedAdminLedgerProgress(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	env.installState(lateJoinSealedFixture(PhaseLive))

	doc := decodeObject(t, env.engine.Snapshot(RoleAdmin, ""))
	rows := asArray(t, doc["sealed_bids"], "sealed_bids")
	if len(rows) != 2 {
		t.Fatalf("the admin ledger lists %d rows, want the current item's 2 — the fixture also holds a row for another item, which must not travel: %+v", len(rows), rows)
	}

	// The order is the tie-break's own (submitted_at ASC, then player id ASC),
	// not the order the state happens to hold the rows in.
	first := asObject(t, rows[0], "sealed_bids[0]")
	assertKeySet(t, "sealed_bids[0]", first, "item_id", "player_id", "nickname", "amount", "submitted_at", "updated_at")
	if first["player_id"] != "p-an" || first["nickname"] != "An" || first["amount"] != float64(150_000) {
		t.Errorf("sealed_bids[0] = %#v, want An's 150000 — the earliest first submission leads (D-117)", first)
	}
	second := asObject(t, rows[1], "sealed_bids[1]")
	if second["player_id"] != "p-binh" || second["nickname"] != "Bình" || second["amount"] != float64(220_000) {
		t.Errorf("sealed_bids[1] = %#v, want Bình's 220000", second)
	}

	progress := asObject(t, doc["sealed_progress"], "sealed_progress")
	assertKeySet(t, "sealed_progress", progress, "submitted", "total")
	if progress["submitted"] != float64(2) || progress["total"] != float64(3) {
		t.Errorf("the admin card's count = %#v, want the server's own {submitted:2, total:3} (D-116)", progress)
	}
}
