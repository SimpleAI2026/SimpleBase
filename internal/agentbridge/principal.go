// 文件说明：principal.go — SimpleBase 宿主与独立 gocloudagent 模块之间的适配层
//
// 用法
//
//	ToModulePrincipal(p auth.Principal) gocloudagent.Principal  // ToModulePrincipal 把宿主身份转成模块身份
//	ToHostPrincipal(p gocloudagent.Principal) auth.Principal  // ToHostPrincipal 把模块身份还原成宿主身份
//	同包调用：Message() → cron_agent.go, errors.go 等；ToModulePrincipal() → cron_agent.go；Agent() → cron_agent.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/principal.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, internal/auth, internal/systemdb
//	[同包] cli.go, command.go, cron_agent.go, delegation.go, errors.go 等 6 个
//	[关键引用] Message() ← cron_agent.go, errors.go, store.go
//
// 主要符号
//
//	ToModulePrincipal(p auth.Principal) gocloudagent.Principal
//	ToHostPrincipal(p gocloudagent.Principal) auth.Principal
//	Agent(a systemdb.CloudAgent) gocloudagent.Agent
//	Messages(msgs []systemdb.AgentMessage) []gocloudagent.Message
//	Message(m systemdb.AgentMessage) gocloudagent.Message
//	... 另有 1 个顶层符号

package agentbridge

import (
	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// convertSet 深拷贝一个集合，并把 key 从 F 转成 T。
//
// 一律深拷贝：模块里的只读降权会修改权限集合，若共享底层 map 会污染调用方的
// 原始 Principal（进而让委托签发拿到被降权/被提升的权限）。nil 输入保持 nil，
// 避免 JSON null 与 {} 的差异。
func convertSet[F, T ~string](in map[F]struct{}) map[T]struct{} {
	if in == nil {
		return nil
	}
	out := make(map[T]struct{}, len(in))
	for k := range in {
		out[T(k)] = struct{}{}
	}
	return out
}

// ToModulePrincipal 把宿主身份转成模块身份。
func ToModulePrincipal(p auth.Principal) gocloudagent.Principal {
	return gocloudagent.Principal{
		APIKeyID:    p.APIKeyID,
		UserID:      p.UserID,
		Username:    p.Username,
		Role:        string(p.Role),
		SessionID:   p.SessionID,
		AccessJTI:   p.AccessJTI,
		TenantID:    p.TenantID,
		Delegated:   p.Delegated,
		ProjectIDs:  convertSet[string, string](p.ProjectIDs),
		Permissions: convertSet[auth.Permission, gocloudagent.Permission](p.Permissions),
	}
}

// ToHostPrincipal 把模块身份还原成宿主身份。
//
// 数据库适配器必须拿到完整 Principal 再走 catalog：角色、租户与项目集合都
// 参与项目授权判定，不能因为「目标 projectID 已经给出」而省略。
func ToHostPrincipal(p gocloudagent.Principal) auth.Principal {
	return auth.Principal{
		APIKeyID:    p.APIKeyID,
		UserID:      p.UserID,
		Username:    p.Username,
		Role:        auth.Role(p.Role),
		SessionID:   p.SessionID,
		AccessJTI:   p.AccessJTI,
		TenantID:    p.TenantID,
		Delegated:   p.Delegated,
		ProjectIDs:  convertSet[string, string](p.ProjectIDs),
		Permissions: convertSet[gocloudagent.Permission, auth.Permission](p.Permissions),
	}
}

// Agent 把系统库里的助手配置转成模块 DTO。
func Agent(a systemdb.CloudAgent) gocloudagent.Agent {
	return gocloudagent.Agent{
		ID:            a.ID,
		ProjectID:     a.ProjectID,
		Name:          a.Name,
		Module:        a.Module,
		Description:   a.Description,
		SystemPrompt:  a.SystemPrompt,
		ToolIDs:       append([]string(nil), a.ToolIDs...),
		ModelOverride: a.ModelOverride,
		Writable:      a.Writable,
	}
}

// Messages 批量转换历史消息。
func Messages(msgs []systemdb.AgentMessage) []gocloudagent.Message {
	if msgs == nil {
		return nil
	}
	out := make([]gocloudagent.Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, Message(m))
	}
	return out
}

// Message 转换单条历史消息。
func Message(m systemdb.AgentMessage) gocloudagent.Message {
	return gocloudagent.Message{Role: m.Role, Content: m.Content, ToolCallsJSON: m.ToolCallsJSON}
}
