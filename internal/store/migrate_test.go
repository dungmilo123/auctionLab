package store

import (
	"auctionlab/internal/testpg"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func isLowerHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// TestMigrationV2ToV3 is D-74's contract: a populated v2 file migrates once to
// the current UserVersion (the 2→3 step plus every later additive step), the row
// the file already had survives, and a key written into the new table is still
// there after a real restart (a new handle on the same file with Schema run
// again — the boot a crash would perform).
func TestServerConfig(t *testing.T) {
	newStore := func(t *testing.T) (*Store, string) {
		t.Helper()

		path := filepath.Join(t.TempDir(), "auction.db")
		st, err := Open(testpg.URL(t, path))
		if err != nil {
			t.Fatalf("Open(%q): %v", path, err)
		}
		t.Cleanup(func() { _ = st.Close() })
		if err := st.Migrate(); err != nil {
			t.Fatalf("Schema: %v", err)
		}
		return st, path
	}

	const (
		persistedKey = "1111111111111111111111111111111111111111111111111111111111111111"
		envKey       = "2222222222222222222222222222222222222222222222222222222222222222"
		mintedKey    = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	)
	noEnv := func(string) string { return "" }
	mintMustNotRun := func() (string, error) {
		return "", errors.New("the mint must not run when a higher rung answered")
	}

	t.Run("an absent row is not found, never an empty value", func(t *testing.T) {
		st, _ := newStore(t)

		value, found, err := st.GetServerConfig(adminKeyRow)
		if err != nil {
			t.Fatalf("GetServerConfig: %v", err)
		}
		if found {
			t.Errorf("found = true for a row that was never written (value %q)", value)
		}
	})

	t.Run("a stored row round-trips and overwrites in place", func(t *testing.T) {
		st, _ := newStore(t)

		if err := st.SetServerConfig(adminKeyRow, persistedKey); err != nil {
			t.Fatalf("SetServerConfig: %v", err)
		}
		value, found, err := st.GetServerConfig(adminKeyRow)
		if err != nil {
			t.Fatalf("GetServerConfig: %v", err)
		}
		if !found || value != persistedKey {
			t.Fatalf("after Set: found = %t, value = %q, want true and %q", found, value, persistedKey)
		}

		if err := st.SetServerConfig(adminKeyRow, mintedKey); err != nil {
			t.Fatalf("SetServerConfig (overwrite): %v", err)
		}
		value, found, err = st.GetServerConfig(adminKeyRow)
		if err != nil {
			t.Fatalf("GetServerConfig (after overwrite): %v", err)
		}
		if !found || value != mintedKey {
			t.Errorf("after overwrite: found = %t, value = %q, want true and %q", found, value, mintedKey)
		}

		var n int
		if err := st.DB.QueryRow(`SELECT COUNT(*) FROM server_config`).Scan(&n); err != nil {
			t.Fatalf("counting server_config rows: %v", err)
		}
		if n != 1 {
			t.Errorf("server_config carries %d rows after an overwrite, want the 1 upserted row", n)
		}
	})

	t.Run("the env pin wins over the persisted row", func(t *testing.T) {
		st, _ := newStore(t)
		if err := st.SetServerConfig(adminKeyRow, persistedKey); err != nil {
			t.Fatalf("seeding the persisted row: %v", err)
		}

		key, err := st.ResolveAdminKey(func(name string) string {
			if name != adminKeyEnv {
				t.Errorf("getenv(%q), want %q", name, adminKeyEnv)
			}
			return envKey
		}, mintMustNotRun)
		if err != nil {
			t.Fatalf("ResolveAdminKey: %v", err)
		}
		if key != envKey {
			t.Errorf("key = %q, want the env pin %q", key, envKey)
		}

		// An env pin does not overwrite the durable row: dropping the env var on
		// the next boot must fall back to the same key, not to a fresh one.
		value, found, err := st.GetServerConfig(adminKeyRow)
		if err != nil {
			t.Fatalf("GetServerConfig: %v", err)
		}
		if !found || value != persistedKey {
			t.Errorf("persisted row = (%t, %q) after an env-pinned boot, want it untouched as (%q)", found, value, persistedKey)
		}
	})

	t.Run("an env pin arrives trimmed", func(t *testing.T) {
		st, _ := newStore(t)

		// A shell export or a .env line can carry trailing whitespace; the
		// printed /admin?k=… link is copy-pasted, so a stray newline would
		// produce a link the gate refuses.
		key, err := st.ResolveAdminKey(func(string) string { return "  " + envKey + "\n" }, mintMustNotRun)
		if err != nil {
			t.Fatalf("ResolveAdminKey: %v", err)
		}
		if key != envKey {
			t.Errorf("key = %q, want the whitespace-trimmed env pin %q", key, envKey)
		}
	})

	t.Run("a blank env falls through to the persisted row", func(t *testing.T) {
		st, _ := newStore(t)
		if err := st.SetServerConfig(adminKeyRow, persistedKey); err != nil {
			t.Fatalf("seeding the persisted row: %v", err)
		}

		key, err := st.ResolveAdminKey(func(string) string { return "   " }, mintMustNotRun)
		if err != nil {
			t.Fatalf("ResolveAdminKey: %v", err)
		}
		if key != persistedKey {
			t.Errorf("key = %q, want the persisted %q", key, persistedKey)
		}
	})

	t.Run("an empty persisted row is absent, not the key", func(t *testing.T) {
		st, _ := newStore(t)
		if err := st.SetServerConfig(adminKeyRow, ""); err != nil {
			t.Fatalf("seeding the empty row: %v", err)
		}

		// A row that lost its value must not authorize an empty key; the
		// resolver heals by minting rather than serving "".
		key, err := st.ResolveAdminKey(noEnv, func() (string, error) { return mintedKey, nil })
		if err != nil {
			t.Fatalf("ResolveAdminKey: %v", err)
		}
		if key != mintedKey {
			t.Errorf("key = %q, want a freshly minted key %q", key, mintedKey)
		}
	})

	t.Run("no env and no row mints and persists before returning", func(t *testing.T) {
		st, path := newStore(t)

		key, err := st.ResolveAdminKey(noEnv, func() (string, error) { return mintedKey, nil })
		if err != nil {
			t.Fatalf("ResolveAdminKey: %v", err)
		}
		if key != mintedKey {
			t.Errorf("key = %q, want the minted %q", key, mintedKey)
		}
		if len(key) != 64 || !isLowerHex(key) {
			t.Errorf("minted key %q is not 64 lowercase hex characters (the 32 crypto/rand bytes D-74 pins)", key)
		}

		// Durable BEFORE it is served: a fresh handle on the same file — the
		// boot a restart performs — reads the same key back.
		if err := st.Close(); err != nil {
			t.Fatalf("closing the store: %v", err)
		}
		reopened, err := Open(testpg.URL(t, path))
		if err != nil {
			t.Fatalf("reopening the file: %v", err)
		}
		t.Cleanup(func() { _ = reopened.Close() })

		got, found, err := reopened.GetServerConfig(adminKeyRow)
		if err != nil {
			t.Fatalf("GetServerConfig after reopen: %v", err)
		}
		if !found || got != mintedKey {
			t.Errorf("the minted key was not persisted before it was returned: found = %t, value = %q", found, got)
		}
	})

	t.Run("a mint failure is an error and nothing is persisted", func(t *testing.T) {
		st, _ := newStore(t)

		_, err := st.ResolveAdminKey(noEnv, func() (string, error) { return "", errors.New("entropy exhausted") })
		if err == nil {
			t.Fatal("ResolveAdminKey returned no error when the mint failed")
		}
		if !strings.Contains(err.Error(), "entropy exhausted") {
			t.Errorf("error = %v, want it to carry the mint's own failure", err)
		}
		if _, found, gerr := st.GetServerConfig(adminKeyRow); gerr != nil {
			t.Fatalf("GetServerConfig: %v", gerr)
		} else if found {
			t.Error("a key row was persisted although the mint failed")
		}
	})

	t.Run("an empty mint is refused, never served as the key", func(t *testing.T) {
		st, _ := newStore(t)

		_, err := st.ResolveAdminKey(noEnv, func() (string, error) { return "", nil })
		if err == nil {
			t.Fatal("ResolveAdminKey served an empty key — an empty cookie would satisfy Equal")
		}
		if _, found, gerr := st.GetServerConfig(adminKeyRow); gerr != nil {
			t.Fatalf("GetServerConfig: %v", gerr)
		} else if found {
			t.Error("an empty key was persisted")
		}
	})

	t.Run("a closed store is an error, never a silent re-mint", func(t *testing.T) {
		st, _ := newStore(t)
		if err := st.Close(); err != nil {
			t.Fatalf("closing the store: %v", err)
		}

		// The row cannot be read (or written) on a dead handle: booting on
		// anything but an error here would mint a key the file does not hold.
		if _, err := st.ResolveAdminKey(noEnv, func() (string, error) { return mintedKey, nil }); err == nil {
			t.Fatal("ResolveAdminKey answered from a closed store instead of failing the boot")
		}
	})

	t.Run("a write failure is an error, never an in-memory key", func(t *testing.T) {
		st, _ := newStore(t)

		// MaxOpenConns(1) puts this pragma on the one connection the resolver
		// uses: the row read still works, the INSERT cannot land.
		if _, err := st.writer.ExecContext(context.Background(), "SET default_transaction_read_only = on"); err != nil {
			t.Fatalf("setting query_only: %v", err)
		}

		_, err := st.ResolveAdminKey(noEnv, func() (string, error) { return mintedKey, nil })
		if err == nil {
			t.Fatal("ResolveAdminKey served a key the write could not persist — a restart would invalidate the link")
		}
		if !strings.Contains(err.Error(), "server_config") {
			t.Errorf("error = %v, want it to name the row that could not be written", err)
		}
	})

	// D-75's browser story is "a correct ?k= sets the capability cookie, then the
	// cookie is the capability". net/http DROPS the invalid bytes `"`, `;`, `\`,
	// every control byte and every non-ASCII byte from a cookie value (and quotes
	// a value with a space or comma), so a key carrying one sets a mutated cookie
	// and the gate can never open again — silently, because the ?k= was correct.
	// The boot must refuse such a key instead of printing a link that cannot work.
	t.Run("an env pin the capability cookie cannot carry fails the boot", func(t *testing.T) {
		st, _ := newStore(t)

		for _, pin := range []string{
			"has;semicolon",
			`has"quote`,
			`has\backslash`,
			"has,comma", // quoted by Set-Cookie; survival is browser-dependent
			"has space", // the same, and outside the guaranteed byte set
			"tab\tkey",
			"khoá-non-ascii",
			"newline\nkey",
			strings.Repeat("x", adminKeyMaxLen+1),
		} {
			if _, err := st.ResolveAdminKey(func(string) string { return pin }, mintMustNotRun); err == nil {
				t.Errorf("ResolveAdminKey served %q: its cookie value would be mutated or dropped, locking the presenter out", pin)
			}
		}
	})

	t.Run("a persisted row the capability cookie cannot carry fails the boot", func(t *testing.T) {
		st, _ := newStore(t)
		if err := st.SetServerConfig(adminKeyRow, "has;semicolon"); err != nil {
			t.Fatalf("seeding the row: %v", err)
		}

		// The row outranks minting, so the failure has to name the row: the
		// operator's fix is to delete it or pin a valid ADMIN_KEY.
		_, err := st.ResolveAdminKey(noEnv, mintMustNotRun)
		if err == nil {
			t.Fatal("ResolveAdminKey served a persisted key whose cookie value net/http mutates")
		}
		if !strings.Contains(err.Error(), adminKeyRow) {
			t.Errorf("error = %v, want it to name the %q row that has to go", err, adminKeyRow)
		}
	})

	t.Run("a minted key the capability cookie cannot carry fails the boot", func(t *testing.T) {
		st, _ := newStore(t)

		_, err := st.ResolveAdminKey(noEnv, func() (string, error) { return "minted;with-semicolon", nil })
		if err == nil {
			t.Fatal("ResolveAdminKey served a minted key whose cookie value net/http mutates")
		}
		if _, found, gerr := st.GetServerConfig(adminKeyRow); gerr != nil {
			t.Fatalf("GetServerConfig: %v", gerr)
		} else if found {
			t.Error("a key the capability cookie cannot carry was persisted")
		}
	})

	t.Run("a human-chosen pin inside the guaranteed byte set is served", func(t *testing.T) {
		st, _ := newStore(t)

		// The rule is transport, not strength: any printable-ASCII value without
		// `"` `;` `\` `,` reaches the browser unchanged and must be served.
		const pin = "venue-2026.secret~7"
		key, err := st.ResolveAdminKey(func(string) string { return pin }, mintMustNotRun)
		if err != nil {
			t.Fatalf("ResolveAdminKey refused a cookie-safe pin: %v", err)
		}
		if key != pin {
			t.Errorf("key = %q, want the pinned %q", key, pin)
		}
	})
}
