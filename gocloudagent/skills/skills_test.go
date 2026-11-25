// 文件说明：skills_test.go — gocloudagent/skills 的单元测试（TestCatalogLoadsPromptDocuments、TestReadDocumentFrontmatterVariants 等）
//
// 用法
//
//	go test ./gocloudagent/skills -run 'TestCatalogLoadsPromptDocuments|TestReadDocumentFrontmatterVariants'
//
// 调用架构
//
//	[上游] internal/agentbridge/agentskills
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/skills/skills_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] skills.go
//
// 主要符号
//
//	TestCatalogLoadsPromptDocuments(t *testing.T)
//	TestReadDocumentFrontmatterVariants(t *testing.T)

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

func TestReadDocumentFrontmatterVariants(t *testing.T) {
	filesystem := fstest.MapFS{
		"plain.md":      {Data: []byte("  \n# T\nbody\n\n")},
		"lf.md":         {Data: []byte("---\nname: x\n---\nbody")},
		"unclosed.md":   {Data: []byte("---\nname: x\nbody")},
		"crlf.md":       {Data: []byte("---\r\nname: x\r\n---\r\nbody")},
		"trailing.md":   {Data: []byte("---\na: b\n--- trailing\nbody")},
		"empty.md":      {Data: nil},
		"only-front.md": {Data: []byte("---\nname: x\n---\n")},
	}
	cases := map[string]string{
		"plain.md":      "# T\nbody",
		"lf.md":         "body",
		"unclosed.md":   "---\nname: x\nbody",
		"crlf.md":       "---\r\nname: x\r\n---\r\nbody", // 仅识别 LF：CRLF 原样保留
		"trailing.md":   "trailing\nbody",                // 结束标记行尾的内容保留（现有行为）
		"empty.md":      "",
		"only-front.md": "",
	}
	for name, want := range cases {
		got, err := ReadDocument(filesystem, name)
		if err != nil || got != want {
			t.Errorf("%s: got %q err=%v, want %q", name, got, err, want)
		}
	}
	if _, err := ReadDocument(filesystem, "missing.md"); err == nil {
		t.Fatal("missing file must return an error")
	}
}
