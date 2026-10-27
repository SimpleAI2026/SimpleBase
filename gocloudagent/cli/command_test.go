package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdio 把 stdout/stderr 换成临时文件，返回读取函数。
func captureStdio(t *testing.T) (func() (string, string), *os.File, *os.File) {
	t.Helper()
	dir := t.TempDir()
	outF, err := os.Create(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	errF, err := os.Create(filepath.Join(dir, "err"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { outF.Close(); errF.Close() })
	read := func() (string, string) {
		ob, _ := os.ReadFile(outF.Name())
		eb, _ := os.ReadFile(errF.Name())
		return string(ob), string(eb)
	}
	return read, outF, errF
}

func TestRunCommandHelpAndVersion(t *testing.T) {
	read, out, errw := captureStdio(t)
	env := testEnv(t)

	if code := RunCommand(env, []string{"help"}, out, errw); code != exitOK {
		t.Fatalf("help code = %d", code)
	}
	stdout, _ := read()
	for _, want := range []string{"gocloudagent", "resume", "sessions", "config", "GOCLOUDAGENT_HOME"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help output missing %q", want)
		}
	}
	if code := RunCommand(env, []string{"version"}, out, errw); code != exitOK {
		t.Fatalf("version code = %d", code)
	}
}

// 未配置 endpoint/token 时给出明确错误与配置类退出码，而不是 panic。
func TestRunCommandFailsWithoutConfig(t *testing.T) {
	read, out, errw := captureStdio(t)
	env := testEnv(t)
	code := RunCommand(env, []string{"run", "hello"}, out, errw)
	if code != exitConfig {
		t.Fatalf("code = %d, want %d", code, exitConfig)
	}
	_, stderr := read()
	if !strings.Contains(stderr, "endpoint") {
		t.Fatalf("stderr = %q, want a hint about endpoint", stderr)
	}
}

func TestConfigSetGetAndPath(t *testing.T) {
	read, out, errw := captureStdio(t)
	env := testEnv(t)

	if code := RunCommand(env, []string{"config", "set", "endpoint", "http://127.0.0.1:9999"}, out, errw); code != exitOK {
		t.Fatalf("set code = %d", code)
	}
	if code := RunCommand(env, []string{"config", "set", "project_id", "p9"}, out, errw); code != exitOK {
		t.Fatalf("set project code = %d", code)
	}
	if code := RunCommand(env, []string{"config", "path"}, out, errw); code != exitOK {
		t.Fatalf("path code = %d", code)
	}
	stdout, _ := read()
	if !strings.Contains(stdout, filepath.Join(".gocloudagent", "config.json")) {
		t.Fatalf("path output = %q", stdout)
	}

	// 落盘的配置要能被 LoadConfig 读回，且 mode 0600。
	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Endpoint != "http://127.0.0.1:9999" || cfg.ProjectID != "p9" {
		t.Fatalf("cfg = %+v", cfg)
	}

	if code := RunCommand(env, []string{"config", "get", "endpoint"}, out, errw); code != exitOK {
		t.Fatalf("get code = %d", code)
	}
	stdout, _ = read()
	if !strings.Contains(stdout, "http://127.0.0.1:9999") {
		t.Fatalf("get output = %q", stdout)
	}
}

func TestConfigRejectsUnknownKey(t *testing.T) {
	_, out, errw := captureStdio(t)
	env := testEnv(t)
	if code := RunCommand(env, []string{"config", "set", "nope", "x"}, out, errw); code != exitUsage {
		t.Fatalf("code = %d, want usage", code)
	}
	if code := RunCommand(env, []string{"config", "get", "nope"}, out, errw); code != exitUsage {
		t.Fatalf("code = %d, want usage", code)
	}
}

// config show/json 必须掩码 token（避免截图或粘贴时泄漏）。
func TestConfigMasksToken(t *testing.T) {
	read, out, errw := captureStdio(t)
	env := testEnv(t)
	secret := "sk-1234567890abcdef"
	if _, err := SaveConfig(env, Config{Token: secret, Endpoint: "http://e", ProjectID: "p"}); err != nil {
		t.Fatal(err)
	}
	if code := RunCommand(env, []string{"config", "show"}, out, errw); code != exitOK {
		t.Fatalf("show code = %d", code)
	}
	stdout, _ := read()
	if strings.Contains(stdout, secret) {
		t.Fatalf("config show leaked the token: %q", stdout)
	}
	if !strings.Contains(stdout, "***") {
		t.Fatalf("config show should mask the token: %q", stdout)
	}
}

// 权限过宽的配置：所有 config 子命令都要报错而不是静默使用。
func TestConfigRejectsInsecureFileOnRead(t *testing.T) {
	read, out, errw := captureStdio(t)
	env := testEnv(t)
	path, err := SaveConfig(env, Config{Token: "sk-x", Endpoint: "http://e", ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	code := RunCommand(env, []string{"config", "show"}, out, errw)
	if code != exitConfig {
		t.Fatalf("code = %d, want config error", code)
	}
	_, stderr := read()
	if !strings.Contains(stderr, "config_insecure") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestSessionsListShowRemove(t *testing.T) {
	read, out, errw := captureStdio(t)
	env := testEnv(t)

	// 空目录：list 输出提示而不是报错。
	if code := RunCommand(env, []string{"sessions", "list"}, out, errw); code != exitOK {
		t.Fatalf("empty list code = %d", code)
	}
	stdout, _ := read()
	if !strings.Contains(stdout, "no sessions") {
		t.Fatalf("stdout = %q", stdout)
	}

	s, err := NewSession(env, Config{Endpoint: "http://e", ProjectID: "p"}, "/tmp/w")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage("user", "hello world"); err != nil {
		t.Fatal(err)
	}
	if code := RunCommand(env, []string{"sessions", "list"}, out, errw); code != exitOK {
		t.Fatalf("list code = %d", code)
	}
	stdout, _ = read()
	if !strings.Contains(stdout, s.ID) {
		t.Fatalf("list should include the session id: %q", stdout)
	}

	if code := RunCommand(env, []string{"sessions", "show", s.ID}, out, errw); code != exitOK {
		t.Fatalf("show code = %d", code)
	}
	stdout, _ = read()
	if !strings.Contains(stdout, "hello world") {
		t.Fatalf("show output = %q", stdout)
	}

	if code := RunCommand(env, []string{"sessions", "rm", s.ID}, out, errw); code != exitOK {
		t.Fatalf("rm code = %d", code)
	}
	if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
		t.Fatalf("session file should be gone: %v", err)
	}
}

func TestSessionsMissingIDIsUsageError(t *testing.T) {
	_, out, errw := captureStdio(t)
	env := testEnv(t)
	for _, args := range [][]string{
		{"sessions", "show"},
		{"sessions", "rm"},
		{"resume"},
	} {
		if code := RunCommand(env, args, out, errw); code != exitUsage {
			t.Fatalf("%v code = %d, want usage", args, code)
		}
	}
}

// 空前缀不能匹配任意会话（否则 rm "" 会删错文件）。
func TestSessionsRemoveEmptyPrefixIsRejected(t *testing.T) {
	_, out, errw := captureStdio(t)
	env := testEnv(t)
	if _, err := NewSession(env, Config{}, "/w"); err != nil {
		t.Fatal(err)
	}
	if code := RunCommand(env, []string{"sessions", "rm", ""}, out, errw); code != exitConfig {
		t.Fatalf("code = %d, want config error", code)
	}
	sessions, err := ListSessions(env)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("session should survive: %+v err=%v", sessions, err)
	}
}

// 位置参数与 -p 都作为 prompt；--json/--yolo 被识别。
func TestParseFlags(t *testing.T) {
	got := parseFlags(Options{}, []string{"-p", "explain this", "--json", "--yolo"})
	if got.Prompt != "explain this" || !got.JSON || !got.Yolo || !got.Headless {
		t.Fatalf("got %+v", got)
	}
	got = parseFlags(Options{}, []string{"--session", "abc", "--cwd", "/tmp"})
	if got.SessionID != "abc" || got.Cwd != "/tmp" {
		t.Fatalf("got %+v", got)
	}
	got = parseFlags(Options{}, []string{"-p", "a", "b", "c"})
	if got.Prompt != "a b c" {
		t.Fatalf("prompt = %q", got.Prompt)
	}
}

// 配置只写在 ~/.gocloudagent/config.json 里时，run 也必须能用——不能要求用户再导一遍环境变量。
func TestRunReadsConfigFile(t *testing.T) {
	read, out, errw := captureStdio(t)
	env := testEnv(t)
	if _, err := SaveConfig(env, Config{Endpoint: "http://127.0.0.1:1", Token: "t", ProjectID: "p"}); err != nil {
		t.Fatal(err)
	}
	code := RunCommand(env, []string{"run", "-p", "hi"}, out, errw)
	if code == exitConfig {
		_, stderr := read()
		t.Fatalf("config file should be enough, got exitConfig: %s", stderr)
	}
}

// 权限过宽的配置文件在 run 路径上也要 fail-closed（不能静默退回 env）。
func TestRunRejectsInsecureConfigFile(t *testing.T) {
	read, out, errw := captureStdio(t)
	env := testEnv(t)
	path, err := SaveConfig(env, Config{Endpoint: "http://127.0.0.1:1", Token: "t", ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	code := RunCommand(env, []string{"run", "-p", "hi"}, out, errw)
	if code != exitConfig {
		t.Fatalf("code = %d, want %d", code, exitConfig)
	}
	_, stderr := read()
	if !strings.Contains(stderr, "config_insecure") {
		t.Fatalf("stderr = %q", stderr)
	}
}

// 非 TTY 且无 prompt 时，从 stdin 读取（管道用法）。
func TestRunReadsPromptFromStdin(t *testing.T) {
	_, out, errw := captureStdio(t)
	env := testEnv(t)
	// 只配置了 endpoint/token，模型调用会失败——但足以证明它读到了 prompt
	// 并走到了网络阶段（错误码不是 usage/no prompt）。
	opts := Options{
		Env: env, Headless: true,
		Config: Config{Endpoint: "http://127.0.0.1:1", Token: "t", ProjectID: "p"},
		Stdin:  strings.NewReader("piped question"),
		Stdout: out, Stderr: errw,
	}
	code := Run(opts)
	if code == exitUsage {
		t.Fatalf("piped prompt should not be treated as missing (code=%d)", code)
	}
}
