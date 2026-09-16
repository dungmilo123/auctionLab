package hub

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"auctionlab/internal/game"
)

// This file is RT-01's wire-level half. internal/game already locks the
// projection TYPES (D-26): the internal state has no JSON tags and the only way
// out is a role projection. That lock cannot see what the hub actually puts on
// the socket, and the hub is free to marshal anything it likes — including, by
// accident, the wrong client's projection. So this test walks the bytes that
// really crossed a real connection.
//
// The rule is duplicated here on purpose: a change to the game package's helper
// must not be able to weaken the wire lock.

// forbiddenKey is D-26's key rule, applied to JSON keys: the item's secret
// common value, any sealed-bid key (the collection matches the prefix, which is
// intended — the collection is a secret too), and any token key.
func forbiddenKey(key string) bool {
	return key == "truth_value" || key == "token" || strings.HasPrefix(key, "sealed_bid")
}

// walkKeysAt visits every key of a decoded JSON document, at every depth, and
// carries the PATH that led to it. D-67's leak boundary is positional: after
// SETTLED, `truth_value` is legal INSIDE `settle` — and a leak anywhere else.
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

// insideSettle reports whether a key sits beneath the settle object, which is
// the one place a revealed truth value may travel.
func insideSettle(path []string) bool {
	for _, segment := range path {
		if segment == "settle" {
			return true
		}
	}
	return false
}

// carriesKeyDeep reports whether any object in the document carries key.
func carriesKeyDeep(value any, key string) bool {
	found := false
	walkKeysAt(value, nil, func(_ []string, got string) {
		if got == key {
			found = true
		}
	})
	return found
}

// decodedObject unmarshals a frame's data into a generic document.
func decodedObject(t *testing.T, env frame) map[string]any {
	t.Helper()

	var doc map[string]any
	if err := json.Unmarshal(env.Data, &doc); err != nil {
		t.Fatalf("frame %s: data is not a JSON object: %v", env.Type, err)
	}
	return doc
}

// decodedDocument unmarshals a frame's data into whatever JSON it is: a frame
// whose payload is an ARRAY (the `leaderboard` fan-out) is a legitimate public
// frame, and the walk must be able to read it rather than die on it (Phase 5's
// Dutch settle group is the first group that carries one).
func decodedDocument(t *testing.T, env frame) any {
	t.Helper()

	var doc any
	if err := json.Unmarshal(env.Data, &doc); err != nil {
		t.Fatalf("frame %s: data is not JSON: %v", env.Type, err)
	}
	return doc
}

// documentObject reports the frame's data as an object, when it is one.
func documentObject(doc any) (map[string]any, bool) {
	obj, ok := doc.(map[string]any)
	return obj, ok
}

// startLiveDutchItem creates one Dutch item through the engine's own mutation
// path (D-92 gates the LIVE edge by type, D-93 leaves the run deadline-less) and
// drives it to LIVE, returning its id — the Dutch twin of ticker_test's
// startLiveItem, so the walk below exercises production mutations rather than a
// hand-built state.
func startLiveDutchItem(t *testing.T, e *game.Engine) string {
	t.Helper()

	floor, step, interval := int64(40_000), int64(10_000), int64(1_000)
	if err := e.ItemUpsert(game.ItemInput{
		Name: "Đồng hồ", AuctionType: "dutch", TruthValue: 350_000,
		StartPrice: 100_000, FloorPrice: &floor, DecrementStep: &step, DecrementIntervalMS: &interval,
	}); err != nil {
		t.Fatalf("creating the Dutch item: %v", err)
	}

	view, ok := e.Snapshot(game.RoleAdmin, "").(game.AdminView)
	if !ok {
		t.Fatal("the admin snapshot is not an AdminView")
	}
	id := ""
	for _, it := range view.Items {
		if it.AuctionType == "dutch" {
			id = it.ID
		}
	}
	if id == "" {
		t.Fatal("no Dutch item reached the admin projection after the upsert")
	}
	if err := e.StartItem(id); err != nil {
		t.Fatalf("start (LOBBY → ITEM_INTRO): %v", err)
	}
	if err := e.StartItem(id); err != nil {
		t.Fatalf("start (ITEM_INTRO → LIVE): %v", err)
	}
	return id
}

