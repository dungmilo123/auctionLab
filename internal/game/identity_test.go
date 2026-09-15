package game

import (
	"strings"
	"testing"
)

// TestNormalizeNickname is D-27's contract, row by row. The expected values are
// literals on purpose: the point of the table is that a later "simplification"
// of the pipeline — reordering the steps, dropping the Unicode-aware fold, or
// switching the cap to bytes — cannot pass review.
//
// The two rows that distinguish the step ORDER are "A\tB" → "A B" (tab becomes
// a space because whitespace is collapsed before controls are dropped) and
// "A\x00B" → "AB" (NUL is a control that is deleted, not spaced). Implementing
// the steps in the other order turns the first row into "AB" and both rows
// would still "look" sanitized (research Pitfall 8).
func TestNormalizeNickname(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{name: "plain name is kept verbatim", in: "An", want: "An", wantOK: true},
		{name: "surrounding whitespace is trimmed", in: "  An  ", want: "An", wantOK: true},
		{name: "display case is preserved", in: "AN", want: "AN", wantOK: true},
		{name: "lowercase is preserved", in: "an", want: "an", wantOK: true},
		{name: "zero-width space is removed", in: "A\u200Bn", want: "An", wantOK: true},
		{name: "ZWNJ, ZWJ and BOM are removed, the space survives", in: "An\u200C\u200D\uFEFF x", want: "An x", wantOK: true},
		{name: "word joiner is removed", in: "\u2060X\u2060", want: "X", wantOK: true},
		{name: "a lone zero-width space normalizes to empty", in: "\u200B", want: "", wantOK: false},
		{name: "whitespace only is rejected", in: "   ", want: "", wantOK: false},
		{name: "empty is rejected", in: "", want: "", wantOK: false},
		{name: "a tab collapses to one space", in: "A\tB", want: "A B", wantOK: true},
		{name: "a newline collapses to one space", in: "A\nB", want: "A B", wantOK: true},
		{name: "NBSP collapses to one space", in: "A\u00A0B", want: "A B", wantOK: true},
		{name: "ideographic space collapses to one space", in: "A\u3000B", want: "A B", wantOK: true},
		{name: "NEL is whitespace and collapses", in: "a\u0085b", want: "a b", wantOK: true},
		{name: "NUL is deleted, not spaced", in: "A\x00B", want: "AB", wantOK: true},
		{name: "DEL is deleted, not spaced", in: "A\x7fB", want: "AB", wantOK: true},
		{name: "an interior whitespace run collapses to one space", in: "Bạn    tên   gì", want: "Bạn tên gì", wantOK: true},
		{name: "Vietnamese uppercase keeps its display form", in: "ĐĂNG", want: "ĐĂNG", wantOK: true},
		{name: "Vietnamese lowercase keeps its display form", in: "đăng", want: "đăng", wantOK: true},
		{name: "a full Vietnamese name is untouched", in: "Nguyễn Văn A", want: "Nguyễn Văn A", wantOK: true},
		{name: "an all-caps Vietnamese name is untouched", in: "NGUYỄN VĂN A", want: "NGUYỄN VĂN A", wantOK: true},
		{name: "a doubled interior space collapses", in: "Nguyễn  Văn  A", want: "Nguyễn Văn A", wantOK: true},
		{name: "20 ASCII runes is the inclusive boundary", in: strings.Repeat("a", 20), want: strings.Repeat("a", 20), wantOK: true},
		{name: "21 ASCII runes is rejected", in: strings.Repeat("a", 21), want: strings.Repeat("a", 21), wantOK: false},
		{name: "20 multi-byte runes is valid", in: strings.Repeat("ă", 20), want: strings.Repeat("ă", 20), wantOK: true},
		{name: "21 multi-byte runes is rejected (42 bytes, 21 runes)", in: strings.Repeat("ă", 21), want: strings.Repeat("ă", 21), wantOK: false},
		{name: "zero-width padding does not spend the rune budget", in: "  \u200B  ", want: "", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NormalizeNickname(tc.in)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("NormalizeNickname(%q) = (%q, %t), want (%q, %t)", tc.in, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestNicknameKeyIsTheUniquenessKey is D-28's collision contract: names that
// differ only in case, spacing or invisible characters are ONE player, and the
// key is what the UNIQUE index decides on.
func TestNicknameKeyIsTheUniquenessKey(t *testing.T) {
	t.Run("case variants collide", func(t *testing.T) {
		keys := make(map[string]string)
		for _, raw := range []string{"An", "an", "AN", "A\u200Bn", "  An  "} {
			normalized, ok := NormalizeNickname(raw)
			if !ok {
				t.Fatalf("NormalizeNickname(%q) rejected a valid name", raw)
			}
			keys[NicknameKey(normalized)] = raw
		}
		if len(keys) != 1 {
			t.Errorf("%q produced %d uniqueness keys, want 1: every spelling of the same name must collide", "An/an/AN", len(keys))
		}
		if _, ok := keys["an"]; !ok {
			t.Errorf("the uniqueness key is %v, want the folded form %q", keys, "an")
		}
	})

	t.Run("Vietnamese case folding is Unicode-aware", func(t *testing.T) {
		// SQLite's NOCASE folds ASCII only, which is exactly why the key is
		// computed in Go (D-28): these three must be one player.
		want := NicknameKey("đăng")
		for _, variant := range []string{"ĐĂNG", "Đăng", "đăng"} {
			if got := NicknameKey(variant); got != want {
				t.Errorf("NicknameKey(%q) = %q, want %q", variant, got, want)
			}
		}
	})

	t.Run("different names stay distinct", func(t *testing.T) {
		pairs := [][2]string{
			{"An", "An B"},
			{"An", "Ân"},
			{"Bình", "Binh"},
		}
		for _, pair := range pairs {
			if NicknameKey(pair[0]) == NicknameKey(pair[1]) {
				t.Errorf("NicknameKey(%q) == NicknameKey(%q); distinct names must not collide", pair[0], pair[1])
			}
		}
	})
}

// TestTokenShape pins D-29: 32 crypto/rand bytes, hex-encoded to 64 lowercase
// characters. A shorter token, an uppercase one, or one that repeats across
// calls all fail here.
func TestTokenShape(t *testing.T) {
	const wantLen = 64

	seen := make(map[string]bool)
	for i := 0; i < 8; i++ {
		token, err := NewToken()
		if err != nil {
			t.Fatalf("NewToken: %v", err)
		}
		if len(token) != wantLen {
			t.Fatalf("token %q has %d characters, want %d (32 bytes hex-encoded)", token, len(token), wantLen)
		}
		if token != strings.ToLower(token) {
			t.Fatalf("token %q is not lowercase hex", token)
		}
		for _, r := range token {
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
				t.Fatalf("token %q contains %q, which is not a lowercase hex digit", token, r)
			}
		}
		if seen[token] {
			t.Fatalf("NewToken returned %q twice: the generator is not random", token)
		}
		seen[token] = true
	}
}

// TestProtocolLiteralsMatchTheHubCatalog pins the literals this package
// duplicates from internal/hub/protocol.go. game cannot import hub (hub imports
// game), so the duplication is deliberate — and it is safe because both sides
// pin the same literal strings in their own tests: renaming either side fails a
// test instead of silently moving the wire protocol (D-24).
func TestProtocolLiteralsMatchTheHubCatalog(t *testing.T) {
	roles := map[Role]string{RolePlayer: "player", RoleScreen: "screen", RoleAdmin: "admin"}
	for got, want := range roles {
		if string(got) != want {
			t.Errorf("role %q, want %q (hub.Role*)", got, want)
		}
	}

	codes := map[string]string{
		CodeNicknameInvalid: "nickname_invalid",
		CodeNicknameTaken:   "nickname_taken",
		CodeTokenInvalid:    "token_invalid",
	}
	for got, want := range codes {
		if got != want {
			t.Errorf("rejection code %q, want %q (hub.Code*)", got, want)
		}
	}
}
