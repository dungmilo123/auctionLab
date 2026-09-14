// Package auctionlab hosts the repository-root embed of the built frontend.
//
// The root file must be a *library* package, never `package main`: cmd/server is
// also `package main` and cannot import another `package main`, and //go:embed
// patterns may not contain `..` — so the embed cannot live inside cmd/server.
package auctionlab

import (
	"embed"
	"io/fs"
)

// The `all:` prefix is load-bearing and must stay verbatim: without it Go skips
// dot-prefixed files, so a clean clone whose web/dist holds only the committed
// `.gitkeep` placeholder fails to compile ("contains no embeddable files").
//
//go:embed all:web/dist
var dist embed.FS

// SPA returns the embedded frontend build rooted at web/dist.
func SPA() (fs.FS, error) {
	return fs.Sub(dist, "web/dist")
}

// Dist returns the raw embedded filesystem (used by tests that assert the
// placeholder survived the build).
func Dist() embed.FS {
	return dist
}
