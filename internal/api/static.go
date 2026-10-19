package api

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Cache directives for the two classes of file the embedded build contains.
const (
	// indexCacheControl keeps the document uncached: index.html keeps its name
	// across deploys, so a cached copy would keep serving yesterday's app after
	// a tunnel restart — indistinguishable from a server bug mid-presentation.
	indexCacheControl = "no-cache"

	// assetCacheControl is safe only for Vite's content-hashed assets/ names,
	// which change whenever their contents do.
	assetCacheControl = "public, max-age=31536000, immutable"

	// assetPrefix is the directory Vite emits content-hashed files into.
	//
	// Everything under it is assumed content-hashed, so it must stay a build
	// output: a file dropped into `web/public/assets/` is copied here verbatim
	// (Vite copies public/ as-is) and would then be granted `immutable` for a
	// year — the silent stale-content failure this cache policy exists to
	// prevent. Keep hand-managed public assets at the dist root instead of
	// under assets/.
	assetPrefix = "assets/"
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

// ServeHTTP serves the SPA document for every path that is not a real file in
// the embedded build — the deep-link fallback that keeps /lots, /account and
// any nested route answering 200 text/html on a refresh, a bookmark or a QR
// link instead of dead-ending on a 404.
//
// The decision is made by fs.Stat alone, so no user-supplied path is ever
// built into a filesystem access: only names that resolve inside the embedded
// build reach the file server. Real files go through http.FileServerFS, which
// keeps range requests, conditional requests and Last-Modified intact.
func (s *Static) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")

	// "" and "." are the root; "index.html" itself is answered from the cached
	// bytes so it never hits the file server's internal "/index.html" → "/"
	// redirect (a 301 the browser would follow, but which breaks a direct
	// assertion and any client that does not follow redirects).
	if name == "" || name == "." || name == "index.html" {
		s.serveIndex(w)
		return
	}

	info, err := fs.Stat(s.fsys, name)
	if err != nil || info.IsDir() {
		// A missing name inside either reserved asset namespace is a 404, never the
		// SPA document: a <script>/<link> that receives an HTML body fails with
		// `Unexpected token '<'` instead of a clean 404, and this is the path
		// most likely to break after a rebuild. `vite dev` already 404s the
		// same URL, so the 404 keeps dev and prod honest with each other.
		if strings.HasPrefix(name, assetPrefix) || strings.HasPrefix(name, "site-assets/") || strings.HasPrefix(r.URL.Path, "/site-assets/") {
			http.NotFound(w, r)
			return
		}
		// Not a real file: a nested website URL or a directory the build
		// happens to contain. All of them are the SPA's business, not the file
		// server's — and a directory is served as the document, never listed.
		s.serveIndex(w)
		return
	}

	// Only the content-hashed assets/ directory may be cached forever; every
	// other real file is served without a cache directive.
	if strings.HasPrefix(name, assetPrefix) {
		w.Header().Set("Cache-Control", assetCacheControl)
	}

	s.files.ServeHTTP(w, r)
}

func (s *Static) serveIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", indexCacheControl)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.index)
}
