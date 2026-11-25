// 文件说明：run_test.go — internal/agentrun 的单元测试（TestAgentRunMetricsAuditOmitsConversation 等）
//
// 用法
//
//	go test ./internal/agentrun -run 'TestAgentRunMetricsAuditOmitsConversation'
//	同包调用：openAgentStore() → plan_tools_test.go, shutdown_test.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/run_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] confirm.go, cron.go, errors.go, group_engine.go, group_runner.go 等 17 个
//	[关键引用] openAgentStore ← plan_tools_test.go, shutdown_test.go
//
// 主要符号
//
//	var memoryDBSeq
//	openMemoryDB(t *testing.T) *sql.DB
//	openAgentStore(t *testing.T) *systemdb.Store
//	type fakeAudit struct{…}
//	(f *fakeAudit) Record(_ context.Context, e audit.Event) error
//	TestAgentRunMetricsAuditOmitsConversation(t *testing.T)

package agentrun

// run_test.go：固定助手会话 run 用例的测试夹具与收尾审计用例（自 internal/api/agent_optimization_test.go 随实现迁入）。

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

var memoryDBSeq atomic.Uint64

// openMemoryDB 打开本测试独占的 sqlite 内存库（shared cache：流式 run 占着第一条连接时，
// 轮询另开连接也能看到刚写入的行）。
func openMemoryDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:agentrun%d?mode=memory&cache=shared", memoryDBSeq.Add(1))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxIdleConns(4)
	pin, err := db.Conn(context.Background())
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pin.Close()
		_ = db.Close()
	})
	return db
}

// openAgentStore 打开带系统 schema 的内存系统库。
func openAgentStore(t *testing.T) *systemdb.Store {
	t.Helper()
	db := openMemoryDB(t)
	if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return systemdb.NewStoreForTest(db)
}

// fakeAudit 记录最后一条审计事件。
type fakeAudit struct {
	last audit.Event
}

func (f *fakeAudit) Record(_ context.Context, e audit.Event) error {
	f.last = e
	return nil
}

// 收尾审计只记元数据：不落 LLM 正文；run 行回填用量。
func TestAgentRunMetricsAuditOmitsConversation(t *testing.T) {
	store := openAgentStore(t)
	thread, err := store.CreateAgentThread(context.Background(), systemdb.AgentThread{ProjectID: "testprj1", Title: "title"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateAgentRun(context.Background(), systemdb.AgentRun{ID: "audit-test-run", ThreadID: thread.ID, ProjectID: "testprj1", AgentID: "agent", InitiatorID: "user:u1"})
	if err != nil {
		t.Fatal(err)
	}
	aud := &fakeAudit{}
	s := NewService(Config{}, Ports{Runs: store, Audit: aud}, nil, nil)
	res := cloudagent.RunResult{Content: "private conversation", ToolCallsJSON: "[]", DurationMS: 500, PromptTokens: 12, CompletionTokens: 3, ToolCalls: 1}
	if err := s.finishRun(context.Background(), run, systemdb.CloudAgent{ID: "agent"}, res, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(aud.last.Detail, res.Content) || aud.last.Kind != "agent.run" ||
		aud.last.PrincipalID != "user:u1" || !strings.Contains(aud.last.Detail, "prompt_tokens=12") {
		t.Fatalf("audit=%+v", aud.last)
	}
	row, err := store.GetAgentRun(context.Background(), "testprj1", run.ID)
	if err != nil || row.DurationMS != 500 || row.CompletionTokens != 3 || row.ToolCalls != 1 || row.Status != systemdb.AgentRunCompleted {
		t.Fatalf("run=%+v err=%v", row, err)
	}
}
