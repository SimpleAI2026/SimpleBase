//go:build llm_integration

package api

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

const integrationModel = "deepseek-ai/DeepSeek-V4-Flash"

type integrationDB struct{ calls int }

func (f *integrationDB) ListDatabases(context.Context, auth.Principal, string) ([]cloudagent.DatabaseInfo, error) {
	f.calls++
	return []cloudagent.DatabaseInfo{{ID: "d1", Name: "orders", Status: "ready"}, {ID: "d2", Name: "users", Status: "ready"}}, nil
}
func (*integrationDB) ListCollections(context.Context, auth.Principal, string, string) ([]string, error) {
	return nil, nil
}
func (*integrationDB) ReadOnlyQuery(context.Context, auth.Principal, string, string, string, int) (cloudagent.SQLResult, error) {
	return cloudagent.SQLResult{}, nil
}

func integrationRuntime(t *testing.T, protocol string) (*cloudagent.Runtime, *integrationDB) {
	t.Helper()
	key := os.Getenv("SIMPLEBASE_LLM_PROVIDER_OPENAI_API_KEY")
	if key == "" {
		t.Skip("set SIMPLEBASE_LLM_PROVIDER_OPENAI_API_KEY to run live model tests")
	}
	base := os.Getenv("SIMPLEBASE_LLM_PROVIDER_OPENAI_BASE_URL")
	if base == "" {
		base = "https://api.siliconflow.cn/v1"
	}
	resolver := llmgateway.NewFallbackResolver(nil, map[string]llmgateway.InstanceProvider{"openai": {
		APIKey: key, BaseURL: base, DefaultModel: integrationModel, AllowedModels: []string{integrationModel},
	}})
	db := &integrationDB{}
	return &cloudagent.Runtime{
		LLM: NewCloudAgentLLM(NewLLMService(llmgateway.NewService(resolver, nil, nil))), DB: db,
		ToolProtocol: protocol, RunTimeout: 90 * time.Second,
	}, db
}

func TestLLMRealStreamAndToolRoundtrip(t *testing.T) {
	for _, protocol := range []string{"native", "text"} {
		t.Run(protocol, func(t *testing.T) {
			rt, db := integrationRuntime(t, protocol)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
			defer cancel()
			started := time.Now()
			first := time.Duration(0)
			res, err := rt.StartRun(ctx, cloudagent.RunRequest{
				ProjectID: "p", ThreadID: "th-" + protocol, RunID: "r-" + protocol, Stream: true,
				Agent: systemdb.CloudAgent{ID: "a", Name: "Database", Module: cloudagent.ModuleDatabase,
					ToolIDs: []string{cloudagent.ToolListDatabases}, ModelOverride: integrationModel},
				UserText: "请使用 list_databases 工具查询数据库，再告诉我项目中 orders 和 users 是否存在。",
			}, func(ev cloudagent.Event) {
				if first == 0 && (ev.Type == "token" || ev.Type == "thinking") {
					first = time.Since(started)
				}
			})
			if err != nil {
				t.Fatalf("agent run (%s): %v", protocol, cloudagent.ClassifyError(err))
			}
			if db.calls < 1 || !strings.Contains(res.Content, "orders") || !strings.Contains(res.Content, "users") {
				t.Fatalf("db calls=%d result=%q", db.calls, res.Content)
			}
			if first == 0 {
				t.Error("no streamed token received")
			}
			t.Logf("protocol=%s first_content=%s tools=%d tokens=%d", protocol, first, res.ToolCalls, res.PromptTokens+res.CompletionTokens)
		})
	}
}

func TestLLMModelAllowlistRejectsBeforeUpstream(t *testing.T) {
	rt, _ := integrationRuntime(t, "native")
	_, err := rt.StartRun(context.Background(), cloudagent.RunRequest{
		ProjectID: "p", ThreadID: "th-invalid", RunID: "r-invalid", Stream: false,
		Agent:    systemdb.CloudAgent{ID: "a", Name: "General", Module: cloudagent.ModuleGeneral, ModelOverride: "not-in-allowlist"},
		UserText: "hello",
	}, nil)
	if err == nil || !errors.Is(err, llmgateway.ErrModelNotAllowed) {
		t.Fatalf("expected allowlist rejection, got %v", err)
	}
}
