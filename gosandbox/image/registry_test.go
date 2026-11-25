// 文件说明：registry_test.go — gosandbox/image 的单元测试（TestParseChallenge、TestParseChallengeEmpty 等）
//
// 用法
//
//	go test ./gosandbox/image -run 'TestParseChallenge|TestParseChallengeEmpty|TestTokenScopeReducedToPull|TestTokenCached'
//
// 调用架构
//
//	[上游] gosandbox/vm/box, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/registry_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] arch.go, import.go, manifest.go, manifest_test.go, pull.go 等 16 个
//
// 主要符号
//
//	TestParseChallenge(t *testing.T)
//	TestParseChallengeEmpty(t *testing.T)
//	TestTokenScopeReducedToPull(t *testing.T)
//	TestTokenCached(t *testing.T)
//	TestTokenRefusesPlainHTTPRealm(t *testing.T)
//	TestIsLoopback(t *testing.T)
//	TestEncodeAuth(t *testing.T)
//	TestReadLimited(t *testing.T)
//	... 另有 1 个顶层符号

package image

// registry_test.go：不联网也能测的部分——challenge 解析、凭据策略、token 缓存。
//
// 联网的拉取路径由 pull_test.go（用本地假 registry）覆盖。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// challenge 解析：realm / service / scope，含引号里带逗号的 scope。
func TestParseChallenge(t *testing.T) {
	got := parseChallenge(`Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/alpine:pull,push"`)
	if got["realm"] != "https://auth.docker.io/token" {
		t.Fatalf("realm = %q", got["realm"])
	}
	if got["service"] != "registry.docker.io" {
		t.Fatalf("service = %q", got["service"])
	}
	// scope 里的逗号不能被切开。
	if got["scope"] != "repository:library/alpine:pull,push" {
		t.Fatalf("scope = %q", got["scope"])
	}
}

// 空的 challenge 不 panic。
func TestParseChallengeEmpty(t *testing.T) {
	if got := parseChallenge(""); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if got := parseChallenge("Bearer"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

// scope 被缩成只 pull：不该向 registry 申请写权限。
func TestTokenScopeReducedToPull(t *testing.T) {
	var gotScope string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotScope = r.URL.Query().Get("scope")
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "tok", "expires_in": 300})
	}))
	defer srv.Close()

	c := NewClient()
	ref, _ := Parse("alpine")
	// realm 用 loopback：允许明文 HTTP 带凭据。
	_, err := c.token(context.Background(), ref, `Bearer realm="`+srv.URL+`",service="s"`)
	if err != nil {
		t.Fatal(err)
	}
	if gotScope != "repository:library/alpine:pull" {
		t.Fatalf("scope = %q", gotScope)
	}
}

// token 被缓存：第二次不再请求 token 端点。
func TestTokenCached(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "tok", "expires_in": 300})
	}))
	defer srv.Close()

	c := NewClient()
	ref, _ := Parse("ghcr.io/o/r")
	challenge := `Bearer realm="` + srv.URL + `"`
	if _, err := c.token(context.Background(), ref, challenge); err != nil {
		t.Fatal(err)
	}
	if _, err := c.token(context.Background(), ref, challenge); err != nil {
		t.Fatal(err)
	}
	// 缓存命中：第二次走 cachedToken，不再打 token 端点。
	if _, ok := c.cachedToken(ref); !ok {
		t.Fatal("token must be cached")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("token endpoint hit %d times, want 1", hits)
	}
}

// 明文 HTTP 的 realm 只在 loopback 上允许带凭据。
func TestTokenRefusesPlainHTTPRealm(t *testing.T) {
	c := NewClient()
	c.SetCredentials("example.com", Credentials{Username: "u", Password: "p"})
	ref, _ := Parse("example.com/o/r")
	_, err := c.token(context.Background(), ref, `Bearer realm="http://example.com/token"`)
	if err == nil || !strings.Contains(err.Error(), "refusing credentials") {
		t.Fatalf("err = %v", err)
	}
}

// loopback 允许明文（本地假 registry 用）。
func TestIsLoopback(t *testing.T) {
	for _, h := range []string{"localhost", "127.0.0.1", "127.1.2.3", "::1", "[::1]"} {
		if !isLoopback(h) {
			t.Errorf("%q must be loopback", h)
		}
	}
	for _, h := range []string{"example.com", "10.0.0.1", "ghcr.io"} {
		if isLoopback(h) {
			t.Errorf("%q must not be loopback", h)
		}
	}
}

// EncodeAuth 生成 docker 认的 base64 JSON。
func TestEncodeAuth(t *testing.T) {
	enc := EncodeAuth("ghcr.io", Credentials{Username: "u", Password: "p"})
	body, err := base64.URLEncoding.DecodeString(enc)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["username"] != "u" || got["serveraddress"] != "ghcr.io" {
		t.Fatalf("auth = %v", got)
	}
}

// readLimited 在超出上限时报错，而不是静默截断。
func TestReadLimited(t *testing.T) {
	if _, err := readLimited(strings.NewReader("0123456789"), 5); err == nil {
		t.Fatal("oversized body must be refused")
	}
	body, err := readLimited(strings.NewReader("0123"), 5)
	if err != nil || string(body) != "0123" {
		t.Fatalf("body = %q, %v", body, err)
	}
}

// digestOf 与 store 的摘要一致。
func TestDigestOf(t *testing.T) {
	a, _ := digestOf([]byte("x"))
	b, _ := digestOf([]byte("x"))
	if a != b || len(a) != len("sha256:")+64 {
		t.Fatalf("digest = %q / %q", a, b)
	}
	c, _ := digestOf([]byte("y"))
	if a == c {
		t.Fatal("different content must give different digests")
	}
}

var _ = time.Second
