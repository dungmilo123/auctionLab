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

// walkKeys visits every key of a decoded JSON document, at every depth.
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

// TestPlayerAndScreenWirePayloadsLeakNothing asserts on the frames that actually
// arrived, not on the marshal of a Go value: it catches a projection sent to the
// wrong role, which is the one leak the type boundary cannot see.
//
// The admin connection is the positive control. Without it this test would pass
// just as happily if the walker were broken, or if the server never sent
// anything secret-shaped at all.
func TestPlayerAndScreenWirePayloadsLeakNothing(t *testing.T) {
	h, _ := newTestHub(t)

	player := dialWS(t, h)
	playerHello, _ := join(t, player, `{"type":"join","role":"player","nickname":"An"}`)
	ownToken := frameString(t, frameData(t, playerHello), "token")

	screen := dialWS(t, h)
	join(t, screen, `{"type":"join","role":"screen"}`)

	admin := dialWS(t, h)
	_, adminSnapshot := join(t, admin, `{"type":"join","role":"admin"}`)

	// Traffic after the handshakes: a fan-out from another player's join, plus a
	// resync on the screen — the two paths that build a projection and hand it
	// to a connection.
	other := dialWS(t, h)
	join(t, other, `{"type":"join","role":"player","nickname":"Binh"}`)
	writeFrame(t, screen, `{"type":"resync"}`)

	playerFrames := drainFrames(t, player, 300*time.Millisecond)
	screenFrames := drainFrames(t, screen, 300*time.Millisecond)

	if len(playerFrames) == 0 || len(screenFrames) == 0 {
		t.Fatalf("no fan-out traffic was captured (player %d frames, screen %d frames); the leak assertions would be vacuous",
			len(playerFrames), len(screenFrames))
	}

	for _, tc := range []struct {
		name   string
		frames []frame
	}{
		{name: "player", frames: append([]frame{playerHello}, playerFrames...)},
		{name: "screen", frames: screenFrames},
	} {
		for i, env := range tc.frames {
			var doc map[string]any
			if err := json.Unmarshal(env.Data, &doc); err != nil {
				t.Fatalf("%s frame %d (%s): data is not a JSON object: %v", tc.name, i, env.Type, err)
			}
			walkKeys(doc, func(key string) {
				if !forbiddenKey(key) {
					return
				}
				if key == "token" && env.Type == TypeHello && doc["token"] == ownToken {
					return // D-29's single exception: the caller's own token in its own hello
				}
				t.Errorf("%s frame %d (%s) carries the forbidden key %q: a player/screen payload must not be able to express a secret (D-26)",
					tc.name, i, env.Type, key)
			})
		}
	}

	// A screen is public: it must never carry a player's own-identity block.
	for i, env := range screenFrames {
		var doc map[string]any
		if err := json.Unmarshal(env.Data, &doc); err != nil {
			t.Fatalf("screen frame %d: %v", i, err)
		}
		if _, ok := doc["me"]; ok {
			t.Errorf("screen frame %d (%s) carries a `me` block: one client's identity reached another (D-25)", i, env.Type)
		}
	}

	// Positive control: the walker can see a secret when one is really there.
	var adminDoc map[string]any
	if err := json.Unmarshal(adminSnapshot.Data, &adminDoc); err != nil {
		t.Fatalf("admin snapshot is not a JSON object: %v", err)
	}
	found := false
	walkKeys(adminDoc, func(key string) {
		if forbiddenKey(key) {
			found = true
		}
	})
	if !found {
		t.Fatal("the admin snapshot carried none of D-26's secret keys; this test cannot claim to detect a leak")
	}
}
