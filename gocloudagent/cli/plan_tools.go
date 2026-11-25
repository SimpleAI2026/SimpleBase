// 文件说明：plan_tools.go — 把 plan / explore / reflect 三个规划工具接到本地会话
//
// 用法
//
//	planTools(session, client, cfg) []tool.BaseTool
//
// 调用架构
//
//	[上游] cli/repl.go buildRunner（Toolset.Extra）
//	         │ 传入会话与 HTTPClient
//	         ▼
//	[本文件] gocloudagent/cli/plan_tools.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/plan（Tools / FileStore / MemoryStore）
//
// 计划文件按 planning-with-files 布局落在 <sessions>/<会话ID>/plan/（planv6.8 §13.5）：
// 目录在首次写入时才创建（FileStore 懒建），FindSession / ListSessions 只认 .jsonl
// 文件并跳过目录，互不干扰。client 为 nil 时 reflect 退化为确定性清单（plan/tool.go）。

package cli

import (
	"path/filepath"

	"github.com/cloudwego/eino/components/tool"

	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// planTools 返回绑定到会话的规划工具组。
//
// session 为 nil 或缺少路径时退回 MemoryStore（不落盘），保证工具仍然可用；
// client 为 nil 时 reflect 走无模型的确定性检查清单。
func planTools(session *Session, client *HTTPClient, cfg Config) []tool.BaseTool {
	t := &plan.Tools{Model: cfg.Model}
	if dir := planDir(session); dir != "" {
		t.Store = &plan.FileStore{Dir: dir}
	}
	if client != nil {
		t.LLM = client
	}
	return t.All()
}

// planDir 返回会话的计划目录；不可定位时返回空串。
func planDir(session *Session) string {
	if session == nil || session.ID == "" || session.Path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(session.Path), session.ID, "plan")
}
