package game

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// configCandidates is D-83's boot-time enumeration as a test fixture: one
// 192.168 address, the docker0 bridge, and a 10/8 address — the shape a real
// laptop produces (research Pitfall 2), in rank order.
var configCandidates = []string{
	"http://192.168.0.103:8080/",
	"http://172.17.0.1:8080/",
	"http://10.0.0.7:8080/",
}

// newConfigEnv is newTestEnvAt with D-83's candidate list, because the
// enumeration is a BOOT input: the candidates reach the engine through
// game.Config, are never persisted, and therefore cannot be installed on a
// running engine. It is local to this plan's tests because engine_test.go's
// fixtures deliberately pass no candidates.
func newConfigEnv(t *testing.T, path string, clock *testClock, candidates []string) *testEnv {
	t.Helper()

	st := openTestStore(t, path)
	env := &testEnv{store: st, clock: clock, fatal: make(chan error, 8)}

	eng, err := New(Config{
		Store:             st,
		Now:               clock.now,
		Fatal:             func(err error) { env.fatal <- err },
		JoinURLCandidates: candidates,
	})
	if err != nil {
		t.Fatalf("game.New: %v", err)
	}
	env.engine = eng
	return env
}

// storedConfig reads the persisted config_json back through the store, which is
// the only witness that a config edit really survived (an in-memory change
// alone would look identical in every snapshot).
func storedConfig(t *testing.T, env *testEnv) engineConfig {
	t.Helper()

	row, err := env.store.LoadGame()
	if err != nil {
		t.Fatalf("store.LoadGame: %v", err)
	}
	var persisted engineConfig
	if err := json.Unmarshal([]byte(row.ConfigJSON), &persisted); err != nil {
		t.Fatalf("unmarshalling the persisted config_json %q: %v", row.ConfigJSON, err)
	}
	return persisted
}

// TestUpdateConfig is D-81/D-84's write path: one `admin.config.update` moves
// the revision exactly once, persists the new configuration, and reaches the
// two roles that render it — the admin panel (which shows the selection) and
// the projector (whose QR codes are built from it) — while a player's own view
// is untouched, because a player never needs the show's URLs.
func TestUpdateConfig(t *testing.T) {
	env := newConfigEnv(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart}, configCandidates)

	before := env.engine.Rev()
	budget := int64(2_000_000)
	selected := "http://10.0.0.7:8080/"

	if err := env.engine.UpdateConfig(ConfigInput{StartingBudget: &budget, JoinURL: &selected}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if got := env.engine.Rev(); got != before+1 {
		t.Errorf("revision = %d, want %d: a config edit is one committed mutation", got, before+1)
	}

	persisted := storedConfig(t, env)
	if persisted.StartingBudget != budget {
		t.Errorf("persisted starting_budget = %d, want %d", persisted.StartingBudget, budget)
	}
	if persisted.JoinURL != selected {
		t.Errorf("persisted join_url = %q, want %q", persisted.JoinURL, selected)
	}
	if persisted.Currency != "VND" {
		t.Errorf("persisted currency = %q, want VND: the currency is not editable", persisted.Currency)
	}

	admin, ok := env.engine.Snapshot(RoleAdmin, "").(AdminView)
	if !ok {
		t.Fatalf("admin snapshot has type %T", env.engine.Snapshot(RoleAdmin, ""))
	}
	if admin.Config.StartingBudget != budget {
		t.Errorf("admin config.starting_budget = %d, want %d", admin.Config.StartingBudget, budget)
	}
	if admin.SelectedJoinURL != selected {
		t.Errorf("admin selected_join_url = %q, want %q", admin.SelectedJoinURL, selected)
	}

	screen, ok := env.engine.Snapshot(RoleScreen, "").(ScreenView)
	if !ok {
		t.Fatalf("screen snapshot has type %T", env.engine.Snapshot(RoleScreen, ""))
	}
	if screen.JoinURL != selected {
		t.Errorf("screen join_url = %q, want %q", screen.JoinURL, selected)
	}

	playerJSON, err := json.Marshal(env.engine.Snapshot(RolePlayer, ""))
	if err != nil {
		t.Fatalf("marshalling the player snapshot: %v", err)
	}
	var player map[string]any
	if err := json.Unmarshal(playerJSON, &player); err != nil {
		t.Fatalf("unmarshalling the player snapshot: %v", err)
	}
	for _, key := range []string{"join_url", "join_urls", "selected_join_url", "config"} {
		if _, present := player[key]; present {
			t.Errorf("the player snapshot carries %q: a player has no use for the show's URLs or config", key)
		}
	}
}

