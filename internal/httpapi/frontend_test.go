package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFontsAreServedWithCrossOriginAccessForTheHostedSignInPage(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "fonts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fonts", "a.woff2"), []byte("font"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := New(Dependencies{WebDirectory: dir}).Handler()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/fonts/a.woff2", nil))
	if w.Code != http.StatusOK || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("font status %d, CORS %q", w.Code, w.Header().Get("Access-Control-Allow-Origin"))
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/index.html", nil))
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("only fonts should allow cross-origin reads")
	}
}
