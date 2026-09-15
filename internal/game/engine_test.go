package game

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"auctionlab/internal/store"
)

// testStart is the injected clock's fixed instant (D-38). Every timestamp the
// engine writes must derive from it, so an expectation can be an exact literal.
var testStart = time.UnixMilli(1_700_000_000_000)

// testClock is the injectable clock D-38 requires: tests move time instead of
// sleeping.
type testClock struct{ t time.Time }

func (c *testClock) now() time.Time { return c.t }

// testEnv is one engine bound to a fresh temp database, with the fatal hook
// captured (instead of exiting) and the publish edge recorded.
type testEnv struct {
	engine *Engine
	store  *store.Store
	clock  *testClock
	fatal  chan error

	mu        sync.Mutex
	published []int64
}

// openTestStore opens (and cleans up) one store on path, schema applied.
func openTestStore(t *testing.T, path string) *store.Store {
	t.Helper()

	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := store.Schema(st.DB); err != nil {
		t.Fatalf("store.Schema(%q): %v", path, err)
	}
	return st
}

// newTestEnvAt builds an engine on path with the injected clock. Calling it
// twice with the same path is how a restart is simulated — the second call is a
// fresh process-equivalent — and the clock parameter is what makes the restart
// land at a SHIFTED instant (research Pitfall 14: a "restart" that reuses the
// same clock and the same engine has not exercised the load path at all).
func newTestEnvAt(t *testing.T, path string, clock *testClock) *testEnv {
	t.Helper()

	st := openTestStore(t, path)
	env := &testEnv{store: st, clock: clock, fatal: make(chan error, 8)}

	eng, err := New(Config{
		Store: st,
		Now:   clock.now,
		Fatal: func(err error) { env.fatal <- err },
	})
	if err != nil {
		t.Fatalf("game.New: %v", err)
	}
	env.engine = eng
	eng.SetPublisher(func(rev int64, _ View, _ []Event) {
		env.mu.Lock()
		defer env.mu.Unlock()
		env.published = append(env.published, rev)
	})
	return env
}

// newTestEnv is newTestEnvAt at the fixture's fixed instant: the shape every
// Phase 2/3 test already uses.
func newTestEnv(t *testing.T, path string) *testEnv {
	t.Helper()
	return newTestEnvAt(t, path, &testClock{t: testStart})
}

// openEngineAt opens path and constructs an engine at the given instant,
// returning the construction error instead of failing the test — the fail-hard
// matrix asserts on that error, so it cannot go through newTestEnvAt.
func openEngineAt(t *testing.T, path string, now time.Time) (*Engine, error) {
	t.Helper()

	st := openTestStore(t, path)
	return New(Config{
		Store: st,
		Now:   func() time.Time { return now },
		Fatal: func(error) {},
	})
}

// revs returns the revisions the engine has published so far.
func (env *testEnv) revs() []int64 {
	env.mu.Lock()
	defer env.mu.Unlock()
	return append([]int64(nil), env.published...)
}

// requireJoinCode asserts err is a *JoinError carrying want (D-24's codes are
// what the client sees, so the code is the contract, not the message).
func requireJoinCode(t *testing.T, err error, want string) {
	t.Helper()

	var rejected *JoinError
	if !errors.As(err, &rejected) {
		t.Fatalf("error = %v, want a *JoinError with code %q", err, want)
	}
	if rejected.Code != want {
		t.Errorf("rejection code = %q, want %q", rejected.Code, want)
	}
}

// TestJoinResolvesByToken is SESS-01/SESS-02's server side: a first join mints a
// durable identity, and the D-29 token resumes exactly that identity — with the
// nickname on the wire ignored and no revision move, because a resume is a
// presence change, not a game mutation (D-30/D-37).
func TestJoinResolvesByToken(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))

	first, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "  An  "}, nil)
	if err != nil {
		t.Fatalf("first join: %v", err)
	}
	if first.PlayerID == "" {
		t.Fatal("the first join returned an empty player id")
	}
	if first.Resumed {
		t.Error("the first join reported itself as a resume")
	}
	if first.Role != RolePlayer {
		t.Errorf("join result role = %q, want %q", first.Role, RolePlayer)
	}
	if got := len(first.Token); got != 64 {
		t.Errorf("token has %d characters, want 64 (32 crypto/rand bytes, hex)", got)
	}
	if got := env.engine.Rev(); got != 1 {
		t.Errorf("revision after the first join = %d, want 1: creating a player is a persisted mutation (D-35)", got)
	}

	second, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "Bình", Token: first.Token}, nil)
	if err != nil {
		t.Fatalf("resuming with the token: %v", err)
	}
	if second.PlayerID != first.PlayerID {
		t.Errorf("resume player id = %q, want the original %q", second.PlayerID, first.PlayerID)
	}
	if second.Token != first.Token {
		t.Errorf("resume token = %q, want the original token back", second.Token)
	}
	if !second.Resumed {
		t.Error("a token resume reported itself as a new join")
	}
	if got := env.engine.Rev(); got != 1 {
		t.Errorf("revision after a token resume = %d, want 1: presence must not bump rev (D-37)", got)
	}
}

