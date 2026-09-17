package tunnel

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// This file owns the two pure rules the supervisor is built on — where
// cloudflared is, and what its output means — plus the restart arithmetic.
// Everything here is a plain function with no I/O of its own (the filesystem
// reads in ConfigConflict are the one exception, and they take the home
// directory as an argument), so the whole surface is table-testable without a
// process or a network.

// urlPattern is D-124's scrape rule. It is deliberately NOT anchored to
// cloudflared's zerolog `INF` prefix or to the ASCII-box characters around the
// URL: the measured 2026.9.1 output prints the URL inside a box on stderr, but
// the format is the vendor's to change, and a log-format tweak must not take
// the venue's QR code down with it.
//
// The character class is lower-case only, and the label must contain at least
// one dash, because that is the measured shape (three dashed words, e.g.
// `fda-fluid-markets-bomb.trycloudflare.com`). The dash is load-bearing, not
// cosmetic: Cloudflare's own 429 body quotes `https://api.trycloudflare.com`,
// which is NOT a quick tunnel, and a rule that accepted the bare label would
// publish Cloudflare's API host as the room's join URL (T-07-03).
var urlPattern = regexp.MustCompile(`https://[a-z0-9]+(?:-[a-z0-9]+)+\.trycloudflare\.com`)

// configFileNames are the two names cloudflared accepts for its config file.
// Measured on 2026.9.1: the binary itself announces
// `No file [config.yml config.yaml] in [~/.cloudflared ~/.cloudflare-warp
// ~/cloudflare-warp /etc/cloudflared /usr/local/etc/cloudflared]`.
var configFileNames = [...]string{"config.yml", "config.yaml"}

// configDirs is the measured five-directory search path, in cloudflared's own
// order: the three home-relative candidates first, then the two system ones.
// home is injected so a test can drive the whole list through one temp dir.
func configDirs(home string) []string {
	return []string{
		filepath.Join(home, ".cloudflared"),
		filepath.Join(home, ".cloudflare-warp"),
		filepath.Join(home, "cloudflare-warp"),
		"/etc/cloudflared",
		"/usr/local/etc/cloudflared",
	}
}

// configSearchDirs is the injectable seam ConfigConflict reads. A test that
// wants to drive all five directories (including the two absolute ones it
// cannot create) swaps this for its own table.
var configSearchDirs = configDirs

// ConfigConflict reports whether a cloudflared config file exists on the
// measured search path, and which one (D-125). A quick tunnel is unsupported
// while such a file exists, and the failure is otherwise silent — cloudflared
// accepts `--url` and then does nothing useful — so the supervisor reports it
// at boot as an actionable warning (rename the file) and keeps running.
func ConfigConflict(home string) (string, bool) {
	for _, dir := range configSearchDirs(home) {
		for _, name := range configFileNames {
			path := filepath.Join(dir, name)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, true
			}
		}
	}
	return "", false
}

// scrapeURL extracts the first quick-tunnel origin from one line of merged
// cloudflared output. A line with no URL, a disclaimer URL, or an
// upper-case/partial host is not a match.
//
// The match must sit on hostname boundaries at BOTH ends: it may not be
// followed by another host character (`https://x.trycloudflare.com.evil` is a
// different host, not a quick tunnel) and it may not be preceded by one
// (`xhttps://…` is one token, not a URL). Cloudflare issues exactly
// `<word>-<word>-<word>.trycloudflare.com`, so neither guard can reject a real
// announcement — they only refuse text that merely contains the pattern.
func scrapeURL(line []byte) (string, bool) {
	loc := urlPattern.FindIndex(line)
	if loc == nil {
		return "", false
	}
	if end := loc[1]; end < len(line) && isHostChar(line[end]) {
		return "", false
	}
	if start := loc[0]; start > 0 && isHostChar(line[start-1]) {
		return "", false
	}
	return string(line[loc[0]:loc[1]]), true
}

// isHostChar reports whether b could continue a hostname. It is the boundary
// test both ends of a match use.
func isHostChar(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '.' || b == '-' || b == '_':
		return true
	default:
		return false
	}
}

// probeURL turns a tunnel origin into the WebSocket URL the probe dials:
// https → wss, http → ws, and the path is replaced by /ws. The origin is the
// only input because it is the only thing the scrape guarantees; a path or
// query on the parsed URL is discarded rather than trusted.
func probeURL(tunnelURL string) (string, bool) {
	parsed, err := url.Parse(tunnelURL)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return "", false
	}
	parsed.Path = "/ws"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), true
}

// resolveBinary is D-124's precedence, in one testable place:
// the --cloudflared-bin flag, then TUNNEL_BIN, then cloudflared on PATH. The
// returned bool is false when none of the three resolves, which the manager
// reports as "not installed" instead of an error: a venue with no internet (or
// no cloudflared) runs the auction on LAN exactly as it always did (D-129).
func resolveBinary(flagPath string, getenv func(string) string, lookPath func(string) (string, error)) (string, bool) {
	if flagPath != "" {
		return flagPath, true
	}
	if envPath := getenv("TUNNEL_BIN"); envPath != "" {
		return envPath, true
	}
	found, err := lookPath("cloudflared")
	if err != nil || found == "" {
		return "", false
	}
	return found, true
}

// ResolveBinary is resolveBinary's exported form: cmd/server resolves the binary
// at boot (D-124) so it can report which rung won and whether the tunnel is
// startable BEFORE a manager exists. Keeping one implementation is the point — a
// second copy of the precedence order in cmd/server is exactly where it would
// drift.
func ResolveBinary(flagPath string, getenv func(string) string, lookPath func(string) (string, error)) (string, bool) {
	return resolveBinary(flagPath, getenv, lookPath)
}

// backoffDelay is D-125's restart schedule: base doubled once per attempt and
// capped. It is deliberately separate from the jitter that follows it, so a
// test can pin the schedule itself (5s, 10s, 20s, 40s, 60s, 60s…) without
// fighting a random number generator.
func backoffDelay(attempt int, base, top time.Duration) time.Duration {
	delay := base
	for i := 0; i < attempt; i++ {
		if delay >= top {
			return top
		}
		delay *= 2
	}
	if delay > top {
		return top
	}
	return delay
}
