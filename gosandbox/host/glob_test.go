package host

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func denyRe(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	expr, err := denyGlobRegex(pattern, nil)
	if err != nil {
		t.Fatalf("denyGlobRegex(%q): %v", pattern, err)
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		t.Fatalf("compile %q: %v", expr, err)
	}
	return re
}

func TestGlobMatching(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/w/**/.env", "/w/.env", true},
		{"/w/**/.env", "/w/a/.env", true},
		{"/w/**/.env", "/w/a/b/.env", true},
		{"/w/**/.env", "/w/a/b/.envx", false},
		{"/w/**/.env", "/w/a/b/.env/key", true}, // 子树一并拒绝
		{"/w/*.pem", "/w/a.pem", true},
		{"/w/*.pem", "/w/sub/a.pem", false}, // * 不跨段
		{"/w/a?c", "/w/abc", true},
		{"/w/a?c", "/w/a/c", false},
		{"/w/f[0-9]", "/w/f5", true},
		{"/w/f[0-9]", "/w/fx", false},
		{"/w/x[abc]y", "/w/xby", true},
		{"/w/dir/**", "/w/dir/a/b", true},
		{"/w/a.b*", "/w/a.bc", true},
		{"/w/a.b*", "/w/aXbc", false}, // . 必须转义
	}
	for _, tc := range tests {
		if got := denyRe(t, tc.pattern).MatchString(tc.path); got != tc.want {
			t.Errorf("%q vs %q = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

// 拼不出集合的方括号是字面文本（srt glob-pattern.ts 的约定）。
func TestGlobBracketsAsText(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/w/[user-id]/x*", "/w/[user-id]/xa", true}, // r-i 倒置
		{"/w/[user-id]/x*", "/w/u/xa", false},
		{"/w/a[b*", "/w/a[bc", true}, // 没有 ]
		{"/w/a[b*", "/w/ab", false},
		{"/w/[]*", "/w/[]x", true}, // 空集合
	}
	for _, tc := range tests {
		if got := denyRe(t, tc.pattern).MatchString(tc.path); got != tc.want {
			t.Errorf("%q vs %q = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestParseGlobCollapsesWildcards(t *testing.T) {
	// 连续通配符合并，避免 SBPL 正则回溯爆炸。
	toks := parseGlob("*****" + "/" + "*" + "*" + "/" + "**" + "/" + "**" + "/" + "x")
	if len(toks) > 6 {
		t.Fatalf("wildcards must collapse, got %d tokens: %+v", len(toks), toks)
	}
	long := ""
	for i := 0; i < 64; i++ {
		long += "*/"
	}
	if _, err := denyGlobRegex("/w/"+long+"x", nil); err != nil {
		t.Fatalf("long pattern must compile: %v", err)
	}
}

func TestValidateDenyEntry(t *testing.T) {
	tests := []struct {
		entry   string
		wantErr bool
	}{
		{"/home/u/.ssh", false},
		{"/home/u/**/.env", false},
		{"/home/u/*/", true},    // glob 以分隔符结尾
		{"/home/u/dir/", false}, // 字面路径末尾 / 沿用旧行为
		{"relative/*.pem", true},
		{"literal:/w/a*b", false},
		{"literal:rel", true},
		{"/w/[user-id]/*", false},
	}
	for _, tc := range tests {
		err := validateDenyEntry(tc.entry)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateDenyEntry(%q) err=%v, wantErr=%v", tc.entry, err, tc.wantErr)
		}
	}
}

func TestDeniesReadGlob(t *testing.T) {
	p := basePolicy()
	p.DenyRead = []string{"/work/**/.env", "/work/*.pem", "literal:/work/a*b"}
	for path, want := range map[string]bool{
		"/work/.env":         true,
		"/work/x/y/.env":     true,
		"/work/x/.envrc":     false,
		"/work/k.pem":        true,
		"/work/sub/k.pem":    false,
		"/work/a*b":          true,
		"/work/a*b/inner":    true,
		"/work/axb":          false, // literal 不解释通配符
		"/work/new/dir/.env": true,  // 未来新建的文件也命中
	} {
		if got := DeniesRead(p, path); got != want {
			t.Errorf("DeniesRead(%q) = %v, want %v", path, got, want)
		}
	}
}

func makeTree(t *testing.T, files ...string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		full := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestExpandGlob(t *testing.T) {
	root := makeTree(t, ".env", "a/.env", "a/b/.env", "a/b/ok.txt", "k.pem", "sub/k.pem", "[WIP]/x")

	got, err := ExpandGlob(root + "/**/.env")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{root + "/.env", root + "/a/.env", root + "/a/b/.env"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("** expand = %v, want %v", got, want)
	}

	got, err = ExpandGlob(root + "/*.pem")
	if err != nil || fmt.Sprint(got) != fmt.Sprint([]string{root + "/k.pem"}) {
		t.Fatalf("* must not cross segments: %v err=%v", got, err)
	}

	got, err = ExpandGlob(root + "/[WIP]/x*") // [WIP] 是合法集合，不匹配名为 "[WIP]" 的目录
	if err != nil || len(got) != 0 {
		t.Fatalf("[WIP] is a set of W/I/P: %v err=%v", got, err)
	}

	got, err = ExpandGlob(root + "/nope/**/.env")
	if err != nil || len(got) != 0 {
		t.Fatalf("missing prefix must expand to nothing: %v err=%v", got, err)
	}
}

func TestExpandGlobStopsAtMatchedDir(t *testing.T) {
	root := makeTree(t, "secrets/a", "secrets/secrets/b")
	got, err := ExpandGlob(root + "/**/secrets")
	if err != nil || fmt.Sprint(got) != fmt.Sprint([]string{root + "/secrets"}) {
		t.Fatalf("matched dir must not be descended: %v err=%v", got, err)
	}
}

func TestExpandGlobLimit(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < MaxGlobMatches+1; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("k%04d.pem", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := ExpandGlob(root + "/*.pem")
	if !errors.Is(err, ErrGlobTooLarge) {
		t.Fatalf("want ErrGlobTooLarge, got %v", err)
	}
}

func TestBwrapArgsExpandsDenyGlob(t *testing.T) {
	root := makeTree(t, "a/.env", "b/c/.env", "keep.txt")
	p := basePolicy()
	p.DenyRead = []string{root + "/**/.env"}
	args, err := BwrapArgs(BwrapInput{
		Policy: p, Helper: "/usr/bin/gosandbox-inner", Argv: []string{"/bin/true"}, Home: "/home/u",
		Exists: func(s string) bool { _, e := os.Lstat(s); return e == nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{root + "/a/.env", root + "/b/c/.env"} {
		if idx(args, "--ro-bind", "/dev/null", f) < 0 {
			t.Errorf("%s must be masked, args=%v", f, args)
		}
	}
	if idx(args, "--ro-bind", "/dev/null", root+"/keep.txt") >= 0 {
		t.Error("non-matching file must stay readable")
	}
}

func TestBwrapArgsDenyGlobTooLarge(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < MaxGlobMatches+1; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("k%04d", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p := basePolicy()
	p.DenyRead = []string{root + "/*"}
	_, err := BwrapArgs(BwrapInput{Policy: p, Helper: "/usr/bin/gosandbox-inner", Argv: []string{"/bin/true"}, Home: "/home/u"})
	if !errors.Is(err, ErrGlobTooLarge) {
		t.Fatalf("over-limit glob must fail closed, got %v", err)
	}
}

func TestSeatbeltProfileDenyGlob(t *testing.T) {
	p := basePolicy()
	p.DenyRead = []string{"/work/**/.env", "/home/u/.ssh"}
	profile, params, err := seatbeltProfile(p, "/home/u", func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`\(deny file-read\* \(regex \(param "D0"\)\)\)`).MatchString(profile) {
		t.Errorf("glob must use regex rule:\n%s", profile)
	}
	if !regexp.MustCompile(`\(deny file-read\* \(subpath \(param "D1"\)\)\)`).MatchString(profile) {
		t.Errorf("literal must keep subpath rule:\n%s", profile)
	}
	var d0 string
	for _, kv := range params {
		if len(kv) > 3 && kv[:3] == "D0=" {
			d0 = kv[3:]
		}
	}
	if d0 == "" || !regexp.MustCompile(d0).MatchString("/work/a/b/.env") {
		t.Errorf("D0 regex %q must match nested .env", d0)
	}
}
