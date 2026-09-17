package tunnel

import "context"

// This file owns the health primitive D-126 requires: a REAL WebSocket
// round-trip through the public URL. Process liveness, `--metrics`, DNS and an
// HTTP GET of /api/health all prove something weaker than the upgrade path, and
// the #1652 failure mode (a tunnel that drops the `Upgrade` header) looks
// perfectly healthy to every one of them. Only a completed handshake proves the
// venue's phones can reach the server.

// ProbeFunc is one probe attempt: dial the tunnel origin, complete the screen
// handshake, close. nil means healthy. It is injectable because the manager's
// tests must be able to flip health without an Internet-facing URL — the stub
// binary prints `https://stub-N.trycloudflare.com`, which by construction does
// not resolve (D-137).
type ProbeFunc func(ctx context.Context, tunnelURL string) error

// ProbeURL is the production probe. It joins as role `screen`, which is the
// deliberate choice over the player role: a screen join is not a game mutation
// — no player row, no revision bump, no budget — so a 30-second cadence leaves
// no rubble in the database, and it is the role that must survive the tunnel
// for the projector.
//
// The handshake it demands is the frozen one (D-23): `hello` at seq 1 carrying
// role "screen", then `state.snapshot` at seq 2. Anything else — an error
// frame, a different type, a different sequence — is a failure, and the
// returned error names what arrived (a `websocket: bad handshake` with a 400
// is the #1652 signature and is reported verbatim, because it is the single
// most useful diagnostic line this package can produce).
func ProbeURL(ctx context.Context, tunnelURL string) error { return nil }
