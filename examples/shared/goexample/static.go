package goexample

import (
	"net/http"
	"path/filepath"
)

// WithAssets serves only the built files from a case directory on the same local origin.
func WithAssets(next http.Handler, caseDir string) http.Handler {
	files := http.FileServer(http.Dir(filepath.Join(caseDir, "dist")))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			switch r.URL.Path {
			case "/", "/index.html", "/app.js", "/style.css":
				w.Header().Set("X-Content-Type-Options", "nosniff")
				files.ServeHTTP(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
