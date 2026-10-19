package catalog

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type LocalMedia struct {
	ImagesDir string
	Embedded  fs.FS
}

func (m LocalMedia) Readable(value string) bool {
	if !MediaPath(value) {
		return false
	}
	var file fs.File
	var err error
	if strings.HasPrefix(value, "/images/") {
		root, e := filepath.EvalSymlinks(m.ImagesDir)
		if e != nil {
			return false
		}
		root, e = filepath.Abs(root)
		if e != nil {
			return false
		}
		resolved, e := filepath.EvalSymlinks(filepath.Join(root, strings.TrimPrefix(value, "/images/")))
		if e != nil {
			return false
		}
		rel, e := filepath.Rel(root, resolved)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return false
		}
		// os.Root confines the actual open as well, including symlink races.
		confined, e := os.OpenRoot(root)
		if e != nil {
			return false
		}
		defer confined.Close()
		file, err = confined.Open(rel)
	} else {
		if m.Embedded == nil {
			return false
		}
		file, err = m.Embedded.Open(strings.TrimPrefix(value, "/"))
	}
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	b := make([]byte, 512)
	n, err := file.Read(b)
	if err != nil && err != io.EOF {
		return false
	}
	b = b[:n]
	content := http.DetectContentType(b)
	if strings.HasSuffix(strings.ToLower(value), ".svg") {
		return strings.Contains(string(b), "<svg")
	}
	return content == "image/png" || content == "image/jpeg" || content == "image/webp"
}
