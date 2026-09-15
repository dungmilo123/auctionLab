package api

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// Fixture markers — recognisable strings, so a response body identifies which
// file served it.
const (
	indexMarker  = "<!doctype html><html lang=\"vi\"><title>auctionlab-static-index</title></html>"
	assetMarker  = "console.log('auctionlab-static-asset');"
	nestedMarker = "/* auctionlab-static-nested-asset */"
	plainMarker  = "auctionlab-static-plain-file"
)

// The exact cache directives BUILD-03 promises.
const (
	wantIndexCache = "no-cache"
	wantAssetCache = "public, max-age=31536000, immutable"
)

// staticFixture stands in for a built web/dist tree: the SPA document at the
// root, a content-hashed asset, a nested asset (its directories exist
// implicitly) and one non-asset file.
//
// A fixture — rather than the real embed — is what keeps this contract test
// runnable on a clean clone, where web/dist holds nothing but .gitkeep.
func staticFixture() fstest.MapFS {
	return fstest.MapFS{
		"index.html":               &fstest.MapFile{Data: []byte(indexMarker)},
		"assets/app.js":            &fstest.MapFile{Data: []byte(assetMarker)},
		"assets/vendor/nested.css": &fstest.MapFile{Data: []byte(nestedMarker)},
		"robots.txt":               &fstest.MapFile{Data: []byte(plainMarker)},
	}
}

type staticCase struct {
	name        string
	path        string
	wantStatus  int
	wantBody    string // marker the body must contain
	wantNotBody string // marker the body must NOT contain ("" = no assertion)
	wantType    string // substring the Content-Type must contain
	wantCache   string // exact Cache-Control, "" = the header must be absent
	wantNosniff bool   // X-Content-Type-Options: nosniff (the SPA document)
}

// TestStaticServingContract pins the serving rules for the embedded SPA:
// every path that is not a real file in the build gets the boot-cached
// document — except inside the assets/ namespace, where a miss is a 404 so a
// stale <script>/<link> never receives an HTML body. The document is never
// cached, and only content-hashed assets/ names are immutable.
func TestStaticServingContract(t *testing.T) {
	s, err := NewStatic(staticFixture())
	if err != nil {
		t.Fatalf("NewStatic: %v", err)
	}

	cases := []staticCase{
		{
			name: "root serves the SPA document", path: "/",
			wantStatus: http.StatusOK, wantBody: indexMarker, wantType: "text/html",
			wantCache: wantIndexCache, wantNosniff: true,
		},
		{
			name: "index dot html serves in place instead of redirecting", path: "/index.html",
			wantStatus: http.StatusOK, wantBody: indexMarker, wantType: "text/html",
			wantCache: wantIndexCache, wantNosniff: true,
		},
		{
			name: "deep link admin falls back to the SPA", path: "/admin",
			wantStatus: http.StatusOK, wantBody: indexMarker, wantType: "text/html",
			wantCache: wantIndexCache, wantNosniff: true,
		},
		{
			name: "nested deep link screen falls back to the SPA", path: "/screen/anything/deep",
			wantStatus: http.StatusOK, wantBody: indexMarker, wantType: "text/html",
			wantCache: wantIndexCache, wantNosniff: true,
		},
		{
			name: "deep link with a trailing slash falls back to the SPA", path: "/admin/",
			wantStatus: http.StatusOK, wantBody: indexMarker, wantType: "text/html",
			wantCache: wantIndexCache, wantNosniff: true,
		},
		{
			name: "an existing directory serves the SPA, not a listing", path: "/assets",
			wantStatus: http.StatusOK, wantBody: indexMarker, wantType: "text/html",
			wantCache: wantIndexCache, wantNosniff: true,
		},
		{
			name: "a missing asset is a 404, never the SPA document", path: "/assets/does-not-exist.js",
			wantStatus: http.StatusNotFound, wantBody: "404 page not found", wantType: "text/plain",
			wantCache: "", wantNotBody: indexMarker,
		},
		{
			name: "a content-hashed asset is served with the immutable cache header", path: "/assets/app.js",
			wantStatus: http.StatusOK, wantBody: assetMarker, wantType: "javascript",
			wantCache: wantAssetCache,
		},
		{
			name: "a nested asset under assets is immutable too", path: "/assets/vendor/nested.css",
			wantStatus: http.StatusOK, wantBody: nestedMarker, wantType: "css",
			wantCache: wantAssetCache,
		},
		{
			name: "a non-asset file gets no cache directive", path: "/robots.txt",
			wantStatus: http.StatusOK, wantBody: plainMarker, wantType: "text/plain",
			wantCache: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Errorf("GET %s: status = %d, want %d (body %q)",
					tc.path, rec.Code, tc.wantStatus, rec.Body.String())
			}
			if loc := rec.Header().Get("Location"); loc != "" {
				t.Errorf("GET %s: unexpected redirect to %q", tc.path, loc)
			}
			if body := rec.Body.String(); !strings.Contains(body, tc.wantBody) {
				t.Errorf("GET %s: body = %q, want it to contain %q", tc.path, body, tc.wantBody)
			}
			if tc.wantNotBody != "" {
				if body := rec.Body.String(); strings.Contains(body, tc.wantNotBody) {
					t.Errorf("GET %s: body = %q, must not contain %q", tc.path, body, tc.wantNotBody)
				}
			}
			if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, tc.wantType) {
				t.Errorf("GET %s: Content-Type = %q, want it to contain %q", tc.path, ct, tc.wantType)
			}
			if got := rec.Header().Get("Cache-Control"); got != tc.wantCache {
				t.Errorf("GET %s: Cache-Control = %q, want %q", tc.path, got, tc.wantCache)
			}
			if tc.wantNosniff {
				if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
					t.Errorf("GET %s: X-Content-Type-Options = %q, want %q", tc.path, got, "nosniff")
				}
			}
		})
	}
}

