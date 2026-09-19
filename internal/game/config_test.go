package game

import (
	"encoding/json"
	"path/filepath"
	"strings"
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
	return newConfigEnvWith(t, path, clock, candidates, "")
}

// newConfigEnvWith is newConfigEnv with D-127's boot override injected: the
// `--public-url` / PUBLIC_URL value reaches the engine through game.Config
// exactly once, at boot, and is never durable state.
func newConfigEnvWith(t *testing.T, path string, clock *testClock, candidates []string, publicURL string) *testEnv {
	t.Helper()

	st := openTestStore(t, path)
	env := &testEnv{store: st, clock: clock, fatal: make(chan error, 8)}

	eng, err := New(Config{
		Store:             st,
		Now:               clock.now,
		Fatal:             func(err error) { env.fatal <- err },
		JoinURLCandidates: candidates,
		PublicURL:         publicURL,
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

// storedConfigJSON is storedConfig's raw form: the byte-for-byte value of
// game.config_json. It is the stronger witness for "this edit wrote nothing",
// which a decoded comparison cannot prove (a rewrite with the same values would
// look identical once parsed).
func storedConfigJSON(t *testing.T, env *testEnv) string {
	t.Helper()

	row, err := env.store.LoadGame()
	if err != nil {
		t.Fatalf("store.LoadGame: %v", err)
	}
	return row.ConfigJSON
}

// TestUpdateConfigRefusals is D-81/D-84's guard: every invalid input is a
// REFUSAL — no error is stored, no revision moves, nothing is rewritten, and the
// fatal hook is never touched, because a bad form value must not take the show
// down (D-34 escalates only real commit failures).
func TestUpdateConfigRefusals(t *testing.T) {
	ptrInt := func(v int64) *int64 { return &v }
	ptrStr := func(v string) *string { return &v }

	cases := []struct {
		name string
		in   ConfigInput
	}{
		{"a zero starting budget", ConfigInput{StartingBudget: ptrInt(0)}},
		{"a negative starting budget", ConfigInput{StartingBudget: ptrInt(-1)}},
		{"a starting budget above MaxAmount", ConfigInput{StartingBudget: ptrInt(MaxAmount + 1)}},
		{"an empty join url", ConfigInput{JoinURL: ptrStr("")}},
		{"a non-http scheme", ConfigInput{JoinURL: ptrStr("ftp://192.168.0.103:8080/")}},
		{"a scheme-less url", ConfigInput{JoinURL: ptrStr("192.168.0.103:8080/")}},
		{"an overlong join url", ConfigInput{JoinURL: ptrStr("http://" + strings.Repeat("a", MaxJoinURLLen) + "/")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newConfigEnv(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart}, configCandidates)
			before := env.engine.Rev()
			beforeJSON := storedConfigJSON(t, env)

			if err := env.engine.UpdateConfig(tc.in); err == nil {
				t.Fatalf("UpdateConfig(%+v) accepted an invalid input", tc.in)
			}
			if got := env.engine.Rev(); got != before {
				t.Errorf("revision = %d, want %d: a refusal is not a mutation", got, before)
			}
			if got := storedConfigJSON(t, env); got != beforeJSON {
				t.Errorf("config_json = %q, want the untouched %q", got, beforeJSON)
			}
			select {
			case err := <-env.fatal:
				t.Fatalf("a refused config edit escalated to the fatal hook: %v", err)
			default:
			}
		})
	}
}

// TestUpdateConfigUnchangedValuesKeepTheStoredJSON pins the middle ground
// between "refuse" and "rewrite": a VALID edit that changes nothing still
// commits (the admin's own panel needs the snapshot that says so, and every
// other admin command behaves the same way), but game.config_json is not
// rewritten, because the write set is a diff.
func TestUpdateConfigUnchangedValuesKeepTheStoredJSON(t *testing.T) {
	env := newConfigEnv(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart}, configCandidates)

	selected := configCandidates[0]
	if err := env.engine.UpdateConfig(ConfigInput{JoinURL: &selected}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	afterFirst := storedConfigJSON(t, env)
	if !strings.Contains(afterFirst, "join_url") {
		t.Fatalf("config_json = %q, want the selected join_url persisted", afterFirst)
	}

	before := env.engine.Rev()
	if err := env.engine.UpdateConfig(ConfigInput{JoinURL: &selected}); err != nil {
		t.Fatalf("UpdateConfig(unchanged): %v", err)
	}
	if got := env.engine.Rev(); got != before+1 {
		t.Errorf("revision = %d, want %d: a valid edit is still one committed mutation", got, before+1)
	}
	if got := storedConfigJSON(t, env); got != afterFirst {
		t.Errorf("config_json = %q, want the unchanged %q: an identical value is not a write", got, afterFirst)
	}
}

// TestUpdateConfigStartingBudgetAffectsFutureJoinsOnly is D-81's economics
// contract: editing starting_budget never rewrites a balance that is already in
// play. It is the budget the next join is minted with, and the budget a reset
// restores — nothing else.
func TestUpdateConfigStartingBudgetAffectsFutureJoinsOnly(t *testing.T) {
	env := newConfigEnv(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart}, configCandidates)

	first, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "An"}, nil)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}

	raised := int64(2_500_000)
	if err := env.engine.UpdateConfig(ConfigInput{StartingBudget: &raised}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	existing, ok := env.engine.Snapshot(RolePlayer, first.PlayerID).(PlayerView)
	if !ok {
		t.Fatalf("player snapshot has type %T", env.engine.Snapshot(RolePlayer, first.PlayerID))
	}
	if existing.Me.Budget != 1_000_000 {
		t.Errorf("the player already in play has budget %d, want the untouched 1_000_000", existing.Me.Budget)
	}

	second, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "Bình"}, nil)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	fresh, ok := env.engine.Snapshot(RolePlayer, second.PlayerID).(PlayerView)
	if !ok {
		t.Fatalf("player snapshot has type %T", env.engine.Snapshot(RolePlayer, second.PlayerID))
	}
	if fresh.Me.Budget != raised {
		t.Errorf("a join after the edit has budget %d, want the new starting budget %d", fresh.Me.Budget, raised)
	}

	if _, err := env.engine.Reset(ResetKeepPlayers); err != nil {
		t.Fatalf("Reset(keep_players): %v", err)
	}
	afterReset, ok := env.engine.Snapshot(RolePlayer, first.PlayerID).(PlayerView)
	if !ok {
		t.Fatalf("player snapshot has type %T", env.engine.Snapshot(RolePlayer, first.PlayerID))
	}
	if afterReset.Me.Budget != raised {
		t.Errorf("after a reset the player has budget %d, want the new starting budget %d", afterReset.Me.Budget, raised)
	}
}

