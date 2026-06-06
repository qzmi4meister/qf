package embeddedui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// FileServer returns an http.Handler serving the embedded SPA.
// Unknown paths fall back to index.html for client-side routing.
func FileServer() http.Handler {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return &spaHandler{fs: dist}
}

type spaHandler struct {
	fs fs.FS
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	f, err := h.fs.Open(path)
	if err != nil {
		// SPA fallback: serve index.html for any unknown path
		http.ServeFileFS(w, r, h.fs, "index.html")
		return
	}
	f.Close()

	http.FileServerFS(h.fs).ServeHTTP(w, r)
}