// TestStaticRequiresIndexDocument pins the boot-refusal path: a build with no
// usable index.html must fail at construction rather than starting up and
// 404-ing every route.
func TestStaticRequiresIndexDocument(t *testing.T) {
	_, err := NewStatic(fstest.MapFS{
		"assets/app.js": &fstest.MapFile{Data: []byte(assetMarker)},
	})
	if err == nil {
		t.Fatal("NewStatic accepted an FS without index.html; want a construction error")
	}
	if !strings.Contains(err.Error(), "index.html") {
		t.Errorf("NewStatic error = %q, want it to name index.html", err)
	}
}

// countingFS counts every read of every name so a test can prove how often the
// document is fetched.
type countingFS struct {
	inner fs.FS
	reads map[string]int
}

func (c *countingFS) Open(name string) (fs.File, error) {
	c.reads[name]++
	return c.inner.Open(name)
}

func (c *countingFS) ReadFile(name string) ([]byte, error) {
	c.reads[name]++
	return fs.ReadFile(c.inner, name)
}

// TestStaticReadsIndexOnce pins the boot-cached document: many requests across
// many paths must never re-read index.html.
func TestStaticReadsIndexOnce(t *testing.T) {
	fsys := &countingFS{inner: staticFixture(), reads: map[string]int{}}
	s, err := NewStatic(fsys)
	if err != nil {
		t.Fatalf("NewStatic: %v", err)
	}
	if got := fsys.reads["index.html"]; got != 1 {
		t.Fatalf("NewStatic read index.html %d times, want exactly 1", got)
	}

	paths := []string{"/", "/index.html", "/admin", "/screen/deep/link", "/assets/app.js"}
	for _, p := range paths {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
	}

	if got := fsys.reads["index.html"]; got != 1 {
		t.Errorf("index.html was read %d times over %d requests, want exactly 1 (construction only)",
			got, len(paths))
	}
}
