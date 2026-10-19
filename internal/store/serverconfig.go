package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
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
// so the `/admin$1k=…` link printed at boot survives a restart or a crash
// instead of forcing the presenter to copy a new one mid-show.
const adminKeyRow = "admin_key"

// GetServerConfig reads one `server_config` row. found reports whether the row
// exists — a missing row is not an empty value, exactly as LoadRuntime's bool
// distinguishes "never written" from a zero row.
func (s *Store) GetServerConfig(key string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const query = `SELECT value FROM server_config WHERE key = $1`

	var value string
	err := s.DB.QueryRowContext(ctx, query, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, sanitize("server_config read", err)
	}
	return value, true, nil
}

// SetServerConfig writes one `server_config` row, upserting on the primary key
// so re-persisting the same key is a no-op rather than a conflict.
//
// The dedicated leased writer connection serializes this boot write with game
// commits; losing that connection never reacquires ownership for stale memory.
func (s *Store) SetServerConfig(key, value string) error {
	const upsert = `
INSERT INTO server_config (key, value) VALUES ($1, $2)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`

	return sanitize("server_config write", s.withWriter(func(ctx context.Context, c *sql.Conn) error {
		_, err := c.ExecContext(ctx, upsert, key, value)
		return err
	}))
}

// adminKeyMaxLen bounds the capability's length. A browser silently drops a
// cookie whose name+value exceeds its per-cookie budget (~4 KiB), which would
// present as a gate page that never accepts the link the presenter is holding.
const adminKeyMaxLen = 4096

// validAdminKey reports whether a capability can survive a Set-Cookie round
// trip byte for byte — the gate's whole browser story is "a correct ?k= sets the
// capability cookie, then the cookie is the capability".
//
// net/http DROPS the bytes `"`, `;`, `\`, every control byte and every byte
// outside ASCII (logging `invalid byte … in Cookie.Value; dropping invalid
// bytes`), so Equal(cookie.Value, key) can never pass for a key carrying one:
// `GET /admin$1k=<key>` sets a mutated cookie, the 303 lands back on the gate
// page, and the wrong-key 401 never fires because the ?k= itself was right — a
// lockout with no diagnostic. It also QUOTES a value containing a space or a
// comma; browsers unquote on storage, and whether the value survives that trip
// is browser-dependent, so those two bytes are refused as well.
//
// The accepted set is therefore exactly the bytes guaranteed to be written
// unquoted and parsed back identically: printable ASCII 0x21..0x7E except
// `"`, `;`, `\` and `,`. It is a transport rule, not a strength rule — the
// minted default (64 lowercase hex characters) and a human-chosen pin like
// `venue-2026.secret` both pass.
func validAdminKey(key string) bool {
	if key == "" || len(key) > adminKeyMaxLen {
		return false
	}
	for i := 0; i < len(key); i++ {
		b := key[i]
		if b <= 0x20 || b >= 0x7f || b == '"' || b == ';' || b == '\\' || b == ',' {
			return false
		}
	}
	return true
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
// Each rung is checked with validAdminKey before it is served, for the same
// reason: a key the capability cookie cannot carry is a boot that prints a link
// which can never open the gate. Failing here is loud and fixable (pin another
// key, or delete the row); serving it is a presenter locked out mid-show.
//
// getenv and mint are injected so the ladder is testable and so the entropy
// source stays where the rest of the project's crypto/rand lives — the real boot
// passes `os.Getenv` and `game.NewToken` (32 crypto/rand bytes, 64 hex chars).
func (s *Store) ResolveAdminKey(getenv func(string) string, mint func() (string, error)) (string, error) {
	if pin := strings.TrimSpace(getenv(adminKeyEnv)); pin != "" {
		if !validAdminKey(pin) {
			return "", fmt.Errorf("store: %s cannot be served: the capability cookie cannot carry this value (use 1..%d printable ASCII characters, none of them a double quote, semicolon, backslash or comma)", adminKeyEnv, adminKeyMaxLen)
		}
		return pin, nil
	}

	persisted, found, err := s.GetServerConfig(adminKeyRow)
	if err != nil {
		return "", err
	}
	if found && persisted != "" {
		if !validAdminKey(persisted) {
			return "", fmt.Errorf("store: the persisted admin key (server_config %q) cannot survive a capability-cookie round trip: delete the row or pin a valid %s", adminKeyRow, adminKeyEnv)
		}
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
	if !validAdminKey(key) {
		// The minted key is served to the browser ONLY, so a value the cookie
		// cannot carry is never a capability at all.
		return "", errors.New("store: the minted admin key cannot survive a capability-cookie round trip (printable ASCII, no double quote, semicolon, backslash or comma)")
	}
	if err := s.SetServerConfig(adminKeyRow, key); err != nil {
		return "", err
	}
	return key, nil
}
