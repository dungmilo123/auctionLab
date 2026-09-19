package api

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Fixture names and markers. The PNG is generated rather than committed: the
// repo carries no binary fixture, and a real encoder output is what proves the
// handler serves bytes it did not author.
const (
	pngName      = "photo.PNG"
	textName     = "notes.txt"
	namelessName = "nameless"
	textMarker   = "auctionlab-images-text-marker"
	outsideName  = "outside.txt"
	outsideMark  = "auctionlab-images-outside-marker"
)

// writeImagesFixture builds a temporary images directory with a generated PNG,
// a text file and an extensionless file (whose type can only come from the
// sniffing fallback). It also drops a marker file in the PARENT of that
// directory, so a path that escaped the root would be visible in a body.
func writeImagesFixture(t *testing.T) (dir string, pngBytes []byte) {
	t.Helper()

	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, outsideName), []byte(outsideMark), 0o644); err != nil {
		t.Fatalf("writing the outside marker: %v", err)
	}
	dir = filepath.Join(parent, "images")
	if err := os.MkdirAll(filepath.Join(dir, "gallery"), 0o755); err != nil {
		t.Fatalf("creating the images directory: %v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 220, G: 30, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding the PNG fixture: %v", err)
	}
	pngBytes = buf.Bytes()
	if err := os.WriteFile(filepath.Join(dir, pngName), pngBytes, 0o644); err != nil {
		t.Fatalf("writing the PNG fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, textName), []byte(textMarker), 0o644); err != nil {
		t.Fatalf("writing the text fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, namelessName), []byte(textMarker), 0o644); err != nil {
		t.Fatalf("writing the extensionless fixture: %v", err)
	}
	// A file inside the nested directory, so a listing (if one ever leaked)
	// would have a name to print.
	if err := os.WriteFile(filepath.Join(dir, "gallery", "inner.txt"), []byte(textMarker), 0o644); err != nil {
		t.Fatalf("writing the nested fixture: %v", err)
	}
	return dir, pngBytes
}

// TestImagesServingContract is D-01's route contract against a real temporary
// directory: a configured file is delivered with the stdlib's content type and
// a no-cache directive, while the directory root, a nested directory and an
// absent name are ordinary 404s that enumerate nothing.
func TestImagesServingContract(t *testing.T) {
	dir, pngBytes := writeImagesFixture(t)
	h := NewImages(dir)

	cases := []struct {
		name        string
		path        string
		wantStatus  int
		wantBody    string // marker the body must contain ("" = skip)
		wantType    string // substring the Content-Type must contain
		wantCache   string // exact Cache-Control, "" = the header must be absent
		wantNotBody string // marker the body must NOT contain ("" = skip)
	}{
		{
			name: "a png is served byte for byte with its extension type", path: "/images/" + pngName,
			wantStatus: http.StatusOK, wantBody: string(pngBytes), wantType: "image/png", wantCache: "no-cache",
		},
		{
			name: "a text file is served as text/plain", path: "/images/" + textName,
			wantStatus: http.StatusOK, wantBody: textMarker, wantType: "text/plain", wantCache: "no-cache",
		},
		{
			name: "an extension with no mime mapping falls back to sniffing", path: "/images/" + namelessName,
			wantStatus: http.StatusOK, wantBody: textMarker, wantType: "text/plain", wantCache: "no-cache",
		},
		{
			name: "an absent file is a plain 404", path: "/images/absent.png",
			wantStatus: http.StatusNotFound, wantBody: "404 page not found", wantNotBody: textMarker,
		},
		{
			name: "the directory root without a slash is not a listing", path: "/images",
			wantStatus: http.StatusNotFound, wantNotBody: textName,
		},
		{
			name: "the directory root is not a listing", path: "/images/",
			wantStatus: http.StatusNotFound, wantNotBody: textName,
		},
		{
			name: "a nested directory is not a listing", path: "/images/gallery/",
			wantStatus: http.StatusNotFound, wantNotBody: "inner.txt",
		},
		{
			name: "a nested directory without a slash is not a listing", path: "/images/gallery",
			wantStatus: http.StatusNotFound, wantNotBody: "inner.txt",
		},
		{
			name: "a traversal attempt never reaches the parent directory", path: "/images/../" + outsideName,
			wantStatus: http.StatusNotFound, wantNotBody: outsideMark,
		},
		{
			name: "a name that only starts like the prefix is not served", path: "/images-not-really.png",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Errorf("GET %s: status = %d, want %d (body %q)", tc.path, rec.Code, tc.wantStatus, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != tc.wantCache {
				t.Errorf("GET %s: Cache-Control = %q, want %q", tc.path, got, tc.wantCache)
			}
			if tc.wantType != "" {
				if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, tc.wantType) {
					t.Errorf("GET %s: Content-Type = %q, want it to contain %q", tc.path, ct, tc.wantType)
				}
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("GET %s: body does not contain the expected %q content (%d bytes)", tc.path, tc.name, rec.Body.Len())
			}
			if tc.wantNotBody != "" && strings.Contains(rec.Body.String(), tc.wantNotBody) {
				t.Errorf("GET %s: body = %q, must not contain %q", tc.path, rec.Body.String(), tc.wantNotBody)
			}
		})
	}
}

