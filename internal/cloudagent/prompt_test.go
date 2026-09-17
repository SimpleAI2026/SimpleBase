package cloudagent

import (
	"strings"
	"testing"
)

func TestAssembleInstructionOrderAndRedact(t *testing.T) {
	got := AssembleInstruction(PromptParts{
		ModuleTemplate: ModuleTemplate(ModuleDatabase),
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
	mods := Modules()
	if len(mods) != 4 {
		t.Fatalf("modules=%d", len(mods))
	}
	for _, m := range mods {
		if m.TeamSupported {
			t.Fatalf("team should be stub false: %+v", m)
		}
	}
}

func TestExtractToolCall(t *testing.T) {
	name, args, ok := extractToolCall(`TOOL_CALL {"name":"list_databases","arguments":{}}`)
	if !ok || name != "list_databases" {
		t.Fatalf("name=%s ok=%v args=%s", name, ok, args)
	}
	if _, _, ok := extractToolCall("hello"); ok {
		t.Fatal("false positive")
	}
}
