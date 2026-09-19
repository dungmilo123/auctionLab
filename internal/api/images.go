package api

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// This file is D-01's HTTP half: the local image folder the presenter drops
// files into is served at /images/<filename> by the same single binary that
// serves the SPA. No upload, no blob in SQLite and no Internet access — the
// venue may be fully offline, so an item image is a file on disk.

// ImagesDirEnv is D-01's override for the served directory. It exists for tests
// and for an operator whose images live elsewhere; the default is cwd-relative,
// matching the auction.db path beside the binary.
const ImagesDirEnv = "IMAGES_DIR"

// DefaultImagesDir is the folder D-01 serves when IMAGES_DIR is unset.
const DefaultImagesDir = "images"

// imagesPrefix is the route the handler owns. It is written once because the
// prefix arithmetic in imageRequestName and the route table in cmd/server must
// agree.
const imagesPrefix = "/images"

// Images serves one local image file per request (D-01).
//
// It is built on os.DirFS + http.FileServerFS so the stdlib keeps doing what it
// already does well — content-type detection from the extension (with a
// sniffed fallback), range requests, Last-Modified and conditional requests.
// The handler adds the three rules a bare file server gets wrong here:
//
//   - a DIRECTORY is an ordinary 404. http.FileServerFS answers a directory
//     with a listing (or its index.html), and enumerating the folder is exactly
//     what the "no listing" rule forbids: /images, /images/ and /images/sub/
//     are all not-found responses.
//   - an absent file is a 404 too, never the SPA document — the same reason
//     static.go keeps a missing /assets/ name a 404 instead of HTML (a target
//     of an <img> tag must not parse as a document).
//   - every served file is `no-cache`. The presenter replaces images in place
//     between runs, and the immutable policy static.go applies to Vite's
//     content-hashed assets would show yesterday's photo mid-show — the same
//     stale-content failure, on a file whose name never changes.
//
// Path confinement is os.DirFS's own rule: every name is checked by
// fs.ValidPath (no "..", no leading "/", no "." elements), so a request path
// can never reach outside the configured root — a request that tries is a
// lookup error, which is the 404 below rather than an error this handler has to
// remember to catch.
type Images struct {
	fsys  fs.FS
	files http.Handler
}

// NewImages builds the handler for dir. A missing directory is not a boot
// failure: D-129's rule that connectivity is never a prerequisite holds for the
// image folder too, and every request simply answers 404 until it exists.
//
// The file server is wrapped in StripPrefix because both halves must agree on
// what the request path means: http.FileServerFS resolves r.URL.Path against
// its root, so "/images/photo.png" would be looked up as "images/photo.png"
// inside the directory. Stripping the route prefix is what makes the URL path
// and the file name the same thing.
func NewImages(dir string) *Images {
	fsys := os.DirFS(dir)
	return &Images{
		fsys:  fsys,
		files: http.StripPrefix(imagesPrefix+"/", http.FileServerFS(fsys)),
	}
}

// ImagesDir resolves the directory D-01 serves: IMAGES_DIR when it names one,
// the cwd-relative images/ folder otherwise. It takes getenv rather than
// reading the environment itself so the resolution is a pure function both the
// boot and its test can drive.
func ImagesDir(getenv func(string) string) string {
	if dir := strings.TrimSpace(getenv(ImagesDirEnv)); dir != "" {
		return dir
	}
	return DefaultImagesDir
}

// ServeHTTP serves one file from the configured directory: the directory root
// in either spelling (a 404 — a folder is not a file), a path that resolves to
// a directory (a 404), a path that resolves to nothing (a 404), or a real file.
func (h *Images) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name, ok := imageRequestName(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	info, err := fs.Stat(h.fsys, name)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	// Set before the file server runs: http.ServeContent only adds
	// Last-Modified/Content-Type and never overwrites a directive set here (a
	// 304 keeps it too).
	w.Header().Set("Cache-Control", "no-cache")
	h.files.ServeHTTP(w, r)
}

// imageRequestName is the request path's file name relative to the images root,
// or ok=false when it names no file under that root. Path.Clean runs first so
// the answer is decided on the canonical form of the request alone: "/images"
// and "/images/" both clean to the root, a "/images/../x" cleans out of the
// prefix entirely, and a trailing separator is a directory request by
// construction (no file name ends in one).
func imageRequestName(urlPath string) (string, bool) {
	cleaned := path.Clean(urlPath)
	if !strings.HasPrefix(cleaned, imagesPrefix+"/") {
		return "", false
	}
	name := strings.TrimPrefix(cleaned, imagesPrefix+"/")
	if name == "" || strings.HasSuffix(name, "/") {
		return "", false
	}
	return name, true
}
