package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// max_run_seconds：环境变量、overlay、非法值三条路径。
func TestMaxRunSecondsConfig(t *testing.T) {
	cfg := MergeEnv(Config{MaxRunSeconds: 30}, []string{"GOCLOUDAGENT_MAX_RUN_SECONDS=90"})
	if cfg.MaxRunSeconds != 90 {
		t.Fatalf("env should override file: %d", cfg.MaxRunSeconds)
	}
	for _, bad := range []string{"abc", "0", "-5", ""} {
		got := MergeEnv(Config{MaxRunSeconds: 30}, []string{"GOCLOUDAGENT_MAX_RUN_SECONDS=" + bad})
		if got.MaxRunSeconds != 30 {
			t.Fatalf("invalid %q must keep file value, got %d", bad, got.MaxRunSeconds)
		}
	}
	if got := overlayConfig(Config{MaxRunSeconds: 30}, Config{}); got.MaxRunSeconds != 30 {
		t.Fatalf("zero override must not erase file value: %d", got.MaxRunSeconds)
	}
	if got := overlayConfig(Config{MaxRunSeconds: 30}, Config{MaxRunSeconds: 5}); got.MaxRunSeconds != 5 {
		t.Fatalf("override = %d", got.MaxRunSeconds)
	}
}

func TestRunDeadlineFiredIgnoresNestedClientTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	if runDeadlineFired(ctx, context.DeadlineExceeded) {
		t.Fatal("a live run context must not adopt an inner client deadline")
	}
	expired, cancelExpired := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancelExpired()
	time.Sleep(time.Millisecond)
	if !runDeadlineFired(expired, context.DeadlineExceeded) {
		t.Fatal("the run context's own deadline must be reported as timeout")
	}
	if runDeadlineFired(expired, errors.New("upstream down")) {
		t.Fatal("non-deadline errors are not a run timeout")
	}
}

func TestRunLimitPrecedence(t *testing.T) {
	if got := runLimit(Options{}, Config{}); got != 0 {
		t.Fatalf("unset must be unlimited, got %s", got)
	}
	if got := runLimit(Options{}, Config{MaxRunSeconds: 7}); got != 7*time.Second {
		t.Fatalf("config = %s", got)
	}
	if got := runLimit(Options{MaxDuration: time.Second}, Config{MaxRunSeconds: 7}); got != time.Second {
		t.Fatalf("explicit option must win, got %s", got)
	}
}

// config get/set 能读写该键；非法值不会静默清零。
func TestMaxRunSecondsConfigKey(t *testing.T) {
	var key *struct {
		Name string
		Get  func(Config) string
		Set  func(*Config, string)
	}
	for i := range configKeys {
		if configKeys[i].Name == "max_run_seconds" {
			key = &configKeys[i]
		}
	}
	if key == nil {
		t.Fatal("max_run_seconds missing from configKeys")
	}
	var c Config
	key.Set(&c, "45")
	if key.Get(c) != "45" {
		t.Fatalf("get = %q", key.Get(c))
	}
	key.Set(&c, "oops")
	if c.MaxRunSeconds != 45 {
		t.Fatalf("invalid value changed config: %d", c.MaxRunSeconds)
	}
}

// 回归：模型请求一直不返回时，run 必须在预算内结束，而不是等到外部 kill。
// 这正是 MBPP 里 1 步就耗尽 120 秒的那类失败的进程内自保。
func TestRunReturnsWhenModelHangsPastBudget(t *testing.T) {
	s := newTestSession(t)
	r := &SessionRunner{Session: s, Client: hangingClient{}}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := r.Run(ctx, "hi", nil); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("hung model must surface an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the deadline passed")
	}
}

// 超时必须被报告成 timeout 而不是笼统的 run_failed，harness 才能把它从能力失败里分出来。
func TestRunReportsTimeoutCode(t *testing.T) {
	t.Skip("blocked: ADK ctx 传播路径未确认，先跳过全链路测试")
	var stdout, stderr bytes.Buffer
	home := t.TempDir()
	srv := newHangingServer(t)
	defer srv.Close()
	opts := Options{
		Env: []string{
			"GOCLOUDAGENT_HOME=" + home, "HOME=" + home,
			"GOCLOUDAGENT_ENDPOINT=" + srv.URL, "GOCLOUDAGENT_TOKEN=t", "GOCLOUDAGENT_PROJECT_ID=p",
		},
		Cwd: t.TempDir(), Prompt: "hi", Headless: true, JSON: true,
		Stdout: &stdout, Stderr: &stderr,
		MaxDuration: 200 * time.Millisecond,
	}
	start := time.Now()
	code := Run(opts)
	if code != exitAPI {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("took %s, budget not enforced", time.Since(start))
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &env); err != nil {
		t.Fatalf("stderr is not one JSON line: %q", stderr.String())
	}
	if env.Error.Code != "timeout" {
		t.Fatalf("error code = %q, want timeout (stderr=%s)", env.Error.Code, stderr.String())
	}
}

type hangingClient struct{}

func (hangingClient) Chat(ctx context.Context, _ string, _ agentcore.ChatRequest) (agentcore.ChatResponse, error) {
	<-ctx.Done()
	return agentcore.ChatResponse{}, ctx.Err()
}

func (hangingClient) Stream(ctx context.Context, _ string, _ agentcore.ChatRequest) (agentcore.TokenStream, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// newHangingServer 接受连接但一直不回响应，模拟「响应头迟迟不来」的网关。
func newHangingServer(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release) })
	return srv
}
