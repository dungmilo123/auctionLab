package tunnel

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This file owns the pure rules: what counts as a quick-tunnel announcement,
// where cloudflared's config file would be, what the probe dials, and how the
// restart schedule grows. The tables are the contract — the measured 2026.9.1
// output is a row, and so is every shape that must NOT be mistaken for a tunnel.

// TestScrapeURL pins the D-124 regex against real output, format drift, and the
// decoys. The decoys matter more than the positives: a URL that is not a quick
// tunnel would be published as the room's join URL and the QR code would point
// at it (T-07-03).
func TestScrapeURL(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
		ok   bool
	}{
		{
			name: "the measured zerolog box line",
			line: `2026-09-17T07:09:35Z INF |  https://fda-fluid-markets-bomb.trycloudflare.com  |`,
			want: "https://fda-fluid-markets-bomb.trycloudflare.com",
			ok:   true,
		},
		{
			name: "a bare URL",
			line: "https://word-word-word.trycloudflare.com",
			want: "https://word-word-word.trycloudflare.com",
			ok:   true,
		},
		{
			name: "format drift: the URL inside a structured log line",
			line: `time=2026-09-17T07:09:35Z level=info msg="quick tunnel ready at https://drift-one-two.trycloudflare.com" env=prod`,
			want: "https://drift-one-two.trycloudflare.com",
			ok:   true,
		},
		{
			name: "a port after the host is dropped: the origin is what gets published",
			line: "https://with-port-a-b.trycloudflare.com:443",
			want: "https://with-port-a-b.trycloudflare.com",
			ok:   true,
		},
		{
			name: "Cloudflare's own API host (the 429 body) is not a quick tunnel",
			line: `ERR Failed to request quick Tunnel: Post "https://api.trycloudflare.com/tunnel": 429 Too Many Requests`,
			ok:   false,
		},
		{
			name: "a disclaimer URL is not a tunnel",
			line: "Quick tunnels are meant for testing and development. See https://www.cloudflare.com/website-terms/",
			ok:   false,
		},
		{
			name: "the bare domain is not a tunnel",
			line: "https://trycloudflare.com",
			ok:   false,
		},
		{
			name: "an upper-case host is not a tunnel",
			line: "https://WORD-WORD-WORD.trycloudflare.com",
			ok:   false,
		},
		{
			name: "a longer host that merely contains one is not a tunnel",
			line: "https://word-word-word.trycloudflare.com.evil.example",
			ok:   false,
		},
		{
			name: "a URL glued to a longer token is not a tunnel",
			line: "xhttps://word-word-word.trycloudflare.com",
			ok:   false,
		},
		{
			name: "a dotted label is not a quick-tunnel label",
			line: "https://a.b.trycloudflare.com",
			ok:   false,
		},
		{
			name: "plain http is not a tunnel",
			line: "http://word-word-word.trycloudflare.com",
			ok:   false,
		},
		{
			name: "an unrelated line",
			line: "INF Registered tunnel connection connIndex=0 location=sin08",
			ok:   false,
		},
		{
			name: "an empty line",
			line: "",
			ok:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := scrapeURL([]byte(tc.line))
			if ok != tc.ok {
				t.Fatalf("scrapeURL(%q) ok = %v, want %v", tc.line, ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("scrapeURL(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

// TestConfigConflict drives all five measured directories through the injectable
// seam, because two of them are absolute paths a test cannot create (D-125).
func TestConfigConflict(t *testing.T) {
	t.Run("the measured search path, in cloudflared's own order", func(t *testing.T) {
		home := filepath.Join("/home", "presenter")
		want := []string{
			"/home/presenter/.cloudflared",
			"/home/presenter/.cloudflare-warp",
			"/home/presenter/cloudflare-warp",
			"/etc/cloudflared",
			"/usr/local/etc/cloudflared",
		}
		got := configDirs(home)
		if len(got) != len(want) {
			t.Fatalf("configDirs(%q) = %v, want the measured five", home, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("configDirs(%q)[%d] = %q, want %q", home, i, got[i], want[i])
			}
		}
	})

	t.Run("finds a config file in every directory and under both names", func(t *testing.T) {
		root := t.TempDir()

		var dirs []string
		for i, name := range []string{"one", "two", "three", "four", "five"} {
			dir := filepath.Join(root, name)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("creating %s: %v", dir, err)
			}
			dirs = append(dirs, dir)
			fileName := configFileNames[i%len(configFileNames)]
			if err := os.WriteFile(filepath.Join(dir, fileName), []byte("tunnel: x\n"), 0o644); err != nil {
				t.Fatalf("writing the config file: %v", err)
			}
		}
		withConfigDirs(t, dirs)

		path, found := ConfigConflict("/ignored")
		if !found {
			t.Fatal("a config file in the first directory was not reported")
		}
		if want := filepath.Join(dirs[0], configFileNames[0]); path != want {
			t.Fatalf("ConfigConflict = %q, want the first match %q (search order is the contract)", path, want)
		}
	})

	t.Run("reports the first directory that holds a file", func(t *testing.T) {
		root := t.TempDir()
		first := filepath.Join(root, "first")
		second := filepath.Join(root, "second")
		for _, dir := range []string{first, second} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("creating %s: %v", dir, err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("x"), 0o644); err != nil {
				t.Fatalf("writing the config file: %v", err)
			}
		}
		withConfigDirs(t, []string{first, second})

		path, found := ConfigConflict("")
		if !found || path != filepath.Join(first, "config.yaml") {
			t.Fatalf("ConfigConflict = (%q, %v), want the first directory's file", path, found)
		}
	})

	t.Run("a directory named like a config file is not a conflict", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "only")
		if err := os.MkdirAll(filepath.Join(dir, "config.yaml"), 0o755); err != nil {
			t.Fatalf("creating the decoy directory: %v", err)
		}
		withConfigDirs(t, []string{dir})

		if path, found := ConfigConflict(""); found {
			t.Fatalf("a directory was reported as a config file: %s", path)
		}
	})

	t.Run("no file anywhere is not a conflict", func(t *testing.T) {
		withConfigDirs(t, []string{filepath.Join(t.TempDir(), "empty")})

		if path, found := ConfigConflict(""); found {
			t.Fatalf("ConfigConflict reported %q in an empty tree", path)
		}
	})
}

// withConfigDirs swaps the search path for one case and restores it after.
func withConfigDirs(t *testing.T, dirs []string) {
	t.Helper()

	previous := configSearchDirs
	configSearchDirs = func(string) []string { return dirs }
	t.Cleanup(func() { configSearchDirs = previous })
}

// TestProbeURL pins the origin→socket translation. The origin is the only input
// because it is the only thing the scrape guarantees.
func TestProbeURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{
			name: "https becomes wss",
			in:   "https://word-word-word.trycloudflare.com",
			want: "wss://word-word-word.trycloudflare.com/ws",
			ok:   true,
		},
		{
			name: "http becomes ws (the in-process test hub)",
			in:   "http://127.0.0.1:8080",
			want: "ws://127.0.0.1:8080/ws",
			ok:   true,
		},
		{
			name: "a path and query are replaced, never trusted",
			in:   "https://word-word-word.trycloudflare.com/some/path?q=1",
			want: "wss://word-word-word.trycloudflare.com/ws",
			ok:   true,
		},
		{
			name: "a non-HTTP scheme is refused",
			in:   "ftp://word-word-word.trycloudflare.com",
			ok:   false,
		},
		{
			name: "a hostless string is refused",
			in:   "not a url",
			ok:   false,
		},
		{
			name: "an empty string is refused",
			in:   "",
			ok:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := probeURL(tc.in)
			if ok != tc.ok {
				t.Fatalf("probeURL(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("probeURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestBackoffSchedule pins the D-125 numbers themselves (5s → cap 60s), not the
// test-sized values the manager cases inject.
func TestBackoffSchedule(t *testing.T) {
	base, top := DefaultBackoffBase, DefaultBackoffCap
	if base != 5*time.Second || top != 60*time.Second {
		t.Fatalf("defaults are %s → %s, want 5s → 60s (D-125)", base, top)
	}

	want := []time.Duration{5, 10, 20, 40, 60, 60, 60}
	for attempt, seconds := range want {
		if got := backoffDelay(attempt, base, top); got != seconds*time.Second {
			t.Fatalf("backoffDelay(%d) = %s, want %s", attempt, got, seconds*time.Second)
		}
	}
	// A huge attempt counter must not overflow into a negative or tiny delay.
	if got := backoffDelay(64, base, top); got != top {
		t.Fatalf("backoffDelay(64) = %s, want the cap", got)
	}
}

// TestFullJitterStaysInsideTheInterval pins full jitter's contract: uniform over
// the closed interval [0, delay]. It samples rather than proving the
// distribution — the boundary is what matters (a jitter that overshot the delay
// would break the cap; one that always returned zero would make the schedule a
// fixed cadence).
func TestFullJitterStaysInsideTheInterval(t *testing.T) {
	const delay = 5 * time.Second
	sawNonZero := false
	for i := 0; i < 512; i++ {
		got := FullJitter(delay)
		if got < 0 || got > delay {
			t.Fatalf("FullJitter(%s) = %s, outside [0, delay]", delay, got)
		}
		if got > 0 {
			sawNonZero = true
		}
	}
	if !sawNonZero {
		t.Fatal("FullJitter never returned a non-zero delay; that is a fixed cadence, not jitter")
	}
	if got := FullJitter(0); got != 0 {
		t.Fatalf("FullJitter(0) = %s, want 0", got)
	}
}

// TestInstallScriptSyntax is the shell half of the installer's contract: the
// file exists, is executable in the repository, and parses under POSIX sh
// before anything tries to run it (a broken installer at the venue is worse
// than no installer).
func TestInstallScriptSyntax(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install-cloudflared.sh")

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the installer is missing: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("%s is not executable (mode %s)", path, info.Mode())
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the installer: %v", err)
	}
	for _, banned := range []string{"| bash", "| sh", "|bash", "|sh"} {
		if strings.Contains(string(body), banned) {
			t.Fatalf("the installer contains a pipe-to-shell form (%q); D-124 forbids it", banned)
		}
	}
}

// TestInstallScriptVerifiesChecksum is the functional half of T-07-01: an
// artifact whose digest does not match is never made executable and never
// lands in the destination, and the same artifact with the right digest does.
// It runs the real script through `sh` against a file:// release directory, so
// no network is involved.
func TestInstallScriptVerifiesChecksum(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no POSIX sh on this machine")
	}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		if _, err := exec.LookPath("shasum"); err != nil {
			t.Skip("neither sha256sum nor shasum is available")
		}
	}
	if _, err := exec.LookPath("curl"); err != nil {
		if _, err := exec.LookPath("wget"); err != nil {
			t.Skip("neither curl nor wget is available")
		}
	}

	script := filepath.Join("..", "..", "scripts", "install-cloudflared.sh")
	root := t.TempDir()

	// A release directory laid out the way the official channel is:
	// <base>/v<version>/cloudflared-<os>-<arch>.
	payload := []byte("#!/bin/sh\necho stub-cloudflared\n")
	release := filepath.Join(root, "release", "v1.2.3")
	if err := os.MkdirAll(release, 0o755); err != nil {
		t.Fatalf("creating the release directory: %v", err)
	}
	asset := filepath.Join(release, "cloudflared-linux-amd64")
	if err := os.WriteFile(asset, payload, 0o644); err != nil {
		t.Fatalf("writing the asset: %v", err)
	}
	sum := sha256.Sum256(payload)
	good := hex.EncodeToString(sum[:])

	run := func(sha, dir string) (string, error) {
		t.Helper()
		cmd := exec.Command("sh", script,
			"--base-url", "file://"+filepath.Join(root, "release"),
			"--version", "1.2.3", "--os", "linux", "--arch", "amd64",
			"--sha256", sha, "--dir", dir)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	t.Run("a mismatched digest installs nothing", func(t *testing.T) {
		dir := filepath.Join(root, "bad")
		out, err := run(strings.Repeat("0", 64), dir)
		if err == nil {
			t.Fatalf("the installer accepted a wrong digest:\n%s", out)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "cloudflared-linux-amd64")); statErr == nil {
			t.Fatal("the unverified download was installed anyway")
		}
		if !strings.Contains(out, "checksum mismatch") {
			t.Fatalf("the failure does not say what happened:\n%s", out)
		}
	})

	t.Run("the published digest installs an executable", func(t *testing.T) {
		dir := filepath.Join(root, "good")
		out, err := run(good, dir)
		if err != nil {
			t.Fatalf("the installer refused a correct digest: %v\n%s", err, out)
		}
		info, statErr := os.Stat(filepath.Join(dir, "cloudflared-linux-amd64"))
		if statErr != nil {
			t.Fatalf("the verified binary was not installed: %v\n%s", statErr, out)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("the installed binary is not executable (mode %s)", info.Mode())
		}
	})

	t.Run("no digest at all is refused before any download", func(t *testing.T) {
		dir := filepath.Join(root, "none")
		cmd := exec.Command("sh", script,
			"--base-url", "file://"+filepath.Join(root, "release"),
			"--version", "1.2.3", "--os", "linux", "--arch", "amd64", "--dir", dir)
		cmd.Env = append(os.Environ(), "CLOUDFLARED_SHA256=")
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("the installer ran without a digest:\n%s", out)
		}
		if !strings.Contains(string(out), "no SHA-256 supplied") {
			t.Fatalf("the refusal is not actionable:\n%s", out)
		}
	})
}
