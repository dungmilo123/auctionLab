package main

import (
	"net/http"
	"testing"
)

// TestNewServerTimeouts pins the slowloris hardening (WR-03 / gosec G112) at
// the constructor the binary actually uses: header reads and idle keep-alives
// are bounded, while ReadTimeout/WriteTimeout stay unset because they span the
// whole connection — which would terminate the long-lived /ws upgrade.
func TestNewServerTimeouts(t *testing.T) {
	srv := newServer(":0", http.NotFoundHandler())

	if srv.ReadHeaderTimeout <= 0 {
		t.Error("ReadHeaderTimeout is unset; a dribbling client can hold a connection slot indefinitely")
	}
	if srv.IdleTimeout <= 0 {
		t.Error("IdleTimeout is unset; an idle keep-alive connection is never reclaimed")
	}
	if srv.ReadTimeout != 0 {
		t.Errorf("ReadTimeout = %v, want 0 — it covers the whole connection and would cut off /ws", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 — it covers the whole connection and would cut off /ws", srv.WriteTimeout)
	}
	if srv.Addr != ":0" || srv.Handler == nil {
		t.Errorf("newServer(:0, h) = %+v, want the addr and handler it was given", srv)
	}
}
