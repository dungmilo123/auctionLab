package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// This file is D-74's persistence half: the additive `server_config` KV table's
// narrow interface, and the one boot resolver that decides which admin
// capability this process serves for the whole show. It is deliberately the only
// reader/writer of that table — the key must never reach game state, a
// projection or `game.config_json`, and a single narrow pair of methods is what
// keeps that true (ADMN-04, T-04-01).

// adminKeyEnv is the operator's pin (D-74): when it is set, the process serves
// exactly that key for the whole run and never reads, writes or mints anything.
// Rotation is therefore an env change plus a restart — the documented v1 path.
const adminKeyEnv = "ADMIN_KEY"

// adminKeyRow is the `server_config` row the generated key is persisted into,
// so the `/admin?k=…` link printed at boot survives a restart or a crash
// instead of forcing the presenter to copy a new one mid-show.
const adminKeyRow = "admin_key"

// GetServerConfig reads one `server_config` row. found reports whether the row
// exists — a missing row is not an empty value, exactly as LoadRuntime's bool
// distinguishes "never written" from a zero row.
func (s *Store) GetServerConfig(key string) (string, bool, error) {
	const query = `SELECT value FROM server_config WHERE key = ?`

	var value string
	err := s.DB.QueryRow(query, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: loading server_config %q: %w", key, err)
	}
	return value, true, nil
}

// SetServerConfig writes one `server_config` row, upserting on the primary key
// so re-persisting the same key is a no-op rather than a conflict.
//
// It is one statement and no transaction of its own: D-74 persists the key
// outside any game mutation, and the single pooled connection (store.Open) is
// what serializes it against the engine's commits.
func (s *Store) SetServerConfig(key, value string) error {
	const upsert = `
INSERT INTO server_config (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`

	if _, err := s.DB.Exec(upsert, key, value); err != nil {
		return fmt.Errorf("store: persisting server_config %q: %w", key, err)
	}
	return nil
}

// ResolveAdminKey returns the admin capability this boot serves, in D-74's
// precedence:
//
//  1. the ADMIN_KEY environment pin (trimmed — a shell export or a .env line can
//     carry whitespace, and the printed link is copy-pasted);
//  2. the persisted `admin_key` row, so a restart keeps the link the presenter
//     already has;
//  3. a freshly minted key, written to the row BEFORE it is returned, so the
//     capability is durable before any request can depend on it.
//
// Every failure is an error and stops the boot: reading a row that exists but
// cannot be read, minting without entropy, or failing to persist a fresh key
// must never degrade into "serve something". A capability that silently differs
// from the one on disk invalidates the link mid-show, which is the exact failure
// this precedence exists to prevent.
//
// getenv and mint are injected so the ladder is testable and so the entropy
// source stays where the rest of the project's crypto/rand lives — the real boot
// passes `os.Getenv` and `game.NewToken` (32 crypto/rand bytes, 64 hex chars).
func (s *Store) ResolveAdminKey(getenv func(string) string, mint func() (string, error)) (string, error) {
	if pin := strings.TrimSpace(getenv(adminKeyEnv)); pin != "" {
		return pin, nil
	}

	persisted, found, err := s.GetServerConfig(adminKeyRow)
	if err != nil {
		return "", err
	}
	if found && persisted != "" {
		return persisted, nil
	}

	key, err := mint()
	if err != nil {
		return "", fmt.Errorf("store: minting the admin key: %w", err)
	}
	if key == "" {
		// An empty key would authorize an empty cookie (or empty ?k=) at the
		// gate; refuse it here, where the cause is still visible.
		return "", errors.New("store: the admin key minter returned an empty key")
	}
	if err := s.SetServerConfig(adminKeyRow, key); err != nil {
		return "", err
	}
	return key, nil
}