// mentionsItem reports whether any object in a frame's data names the item —
// the filter the Dutch half uses, because the mutation that CREATES a second
// item still fans out a snapshot of the item that is current until the next
// command moves the room (so an unfiltered drain mixes the previous item's
// post-settle frames into a Dutch group).
func mentionsItem(t *testing.T, env frame, want string) bool {
	t.Helper()

	found := false
	var visit func(value any)
	visit = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			if node["id"] == want || node["item_id"] == want {
				found = true
			}
			for _, child := range node {
				visit(child)
			}
		case []any:
			for _, child := range node {
				visit(child)
			}
		}
	}
	visit(decodedDocument(t, env))
	return found
}

// filterFrames keeps the frames that name want.
func filterFrames(t *testing.T, frames []frame, want string) []frame {
	t.Helper()

	kept := make([]frame, 0, len(frames))
	for _, env := range frames {
		if mentionsItem(t, env, want) {
			kept = append(kept, env)
		}
	}
	return kept
}

// TestPlayerAndScreenWirePayloadsLeakNothing asserts on the frames that actually
// arrived, not on the marshal of a Go value: it catches a projection sent to the
// wrong role, which is the one leak the type boundary cannot see.
//
// Phase 3 extends it with the reveal's two-sided contract (D-68): BEFORE
// SETTLED a player or screen frame carries neither `truth_value` nor the `settle`
// key at all; a frame that delivered the reveal carries it INSIDE `settle` and
// nowhere else. The admin connection is the positive control — without it this
// test would pass just as happily if the walker were broken, or if the server
// never sent anything secret-shaped at all.
//
// Phase 5 adds a Dutch run to the same walk (D-94/D-105): the falling price's
// clock and mechanics reach both public roles, the run's own accept moves the
// reveal boundary exactly as an English settle does, and no Dutch frame carries
// a ladder field or a secret before SETTLED.
func TestPlayerAndScreenWirePayloadsLeakNothing(t *testing.T) {
	h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(64))

	// --- Phase 2's traffic: the handshake, a fan-out and a resync -----------
	player := dialWS(t, h)
	playerHello, _ := join(t, player, `{"type":"join","role":"player","nickname":"An"}`)
	ownToken := frameString(t, frameData(t, playerHello), "token")

	screen := dialWS(t, h)
	join(t, screen, `{"type":"join","role":"screen"}`)

	admin := dialWS(t, h)
	_, adminSnapshot := join(t, admin, adminJoinJSON)

	other := dialWS(t, h)
	join(t, other, `{"type":"join","role":"player","nickname":"Binh"}`)
	writeFrame(t, screen, `{"type":"resync"}`)

	playerFrames := drainFrames(t, player, 300*time.Millisecond)
	screenFrames := drainFrames(t, screen, 300*time.Millisecond)

	if len(playerFrames) == 0 || len(screenFrames) == 0 {
		t.Fatalf("no fan-out traffic was captured (player %d frames, screen %d frames); the leak assertions would be vacuous",
			len(playerFrames), len(screenFrames))
	}

	// --- Phase 3's auction round, up to but NOT including the reveal --------
	bidder := dialWS(t, h)
	join(t, bidder, `{"type":"join","role":"player","nickname":"Cuong"}`)
	bigScreen := dialWS(t, h)
	join(t, bigScreen, `{"type":"join","role":"screen"}`)

	id := createItem(t, admin)
	startItem(t, admin, id)
	startItem(t, admin, id)
	placeBid(t, bidder, 100_000)

	bidderFrames := drainFrames(t, bidder, 300*time.Millisecond)
	bigScreenFrames := drainFrames(t, bigScreen, 300*time.Millisecond)
	preSettle := append(append([]frame{}, bidderFrames...), bigScreenFrames...)
	if len(preSettle) == 0 {
		t.Fatal("no auction traffic was captured pre-settle; the pre-reveal assertions would be vacuous")
	}

	// --- the reveal --------------------------------------------------------
	writeFrame(t, admin, `{"type":"admin.item.force_end"}`)

	_, after := joinPlayer(t, h, "Dung") // a late joiner after the crash-equivalent: snapshot only

	// --- Phase 5's Dutch half: fresh public sockets watch a Dutch item run, so
	// every frame below is a Dutch frame and nothing else. drainFrames is
	// TERMINAL for the socket it drains (its quiet window is a read deadline),
	// so the LIVE half and the accept half get their own connections.
	livePlayer, _ := joinPlayer(t, h, "Hùng")
	liveScreen := joinRole(t, h, RoleScreen)

	dutchID := startLiveDutchItem(t, engine)

	// The LIVE half — every Dutch frame a player or the projector can receive
	// before anything settles. The filter keeps only frames that name the Dutch
	// item: the upsert and the intro reproduce the PREVIOUS current item's
	// snapshot, which is the English item's post-settle state.
	dutchLive := filterFrames(t, append(
		drainFrames(t, livePlayer, 300*time.Millisecond),
		drainFrames(t, liveScreen, 300*time.Millisecond)...,
	), dutchID)

	// Then the accept: the winner's own frame settles the item, and the reveal
	// reaches both public roles exactly the way the English force-end did.
	accepter, _ := joinPlayer(t, h, "Linh")
	settleScreen := joinRole(t, h, RoleScreen)

	writeFrame(t, accepter, `{"type":"dutch.accept"}`)
	dutchSettle := filterFrames(t, append(
		drainFrames(t, accepter, 300*time.Millisecond),
		drainFrames(t, settleScreen, 300*time.Millisecond)...,
	), dutchID)
	if len(dutchLive) == 0 || len(dutchSettle) == 0 {
		t.Fatalf("no Dutch frames crossed the wire (live %d, settle %d); the Dutch assertions would be vacuous", len(dutchLive), len(dutchSettle))
	}

	// --- the assertions ----------------------------------------------------
	staged := []struct {
		name   string
		frames []frame
		reveal bool
	}{
		{name: "player", frames: append([]frame{playerHello}, playerFrames...)},
		{name: "screen", frames: screenFrames},
		{name: "pre-settle", frames: preSettle},
		{name: "dutch pre-settle", frames: dutchLive},
		{name: "settled late join", frames: []frame{after}, reveal: true},
		{name: "dutch settle", frames: dutchSettle, reveal: true},
	}

	sawSettle := false
	for _, tc := range staged {
		for i, env := range tc.frames {
			doc := decodedDocument(t, env)
			walkKeysAt(doc, nil, func(path []string, key string) {
				if !forbiddenKey(key) {
					return
				}
				if key == "token" && env.Type == TypeHello {
					if obj, ok := documentObject(doc); ok && obj["token"] == ownToken {
						return // D-29's single exception: the caller's own token in its own hello
					}
				}
				if key == "truth_value" && tc.reveal && insideSettle(path) {
					sawSettle = true
					return // D-68: the reveal travels inside the settle object
				}
				t.Errorf("%s frame %d (%s) carries the forbidden key %q at %v: a player/screen payload must not be able to express a secret (D-26/D-67)",
					tc.name, i, env.Type, key, path)
			})

			// D-67's boundary is the presence of the whole object: before the
			// reveal the key does not exist at all (nil pointer + omitempty).
			if !tc.reveal && carriesKeyDeep(doc, "settle") {
				t.Errorf("%s frame %d (%s) carries the settle key before SETTLED: the boundary is the object's absence",
					tc.name, i, env.Type)
			}
		}
	}

	// Non-vacuity: the pre-settle traffic must really have carried a live item,
	// and the late joiner must really have carried the reveal.
	liveSeen := false
	for _, env := range preSettle {
		if carriesKeyDeep(decodedObject(t, env), "current_price") {
			liveSeen = true
		}
	}
	if !liveSeen {
		t.Fatal("no pre-settle frame carried a live price; the absence assertions would be vacuous")
	}
	if !sawSettle {
		t.Fatal("the late joiner's snapshot carried no revealed truth_value inside settle (D-63/D-68)")
	}

	// The Dutch LIVE half must really have carried the falling price's clock:
	// asserting an empty walk would be vacuous. Every LIVE payload of the Dutch
	// item carries started_at and the mechanics, and none carries a ladder
	// minimum or a deadline (D-93/D-94/D-98).
	dutchLiveItems := 0
	for _, env := range dutchLive {
		doc := decodedObject(t, env)
		if wrapped, ok := doc["item"].(map[string]any); ok {
			doc = wrapped // phase.changed wraps the item; item.live carries it directly
		}
		if doc["id"] != dutchID || doc["status"] != "live" {
			continue
		}
		dutchLiveItems++
		if _, ok := doc["started_at"]; !ok {
			t.Errorf("Dutch frame %s carries the LIVE item without started_at: a client cannot re-derive the price (D-94)", env.Type)
		}
		if _, ok := doc["current_price"]; !ok {
			t.Errorf("Dutch frame %s carries the LIVE item without current_price (DTCH-01)", env.Type)
		}
		for _, forbidden := range []string{"min_next_bid", "deadline_at", "top_bidder"} {
			if _, ok := doc[forbidden]; ok {
				t.Errorf("Dutch frame %s carries %q: a Dutch item has no ladder and no deadline (D-93/D-98)", env.Type, forbidden)
			}
		}
	}
	if dutchLiveItems == 0 {
		t.Fatal("no LIVE Dutch item payload crossed the wire; the Dutch assertions would be vacuous")
	}

	// A screen is public: it must never carry a player's own-identity block.
	for i, env := range append(append([]frame{}, screenFrames...), bigScreenFrames...) {
		if _, ok := decodedObject(t, env)["me"]; ok {
			t.Errorf("screen frame %d (%s) carries a `me` block: one client's identity reached another (D-25)", i, env.Type)
		}
	}

	// Positive control: the walker can see a secret when one is really there.
	adminDoc := decodedObject(t, adminSnapshot)
	found := false
	walkKeysAt(adminDoc, nil, func(_ []string, key string) {
		if forbiddenKey(key) {
			found = true
		}
	})
	if !found {
		t.Fatal("the admin snapshot carried none of D-26's secret keys; this test cannot claim to detect a leak")
	}
}

