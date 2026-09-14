package api

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Static serves the embedded frontend build.
type Static struct {
	fsys  fs.FS
	files http.Handler
	index []byte
}

// NewStatic reads index.html once at construction and fails when it is missing
// or unreadable.
//
// That failure is the point: a binary built without a usable frontend must
// refuse to boot instead of starting up and 404-ing every route (a silent
// failure is far worse at the venue than a loud one). Caching the bytes also
// keeps the hot path from re-reading and sidesteps http.FileServerFS's
// internal "/index.html" → "/" redirect.
func NewStatic(fsys fs.FS) (*Static, error) {
	index, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		return nil, fmt.Errorf("embedded SPA has no readable index.html — run `make build` before `go build`: %w", err)
	}

	return &Static{
		fsys:  fsys,
		files: http.FileServerFS(fsys),
		index: index,
	}, nil
}

// ServeHTTP serves the boot-cached index for the root and for index.html
// itself, and everything else that exists through http.FileServerFS (ranges,
// conditional requests and Last-Modified keep working).
//
// Wave 1 ships the thin form on purpose: a path with no matching file, and a
// directory, both return 404. The deep-link fallback and the immutable
// /assets/* cache header arrive in plan 01-02, test-first.
func (s *Static) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." || name == "index.html" {
		s.serveIndex(w)
		return
	}

	info, err := fs.Stat(s.fsys, name)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	s.files.ServeHTTP(w, r)
}

func (s *Static) serveIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// index.html keeps its name across builds, so a cached copy would keep
	// serving a stale app after a tunnel restart.
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.index)
}