// TestJoinURLPersistence is D-84's durability contract, and it pins research
// Pitfall 11's interpretation of it: the persisted selection is authoritative
// whenever one exists, even when it is NOT among the boot-time candidates —
// which is exactly the case D-86 creates by letting the admin select
// `location.origin` (a dev box, a hotspot, a tunnel URL) that the server
// deliberately does not check against its own enumeration.
func TestJoinURLPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	selected := configCandidates[2] // deliberately not the first candidate

	first := newConfigEnv(t, path, &testClock{t: testStart}, configCandidates)
	if err := first.engine.UpdateConfig(ConfigInput{JoinURL: &selected}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	// A restart at a SHIFTED instant, with a candidate list that no longer
	// contains the selection: the admin's own URL must survive both.
	second := newConfigEnv(t, path, &testClock{t: testStart.Add(2 * time.Minute)}, []string{configCandidates[0]})
	screen, ok := second.engine.Snapshot(RoleScreen, "").(ScreenView)
	if !ok {
		t.Fatalf("screen snapshot has type %T", second.engine.Snapshot(RoleScreen, ""))
	}
	if screen.JoinURL != selected {
		t.Errorf("screen join_url after restart = %q, want the persisted %q — a restart must not re-default to a candidate", screen.JoinURL, selected)
	}
	admin, ok := second.engine.Snapshot(RoleAdmin, "").(AdminView)
	if !ok {
		t.Fatalf("admin snapshot has type %T", second.engine.Snapshot(RoleAdmin, ""))
	}
	if admin.SelectedJoinURL != selected {
		t.Errorf("admin selected_join_url after restart = %q, want %q", admin.SelectedJoinURL, selected)
	}
	if persisted := storedConfig(t, second); persisted.JoinURL != selected {
		t.Errorf("persisted join_url after restart = %q, want %q", persisted.JoinURL, selected)
	}
}

// TestJoinURLDefaultsToFirstCandidate is D-84's default arm: only a show that
// has never persisted a selection falls back to the first candidate (the
// ranking exists precisely so that this default is stable).
func TestJoinURLDefaultsToFirstCandidate(t *testing.T) {
	env := newConfigEnv(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart}, configCandidates)

	screen, ok := env.engine.Snapshot(RoleScreen, "").(ScreenView)
	if !ok {
		t.Fatalf("screen snapshot has type %T", env.engine.Snapshot(RoleScreen, ""))
	}
	if screen.JoinURL != configCandidates[0] {
		t.Errorf("screen join_url = %q, want the first candidate %q", screen.JoinURL, configCandidates[0])
	}
	if persisted := storedConfig(t, env); persisted.JoinURL != "" {
		t.Errorf("persisted join_url = %q, want empty: the default is not a selection", persisted.JoinURL)
	}

	empty := newConfigEnv(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart}, nil)
	emptyScreen, ok := empty.engine.Snapshot(RoleScreen, "").(ScreenView)
	if !ok {
		t.Fatalf("screen snapshot has type %T", empty.engine.Snapshot(RoleScreen, ""))
	}
	if emptyScreen.JoinURL != "" {
		t.Errorf("screen join_url with no candidates = %q, want empty", emptyScreen.JoinURL)
	}
}
