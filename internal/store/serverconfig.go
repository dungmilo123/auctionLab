package store

// This file is D-74's persistence half: the additive `server_config` KV table's
// narrow interface, and the one boot resolver that decides which admin
// capability this process serves for the whole show.
//
// RED PHASE — the vocabulary below is what migrate_test.go compiles against; the
// row read/write and the precedence ladder land in the GREEN commit, so every
// test that touches a value fails on its own assertion first (the 04-01
// `Reset`/`ReleaseSession` stubs are the precedent for this shape).

// adminKeyEnv is the operator's pin (D-74): when it is set, the process serves
// exactly that key and never reads, writes or mints anything.
const adminKeyEnv = "ADMIN_KEY"

// adminKeyRow is the `server_config` row the generated key is persisted into,
// so the `/admin?k=…` link printed at boot survives a restart or a crash.
const adminKeyRow = "admin_key"

// GetServerConfig reads one `server_config` row. found reports whether the row
// exists — a missing row is not an empty value, exactly as LoadRuntime's bool
// distinguishes "never written" from a zero row.
//
// RED PHASE: reports every row as absent.
func (s *Store) GetServerConfig(key string) (string, bool, error) {
	return "", false, nil
}

// SetServerConfig writes one `server_config` row, upserting on the primary key
// so re-persisting the same key is a no-op rather than a conflict.
//
// RED PHASE: writes nothing.
func (s *Store) SetServerConfig(key, value string) error {
	return nil
}

// ResolveAdminKey returns the admin capability this boot serves, in D-74's
// precedence: the ADMIN_KEY environment pin, then the persisted row, then a
// freshly minted key that is written to the row BEFORE it is returned. The
// injectable getenv/mint keep the ladder testable and keep the entropy source
// (`game.NewToken` in the real boot) outside this package.
//
// RED PHASE: returns an empty key and never persists anything.
func (s *Store) ResolveAdminKey(getenv func(string) string, mint func() (string, error)) (string, error) {
	return "", nil
}
