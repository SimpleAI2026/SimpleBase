// 文件说明：output.go — internal/sbcli 的内部实现（exitOK、exitUsage 等）
//
// 用法
//
//	同包调用：exitOK → run.go；exitUsage → run.go；exitAPI → run.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sbcli/output.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] collection_transfer.go, commands.go, config.go, migrations.go, redact.go 等 6 个
//	[关键引用] exitOK ← run.go
//
// 主要符号
//
//	const exitOK
//	const exitUsage
//	const exitAPI
//	const exitConfig
//	writeOK(w io.Writer, data any)
//	writeFail(w io.Writer, code, message string, httpStatus int, requestID string)
//	... 另有 1 个顶层符号

package sbcli

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
)

const (
	exitOK     = 0
	exitUsage  = 1
	exitAPI    = 2
	exitConfig = 4
)

func writeOK(w io.Writer, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	}
	var decoded any
	if json.Unmarshal(payload, &decoded) == nil {
		data = redact(decoded)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": data})
}

func writeFail(w io.Writer, code, message string, httpStatus int, requestID string) {
	errObj := map[string]any{"code": code, "message": message}
	if httpStatus != 0 {
		errObj["http_status"] = httpStatus
	}
	if requestID != "" {
		errObj["request_id"] = requestID
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": errObj})
}

// delegationProject reads project_id from a delegation JWT payload without verifying it.
// Non-delegation tokens return "".
func delegationProject(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Type      string `json:"typ"`
		ProjectID string `json:"project_id"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return ""
	}
	if claims.Type != "agent_delegation" {
		return ""
	}
	return claims.ProjectID
}
