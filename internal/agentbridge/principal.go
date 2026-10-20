// Package agentbridge 是 SimpleBase 宿主与独立 cloudagent 模块之间的适配层。
//
// 职责边界：
//   - 把宿主 auth.Principal / systemdb.* 转成 cloudagent 的自有无密钥 DTO；
//   - 提供命令安全分类（唯一事实来源仍是 sbcli 与 database/sqlguard）；
//   - 提供最终错误分类（先匹配 llmgateway 哨兵，再交给 cloudagent）；
//   - 把 systemdb.Store 包装成 cloudagent.LogAccess / SettingsAccess；
//   - 把 auth 的委托签发能力适配成 cloudagent.DelegationIssuer。
//
// 本包可以 import cloudagent、auth、systemdb、sbcli、sqlguard、llmgateway，
// 但**不得** import internal/api、internal/app、internal/agentjob：
// app 是唯一装配点，api 与 agentjob 复用本包，不允许反向依赖。
package agentbridge

import (
	"github.com/linkxzhou/SimpleBase/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// ToModulePrincipal 把宿主身份转成模块身份。
//
// map 一律深拷贝：模块里的只读降权会修改权限集合，若共享底层 map 会污染
// 调用方的原始 Principal（进而让委托签发拿到被降权/被提升的权限）。
func ToModulePrincipal(p auth.Principal) cloudagent.Principal {
	out := cloudagent.Principal{
		APIKeyID:   p.APIKeyID,
		UserID:     p.UserID,
		Username:   p.Username,
		Role:       string(p.Role),
		SessionID:  p.SessionID,
		AccessJTI:  p.AccessJTI,
		TenantID:   p.TenantID,
		Delegated:  p.Delegated,
		ProjectIDs: make(map[string]struct{}, len(p.ProjectIDs)),
	}
	for id := range p.ProjectIDs {
		out.ProjectIDs[id] = struct{}{}
	}
	if p.ProjectIDs == nil {
		out.ProjectIDs = nil
	}
	if p.Permissions != nil {
		out.Permissions = make(map[cloudagent.Permission]struct{}, len(p.Permissions))
		for perm := range p.Permissions {
			out.Permissions[cloudagent.Permission(perm)] = struct{}{}
		}
	}
	return out
}

// ToHostPrincipal 把模块身份还原成宿主身份。
//
// 数据库适配器必须拿到完整 Principal 再走 catalog：角色、租户与项目集合都
// 参与项目授权判定，不能因为「目标 projectID 已经给出」而省略。
func ToHostPrincipal(p cloudagent.Principal) auth.Principal {
	out := auth.Principal{
		APIKeyID:   p.APIKeyID,
		UserID:     p.UserID,
		Username:   p.Username,
		Role:       auth.Role(p.Role),
		SessionID:  p.SessionID,
		AccessJTI:  p.AccessJTI,
		TenantID:   p.TenantID,
		Delegated:  p.Delegated,
		ProjectIDs: make(map[string]struct{}, len(p.ProjectIDs)),
	}
	for id := range p.ProjectIDs {
		out.ProjectIDs[id] = struct{}{}
	}
	if p.ProjectIDs == nil {
		out.ProjectIDs = nil
	}
	if p.Permissions != nil {
		out.Permissions = make(map[auth.Permission]struct{}, len(p.Permissions))
		for perm := range p.Permissions {
			out.Permissions[auth.Permission(perm)] = struct{}{}
		}
	}
	return out
}

// Agent 把系统库里的助手配置转成模块 DTO。
func Agent(a systemdb.CloudAgent) cloudagent.Agent {
	return cloudagent.Agent{
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
func Messages(msgs []systemdb.AgentMessage) []cloudagent.Message {
	if msgs == nil {
		return nil
	}
	out := make([]cloudagent.Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, Message(m))
	}
	return out
}

// Message 转换单条历史消息。
func Message(m systemdb.AgentMessage) cloudagent.Message {
	return cloudagent.Message{Role: m.Role, Content: m.Content, ToolCallsJSON: m.ToolCallsJSON}
}
