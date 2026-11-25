// 文件说明：agent_shutdown_test.go — internal/app 的单元测试（TestAgentRunShutdownCancelsAndDrains 等）
//
// 用法
//
//	go test ./internal/app -run 'TestAgentRunShutdownCancelsAndDrains'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/agent_shutdown_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] app.go, app_alerts.go, app_lease.go, app_test.go, arch_gate_test.go 等 41 个
//
// 主要符号
//
//	TestAgentRunShutdownCancelsAndDrains(t *testing.T)

package app

// agent_shutdown_test.go：D6 的 app 级关停用例（internal-ddd-hexagonal-implementation-plan §8.1）。
// 在途 run 遇到 App.Shutdown 必须被取消并等到收尾落库，且整个关停不被在途 LLM 调用拖满预算。

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// TestAgentRunShutdownCancelsAndDrains：模型调用永不返回（NeverFinish）的流式 run 在途时执行 Shutdown：
//   - Shutdown 在远小于 http.shutdown_timeout 的时间内返回（先取消、再关 HTTP，见 App.Shutdown 第 0 步）；
//   - run 以 context.Canceled 结束且收尾落库成功（PersistErr 为空），即落 canceled 终态；
//   - 停机后应用服务拒绝新建 run。
func TestAgentRunShutdownCancelsAndDrains(t *testing.T) {
	// 伪上游：输出一帧后永久阻塞，模拟卡住的 LLM 调用。
	fake := fakellm.New([]fakellm.Script{{NeverFinish: true}}, fakellm.Script{NeverFinish: true})
	defer fake.Close()
	cfg := llmTestConfig(t, map[string]config.ProviderConfig{"openai": {
		APIKey: "test-key", BaseURL: fake.URL + "/v1", DefaultModel: "fake-model", AllowedModels: []string{"fake-model"},
	}})
	a := newLLMApp(t, cfg)
	ctx := context.Background()

	if err := a.systemStore.SeedDefaultCloudAgents(ctx, "testprj1"); err != nil {
		t.Fatal(err)
	}
	thread, err := a.systemStore.CreateAgentThread(ctx, systemdb.AgentThread{ProjectID: "testprj1", Title: "shutdown"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.agentRun.StartDirectRun(ctx, agentrun.DirectRunInput{
		ProjectID: "testprj1", Thread: thread, Principal: auth.Principal{UserID: "u1"}, Content: "work",
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	outcomes := make(chan agentrun.RunOutcome, 1)
	go func() { outcomes <- a.agentRun.ExecuteRun(nil, d, nil) }()

	// 等模型调用真正进入伪上游，再开始关停。
	deadline := time.Now().Add(10 * time.Second)
	for len(fake.Requests()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(fake.Requests()) == 0 {
		t.Fatal("model was not called")
	}

	started := time.Now()
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Shutdown(sctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= 5*time.Second {
		t.Fatalf("Shutdown took %s: in-flight run must be cancelled first, not waited out", elapsed)
	}
	select {
	case out := <-outcomes:
		if !errors.Is(out.RunErr, context.Canceled) {
			t.Fatalf("run error = %v, want context.Canceled", out.RunErr)
		}
		if out.PersistErr != nil {
			t.Fatalf("finish must be persisted before resources close: %v", out.PersistErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after shutdown")
	}
	if !a.agentRun.Capabilities().Stopping {
		t.Fatal("service must be stopping after Shutdown")
	}
	if _, err := a.agentRun.StartDirectRun(ctx, agentrun.DirectRunInput{ProjectID: "testprj1", Thread: thread, Content: "again"}); !errors.Is(err, agentrun.ErrStopping) {
		t.Fatalf("StartDirectRun after shutdown = %v, want ErrStopping", err)
	}
}