// sealedAmountA and sealedAmountB are the two REAL effective amounts the sealed
// wire group is built from. A populated fixture is the point: an empty one would
// let every absence assertion below pass without proving anything (D-26).
const (
	sealedAmountA = int64(150_000)
	sealedAmountB = int64(220_000)
)

// sealedAmountPaths walks a decoded frame and collects the key path of every
// number equal to want. Asserting on PATHS rather than on a key name is what
// catches an amount that leaked under a key nobody thought to forbid.
func sealedAmountPaths(value any, path []string, want int64, out *[][]string) {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			next := append(append([]string(nil), path...), key)
			if number, ok := child.(float64); ok && int64(number) == want {
				*out = append(*out, next)
			}
			sealedAmountPaths(child, next, want, out)
		}
	case []any:
		for _, child := range node {
			sealedAmountPaths(child, path, want, out)
		}
	}
}

// amountsIn returns every path in one frame's data whose value is want.
func amountsIn(t *testing.T, env frame, want int64) [][]string {
	t.Helper()

	var paths [][]string
	sealedAmountPaths(decodedDocument(t, env), nil, want, &paths)
	return paths
}

// isOwnBidPath reports whether a path is exactly the ONE legal exit a player has
// for an amount: its own `me.my_bid` (D-115).
func isOwnBidPath(path []string) bool {
	return len(path) == 2 && path[0] == "me" && path[1] == "my_bid"
}

