package store

// This file is the parameterized snapshot repository: the only code in the
// project that reads game state out of SQLite or writes a mutation into it.
// DDL and PRAGMA statements live exclusively in schema.go (research Pitfall 12:
// no schema work inside the mutation path).
//
// RED contract declaration: the query bodies land with the GREEN commit, which
// is what makes TestRestartRoundTrip and TestCommitStressHaveNoBusyErrors fail
// first.

// GameRow is the single row of the `game` table (PLAN.md §5, id = 1).
type GameRow struct {
	Phase         string // wire phase name, e.g. "LOBBY"
	CurrentItemID string // empty when no item is current
	Revision      int64  // the one canonical game revision (D-35)
	ConfigJSON    string // starting_budget, currency, …
	UpdatedAt     int64  // server clock, milliseconds
}

// PlayerRow is one row of the `players` table (PLAN.md §5).
type PlayerRow struct {
	ID          string
	Nickname    string // display form, normalized (D-27), case preserved
	NicknameKey string // strings.ToLower(Nickname), the UNIQUE key (D-28)
	Token       string // 64 lowercase hex chars, crypto/rand (D-29)
	Budget      int64
	TotalValue  int64
	JoinedAt    int64
	LastSeenAt  int64
}

// Mutation is the write set of one engine mutation: the game row plus every
// player row, committed in ONE transaction together with game.revision (D-34).
// Phase 3 extends it additively with item and bid write sets.
type Mutation struct {
	Phase         string
	CurrentItemID string
	Revision      int64
	UpdatedAt     int64
	Players       []PlayerRow
}

// LoadGame returns the `game` row. A missing row is an error: boot is
// Bootstrap → LoadGame, so a missing row means the bootstrap did not run.
func (s *Store) LoadGame() (GameRow, error) {
	return GameRow{}, errPendingImplementation
}

// LoadPlayers returns every player row in join order (joined_at, then id so the
// order is total).
func (s *Store) LoadPlayers() ([]PlayerRow, error) {
	return nil, errPendingImplementation
}

// Commit writes one mutation in a single transaction. It is parameterized
// throughout (no string-built SQL) and relies on the `players.nickname_key`
// UNIQUE index for the case-insensitive nickname guarantee (D-28).
func (s *Store) Commit(m Mutation) error {
	return errPendingImplementation
}

// IsUniqueViolation reports whether err is a SQLite UNIQUE-constraint failure,
// by numeric code (2067) and never by message text, so "nickname taken" keeps
// working across driver upgrades (research §4).
func IsUniqueViolation(err error) bool {
	return false
}
