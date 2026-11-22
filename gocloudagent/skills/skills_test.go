package skills

import (
	"reflect"
	"testing"
	"testing/fstest"
)

func TestCatalogLoadsPromptDocuments(t *testing.T) {
	filesystem := fstest.MapFS{
		"agent/SKILL.md":              {Data: []byte("---\nname: agent\n---\n# Agent\nUse the available tools.")},
		"agent/references/search.md":  {Data: []byte("# Search\nSearch before answering.")},
		"agent/references/ignore.txt": {Data: []byte("not markdown")},
	}
	catalog, err := Load(filesystem, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := catalog.IDs(), []string{"SKILL", "references/search"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids=%v want %v", got, want)
	}
	if body := catalog.Body("SKILL"); body != "# Agent\nUse the available tools." {
		t.Fatalf("body=%q", body)
	}
	if got := catalog.Prompt("references/search", "missing", "SKILL"); got != "# Search\nSearch before answering.\n\n# Agent\nUse the available tools." {
		t.Fatalf("prompt=%q", got)
	}
	var nilCatalog *Catalog
	if nilCatalog.IDs() != nil || nilCatalog.Body("x") != "" || nilCatalog.Prompt("x") != "" {
		t.Fatal("nil catalog must be empty")
	}
}
