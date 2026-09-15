package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRestartRoundTrip is criterion 5's persistence half: a mutation is written
// in one transaction and a reopened database returns exactly what was written —
// players, budgets, the phase and the one canonical game revision.
func TestRestartRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	now := time.UnixMilli(1_700_000_000_000)

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	if err := Schema(st.DB); err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if err := Bootstrap(st.DB, now); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// Literals, not values derived from the implementation: the round trip is
	// only meaningful if the expectation was written down before the write.
	players := []PlayerRow{
		{
			ID: "p-an", Nickname: "An", NicknameKey: "an",
			Token:  strings.Repeat("a", 64),
			Budget: 1_000_000, TotalValue: 0,
			JoinedAt: now.UnixMilli(), LastSeenAt: now.UnixMilli(),
		},
		{
			ID: "p-binh", Nickname: "Bình", NicknameKey: "bình",
			Token:  strings.Repeat("b", 64),
			Budget: 750_000, TotalValue: 250_000,
			JoinedAt: now.Add(time.Second).UnixMilli(), LastSeenAt: now.Add(2 * time.Second).UnixMilli(),
		},
	}

	if err := st.Commit(Mutation{
		Phase:     "LOBBY",
		Revision:  2,
		UpdatedAt: now.UnixMilli(),
		Players:   players,
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("closing the first handle: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopening %q: %v", path, err)
	}
	defer func() { _ = reopened.Close() }()

	game, err := reopened.LoadGame()
	if err != nil {
		t.Fatalf("LoadGame after restart: %v", err)
	}
	if game.Phase != "LOBBY" {
		t.Errorf("phase after restart = %q, want %q", game.Phase, "LOBBY")
	}
	if game.Revision != 2 {
		t.Errorf("revision after restart = %d, want 2: game.revision is the one canonical counter (D-35)", game.Revision)
	}

	got, err := reopened.LoadPlayers()
	if err != nil {
		t.Fatalf("LoadPlayers after restart: %v", err)
	}
	if !reflect.DeepEqual(got, players) {
		t.Errorf("players after restart = %+v,\nwant the rows written before the restart %+v", got, players)
	}
}

// TestCommitStressHasNoBusyErrors is research Pitfall 10's acceptance check: the
// single-connection pool serializes concurrent writers, and the DSN's
// busy_timeout is the second line of defence. A SQLITE_BUSY here would be a
// process exit under D-34's fail-hard policy.
func TestCommitStressHasNoBusyErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer func() { _ = st.Close() }()
	if err := Schema(st.DB); err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if err := Bootstrap(st.DB, time.UnixMilli(1_700_000_000_000)); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	rows := []PlayerRow{{
		ID: "p-1", Nickname: "An", NicknameKey: "an", Token: strings.Repeat("c", 64),
		Budget: 1_000_000, JoinedAt: 1_700_000_000_000, LastSeenAt: 1_700_000_000_000,
	}}

	const workers = 20
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := st.Commit(Mutation{
				Phase:     "LOBBY",
				Revision:  int64(i + 1),
				UpdatedAt: int64(i + 1),
				Players:   rows,
			}); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent commit failed: %v", err)
		if strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked") {
			t.Errorf("concurrent writers hit %v; SetMaxOpenConns(1) must serialize them", err)
		}
	}

	game, err := st.LoadGame()
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if game.Revision < 1 || game.Revision > workers {
		t.Errorf("revision = %d after %d commits, want one of the committed values", game.Revision, workers)
	}
}

// TestLoadGameWithoutBootstrapIsAnError pins the boot order: Bootstrap runs
// before the first LoadGame, so a missing row must fail loudly instead of
// handing back a zero-valued game row that looks like LOBBY revision 0.
func TestLoadGameWithoutBootstrapIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer func() { _ = st.Close() }()
	if err := Schema(st.DB); err != nil {
		t.Fatalf("Schema: %v", err)
	}

	_, err = st.LoadGame()
	if err == nil {
		t.Fatal("LoadGame returned a row for a database that was never bootstrapped")
	}
	if !errors.Is(err, sql.ErrNoRows) && !strings.Contains(err.Error(), "game row") {
		t.Errorf("LoadGame error = %v, want it to say the game row is missing", err)
	}
}
