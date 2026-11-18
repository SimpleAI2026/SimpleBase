// 文件说明：agent_bi.go — 数据看板「Agent 生成图表」的请求校验与运行限权（planv6.7 §3.4）
//
// 复用现有数据助手与 run 链路，不新增路由、工具与表。这里只做两件事：
//  1. 校验并清洗 bi_context；
//  2. 把本轮运行强制降为只读（即使助手被用户设为可写）。

package api

import (
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// biFieldBody / biContextBody 是 createRunBody 里的 bi_context 请求体。
type biFieldBody struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type biContextBody struct {
	DatabaseID string `json:"database_id"`
	// Collection 是数据源名：集合名或 SQL 表名（Source 决定是哪一种）。
	Collection string `json:"collection"`
	// Source 缺省为 collection；SQL 表传 "table"。
	Source        string        `json:"source,omitempty"`
	PreferredType string        `json:"preferred_type"`
	Fields        []biFieldBody `json:"fields"`
}

// resolveBIContext 校验 bi_context。body 为 nil 表示非 BI 运行，返回 (nil, nil)。
//
// 拒绝条件（统一 ErrBIContextInvalid）：
//   - 与搭建会话 / starter_stage 同传；
//   - 助手 module 不是 database / general；
//   - database_id 不是 UUID、collection 不合规、preferred_type 不在白名单。
//
// 字段逐项过滤（见 cloudagent.NormalizeBIContext），不合规的字段被丢弃而不是拒绝整个请求。
func resolveBIContext(body *biContextBody, starterStage string, thread systemdb.AgentThread, agent systemdb.CloudAgent) (*cloudagent.BIContext, error) {
	if body == nil {
		return nil, nil
	}
	if strings.TrimSpace(starterStage) != "" || thread.StarterTemplate != "" {
		return nil, ErrBIContextInvalid
	}
	switch agent.Module {
	case cloudagent.ModuleDatabase, cloudagent.ModuleGeneral:
	default:
		return nil, ErrBIContextInvalid
	}
	in := cloudagent.BIContext{
		DatabaseID:    body.DatabaseID,
		Collection:    body.Collection,
		Source:        body.Source,
		PreferredType: body.PreferredType,
		Fields:        make([]cloudagent.BIField, 0, len(body.Fields)),
	}
	for _, f := range body.Fields {
		in.Fields = append(in.Fields, cloudagent.BIField{Name: f.Name, Type: f.Type})
	}
	ctx, err := cloudagent.NormalizeBIContext(in)
	if err != nil {
		return nil, ErrBIContextInvalid
	}
	return &ctx, nil
}

// applyBIRunPolicy 把 BI 上下文写入 RunRequest，并强制只读。
//
// BI 生成永远不写：Writable=false 后，CLI 技能模式下的写命令在确认之前就被
// write_not_allowed 拒绝。覆盖点与搭建会话把 Writable 提到 true 是同一处。
// 技能与权限不接受客户端输入：技能仍由 SkillsForModule 决定，委托令牌仍是调用者权限的子集。
func applyBIRunPolicy(req *cloudagent.RunRequest, bi *cloudagent.BIContext) {
	if bi == nil {
		return
	}
	req.BI = bi
	req.Agent.Writable = false
}
