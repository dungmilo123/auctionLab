package hub

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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
func TestPlayerAndScreenWirePayloadsLeakNothing(t *testing.T) {
	h, _ := newTestHubWithPolicy(t, testPolicyWithBuffer(64))

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

	// --- the assertions ----------------------------------------------------
	staged := []struct {
		name   string
		frames []frame
		reveal bool
	}{
		{name: "player", frames: append([]frame{playerHello}, playerFrames...)},
		{name: "screen", frames: screenFrames},
		{name: "pre-settle", frames: preSettle},
		{name: "settled late join", frames: []frame{after}, reveal: true},
	}

	sawSettle := false
	for _, tc := range staged {
		for i, env := range tc.frames {
			doc := decodedObject(t, env)
			walkKeysAt(doc, nil, func(path []string, key string) {
				if !forbiddenKey(key) {
					return
				}
				if key == "token" && env.Type == TypeHello && doc["token"] == ownToken {
					return // D-29's single exception: the caller's own token in its own hello
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
