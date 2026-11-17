package cloudagent

import (
	"strings"
	"testing"
)

func TestModulesCatalogAndDefaults(t *testing.T) {
	if !KnownModule(" DATABASE ") || !KnownModule("S3") || !KnownModule("Logs") || !KnownModule("GENERAL") {
		t.Fatal("KnownModule should be case/space insensitive for built-ins")
	}
	if KnownModule("") || KnownModule("team") {
		t.Fatal("unknown modules")
	}
	if got := moduleTemplate("database"); !strings.Contains(got, "Module: database") {
		t.Fatalf("database template: %s", got)
	}
	if got := moduleTemplate("S3"); !strings.Contains(got, "Module: s3") {
		t.Fatalf("s3 template: %s", got)
	}
	if got := moduleTemplate(" logs "); !strings.Contains(got, "Module: logs") {
		t.Fatalf("logs template: %s", got)
	}
	if got := moduleTemplate("other"); !strings.Contains(got, "Module: general") {
		t.Fatalf("default template: %s", got)
	}
	if tools := DefaultToolsForModule(ModuleDatabase); len(tools) != 3 {
		t.Fatalf("db tools=%v", tools)
	}
	if tools := DefaultToolsForModule(ModuleS3); len(tools) != 2 {
		t.Fatalf("s3 tools=%v", tools)
	}
	if tools := DefaultToolsForModule(ModuleLogs); len(tools) != 2 {
		t.Fatalf("logs tools=%v", tools)
	}
	if tools := DefaultToolsForModule(ModuleGeneral); len(tools) != 7 {
		t.Fatalf("general tools=%v", tools)
	}
	if tools := DefaultToolsForModule("nope"); tools != nil {
		t.Fatalf("unknown tools=%v", tools)
	}
	if KnownTool("") || KnownTool("write_sql") {
		t.Fatal("unknown tool")
	}
	for _, id := range []string{ToolListDatabases, ToolListCollections, ToolReadonlySQL, ToolListObjects, ToolHeadObject, ToolSearchLogs, ToolLogLevelStats} {
		if !KnownTool(id) {
			t.Fatalf("expected known tool %s", id)
		}
	}
}
