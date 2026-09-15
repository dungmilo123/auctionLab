package store

import (
	"database/sql"
	"errors"
	"time"
)

// errPendingImplementation is the RED contract's placeholder. Go cannot compile
// a test that names a package member which does not exist yet, so this commit
// declares the schema/load/commit API the tests bind to and the GREEN commit
// replaces every returning statement with the real implementation.
var errPendingImplementation = errors.New("store: not implemented yet (RED contract)")

// UserVersion is D-36's schema version. The bootstrap is one idempotent DDL
// pass plus this pragma — deliberately no migration framework, because the
// Phase 2 schema is the complete PLAN.md §5 schema and a demo database is
// disposable (delete auction.db to reset).
const UserVersion = 1

// Schema applies the complete PLAN.md §5 schema (D-36). It must be idempotent:
// a second boot against an existing file changes nothing and still reports
// user_version 1.
//
// RED contract declaration: the DDL body lands with the GREEN commit, which is
// what makes TestSchema fail first.
func Schema(db *sql.DB) error {
	return errPendingImplementation
}

// Bootstrap inserts the single `game` row if it does not exist (D-36:
// phase LOBBY, revision 0, config_json {"starting_budget":1000000,"currency":"VND"}).
//
// RED contract declaration: implemented in the GREEN commit so TestSchema's
// boot-row case fails first.
func Bootstrap(db *sql.DB, now time.Time) error {
	return errPendingImplementation
}