// TestUpdateConfigCannotCarryAKeyShapedValue is D-74's boundary spelled out:
// the persisted config carries the budget and the join URL, and the ADMIN_KEY
// (which lives in server_config, never here) must not be reachable through any
// config path — including by being smuggled in as a URL.
func TestUpdateConfigCannotCarryAKeyShapedValue(t *testing.T) {
	env := newConfigEnv(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart}, configCandidates)

	key := strings.Repeat("a", 64)
	if err := env.engine.UpdateConfig(ConfigInput{JoinURL: &key}); err == nil {
		// The scheme guard is what keeps this closed even for a value shaped
		// like a secret.
		t.Fatalf("UpdateConfig accepted %q as a join url", key)
	}
	if stored := storedConfigJSON(t, env); strings.Contains(stored, key) {
		t.Errorf("config_json = %q, want no key-shaped value persisted", stored)
	}
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
	if persisted.Currency != "USD" {
		t.Errorf("persisted currency = %q, want USD (D-02): the currency is not editable", persisted.Currency)
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

// --- Phase 7 (D-127/D-128): the effective join URL and its runtime-only rungs ---

// TestEffectiveJoinURLPrecedence pins D-127's ONE resolution rule:
// PUBLIC_URL > a healthy tunnel > the persisted selection > the first LAN
// candidate, with the source tag naming the rung that won. Every case reads the
// SAME two projections the room actually renders from — the projector's
// join_url (the QR text) and the admin's selected_join_url + join_url_source —
// so the URL and its provenance cannot disagree.
func TestEffectiveJoinURLPrecedence(t *testing.T) {
	const (
		publicURL = "https://demo.example/join?k=show"
		tunnelURL = "https://three-dashed-words.trycloudflare.com"
	)

	cases := []struct {
		name       string
		publicURL  string
		tunnelURL  string
		healthy    bool
		selection  string // persisted through admin.config.update before the status is applied
		candidates []string
		wantURL    string
		wantSource string
	}{
		{
			name:      "PUBLIC_URL wins a healthy tunnel",
			publicURL: publicURL, tunnelURL: tunnelURL, healthy: true,
			selection: configCandidates[1], candidates: configCandidates,
			wantURL: publicURL, wantSource: "public_url",
		},
		{
			name:      "PUBLIC_URL wins the persisted selection",
			publicURL: publicURL,
			selection: configCandidates[1], candidates: configCandidates,
			wantURL: publicURL, wantSource: "public_url",
		},
		{
			name:      "a healthy tunnel wins the persisted selection",
			tunnelURL: tunnelURL, healthy: true,
			selection: configCandidates[1], candidates: configCandidates,
			wantURL: tunnelURL, wantSource: "tunnel",
		},
		{
			name:      "a healthy tunnel wins the first candidate",
			tunnelURL: tunnelURL, healthy: true,
			candidates: configCandidates,
			wantURL:    tunnelURL, wantSource: "tunnel",
		},
		{
			name:      "an unhealthy tunnel falls back to the persisted selection",
			tunnelURL: tunnelURL,
			selection: configCandidates[1], candidates: configCandidates,
			wantURL: configCandidates[1], wantSource: "selected",
		},
		{
			name:       "an unhealthy tunnel falls back to the first candidate",
			tunnelURL:  tunnelURL,
			candidates: configCandidates,
			wantURL:    configCandidates[0], wantSource: "candidate",
		},
		{
			name:      "an empty tunnel URL is not a rung",
			selection: configCandidates[2], candidates: configCandidates,
			wantURL: configCandidates[2], wantSource: "selected",
		},
		{
			name:      "an invalid PUBLIC_URL is ignored rather than advertised",
			publicURL: "not-a-url",
			tunnelURL: tunnelURL, healthy: true,
			candidates: configCandidates,
			wantURL:    tunnelURL, wantSource: "tunnel",
		},
		{
			name:    "nothing configured at all",
			wantURL: "", wantSource: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newConfigEnvWith(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart}, tc.candidates, tc.publicURL)
			if tc.selection != "" {
				selected := tc.selection
				if err := env.engine.UpdateConfig(ConfigInput{JoinURL: &selected}); err != nil {
					t.Fatalf("UpdateConfig(%q): %v", selected, err)
				}
			}
			env.engine.SetTunnelStatus(tc.tunnelURL, tc.healthy)

			screen, ok := env.engine.Snapshot(RoleScreen, "").(ScreenView)
			if !ok {
				t.Fatalf("screen snapshot has type %T", env.engine.Snapshot(RoleScreen, ""))
			}
			if screen.JoinURL != tc.wantURL {
				t.Errorf("screen join_url = %q, want %q", screen.JoinURL, tc.wantURL)
			}

			admin, ok := env.engine.Snapshot(RoleAdmin, "").(AdminView)
			if !ok {
				t.Fatalf("admin snapshot has type %T", env.engine.Snapshot(RoleAdmin, ""))
			}
			if admin.SelectedJoinURL != tc.wantURL {
				t.Errorf("admin selected_join_url = %q, want %q (the two projections read one rule)", admin.SelectedJoinURL, tc.wantURL)
			}
			if admin.JoinURLSource != tc.wantSource {
				t.Errorf("admin join_url_source = %q, want %q", admin.JoinURLSource, tc.wantSource)
			}
		})
	}
}

