// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// staticHandler serves the built SPA from the public directory with the
// classic single-page fallback: real files are served directly, everything
// else (client-side routes) gets index.html. index.html is never cached so
// deploys take effect immediately; Vite's hashed assets are safe to cache
// hard.
func (a *App) staticHandler() http.Handler {
	fsys := os.DirFS(a.cfg.PublicDir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		if !fs.ValidPath(name) {
			http.NotFound(w, r)
			return
		}
		if info, err := fs.Stat(fsys, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else if name == "index.html" {
				w.Header().Set("Cache-Control", "no-cache")
				a.setHTMLHeaders(w)
			}
			http.ServeFileFS(w, r, fsys, name)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		a.setHTMLHeaders(w)
		http.ServeFileFS(w, r, fsys, "index.html")
	})
}
