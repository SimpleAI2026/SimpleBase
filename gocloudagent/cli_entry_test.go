// 文件说明：cli_entry_test.go — gocloudagent 的单元测试（TestCLIFacadeIsWired 等）
//
// 用法
//
//	go test ./gocloudagent -run 'TestCLIFacadeIsWired'
//
// 调用架构
//
//	[上游] gocloudagent/cmd
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli_entry_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] cli_entry.go
//
// 主要符号
//
//	TestCLIFacadeIsWired(t *testing.T)

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
