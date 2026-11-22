// planv6.7 AGC-ROUTE：路由器纯函数测试（§7.12）。
// parseDispatchAnswer（合法/代码块/越界/none/垃圾）、buildDispatchPrompt 截断、
// fallbackAgent（有通用/无通用）、fake ChatClient 超时 → fallback。
package api

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func TestParseDispatchAnswer(t *testing.T) {
	cands := []dispatchCandidate{
		{ID: "a1", Name: "数据库助手", Module: "database"},
		{ID: "a2", Name: "对象存储助手", Module: "s3"},
	}
	cases := []struct {
		name, in string
		want     string
	}{
		{"legal", `{"agent_id":"a1"}`, "a1"},
		{"codeblock", "```json\n{\"agent_id\":\"a2\"}\n```", "a2"},
		{"none", `{"agent_id":"none"}`, ""},
		{"NONE-case", `{"agent_id":"NONE"}`, ""},
		{"outside", `{"agent_id":"a3"}`, ""},
		{"garbage", "我认为应该选数据库助手", ""},
		{"empty", "", ""},
		{"null id", `{"agent_id":""}`, ""},
	}
	for _, tc := range cases {
		if got := parseDispatchAnswer(tc.in, cands); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestBuildDispatchPromptTruncation(t *testing.T) {
	in := dispatchInput{
		UserText: strings.Repeat("长", 2500),
		Recent: []dispatchTurn{
			{Role: "user", Content: strings.Repeat("U", 300)},
			{Role: "assistant", AgentName: "数据库助手", Content: strings.Repeat("A", 300)},
		},
		Candidates: []dispatchCandidate{
			{ID: "a1", Name: "db", Module: "database", Description: strings.Repeat("D", 400)},
		},
	}
	msgs := buildDispatchPrompt(in)
	if len(msgs) != 2 || msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("shape: %+v", msgs)
	}
	// 用户正文截断到 2000 字符（含前缀行，至少不超过原始）
	if got := len([]rune(msgs[1].Content)); got > 3000 {
		t.Fatalf("user not truncated: %d", got)
	}
	// 候选简介截断
	if strings.Contains(msgs[0].Content, strings.Repeat("D", 300)) {
		t.Fatal("description not truncated")
	}
	// system 提示包含候选 JSON
	if !strings.Contains(msgs[0].Content, `"agent_id":"a1"`) {
		t.Fatalf("system missing candidates: %s", msgs[0].Content)
	}
}

func TestFallbackAgent(t *testing.T) {
	general := systemdb.CloudAgent{ID: "g", BuiltinKey: "general"}
	other := systemdb.CloudAgent{ID: "o"}
	// 有通用 → 通用
	if got := fallbackAgent([]systemdb.CloudAgent{other, general}); got.ID != "g" {
		t.Fatalf("general present: %+v", got)
	}
	// 无通用 → 第一个
	if got := fallbackAgent([]systemdb.CloudAgent{other}); got.ID != "o" {
		t.Fatalf("no general: %+v", got)
	}
	// 空 → 零值
	if got := fallbackAgent(nil); got.ID != "" {
		t.Fatalf("empty: %+v", got)
	}
}

// fakeDispatcher 用于路由编排测试。
type fakeDispatcher struct {
	agentID string
	err     error
	calls   int
	delay   time.Duration
	// planRaw 是 RoutePlan 的返回（v2 多任务）；planErr 优先于 err。
	planRaw   string
	planErr   error
	planCalls int
	// lastPlanInput 供断言群路由提示词/候选是否正确传入。
	lastPlanInput cloudagent.CollabDispatchInput
}

func (f *fakeDispatcher) Route(ctx context.Context, projectID string, in dispatchInput) (string, error) {
	f.calls++
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return f.agentID, f.err
}

func (f *fakeDispatcher) RoutePlan(ctx context.Context, projectID string, in cloudagent.CollabDispatchInput) (string, error) {
	f.planCalls++
	f.lastPlanInput = in
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.planErr != nil {
		return "", f.planErr
	}
	return f.planRaw, nil
}

// fakeChatClient 实现 agentcore.ChatClient（超时 → fallback 用例）。
type fakeChatClient struct {
	err error
}

func (f *fakeChatClient) Chat(ctx context.Context, projectID string, req agentcore.ChatRequest) (agentcore.ChatResponse, error) {
	if f.err != nil {
		return agentcore.ChatResponse{}, f.err
	}
	// 模拟慢上游：等 ctx 取消
	<-ctx.Done()
	return agentcore.ChatResponse{}, ctx.Err()
}

func (f *fakeChatClient) Stream(ctx context.Context, projectID string, req agentcore.ChatRequest) (agentcore.TokenStream, error) {
	return nil, errors.New("not implemented")
}

func TestLLMDispatcherTimeoutFallsBack(t *testing.T) {
	d := newLLMDispatcher(&fakeChatClient{})
	if d == nil {
		t.Fatal("dispatcher should be created")
	}
	// fakeChatClient.Chat 阻塞到 ctx 取消 → Route 返回 err（超时）
	picked, err := d.Route(context.Background(), "p1", dispatchInput{UserText: "hello"})
	if err == nil {
		t.Fatalf("expected timeout error, got %q", picked)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
}

func TestNewLLMDispatcherNilSafe(t *testing.T) {
	if newLLMDispatcher(nil) != nil {
		t.Fatal("nil client must return nil dispatcher")
	}
}
