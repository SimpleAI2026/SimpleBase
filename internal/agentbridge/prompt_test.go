// 文件说明：prompt_test.go — internal/agentbridge 的单元测试（TestAssembleInstructionOrderAndRedact、TestKnownModuleAndTools 等）
//
// 用法
//
//	go test ./internal/agentbridge -run 'TestAssembleInstructionOrderAndRedact|TestKnownModuleAndTools|TestExtractToolCall|TestPromptRedactAndTruncate'
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/prompt_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, access_test.go, bridge_test.go 等 33 个
//
// 主要符号
//
//	TestAssembleInstructionOrderAndRedact(t *testing.T)
//	TestKnownModuleAndTools(t *testing.T)
//	TestExtractToolCall(t *testing.T)
//	TestPromptRedactAndTruncate(t *testing.T)

package agentbridge

import (
	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"strings"
	"testing"
)

func TestAssembleInstructionOrderAndRedact(t *testing.T) {
	got := AssembleInstruction(PromptParts{
		ModuleTemplate: moduleTemplate(ModuleDatabase),
		AgentPrompt:    "You prefer concise answers.",
		ProjectEnv:     "Project env:\n- project_id: abc\n- api_key: sk-secret",
		Snapshot:       "AKIAIOSFODNN7EXAMPLE objects",
	})
	if !strings.Contains(got, "SimpleBase Cloud Agent") {
		t.Fatal("missing platform base")
	}
	idxPlat := strings.Index(got, "SimpleBase Cloud Agent")
	idxMod := strings.Index(got, "Module: database")
	idxAgent := strings.Index(got, "prefer concise")
	idxEnv := strings.Index(got, "project_id: abc")
	if idxPlat < 0 || idxMod < idxPlat || idxAgent < idxMod || idxEnv < idxAgent {
		t.Fatalf("assembly order wrong:\n%s", got)
	}
	if strings.Contains(got, "sk-secret") || strings.Contains(got, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("secrets leaked into prompt:\n%s", got)
	}
	if !strings.Contains(got, "[REDACTED") {
		t.Fatalf("expected redaction markers:\n%s", got)
	}
}

func TestKnownModuleAndTools(t *testing.T) {
	if !KnownModule("database") || KnownModule("write") {
		t.Fatal("module catalog")
	}
	if !KnownTool(ToolReadonlySQL) || KnownTool("delete_object") {
		t.Fatal("tool catalog")
	}
	mods := Modules(false)
	if len(mods) != 8 {
		t.Fatalf("modules=%d", len(mods))
	}
	for _, m := range mods {
		if m.TeamSupported {
			t.Fatalf("team should be stub false: %+v", m)
		}
	}
	if !KnownModule(ModuleSandbox) || !IsSandboxTool(ToolSandboxExec) {
		t.Fatal("sandbox module/tool catalog")
	}
	if !KnownTool(ToolSandboxShell) {
		t.Fatal("sandbox tools must be known")
	}
	sb := Modules(true)
	var found bool
	for _, m := range sb {
		if m.ID == ModuleSandbox {
			found = m.SandboxAvailable
		}
	}
	if !found {
		t.Fatal("sandbox_available must reflect the flag")
	}
}

func TestExtractToolCall(t *testing.T) {
	name, args, ok := agentcore.ExtractToolCall(`TOOL_CALL {"name":"list_databases","arguments":{}}`)
	if !ok || name != "list_databases" {
		t.Fatalf("name=%s ok=%v args=%s", name, ok, args)
	}
	if _, _, ok := agentcore.ExtractToolCall("hello"); ok {
		t.Fatal("false positive")
	}
}

func TestPromptRedactAndTruncate(t *testing.T) {
	if agentcore.RedactSecrets("") != "" {
		t.Fatal("empty redact")
	}
	got := agentcore.RedactSecrets("Bearer abcdef.ghij+/= password=hunter2")
	if strings.Contains(got, "abcdef") || strings.Contains(got, "hunter2") {
		t.Fatalf("leaked: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redaction: %s", got)
	}
	if truncate("ab", 0) != "ab" {
		t.Fatal("max<=0 should return original")
	}
	if truncate("ab", 10) != "ab" {
		t.Fatal("short string")
	}
	long := strings.Repeat("你", 8)
	out := truncate(long, 3)
	if !strings.HasSuffix(out, "\n…[truncated]") {
		t.Fatalf("truncate suffix: %q", out)
	}
	empty := AssembleInstruction(PromptParts{})
	if !strings.Contains(empty, "SimpleBase Cloud Agent") {
		t.Fatal("platform prompt required")
	}
}
