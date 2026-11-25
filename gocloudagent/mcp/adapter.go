// 文件说明：adapter.go — gocloudagent/mcp 的内部实现（toToolInfo、marshalSchema 等）
//
// 用法
//
//	同包调用：toToolInfo() → adapter_test.go, client.go；marshalSchema() → adapter_test.go；sanitizeDescription() → adapter_test.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/tools, internal/agentbridge 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/mcp/adapter.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter_test.go, client.go, http_transport.go, http_transport_test.go, limits.go 等 10 个
//	[关键引用] toToolInfo ← adapter_test.go, client.go；marshalSchema ← adapter_test.go；sanitizeDescription ← adapter_test.go
//
// 主要符号
//
//	toToolInfo(t *sdkmcp.Tool) (ToolInfo, bool)
//	marshalSchema(v any) ([]byte, bool)
//	sanitizeDescription(v string) string
//	toCallResult(res *sdkmcp.CallToolResult) CallResult

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// toToolInfo 把 SDK 的 Tool 转成自有 ToolInfo。
// 返回 false 表示该工具应被跳过（schema 不可用或超限）。
func toToolInfo(t *sdkmcp.Tool) (ToolInfo, bool) {
	name := strings.TrimSpace(t.Name)
	if name == "" {
		return ToolInfo{}, false
	}
	schema, ok := marshalSchema(t.InputSchema)
	if !ok {
		return ToolInfo{}, false
	}
	info := ToolInfo{
		Name:        name,
		Description: sanitizeDescription(t.Description),
		InputSchema: schema,
	}
	if t.Annotations != nil {
		info.ReadOnlyHint = t.Annotations.ReadOnlyHint
		if t.Annotations.DestructiveHint != nil {
			info.DestructiveHint = *t.Annotations.DestructiveHint
		}
	}
	return info, true
}

// marshalSchema 序列化 inputSchema 并做上限与顶层类型校验（§3.3）。
// 顶层必须是 object：eino 的 ParamsOneOf 只接受对象型参数。
func marshalSchema(v any) ([]byte, bool) {
	if v == nil {
		// 无参数工具：给一个空对象 schema，避免下游 nil 分支。
		return []byte(`{"type":"object","properties":{}}`), true
	}
	b, err := json.Marshal(v)
	if err != nil || len(b) > MaxSchemaBytes {
		return nil, false
	}
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, false
	}
	// 缺省 type 视为 object（部分 Server 省略），显式非 object 则跳过。
	if probe.Type != "" && probe.Type != "object" {
		return nil, false
	}
	return b, true
}

// sanitizeDescription 把描述当不可信文本处理：去控制字符、截断、限长（§3.3）。
func sanitizeDescription(v string) string {
	v = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, v)
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) <= MaxDescriptionRunes {
		return v
	}
	runes := []rune(v)
	return string(runes[:MaxDescriptionRunes])
}

// toCallResult 把 SDK 结果映射为自有 CallResult（§3.4）。
//
// 图片/音频只留占位，不把 base64 塞给模型；未知类型同样占位。
func toCallResult(res *sdkmcp.CallToolResult) CallResult {
	var b strings.Builder
	for _, c := range res.Content {
		switch v := c.(type) {
		case *sdkmcp.TextContent:
			b.WriteString(v.Text)
			b.WriteByte('\n')
		case *sdkmcp.ImageContent:
			fmt.Fprintf(&b, "[image: %s, %d bytes]\n", v.MIMEType, len(v.Data))
		case *sdkmcp.AudioContent:
			fmt.Fprintf(&b, "[audio: %s, %d bytes]\n", v.MIMEType, len(v.Data))
		case *sdkmcp.ResourceLink:
			fmt.Fprintf(&b, "[resource: %s]\n", v.URI)
		case *sdkmcp.EmbeddedResource:
			if v.Resource != nil && v.Resource.Text != "" {
				b.WriteString(v.Resource.Text)
				b.WriteByte('\n')
			} else {
				b.WriteString("[embedded resource]\n")
			}
		default:
			fmt.Fprintf(&b, "[unsupported content: %T]\n", c)
		}
	}
	if res.StructuredContent != nil {
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			b.WriteString(string(raw))
			b.WriteByte('\n')
		}
	}
	text := strings.TrimRight(b.String(), "\n")
	truncated := false
	if len(text) > MaxResultBytes {
		text = text[:MaxResultBytes]
		truncated = true
	}
	return CallResult{Text: text, IsError: res.IsError, Truncated: truncated}
}
