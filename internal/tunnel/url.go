package tunnel

import (
	"regexp"
	"time"
)

// This file owns the two pure rules the supervisor is built on — where
// cloudflared is, and what its output means — plus the restart arithmetic.
// Everything here is a plain function with no I/O, so the whole surface is
// table-testable without a process or a network.

// urlPattern is D-124's scrape rule. It is deliberately NOT anchored to
// cloudflared's zerolog `INF` prefix or to the ASCII-box characters around the
// URL: the measured 2026.9.1 output prints the URL inside a box on stderr, but
// the format is the vendor's to change, and a log-format tweak must not take
// the venue's QR code down with it. The character class is lower-case only,
// which is the shape Cloudflare actually issues.
var urlPattern = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

// configFileNames are the two names cloudflared accepts for its config file.
// Measured on 2026.9.1: the binary itself announces
// `No file [config.yml config.yaml] in [~/.cloudflared ~/.cloudflare-warp
// ~/cloudflare-warp /etc/cloudflared /usr/local/etc/cloudflared]`.
var configFileNames = [...]string{"config.yml", "config.yaml"}

// configDirs is the measured five-directory search path, in cloudflared's own
// order: the three home-relative candidates first, then the two system ones.
// home is injected so a test can drive the whole list through one temp dir.
func configDirs(home string) []string { return nil }

// configSearchDirs is the injectable seam ConfigConflict reads. A test that
// wants to drive all five directories (including the two absolute ones it
// cannot create) swaps this for its own table.
var configSearchDirs = configDirs

// ConfigConflict reports whether a cloudflared config file exists on the
// measured search path, and which one (D-125). A quick tunnel is unsupported
// while such a file exists, and the failure is otherwise silent — cloudflared
// accepts `--url` and then does nothing useful — so the supervisor reports it
// at boot as an actionable warning (rename the file) and keeps running.
func ConfigConflict(home string) (string, bool) { return "", false }

// scrapeURL extracts the first quick-tunnel origin from one line of merged
// cloudflared output. A line with no URL, a disclaimer URL, or an
// upper-case/partial host is not a match. The match must END at a hostname
// boundary so `https://x.trycloudflare.com.evil.example` is not read as
// `https://x.trycloudflare.com`.
func scrapeURL(line []byte) (string, bool) { return "", false }

// probeURL turns a tunnel origin into the WebSocket URL the probe dials:
// https → wss, http → ws, and the path is replaced by /ws. The origin is the
// only input because it is the only thing the scrape guarantees; a path or
// query on the parsed URL is discarded rather than trusted.
func probeURL(tunnelURL string) (string, bool) { return "", false }

// resolveBinary is D-124's precedence, in one testable place:
// the --cloudflared-bin flag, then TUNNEL_BIN, then cloudflared on PATH. The
// returned bool is false when none of the three resolves, which the manager
// reports as "not installed" instead of an error.
func resolveBinary(flagPath string, getenv func(string) string, lookPath func(string) (string, error)) (string, bool) {
	return "", false
}

// backoffDelay is D-125's restart schedule: base doubled once per attempt and
// capped. It is deliberately separate from the jitter that follows it, so a
// test can pin the schedule itself (5s, 10s, 20s, 40s, 60s, 60s…) without
// fighting a random number generator.
func backoffDelay(attempt int, base, top time.Duration) time.Duration { return 0 }
