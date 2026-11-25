// 文件说明：toolresult_test.go — gocloudagent/agentcore 的单元测试（TestMarshalToolJSONRedactsAfterEncoding、TestErrorEnvelopeKeepsMessageVerbatim 等）
//
// 用法
//
//	go test ./gocloudagent/agentcore -run 'TestMarshalToolJSONRedactsAfterEncoding|TestErrorEnvelopeKeepsMessageVerbatim|TestToolResultIsError'
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/group, gocloudagent/plan 等 6 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/agentcore/toolresult_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] emit_token_test.go, messages.go, model.go, model_internal_test.go, model_length_test.go 等 13 个
//
// 主要符号
//
//	TestMarshalToolJSONRedactsAfterEncoding(t *testing.T)
//	TestErrorEnvelopeKeepsMessageVerbatim(t *testing.T)
//	TestToolResultIsError(t *testing.T)

package agentcore

import (
	"math"
	"strings"
	"testing"
)

func TestMarshalToolJSONRedactsAfterEncoding(t *testing.T) {
	out, err := MarshalToolJSON(map[string]any{"password": "hunter2", "n": 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "hunter2") || !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("secret leaked: %s", out)
	}
	if _, err := MarshalToolJSON(make(chan int)); err == nil {
		t.Fatal("unsupported type must return the encoding error")
	}
	if _, err := MarshalToolJSON(math.NaN()); err == nil {
		t.Fatal("NaN must return the encoding error")
	}
	if got, _ := MarshalToolJSON(nil); got != "null" {
		t.Fatalf("nil=%q", got)
	}
}

func TestErrorEnvelopeKeepsMessageVerbatim(t *testing.T) {
	got := ErrorEnvelope("c", `a "b" <x> password=hunter2`)
	want := `{"error":{"code":"c","message":"a \"b\" \u003cx\u003e password=hunter2"},"ok":false}`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestToolResultIsError(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"  ", false},
		{"plain text", false},
		{"null", false},
		{"[]", false},
		{`{"ok":true}`, false},
		{`{"ok":false}`, true},
		{`{"is_error":true}`, true},
		{`{"is_error":false}`, false},
		{"  {\"ok\":false}  ", true},
		// 字段类型不符时整体解码失败：不做部分挽救。
		{`{"ok":false,"is_error":"bad"}`, false},
		{`{"unknown":1}`, false},
	}
	for _, tc := range cases {
		if got := ToolResultIsError(tc.in); got != tc.want {
			t.Errorf("ToolResultIsError(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}