// TestJoinPolicy covers D-27/D-28/D-30's rejection ladder and D-32's nickname
// reservation, and proves a rejected join moves nothing.
func TestJoinPolicyRejectsBadIdentity(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))

	if _, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "An"}, nil); err != nil {
		t.Fatalf("first join: %v", err)
	}

	cases := []struct {
		name     string
		req      JoinRequest
		wantCode string
	}{
		{
			name:     "whitespace-only nickname is invalid",
			req:      JoinRequest{Role: RolePlayer, Nickname: "   "},
			wantCode: CodeNicknameInvalid,
		},
		{
			name:     "21 runes is over the cap",
			req:      JoinRequest{Role: RolePlayer, Nickname: strings.Repeat("a", MaxNicknameRunes+1)},
			wantCode: CodeNicknameInvalid,
		},
		{
			name:     "a case variant of a taken nickname is taken",
			req:      JoinRequest{Role: RolePlayer, Nickname: "AN"},
			wantCode: CodeNicknameTaken,
		},
		{
			name:     "a zero-width variant of a taken nickname is taken",
			req:      JoinRequest{Role: RolePlayer, Nickname: "A\u200Bn"},
			wantCode: CodeNicknameTaken,
		},
		{
			name:     "an unknown token with no nickname is token_invalid",
			req:      JoinRequest{Role: RolePlayer, Token: "0123456789abcdef"},
			wantCode: CodeTokenInvalid,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := env.engine.Join(tc.req, nil)
			requireJoinCode(t, err, tc.wantCode)
		})
	}

	if got := env.engine.Rev(); got != 1 {
		t.Errorf("revision = %d after five rejected joins, want 1: a rejection must not touch the state", got)
	}

	t.Run("an unknown token with a nickname is a new join", func(t *testing.T) {
		// D-30 (2): the client replaces its stale token with the fresh one from
		// hello, so the join succeeds instead of stranding the player.
		res, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "Bình", Token: "stale-token"}, nil)
		if err != nil {
			t.Fatalf("join with a stale token and a free nickname: %v", err)
		}
		if res.Resumed {
			t.Error("a stale token was treated as a resume")
		}
		if res.Token == "stale-token" {
			t.Error("the join handed back the stale token instead of minting a new one")
		}
		if got := env.engine.Rev(); got != 2 {
			t.Errorf("revision = %d after a new join, want 2", got)
		}
	})
}

// TestCommitFailureFailsHard is D-34's contract: a failed commit calls the
// injected fatal hook, publishes nothing and does not advance the in-memory
// state. Production's hook logs and exits; the in-memory state is deliberately
// not rolled back because the process is not expected to survive.
func TestCommitFailureFailsHard(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))

	if _, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "An"}, nil); err != nil {
		t.Fatalf("join before breaking the store: %v", err)
	}
	revBefore := env.engine.Rev()
	publishedBefore := len(env.revs())

	// Break the store the way a dead disk would: the handle is closed, so the
	// next transaction cannot even begin.
	if err := env.store.Close(); err != nil {
		t.Fatalf("closing the store: %v", err)
	}

	applyErr := env.engine.Apply(func(s *state) error {
		s.phase = PhaseItemIntro
		return nil
	})
	if applyErr == nil {
		t.Fatal("Apply returned nil with a closed store; a commit failure must surface")
	}

	select {
	case got := <-env.fatal:
		if !strings.Contains(got.Error(), "commit revision") {
			t.Errorf("the fatal hook received %v; it must name the failed commit so the boot log is actionable", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the injected fatal hook was not called: D-34 requires a failed commit to fail hard")
	}

	if got := len(env.revs()); got != publishedBefore {
		t.Errorf("a failed commit published %d event(s); want none", got-publishedBefore)
	}
	if got := env.engine.Rev(); got != revBefore {
		t.Errorf("revision = %d across a failed commit, want %d", got, revBefore)
	}
	if got := env.engine.Phase(); got != PhaseLobby {
		t.Errorf("phase = %q across a failed commit, want %q: nothing is published before durability", got, PhaseLobby)
	}
}

// TestBootRefusesStateTheMachineCannotUse is D-39's fail-hard half: a persisted
// row that the engine cannot turn into a valid state must abort the boot instead
// of starting a lobby that quietly disagrees with the database.
func TestBootRefusesStateTheMachineCannotUse(t *testing.T) {
	newEngineAt := func(t *testing.T, path string) (*Engine, error) {
		t.Helper()
		return openEngineAt(t, path, testStart)
	}

	t.Run("a phase outside the section 6 machine", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "auction.db")
		st := openTestStore(t, path)
		if err := store.Bootstrap(st.DB, testStart); err != nil {
			t.Fatalf("Bootstrap: %v", err)
		}
		// The store does not validate the phase — that is the machine's job —
		// so writing a bogus one simulates a database edited by hand or by a
		// future phase that forgot to migrate.
		if err := st.Commit(store.Mutation{Phase: "PAUSED", Revision: 1, UpdatedAt: testStart.UnixMilli()}); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		if err := st.Close(); err != nil {
			t.Fatalf("closing: %v", err)
		}

		eng, err := newEngineAt(t, path)
		if err == nil {
			t.Fatalf("New accepted phase %q; want a boot failure (D-39)", eng.Phase())
		}
		if !strings.Contains(err.Error(), "PAUSED") {
			t.Errorf("boot error = %v, want it to name the offending phase", err)
		}
	})

	t.Run("a config_json without a usable budget", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "auction.db")
		st := openTestStore(t, path)
		if err := store.Bootstrap(st.DB, testStart); err != nil {
			t.Fatalf("Bootstrap: %v", err)
		}
		if _, err := st.DB.Exec(`UPDATE game SET config_json = ? WHERE id = 1`, `{"starting_budget":0,"currency":"VND"}`); err != nil {
			t.Fatalf("corrupting the config: %v", err)
		}
		if err := st.Close(); err != nil {
			t.Fatalf("closing: %v", err)
		}

		if _, err := newEngineAt(t, path); err == nil {
			t.Fatal("New accepted a starting_budget of 0; every join would be unplayable")
		}
	})
}

