package sbcli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocumentTransferCLIAndAgentExportBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/export") {
			_, _ = w.Write([]byte("{\"secret\":\"raw\"}\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"dry_run":false,"scanned":1,"imported":1}`))
	}))
	defer server.Close()
	env := []string{"HOME=" + t.TempDir(), "SIMPLEBASE_URL=" + server.URL, "SIMPLEBASE_TOKEN=token", "SIMPLEBASE_PROJECT_ID=p1"}
	code, out, errOut := runCLI(t, env, []string{"document", "import", "--db", "db", "--collection", "tasks"}, "{\"title\":\"one\"}\n")
	if code != exitOK || !strings.Contains(out, `"imported":1`) {
		t.Fatalf("import code=%d out=%s err=%s", code, out, errOut)
	}
	code, out, errOut = runCLI(t, env, []string{"document", "export", "--db", "db", "--collection", "tasks"}, "")
	if code != exitOK || out != "{\"secret\":\"raw\"}\n" {
		t.Fatalf("export code=%d out=%q err=%s", code, out, errOut)
	}
	code, _, errOut = runCLI(t, append(env, "SIMPLEBASE_AGENT_RUN=1"), []string{"document", "export", "--db", "db", "--collection", "tasks"}, "")
	if code != exitUsage || !strings.Contains(errOut, "agent_raw_export_disabled") {
		t.Fatalf("agent export code=%d err=%s", code, errOut)
	}
}
