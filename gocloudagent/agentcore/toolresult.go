// 文件说明：toolresult.go — 工具结果的统一编码：JSON 序列化、失败信封与失败标志识别
//
// 三者都是与运行时无关的纯函数，云端 Runtime、本地 CLI/本机工具与宿主业务工具共用：
//   - MarshalToolJSON：json.Marshal 后对整段文本做凭据脱敏；编码错误原样返回。
//   - ErrorEnvelope：{"ok":false,"error":{"code","message"}}，message 原样写入，不脱敏。
//   - ToolResultIsError：只识别顶层 ok:false 或 is_error:true；任何解析失败都视为未失败。

package agentcore

import (
	"encoding/json"
	"strings"
)

// MarshalToolJSON 序列化工具结果并脱敏凭据样式子串。
func MarshalToolJSON(v any) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return RedactSecrets(string(body)), nil
}

// ErrorEnvelope 构造统一失败信封。message 原样写入；需要脱敏的调用方先自行处理。
func ErrorEnvelope(code, message string) string {
	b, _ := json.Marshal(map[string]any{"ok": false, "error": map[string]any{"code": code, "message": message}})
	return string(b)
}

// ToolResultIsError 报告工具结果是否带失败标志（顶层 ok:false 或 is_error:true）。
//
// 空串、非 JSON、非对象或字段类型不符一律返回 false：宁可漏判，也不把普通文本当失败。
func ToolResultIsError(content string) bool {
	var envelope struct {
		OK      *bool `json:"ok"`
		IsError *bool `json:"is_error"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(content)), &envelope) != nil {
		return false
	}
	return (envelope.OK != nil && !*envelope.OK) || (envelope.IsError != nil && *envelope.IsError)
}
