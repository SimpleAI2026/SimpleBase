package gocloudagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIFacadeIsWired(t *testing.T) {
	dir := t.TempDir()
	out, err := os.Create(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	errf, err := os.Create(filepath.Join(dir, "err"))
	if err != nil {
		t.Fatal(err)
	}
	defer errf.Close()

	env := []string{"HOME=" + dir, "GOCLOUDAGENT_HOME=" + filepath.Join(dir, ".gocloudagent")}
	SetCLIVersion("test-version")
	if code := RunCLI(env, []string{"version"}, out, errf); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	body, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "test-version") {
		t.Fatalf("version output = %q, want the injected version", string(body))
	}
}
