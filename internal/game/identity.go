package game

import "errors"

// Package game owns the authoritative game state: the phase machine, the single
// mutation path, presence and the role projections.
//
// RED contract declaration: NormalizeNickname, NicknameKey and NewToken are
// declared here so identity_test.go compiles and fails on its assertions; the
// GREEN commit replaces the bodies (identity.go is the exact D-27/D-28/D-29
// pipeline).

// errPendingImplementation is the RED contract's placeholder.
var errPendingImplementation = errors.New("game: not implemented yet (RED contract)")

// MaxNicknameRunes is the nickname length cap. It counts RUNES, never bytes:
// 20 Vietnamese characters are 40 bytes, so a byte check would reject a name
// the player's own keyboard counted as 20 characters (D-28, research Pitfall 9).
const MaxNicknameRunes = 20

// NormalizeNickname applies the D-27 pipeline in its mandated order (strip
// zero-width/format, collapse every whitespace run, strip residual controls).
// The second return value is false when the result is empty or longer than
// MaxNicknameRunes, and the caller then answers error{code:"nickname_invalid"}.
func NormalizeNickname(raw string) (string, bool) {
	return "", false
}

// NicknameKey is the uniqueness key stored in players.nickname_key (D-28):
// Unicode-aware lowercasing, in Go, because SQLite's NOCASE folds ASCII only.
func NicknameKey(normalized string) string {
	return ""
}

// NewToken mints one player token: 32 crypto/rand bytes, hex-encoded to 64
// lowercase characters (D-29). math/rand is forbidden here (PLAN §3.1 #7).
func NewToken() (string, error) {
	return "", errPendingImplementation
}
