package agentcore

import (
	"encoding/json"
	"testing"
)

// 回归：模型漏写 "arguments" 包装，把参数直接放在顶层。
// 这是用户在 CLI 里实际遇到的输出，之前解析成 args={}，shell 工具报 command is required，
// 模型再重复同样的格式，整轮卡死。
func TestExtractToolCallFlattensMissingArgumentsWrapper(t *testing.T) {
	in := `我先看一下当前工作区里有什么内容。TOOL_CALL {"name":"shell","command":"ls -la; echo '---'; find . -maxdepth 2 -type f | head -50"}`
	name, args, ok := ExtractToolCall(in)
	if !ok || name != "shell" {
		t.Fatalf("ok=%v name=%q", ok, name)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(args), &got); err != nil {
		t.Fatalf("args must be a JSON object, got %q: %v", args, err)
	}
	want := "ls -la; echo '---'; find . -maxdepth 2 -type f | head -50"
	if got["command"] != want {
		t.Fatalf("command = %q, want %q", got["command"], want)
	}
	if _, leaked := got["name"]; leaked {
		t.Fatalf("name must not leak into the arguments: %q", args)
	}
}

// 多个顶层参数都要带上，且保持各自的 JSON 类型（数字不被字符串化）。
func TestExtractToolCallFlattenKeepsTypes(t *testing.T) {
	_, args, ok := ExtractToolCall(`TOOL_CALL {"name":"read_file","path":"a.go","limit":20,"all":true}`)
	if !ok {
		t.Fatal("want ok")
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(args), &got); err != nil {
		t.Fatal(err)
	}
	if got["path"] != "a.go" || got["limit"] != float64(20) || got["all"] != true {
		t.Fatalf("args = %q", args)
	}
}

// 格式正确的调用不受影响：arguments 存在时，顶层多余字段一律忽略。
func TestExtractToolCallArgumentsWinsOverTopLevel(t *testing.T) {
	_, args, ok := ExtractToolCall(`TOOL_CALL {"name":"shell","command":"WRONG","arguments":{"command":"ls"}}`)
	if !ok {
		t.Fatal("want ok")
	}
	if args != `{"command":"ls"}` {
		t.Fatalf("args = %q, want the arguments object untouched", args)
	}
}

// arguments 为空对象是合法的「无参调用」，不能被顶层字段"救活"成另一个调用。
func TestExtractToolCallEmptyArgumentsStaysEmpty(t *testing.T) {
	_, args, ok := ExtractToolCall(`TOOL_CALL {"name":"list_databases","arguments":{},"command":"x"}`)
	if !ok || args != "{}" {
		t.Fatalf("ok=%v args=%q, want an explicit empty object kept as-is", ok, args)
	}
}

// 没有任何参数字段时仍是 "{}"（既有行为）。
func TestExtractToolCallNoArgsDefaultsToEmptyObject(t *testing.T) {
	name, args, ok := ExtractToolCall(`TOOL_CALL {"name":"only"}`)
	if !ok || name != "only" || args != "{}" {
		t.Fatalf("ok=%v name=%q args=%q", ok, name, args)
	}
}

// 容错只针对 name 以外的字段；name 缺失/非字符串仍然拒绝，不能凭空造出调用。
func TestExtractToolCallStillRejectsBadName(t *testing.T) {
	for _, in := range []string{
		`TOOL_CALL {"command":"ls"}`,
		`TOOL_CALL {"name":"","command":"ls"}`,
		`TOOL_CALL {"name":123,"command":"ls"}`,
		`TOOL_CALL {"name":null,"command":"ls"}`,
		`TOOL_CALL null`,
		`TOOL_CALL []`,
	} {
		if name, args, ok := ExtractToolCall(in); ok {
			t.Fatalf("in=%q parsed as name=%q args=%q, want rejection", in, name, args)
		}
	}
}

// "arguments": null 等价于没给：走顶层兜底，而不是把字面 null 当参数。
func TestExtractToolCallNullArgumentsFallsBackToTopLevel(t *testing.T) {
	_, args, ok := ExtractToolCall(`TOOL_CALL {"name":"shell","arguments":null,"command":"ls"}`)
	if !ok {
		t.Fatal("want ok")
	}
	if args != `{"command":"ls"}` {
		t.Fatalf("args = %q", args)
	}
}

// ParseAssistant 端到端：偏差格式也要产出一个带正确参数的 ToolCall。
func TestParseAssistantRecoversMisformattedToolCall(t *testing.T) {
	msg := ParseAssistant(`TOOL_CALL {"name":"shell","command":"pwd"}`)
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v", msg.ToolCalls)
	}
	tc := msg.ToolCalls[0]
	if tc.Function.Name != "shell" || tc.Function.Arguments != `{"command":"pwd"}` {
		t.Fatalf("call = %+v", tc.Function)
	}
}

// 回归：真实模型输出里，调用后面紧跟空代码块和第二个（拼错的）调用。
// 旧逻辑取「第一个 { 到最后一个 }」整段 Unmarshal，尾部多余内容让整段失败，
// 本来完整的第一个调用被当成普通文本，agent 当轮直接结束、什么工具都没跑。
func TestExtractToolCallIgnoresTrailingContent(t *testing.T) {
	in := "TOOL_CALL {\"name\":\"glob\",\"pattern\":\"*\"}\n```json\n```\n\nTO_CALL {\"name\":\"glob\",\"pattern\":\"**/*\"}"
	name, args, ok := ExtractToolCall(in)
	if !ok || name != "glob" {
		t.Fatalf("ok=%v name=%q: the first complete call must survive trailing noise", ok, name)
	}
	if args != `{"pattern":"*"}` {
		t.Fatalf("args = %q, want the first call's parameters only", args)
	}
}

// 连续两个合法 TOOL_CALL：只取第一个，不能把两段 JSON 的字段混到一起。
func TestExtractToolCallTakesOnlyTheFirstOfSeveral(t *testing.T) {
	in := `TOOL_CALL {"name":"shell","arguments":{"command":"ls"}}` + "\n" +
		`TOOL_CALL {"name":"shell","arguments":{"command":"rm -rf x"}}`
	name, args, ok := ExtractToolCall(in)
	if !ok || name != "shell" || args != `{"command":"ls"}` {
		t.Fatalf("ok=%v name=%q args=%q", ok, name, args)
	}
}

// 前缀说明文字 + 调用 + 后缀说明文字：这是最常见的"话痨"形态。
func TestExtractToolCallWithProseOnBothSides(t *testing.T) {
	in := `让我看一下目录。TOOL_CALL {"name":"shell","arguments":{"command":"ls"}} 等结果出来我再继续。`
	name, args, ok := ExtractToolCall(in)
	if !ok || name != "shell" || args != `{"command":"ls"}` {
		t.Fatalf("ok=%v name=%q args=%q", ok, name, args)
	}
}

// 不完整/损坏的 JSON 仍然拒绝，不能因为放宽尾部而把残缺调用当成功。
func TestExtractToolCallRejectsTruncatedJSON(t *testing.T) {
	for _, in := range []string{
		`TOOL_CALL {"name":"shell","arguments":{"command":"ls"`,
		`TOOL_CALL {"name":"shell",`,
		`TOOL_CALL {`,
		`TOOL_CALL nothing here`,
	} {
		if name, _, ok := ExtractToolCall(in); ok {
			t.Fatalf("in=%q parsed as %q, want rejection", in, name)
		}
	}
}