// TestSealedWirePayloadsLeakNothing is D-118's wire lock for the sealed secret,
// asserted on the bytes that really crossed a socket while a POPULATED sealed
// window ran. The walk covers every message a player or the projector can
// receive — the handshake, both roles' snapshots, a private refusal, an error
// frame and the private confirmation — and the only two legal exits for an
// effective amount are the OWNER's `my_bid` and the admin's ledger (D-26/D-117).
//
// The admin connection is the positive control: the amounts are really in the
// room's state, so their absence from the player and screen streams is a
// boundary rather than an accident.
func TestSealedWirePayloadsLeakNothing(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	clock := &testClock{t: base.Add(1 * time.Second)}
	h, _, _ := newLiveSealedHub(t, clock, base.UnixMilli(), 1_000_000)

	// Player A resumes the sealed identity its token owns; player B joins fresh.
	// BOTH submit, so two real amounts exist and either one could leak.
	playerA := dialWS(t, h)
	helloA, _ := join(t, playerA, `{"type":"join","role":"player","token":"`+sealedPlayerToken+`"}`)
	expectFrames(t, playerA, TypeStateSnapshot) // the resume's own presence fan-out

	playerB := dialWS(t, h)
	helloB, _ := join(t, playerB, `{"type":"join","role":"player","nickname":"Bình"}`)
	expectFrames(t, playerB, TypeStateSnapshot) // the join's own mutation snapshot

	screen := joinRole(t, h, RoleScreen)
	admin := joinRole(t, h, RoleAdmin)

	// The traffic: two accepted submissions (each answered privately and then
	// fanned out), one refusal, and one unknown command.
	writeFrame(t, playerA, fmt.Sprintf(`{"type":"sealed.submit","amount":%d}`, sealedAmountA))
	writeFrame(t, playerB, fmt.Sprintf(`{"type":"sealed.submit","amount":%d}`, sealedAmountB))
	writeFrame(t, playerA, `{"type":"sealed.submit","amount":0}`)
	writeFrame(t, playerB, `{"type":"future.command"}`)

	playerAFrames := append([]frame{helloA}, drainFrames(t, playerA, 300*time.Millisecond)...)
	playerBFrames := append([]frame{helloB}, drainFrames(t, playerB, 300*time.Millisecond)...)
	screenFrames := drainFrames(t, screen, 300*time.Millisecond)
	adminFrames := drainFrames(t, admin, 300*time.Millisecond)

	if len(playerAFrames) < 4 || len(playerBFrames) < 4 || len(screenFrames) < 2 || len(adminFrames) < 2 {
		t.Fatalf("the sealed fixture produced too little traffic (A %d, B %d, screen %d, admin %d frames); the leak assertions would be vacuous",
			len(playerAFrames), len(playerBFrames), len(screenFrames), len(adminFrames))
	}

	// D-26: no forbidden key anywhere on a player or screen stream.
	for _, group := range []struct {
		name   string
		frames []frame
	}{
		{name: "player A", frames: playerAFrames},
		{name: "player B", frames: playerBFrames},
		{name: "screen", frames: screenFrames},
	} {
		for i, env := range group.frames {
			doc := decodedDocument(t, env)
			walkKeysAt(doc, nil, func(path []string, key string) {
				if !forbiddenKey(key) {
					return
				}
				if key == "token" && env.Type == TypeHello {
					return // D-29's single exception: the caller's own token
				}
				t.Errorf("%s frame %d (%s) carries the forbidden key %q at %v: a player/screen payload must not be able to express a secret (D-26)",
					group.name, i, env.Type, key, path)
			})
			if obj, ok := doc.(map[string]any); ok && group.name == "screen" {
				if _, present := obj["me"]; present {
					t.Errorf("screen frame %d (%s) carries a `me` block: one client's identity reached another (D-25)", i, env.Type)
				}
			}
		}
	}

	// The owner's amount is legal at exactly ONE path, and the other player's is
	// nowhere at all.
	for _, tc := range []struct {
		name    string
		frames  []frame
		own     int64
		foreign int64
	}{
		{name: "player A", frames: playerAFrames, own: sealedAmountA, foreign: sealedAmountB},
		{name: "player B", frames: playerBFrames, own: sealedAmountB, foreign: sealedAmountA},
	} {
		sawOwn := false
		for i, env := range tc.frames {
			if paths := amountsIn(t, env, tc.foreign); len(paths) != 0 {
				t.Errorf("%s frame %d (%s) carries the OTHER player's amount %d at %v: the only legal exits are the owner and the admin (D-26/D-118)",
					tc.name, i, env.Type, tc.foreign, paths)
			}
			for _, path := range amountsIn(t, env, tc.own) {
				sawOwn = true
				if !isOwnBidPath(path) {
					t.Errorf("%s frame %d (%s) carries its own amount %d at %v, want only me.my_bid — the D-26 leak lock forbids every sealed_bid* spelling",
						tc.name, i, env.Type, tc.own, path)
				}
			}
		}
		if !sawOwn {
			t.Errorf("%s never received its own amount: the own-bid exit (me.my_bid) is missing from the live sealed window (D-115)", tc.name)
		}
	}

	// The projector may read the count and the deadline and nothing else.
	sawProgress := false
	highestSubmitted := 0
	for i, env := range screenFrames {
		doc := decodedDocument(t, env)
		if paths := amountsIn(t, env, sealedAmountA); len(paths) != 0 {
			t.Errorf("screen frame %d (%s) carries player A's amount at %v: the projector sees the count and never an amount (D-116)", i, env.Type, paths)
		}
		if paths := amountsIn(t, env, sealedAmountB); len(paths) != 0 {
			t.Errorf("screen frame %d (%s) carries player B's amount at %v: the projector sees the count and never an amount (D-116)", i, env.Type, paths)
		}
		obj, ok := documentObject(doc)
		if !ok {
			continue
		}
		if progress, ok := obj["sealed_progress"].(map[string]any); ok {
			sawProgress = true
			if progress["total"] != float64(2) {
				t.Errorf("screen frame %d sealed_progress.total = %#v, want the room's 2", i, progress["total"])
			}
			if submitted, ok := progress["submitted"].(float64); ok && int(submitted) > highestSubmitted {
				highestSubmitted = int(submitted)
			}
		}
		if item, ok := obj["item"].(map[string]any); ok && item["status"] == "live" {
			if _, present := item["deadline_at"]; !present {
				t.Errorf("screen frame %d carries a LIVE sealed item without deadline_at: the countdown has no anchor (D-116)", i)
			}
			for _, ladder := range []string{"current_price", "min_next_bid", "top_bidder", "extensions_used"} {
				if value, present := item[ladder]; present {
					t.Errorf("screen frame %d carries item.%s = %#v: a sealed run has no ladder to describe (D-116)", i, ladder, value)
				}
			}
		}
	}
	if !sawProgress {
		t.Fatal("no screen frame carried sealed_progress; the count assertions would be vacuous")
	}
	if highestSubmitted != 2 {
		t.Errorf("the projector's count only reached %d of 2 while both players submitted: the count must move on every sealed submit (D-110)", highestSubmitted)
	}

	// The private confirmation is the count and nothing else (D-111), and the
	// refusal is the ordinary private frame.
	confirmations := 0
	for i, env := range playerAFrames {
		switch env.Type {
		case TypeSealedReceived:
			confirmations++
			data := frameData(t, env)
			if len(data) != 2 || !hasKey(data, "submitted") || !hasKey(data, "total") {
				t.Errorf("player A frame %d: sealed.received keys = %v, want exactly submitted+total (SEAL-01: the confirmation contains no price)", i, data)
			}
		case TypeError:
			// An error frame is a code and a message; the walk above already
			// proved it carries no amount and no forbidden key.
		}
	}
	if confirmations != 1 {
		t.Errorf("player A received %d sealed.received frame(s), want exactly 1", confirmations)
	}

	// Positive control: the walker CAN see the amounts, and the admin ledger is
	// where they are allowed to be (D-117).
	adminA, adminB := false, false
	for _, env := range adminFrames {
		if len(amountsIn(t, env, sealedAmountA)) > 0 {
			adminA = true
		}
		if len(amountsIn(t, env, sealedAmountB)) > 0 {
			adminB = true
		}
	}
	if !adminA || !adminB {
		t.Fatalf("the admin stream carried A=%t B=%t: the presenter's ledger is the positive control and must hold both amounts (D-26/D-117)", adminA, adminB)
	}
}
