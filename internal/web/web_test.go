package web

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestStaticFS(t *testing.T) {
	root := StaticFS()
	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embedded dist should contain .keep")
	}
	// 未构建：仅 .keep；已构建：含 index.html。两种状态均合法。
	if _, err := fs.Stat(root, ".keep"); err != nil {
		if _, err2 := fs.Stat(root, "index.html"); err2 != nil {
			t.Fatalf("expected .keep or built index.html: %v / %v", err, err2)
		}
	}
}

func TestRegisterUnbuilt(t *testing.T) {
	orig := indexHTML
	indexHTML = nil
	t.Cleanup(func() { indexHTML = orig })

	e := echo.New()
	Register(e)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "管理端未构建") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestRegisterBuiltSPAAndStatic(t *testing.T) {
	orig := indexHTML
	indexHTML = []byte("<html>spa</html>")
	t.Cleanup(func() { indexHTML = orig })

	e := echo.New()
	Register(e)

	cases := []struct {
		path       string
		wantStatus int
		wantBody   string
		wantCT     string
	}{
		{"/", http.StatusOK, "<html>spa</html>", "text/html"},
		{"/dashboard", http.StatusOK, "<html>spa</html>", "text/html"},
		{"/missing.js", http.StatusOK, "<html>spa</html>", "text/html"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("body=%q", rec.Body.String())
			}
			if tc.wantCT != "" && !strings.Contains(rec.Header().Get(echo.HeaderContentType), tc.wantCT) {
				t.Fatalf("ct=%q", rec.Header().Get(echo.HeaderContentType))
			}
		})
	}

	// Existing static file (.keep) is served by FileServer, not SPA fallback.
	req := httptest.NewRequest(http.MethodGet, "/.keep", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(".keep status=%d body=%q", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "Placeholder") && !strings.Contains(string(body), "embed") && len(body) == 0 {
		t.Fatalf(".keep body empty: %q", body)
	}
}

func TestLoadIndexHTMLAbsent(t *testing.T) {
	// 工作区已构建 UI 时 loadIndexHTML 返回真实 index.html；未构建返回 nil。
	got := loadIndexHTML()
	if got == nil {
		return
	}
	if !strings.Contains(string(got), "<html") && !strings.Contains(string(got), "<!doctype") {
		t.Fatalf("unexpected index.html: %q", got)
	}
}