// TestTunnelURLIsNeverPersisted is D-128's durability rule: the quick-tunnel URL
// is EPHEMERAL, so it may become the effective selection while it is healthy and
// must never enter game.config_json — otherwise the next offline boot would
// advertise a host that no longer exists. PUBLIC_URL (D-127) is the same kind of
// value: a boot input that must not survive the process that received it.
//
// It also pins the two boundaries that keep the rule honest: the revision does
// not move for a transport fact (D-35), and the player projection never gains a
// URL field (T-07-05).
func TestTunnelURLIsNeverPersisted(t *testing.T) {
	const (
		tunnelURL   = "https://four-dashed-words.trycloudflare.com"
		overrideURL = "https://override.example/"
	)
	path := filepath.Join(t.TempDir(), "auction.db")
	selected := configCandidates[1]

	env := newConfigEnv(t, path, &testClock{t: testStart}, configCandidates)
	selection := selected
	if err := env.engine.UpdateConfig(ConfigInput{JoinURL: &selection}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	afterSelection := storedConfigJSON(t, env)

	// The tunnel becomes healthy: the QR moves, nothing is written, and the game
	// revision does not move either.
	before := env.engine.Rev()
	env.engine.SetTunnelStatus(tunnelURL, true)

	screen, ok := env.engine.Snapshot(RoleScreen, "").(ScreenView)
	if !ok {
		t.Fatalf("screen snapshot has type %T", env.engine.Snapshot(RoleScreen, ""))
	}
	if screen.JoinURL != tunnelURL {
		t.Errorf("screen join_url with a healthy tunnel = %q, want %q", screen.JoinURL, tunnelURL)
	}
	admin, ok := env.engine.Snapshot(RoleAdmin, "").(AdminView)
	if !ok {
		t.Fatalf("admin snapshot has type %T", env.engine.Snapshot(RoleAdmin, ""))
	}
	if admin.SelectedJoinURL != tunnelURL || admin.JoinURLSource != "tunnel" {
		t.Errorf("admin selection = %q (%q), want %q (%q)", admin.SelectedJoinURL, admin.JoinURLSource, tunnelURL, "tunnel")
	}
	if got := storedConfigJSON(t, env); got != afterSelection {
		t.Errorf("config_json = %q, want the untouched %q — a quick-tunnel URL is ephemeral and must not be persisted (D-128)", got, afterSelection)
	}
	if got := env.engine.Rev(); got != before {
		t.Errorf("revision = %d, want the unchanged %d: a transport fact is not a game mutation (D-35)", got, before)
	}

	// Unhealthy falls straight back to the persisted selection: the room keeps
	// playing on LAN/local while the tunnel restarts (D-129).
	env.engine.SetTunnelStatus(tunnelURL, false)
	if screen := env.engine.Snapshot(RoleScreen, "").(ScreenView); screen.JoinURL != selected {
		t.Errorf("screen join_url after the tunnel went unhealthy = %q, want the persisted %q", screen.JoinURL, selected)
	}

	// A restart on the same file: the ephemeral URL is gone for good, the
	// selection is still there, and the tunnel host never reached the database.
	second := newConfigEnv(t, path, &testClock{t: testStart.Add(5 * time.Minute)}, []string{configCandidates[0]})
	if screen := second.engine.Snapshot(RoleScreen, "").(ScreenView); screen.JoinURL != selected {
		t.Errorf("screen join_url after a restart = %q, want the persisted %q — a runtime URL must not survive the process", screen.JoinURL, selected)
	}
	if got := storedConfigJSON(t, second); strings.Contains(got, "trycloudflare") {
		t.Errorf("config_json after a restart = %q, want no tunnel host in durable state", got)
	}

	// PUBLIC_URL is a boot input with the same rule: it outranks the healthy
	// tunnel and is never written either.
	third := newConfigEnvWith(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart}, configCandidates, overrideURL)
	third.engine.SetTunnelStatus(tunnelURL, true)
	if screen := third.engine.Snapshot(RoleScreen, "").(ScreenView); screen.JoinURL != overrideURL {
		t.Errorf("screen join_url with PUBLIC_URL set = %q, want the override %q (D-127)", screen.JoinURL, overrideURL)
	}
	if got := storedConfigJSON(t, third); strings.Contains(got, "override.example") {
		t.Errorf("config_json = %q, want no public URL in durable state", got)
	}

	// T-07-05: the player projection never gains a URL field, however the URL
	// was resolved.
	playerJSON, err := json.Marshal(env.engine.Snapshot(RolePlayer, ""))
	if err != nil {
		t.Fatalf("marshalling the player snapshot: %v", err)
	}
	var player map[string]any
	if err := json.Unmarshal(playerJSON, &player); err != nil {
		t.Fatalf("unmarshalling the player snapshot: %v", err)
	}
	for _, key := range []string{"join_url", "join_urls", "selected_join_url", "join_url_source"} {
		if _, present := player[key]; present {
			t.Errorf("the player snapshot carries %q: a player has no use for the show's URLs (T-07-05)", key)
		}
	}
}

