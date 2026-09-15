package game

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file is identity: D-27's normalization pipeline, D-28's uniqueness key
// and D-29's token minting. All three are pure — no clock, no store, no lock —
// so they are the one part of the package a test can reason about exhaustively.

// MaxNicknameRunes is the nickname length cap. It counts RUNES, never bytes:
// 20 Vietnamese characters are 40 bytes, so a byte check would reject a name
// the player's own keyboard counted as 20 characters (D-28, research Pitfall 9).
const MaxNicknameRunes = 20

// tokenBytes is D-29's entropy: 32 crypto/rand bytes, hex-encoded to the 64
// lowercase characters a client stores in localStorage.
const tokenBytes = 32

// playerIDBytes is the player id's entropy. Ids are not secrets in the same
// sense as a token (a snapshot carries its own id), but they must not be
// guessable: a predictable id would let anyone forge presence or, later, bid as
// someone else.
const playerIDBytes = 16

// itemIDBytes is the item id's entropy, the same size and source as a player id
// (D-66/H14): a server-minted id must not be predictable, and it must never come
// from math/rand (the repo-wide rule, and gorilla's own CVE).
const itemIDBytes = 16

// zeroWidth is D-27's explicit set. The Cf category is checked as well so an
// unlisted format character cannot slip through, but this named set is what the
// policy promises.
var zeroWidth = map[rune]bool{
	'\u200B': true, // ZERO WIDTH SPACE
	'\u200C': true, // ZERO WIDTH NON-JOINER
	'\u200D': true, // ZERO WIDTH JOINER
	'\uFEFF': true, // ZERO WIDTH NO-BREAK SPACE / BOM
	'\u2060': true, // WORD JOINER
}

// dropZeroWidth is step 1 of the pipeline. strings.Map deletes a rune when the
// mapping returns a negative value — the signature is func(rune) rune, NOT
// func(rune) bool.
func dropZeroWidth(r rune) rune {
	if zeroWidth[r] || (unicode.In(r, unicode.Cf) && !unicode.IsSpace(r)) {
		return -1
	}
	return r
}

// dropControl is step 3. unicode.IsControl covers C0/C1/DEL; whitespace
// controls (tab, newline, U+0085 NEL) are already a plain space by now and must
// survive here, which is why the IsSpace guard is part of the step.
func dropControl(r rune) rune {
	if unicode.IsControl(r) && !unicode.IsSpace(r) {
		return -1
	}
	return r
}

// NormalizeNickname applies the D-27 pipeline in its mandated order:
//
//  1. strip zero-width and other format characters
//  2. collapse every whitespace run to one space, trimming the ends
//  3. strip the remaining control characters
//
// The order is part of the contract, not an implementation detail: collapsing
// first would turn "A\tB" into "AB" instead of "A B" (research Pitfall 8).
// strings.Fields in step 2 splits on every Unicode space — NBSP, ideographic
// space, U+2028/29 — so an invisible byte cannot survive as a separate name.
//
// The second return value is false when the result is empty or longer than
// MaxNicknameRunes; the caller answers error{code:"nickname_invalid"} with a
// reason (D-27/D-28).
func NormalizeNickname(raw string) (string, bool) {
	s := strings.Map(dropZeroWidth, raw)
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Map(dropControl, s)
	if s == "" || utf8.RuneCountInString(s) > MaxNicknameRunes {
		return s, false
	}
	return s, true
}

// NicknameKey is the uniqueness key stored in players.nickname_key (D-28):
// Unicode-aware lowercasing, computed in Go. SQLite's NOCASE and lower() fold
// ASCII only, so "ĐĂNG" and "đăng" would be two rows under a NOCASE index —
// which is the whole reason this key exists.
func NicknameKey(normalized string) string { return strings.ToLower(normalized) }

// NewToken mints one player token: 32 crypto/rand bytes, hex-encoded to 64
// lowercase characters (D-29). A non-crypto source is forbidden anywhere near
// this (PLAN §3.1 #7 — gorilla's own CVE was exactly that bug).
func NewToken() (string, error) { return randomHex(tokenBytes) }

// newPlayerID mints a player id from crypto/rand (see playerIDBytes).
func newPlayerID() (string, error) { return randomHex(playerIDBytes) }

// newItemID mints an item id from crypto/rand (see itemIDBytes): the create arm
// of `admin.item.upsert` has no client-chosen key (H14).
func newItemID() (string, error) { return randomHex(itemIDBytes) }

// randomHex returns n cryptographically random bytes as lowercase hex.
func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("game: reading %d random bytes: %w", n, err)
	}
	return hex.EncodeToString(buf), nil
}
