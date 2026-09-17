// Package tunnel supervises the OPTIONAL cloudflared quick tunnel behind
// `--tunnel` (D-123/CONN-01).
//
// The package is a failure domain of its own by construction. Nothing in
// game/, hub/ or store/ may import it: the manager's only output is a
// Status{URL, Healthy} handed to a PublishFunc that cmd/server wires into the
// hub's control-plane broadcaster. Every failure — a missing binary, an
// unusable config file, a refused creation, an unexpected exit, a failed
// round-trip probe — is logged and published as unhealthy, and the auction
// engine keeps running on LAN/local exactly as if `--tunnel` had never been
// passed (D-123/D-129). Tunnel availability is never a boot prerequisite.
//
// The two seams the tests drive are declared here because they are the
// package's whole contract: the probe (a round-trip through the public URL,
// D-126) and the publish callback.
package tunnel

// Status is the published tunnel state (D-123): the URL the room should be
// sent to, and whether a real WebSocket round-trip through it has succeeded.
//
// The zero value — {URL: "", Healthy: false} — is a declared state, not an
// absence: it says "there is no tunnel rung to stand on", which is exactly what
// the engine needs in order to fall back to the persisted/LAN join URL (D-128).
// The manager publishes it when the child dies and when no binary is available.
type Status struct {
	// URL is the quick-tunnel origin (https://<name>.trycloudflare.com), or ""
	// when no tunnel is currently serving.
	URL string
	// Healthy is true only after a screen-role WebSocket handshake completed
	// through URL (D-126). A published URL is announced unhealthy: the URL is
	// printed about a second before the edge path is live, and its DNS name is
	// not resolvable for the first few seconds after that.
	Healthy bool
}

// PublishFunc receives every Status CHANGE. It is edge-triggered — the manager
// dedupes against the last value it published — because the consumer fans the
// status out to every connection as a control-plane frame and to the engine's
// effective-join-URL resolution, and neither wants a heartbeat of identical
// values.
//
// The callback is the seam that keeps this package a leaf: cmd/server closes
// over the hub broadcaster and the engine setter, so neither of those packages
// ever imports internal/tunnel (D-123).
type PublishFunc func(Status)