// TestPresenceDoesNotBumpRevision is D-37's contract: presence lives in memory,
// rides the current revision, still fans out, and is a no-op for an unknown id
// so a teardown can never depend on the row still existing.
func TestPresenceDoesNotBumpRevision(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))

	res, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "An"}, nil)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if got := env.engine.Rev(); got != 1 {
		t.Fatalf("revision after join = %d, want 1", got)
	}

	before := len(env.revs())
	if err := env.engine.Disconnect(res.PlayerID); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if err := env.engine.Connect(res.PlayerID); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if got := env.engine.Rev(); got != 1 {
		t.Errorf("revision = %d after a disconnect+connect, want 1: presence is in-memory only (D-37)", got)
	}
	for _, rev := range env.revs()[before:] {
		if rev != 1 {
			t.Errorf("a presence change published revision %d, want the unchanged revision 1", rev)
		}
	}
	if got := len(env.revs()[before:]); got != 2 {
		t.Errorf("two presence changes published %d events, want 2", got)
	}

	for _, id := range []string{"no-such-player", ""} {
		if err := env.engine.Disconnect(id); err != nil {
			t.Errorf("Disconnect(%q) = %v, want a no-op: teardown must not depend on the row existing", id, err)
		}
		if err := env.engine.Connect(id); err != nil {
			t.Errorf("Connect(%q) = %v, want a no-op", id, err)
		}
	}
}

// TestRestartReloadsLobby is D-39's contract end to end: a restarted process
// rebuilds the lobby from the database — the one canonical revision, the phase,
// and the identities (token resume still works), while every player starts the
// new process disconnected. The raw row-by-row round trip is pinned separately
// by internal/store's TestRestartRoundTrip.
func TestRestartReloadsLobby(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")

	env1 := newTestEnv(t, path)
	first, err := env1.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "An"}, nil)
	if err != nil {
		t.Fatalf("first join: %v", err)
	}
	if _, err := env1.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "Bình"}, nil); err != nil {
		t.Fatalf("second join: %v", err)
	}
	if got := env1.engine.Rev(); got != 2 {
		t.Fatalf("revision before restart = %d, want 2", got)
	}
	if err := env1.store.Close(); err != nil {
		t.Fatalf("closing the store before restart: %v", err)
	}

	env2 := newTestEnv(t, path)

	if got := env2.engine.Rev(); got != 2 {
		t.Errorf("revision after restart = %d, want 2: game.revision is the one canonical counter (D-35)", got)
	}
	if got := env2.engine.Phase(); got != PhaseLobby {
		t.Errorf("phase after restart = %q, want %q", got, PhaseLobby)
	}

	resumed, err := env2.engine.Join(JoinRequest{Role: RolePlayer, Token: first.Token}, nil)
	if err != nil {
		t.Fatalf("resuming a persisted token after restart: %v", err)
	}
	if !resumed.Resumed || resumed.PlayerID != first.PlayerID {
		t.Errorf("resume after restart = %+v, want the persisted player %q resumed", resumed, first.PlayerID)
	}
	if got := env2.engine.Rev(); got != 2 {
		t.Errorf("revision after a post-restart resume = %d, want 2", got)
	}

	// The nickname row survived, so the reservation survives too (D-32).
	_, err = env2.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "bình"}, nil)
	requireJoinCode(t, err, CodeNicknameTaken)
}