// TestJoinURLRotatesWithTunnelStatus is D-128's rotation half: a restarted
// cloudflared announces a NEW origin, and the projection every projector renders
// its QR from follows it in place — no mutation, no restart, no revision move.
// It also pins the tag's audience: `join_url_source` is additive on the ADMIN
// view alone, so neither the projector nor a player ever learns why the URL
// moved (T-07-05).
func TestJoinURLRotatesWithTunnelStatus(t *testing.T) {
	const (
		first  = "https://first-rotated-label.trycloudflare.com"
		second = "https://second-rotated-label.trycloudflare.com"
	)
	selected := configCandidates[1]

	env := newConfigEnv(t, filepath.Join(t.TempDir(), "auction.db"), &testClock{t: testStart}, configCandidates)
	selection := selected
	if err := env.engine.UpdateConfig(ConfigInput{JoinURL: &selection}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	screenOf := func() ScreenView {
		t.Helper()
		view, ok := env.engine.Snapshot(RoleScreen, "").(ScreenView)
		if !ok {
			t.Fatalf("screen snapshot has type %T", env.engine.Snapshot(RoleScreen, ""))
		}
		return view
	}
	adminOf := func() AdminView {
		t.Helper()
		view, ok := env.engine.Snapshot(RoleAdmin, "").(AdminView)
		if !ok {
			t.Fatalf("admin snapshot has type %T", env.engine.Snapshot(RoleAdmin, ""))
		}
		return view
	}

	env.engine.SetTunnelStatus(first, true)
	if got := screenOf().JoinURL; got != first {
		t.Fatalf("screen join_url = %q, want %q", got, first)
	}
	if admin := adminOf(); admin.SelectedJoinURL != first || admin.JoinURLSource != "tunnel" {
		t.Fatalf("admin selection = %q (%q), want %q (\"tunnel\")", admin.SelectedJoinURL, admin.JoinURLSource, first)
	}

	// A restart: the supervisor publishes a NEW origin, and the QR follows it.
	env.engine.SetTunnelStatus(second, true)
	if got := screenOf().JoinURL; got != second {
		t.Errorf("screen join_url after the tunnel rotated = %q, want the new origin %q", got, second)
	}
	if got := adminOf().JoinURLSource; got != "tunnel" {
		t.Errorf("admin join_url_source after the rotation = %q, want %q", got, "tunnel")
	}

	// The tag is admin-only: the projector renders a QR from join_url and has no
	// business guessing which rung produced it.
	screenJSON, err := json.Marshal(screenOf())
	if err != nil {
		t.Fatalf("marshalling the screen snapshot: %v", err)
	}
	var screenKeys map[string]any
	if err := json.Unmarshal(screenJSON, &screenKeys); err != nil {
		t.Fatalf("unmarshalling the screen snapshot: %v", err)
	}
	if _, present := screenKeys["join_url_source"]; present {
		t.Error("the screen snapshot carries join_url_source: the source tag is additive on the admin view alone")
	}

	// The tunnel dies: the QR falls straight back to the persisted selection and
	// the tag names that rung instead.
	env.engine.SetTunnelStatus(second, false)
	if got := screenOf().JoinURL; got != selected {
		t.Errorf("screen join_url after the tunnel went unhealthy = %q, want the persisted %q (D-129)", got, selected)
	}
	if admin := adminOf(); admin.JoinURLSource != "selected" {
		t.Errorf("admin join_url_source after the fallback = %q, want %q", admin.JoinURLSource, "selected")
	}
}
