package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const hello = `package main

type Req struct{ A, B int }

func Add(r Req) int { return r.A + r.B }

func helper() {}
`

func exec(t *testing.T, stdin string, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(args, strings.NewReader(stdin), &o, &e)
	return code, o.String(), e.String()
}

func TestRunFromStdin(t *testing.T) {
	code, out, errOut := exec(t, hello, "run", "-func", "Add", "-body", `{"A":20,"B":22}`)
	if code != 0 || strings.TrimSpace(out) != "42" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestRunFromFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.go")
	if err := os.WriteFile(p, []byte(hello), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := exec(t, "", "run", "-f", p, "-func", "Add", "-body", `{"A":1,"B":2}`)
	if code != 0 || strings.TrimSpace(out) != "3" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestRunErrors(t *testing.T) {
	if code, _, _ := exec(t, hello, "run"); code != 2 {
		t.Fatalf("缺 -func 应为 2，得 %d", code)
	}
	if code, _, errOut := exec(t, hello, "run", "-func", "Nope"); code != 1 || !strings.Contains(errOut, "function not found") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if code, _, errOut := exec(t, hello, "run", "-func", "Add", "-body", "{bad"); code != 1 || !strings.Contains(errOut, "参数绑定失败") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if code, _, errOut := exec(t, "not go", "run", "-func", "Add"); code != 1 || !strings.Contains(errOut, "编译失败") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if code, _, _ := exec(t, "", "run", "-f", filepath.Join(t.TempDir(), "missing.go"), "-func", "Add"); code != 1 {
		t.Fatalf("读文件失败应为 1，得 %d", code)
	}
}

func TestCheck(t *testing.T) {
	code, out, errOut := exec(t, hello, "check")
	if code != 0 || !strings.Contains(out, "Add(") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	bad := "package main\nfunc Bad(a, b int) int { return a }\n"
	if code, _, errOut := exec(t, bad, "check"); code != 1 || errOut == "" {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}

func TestList(t *testing.T) {
	code, out, _ := exec(t, hello, "list")
	if code != 0 || strings.TrimSpace(out) != "Add" {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, out, _ = exec(t, hello, "list", "-all")
	if code != 0 || !strings.Contains(out, "helper") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if code, _, _ := exec(t, "bad", "list"); code != 1 {
		t.Fatalf("解析失败应为 1，得 %d", code)
	}
}

func TestPackages(t *testing.T) {
	code, out, _ := exec(t, "", "packages")
	if code != 0 || !strings.Contains(out, "encoding/json") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestUsage(t *testing.T) {
	if code, _, _ := exec(t, ""); code != 2 {
		t.Fatalf("无参应为 2，得 %d", code)
	}
	if code, _, _ := exec(t, "", "bogus"); code != 2 {
		t.Fatalf("未知子命令应为 2，得 %d", code)
	}
	if code, out, _ := exec(t, "", "help"); code != 0 || !strings.Contains(out, "用法") {
		t.Fatalf("help code=%d", code)
	}
}
