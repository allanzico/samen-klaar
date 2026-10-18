package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func frontend(api http.Handler, directory string) http.Handler {
	root := strings.TrimSpace(directory)
	if root == "" {
		return api
	}
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		clean := filepath.Clean("/" + r.URL.Path)
		name := filepath.Join(root, strings.TrimPrefix(clean, string(filepath.Separator)))
		if info, err := os.Stat(name); err == nil && !info.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if strings.HasPrefix(r.URL.Path, "/fonts/") {
				// Open-licence fonts are also loaded by the hosted sign-in page, which is another origin.
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			files.ServeHTTP(w, r)
			return
		}
		if strings.Contains(filepath.Base(r.URL.Path), ".") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
}
