package api

import "net/http"

// This file is D-75/D-77's admin capability gate: the project's only server-side
// authorization. It wraps the embedded SPA handler for /admin and everything
// under it, so the admin document is unreachable without the capability cookie,
// and it renders its own offline page — no JavaScript, no external asset, no
// auth endpoint — when the key is missing or wrong.
//
// RED PHASE — AdminGate and Equal are stubs: the gate hands every request
// straight to the wrapped handler, and Equal reports every comparison unequal, so
// admin_gate_test.go compiles and fails on its own assertions (the 04-01
// precedent for this shape).

// AdminGate returns the handler for GET /admin and the /admin/ subtree.
//
// RED PHASE: the request is passed through ungated.
func AdminGate(key string, next http.Handler) http.Handler {
	return next
}

// Equal is the one constant-time comparison the phase has: both sides are hashed
// to a fixed width before crypto/subtle compares them.
//
// RED PHASE: every comparison is unequal.
func Equal(supplied, expected string) bool {
	return false
}
