// 文件说明：adapter_test.go — gocloudagent/mcp 的单元测试（TestMarshalSchemaRejectsNonObjectAndOversize、TestSanitizeDescriptionStripsControlCharsAndTruncates 等）
//
// 用法
//
//	go test ./gocloudagent/mcp -run 'TestMarshalSchemaRejectsNonObjectAndOversize|TestSanitizeDescriptionStripsControlCharsAndTruncates|TestToToolInfoReadsAnnotations|TestToCallResultMapsContentTypes'
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/tools, internal/agentbridge 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/mcp/adapter_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, client.go, http_transport.go, http_transport_test.go, limits.go 等 10 个
//
// 主要符号
//
//	TestMarshalSchemaRejectsNonObjectAndOversize(t *testing.T)
//	TestSanitizeDescriptionStripsControlCharsAndTruncates(t *testing.T)
//	TestToToolInfoReadsAnnotations(t *testing.T)
//	TestToCallResultMapsContentTypes(t *testing.T)
//	TestToCallResultTruncates(t *testing.T)

package mcp

import (
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMarshalSchemaRejectsNonObjectAndOversize(t *testing.T) {
	// 缺省 type 视为 object。
	if _, ok := marshalSchema(map[string]any{"properties": map[string]any{}}); !ok {
		t.Fatal("missing type should be accepted as object")
	}
	// 显式非 object 跳过。
	if _, ok := marshalSchema(map[string]any{"type": "array"}); ok {
		t.Fatal("array schema should be rejected")
	}
	// nil 给空对象 schema。
	b, ok := marshalSchema(nil)
	if !ok || !strings.Contains(string(b), `"object"`) {
		t.Fatalf("nil schema=%s ok=%v", b, ok)
	}
	// 超限跳过。
	huge := map[string]any{"type": "object", "description": strings.Repeat("x", MaxSchemaBytes)}
	if _, ok := marshalSchema(huge); ok {
		t.Fatal("oversize schema should be rejected")
	}
}

func TestSanitizeDescriptionStripsControlCharsAndTruncates(t *testing.T) {
	got := sanitizeDescription("hello\x00\x07world\nline2")
	if strings.ContainsAny(got, "\x00\x07") {
		t.Fatalf("control chars kept: %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Fatalf("newline should be kept: %q", got)
	}
	long := strings.Repeat("字", MaxDescriptionRunes+50)
	if n := len([]rune(sanitizeDescription(long))); n != MaxDescriptionRunes {
		t.Fatalf("truncated runes=%d want %d", n, MaxDescriptionRunes)
	}
}

func TestToToolInfoReadsAnnotations(t *testing.T) {
	destructive := true
	info, ok := toToolInfo(&sdkmcp.Tool{
		Name:        "delete_repo",
		Description: "delete it",
		InputSchema: map[string]any{"type": "object"},
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destructive},
	})
	if !ok {
		t.Fatal("tool should be accepted")
	}
	if info.ReadOnlyHint || !info.DestructiveHint {
		t.Fatalf("hints=%v/%v", info.ReadOnlyHint, info.DestructiveHint)
	}
	// 空名字跳过。
	if _, ok := toToolInfo(&sdkmcp.Tool{Name: "  "}); ok {
		t.Fatal("empty name should be skipped")
	}
}

func TestToCallResultMapsContentTypes(t *testing.T) {
	res := toCallResult(&sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: "hello"},
			&sdkmcp.ImageContent{MIMEType: "image/png", Data: []byte("1234")},
			&sdkmcp.AudioContent{MIMEType: "audio/wav", Data: []byte("12")},
			&sdkmcp.ResourceLink{URI: "file:///a"},
			&sdkmcp.EmbeddedResource{Resource: &sdkmcp.ResourceContents{Text: "embedded"}},
		},
		StructuredContent: map[string]any{"k": "v"},
		IsError:           true,
	})
	if !res.IsError {
		t.Fatal("isError not propagated")
	}
	for _, want := range []string{"hello", "[image: image/png, 4 bytes]", "[audio: audio/wav, 2 bytes]", "[resource: file:///a]", "embedded", `{"k":"v"}`} {
		if !strings.Contains(res.Text, want) {
			t.Fatalf("missing %q in %q", want, res.Text)
		}
	}
}

func TestToCallResultTruncates(t *testing.T) {
	res := toCallResult(&sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: strings.Repeat("a", MaxResultBytes+10)}},
	})
	if !res.Truncated || len(res.Text) != MaxResultBytes {
		t.Fatalf("truncated=%v len=%d", res.Truncated, len(res.Text))
	}
}