// TestImagesDirectoryIsNeverAListing spells the listing rule out on its own:
// the response for a directory is the stdlib's 404 text and nothing else, so no
// file name under the folder can appear in it however the listing would have
// been formatted.
func TestImagesDirectoryIsNeverAListing(t *testing.T) {
	dir, _ := writeImagesFixture(t)
	h := NewImages(dir)

	for _, path := range []string{"/images", "/images/", "/images/gallery", "/images/gallery/"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		body := rec.Body.String()
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, rec.Code)
		}
		for _, name := range []string{pngName, textName, namelessName, "inner.txt"} {
			if strings.Contains(body, name) {
				t.Errorf("GET %s: body %q enumerates %q", path, body, name)
			}
		}
	}
}

// TestImagesServesTheConfiguredDirectoryNotTheCwd pins IMAGES_DIR's resolution
// as a whole: the env value reaches NewImages, a blank value falls back to the
// cwd-relative default, and neither can be replaced by whatever happens to sit
// in the process working directory.
func TestImagesServesTheConfiguredDirectoryNotTheCwd(t *testing.T) {
	configured, _ := writeImagesFixture(t)
	other, _ := writeImagesFixture(t)
	if err := os.WriteFile(filepath.Join(other, "other.txt"), []byte("auctionlab-images-other-marker"), 0o644); err != nil {
		t.Fatalf("writing the other fixture: %v", err)
	}

	get := func(m map[string]string) func(string) string {
		return func(name string) string { return m[name] }
	}

	t.Run("an unset variable resolves to the cwd-relative default", func(t *testing.T) {
		if got := ImagesDir(get(nil)); got != DefaultImagesDir {
			t.Errorf("ImagesDir(unset) = %q, want %q", got, DefaultImagesDir)
		}
	})

	t.Run("a blank or whitespace value is not a directory", func(t *testing.T) {
		for _, raw := range []string{"", "   ", "\n"} {
			if got := ImagesDir(func(string) string { return raw }); got != DefaultImagesDir {
				t.Errorf("ImagesDir(%q) = %q, want the default %q", raw, got, DefaultImagesDir)
			}
		}
	})

	t.Run("the override is served, and only from there", func(t *testing.T) {
		env := get(map[string]string{ImagesDirEnv: "  " + configured + "  "})
		dir := ImagesDir(env)
		if dir != configured {
			t.Fatalf("ImagesDir(IMAGES_DIR) = %q, want the trimmed %q", dir, configured)
		}
		h := NewImages(dir)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/images/"+textName, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /images/%s from the override: status = %d, want 200", textName, rec.Code)
		}

		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/images/other.txt", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET /images/other.txt: status = %d, want 404 — another directory's file must not be reachable", rec.Code)
		}
	})
}

// TestImagesMissingDirectoryIsNotABootFailure pins D-129's shape for the image
// folder: an empty or absent directory is a working handler that answers 404,
// not a construction error — the venue can start with no images at all.
func TestImagesMissingDirectoryIsNotABootFailure(t *testing.T) {
	h := NewImages(filepath.Join(t.TempDir(), "never-created"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/images/photo.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a missing images directory", rec.Code)
	}
}

// TestImagesHeadAndRangeKeepWorking pins the reason the handler delegates to
// http.FileServerFS instead of reading the file itself: the stdlib's HEAD,
// range and conditional-request handling stay intact on every served file.
func TestImagesHeadAndRangeKeepWorking(t *testing.T) {
	dir, pngBytes := writeImagesFixture(t)
	h := NewImages(dir)

	t.Run("HEAD answers the headers with no body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/images/"+pngName, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("HEAD status = %d, want 200", rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("HEAD body has %d bytes, want 0", rec.Body.Len())
		}
		if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(len(pngBytes)) {
			t.Errorf("Content-Length = %q, want %d", got, len(pngBytes))
		}
	})

	t.Run("a range request is served as a partial response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/images/"+pngName, nil)
		req.Header.Set("Range", "bytes=0-3")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusPartialContent {
			t.Fatalf("range status = %d, want 206 (body %q)", rec.Code, rec.Body.String())
		}
		if !bytes.Equal(rec.Body.Bytes(), pngBytes[:4]) {
			t.Errorf("range body = %d bytes, want the first %d bytes of the file", rec.Body.Len(), 4)
		}
	})
}
