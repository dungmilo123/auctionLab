// Package auctionlab tests: the repo-root embed is the single place where the
// clean-clone trap lives, so these assertions are the ones that must stay green
// with an empty web/dist.
package auctionlab

import (
	"io/fs"
	"strings"
	"testing"
)

// TestPlaceholder pins the `all:` prefix decision through its observable
// effect: the committed dot-prefixed placeholder is embedded.
//
// If the pattern is ever "simplified" to the bare `web/dist` form, Go skips
// dot-files and a fresh clone — whose web/dist holds only `.gitkeep` — fails to
// compile with "cannot embed directory web/dist: contains no embeddable files".
// This test fails first, with a message that names the cause.
func TestPlaceholder(t *testing.T) {
	if _, err := fs.Stat(Dist(), "web/dist/.gitkeep"); err != nil {
		t.Fatalf("embedded FS has no web/dist/.gitkeep: %v\n"+
			"Keep the pattern verbatim: `//go:embed all:web/dist` — without the `all:` "+
			"prefix Go skips dot-files and a clean clone no longer builds.", err)
	}

	spa, err := SPA()
	if err != nil {
		t.Fatalf("SPA(): %v", err)
	}
	if _, err := fs.Stat(spa, ".gitkeep"); err != nil {
		t.Fatalf("SPA() does not expose .gitkeep at its root: %v\n"+
			"fs.Sub must be rooted at web/dist, not at the module root.", err)
	}
}

// TestSPAIsRooted proves the `fs.Sub(dist, "web/dist")` prefix is exactly right:
// the entry set SPA() exposes at its root is the same entry set the raw embed
// holds under web/dist — and the raw embed does not hold it at its own root.
//
// This is what keeps `http.FileServerFS(SPA())` from serving a path one level
// too high (or too low) after somebody edits the embed file.
func TestSPAIsRooted(t *testing.T) {
	spa, err := SPA()
	if err != nil {
		t.Fatalf("SPA(): %v", err)
	}

	spaNames, err := entryNames(fs.ReadDir, spa, ".")
	if err != nil {
		t.Fatalf("reading SPA() root: %v", err)
	}
	distNames, err := entryNames(fs.ReadDir, Dist(), "web/dist")
	if err != nil {
		t.Fatalf("reading Dist()/web/dist: %v", err)
	}

	if got, want := joinNames(spaNames), joinNames(distNames); got != want {
		t.Fatalf("SPA() root entries = %s, want the web/dist entries %s", got, want)
	}
	// The root of the raw embed must NOT look like the sub-tree: if it did, the
	// fs.Sub prefix would be "." and every asset URL would be off by one level.
	rawNames, err := entryNames(fs.ReadDir, Dist(), ".")
	if err != nil {
		t.Fatalf("reading Dist() root: %v", err)
	}
	if joinNames(rawNames) == joinNames(spaNames) {
		t.Fatal("Dist() root and SPA() root expose the same entries; " +
			"fs.Sub is not trimming the web/dist prefix")
	}

	t.Run("a built index.html is served from the SPA root when present", func(t *testing.T) {
		if _, err := fs.Stat(spa, "index.html"); err != nil {
			t.Skip("web/dist has no index.html yet (clean clone) — run `make build` to produce it")
		}
		body, err := fs.ReadFile(spa, "index.html")
		if err != nil {
			t.Fatalf("reading index.html from SPA(): %v", err)
		}
		if len(body) == 0 {
			t.Fatal("embedded index.html is empty")
		}
	})
}

// entryNames reads one directory and returns its entry names.
func entryNames(readDir func(fs.FS, string) ([]fs.DirEntry, error), fsys fs.FS, name string) ([]string, error) {
	entries, err := readDir(fsys, name)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, nil
}

// joinNames renders a name list for the failure message; fs.ReadDir already
// sorts by filename, so the comparison needs no extra ordering.
func joinNames(names []string) string {
	return "[" + strings.Join(names, " ") + "]"
}
