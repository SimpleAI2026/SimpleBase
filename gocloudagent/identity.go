// 文件说明：identity.go — gocloudagent 中 Permission、DatabaseRead 等符号的实现
//
// 用法
//
//	(p Principal) HasPermission(perm Permission) bool  // HasPermission 判断是否具有给定权限位（nil 集合视为无权限）
//	type Permission（导出类型，同包与上层均可使用）
//	type Principal（导出类型，同包与上层均可使用）
//	同包调用：Agent → cli_tool.go, errors.go 等；Principal → access.go, cli_tool.go 等；Message → cli_tool.go, errors.go 等
//
// 调用架构
//
//	[上游] gocloudagent/cmd/gocloudagent, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/identity.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access.go, cli_entry.go, cli_runner.go, cli_tool.go, errors.go 等 11 个
//	[关键引用] Agent ← cli_tool.go, errors.go, modules.go
//
// 主要符号
//
//	type Permission string
//	const DatabaseRead
//	const DatabaseWrite
//	const DatabaseAdmin
//	const ProjectAdmin
//	type Principal struct {…}
//	... 另有 10 个顶层符号

package gocloudagent

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// identity.go 定义云助手模块与宿主之间的最小契约。
//
// 为什么需要这一层：gocloudagent 是独立模块，不得 import 根模块的 auth /
// systemdb / observability。宿主的鉴权、系统库与日志实现留在根模块，由
// internal/agentbridge 做双向映射。这里的类型是「无损、无密钥」的数据载体，
// 不含任何授权判定逻辑——角色与项目校验始终由宿主完成。

// Permission 是宿主权限位在模块内的镜像。
// 不是类型别名：模块不得与 auth 包产生编译期耦合。
type Permission string

const (
	// DatabaseRead / DatabaseWrite / DatabaseAdmin 与宿主权限位字面量一致。
	DatabaseRead  Permission = "database:read"
	DatabaseWrite Permission = "database:write"
	DatabaseAdmin Permission = "database:admin"
	// ProjectAdmin 用于不可写助手的降权保留判定。
	ProjectAdmin Permission = "project:admin"
)

// Principal 是一次 run 的调用方身份快照（无密钥）。
//
// 字段覆盖宿主 Principal 的全部非密钥字段：委托签发会读取 Role /
// TenantID / Username，数据库适配器会带着 Role / ProjectIDs 继续做项目
// 授权，因此不能用「只留模块直接消费字段」的最小集合。
type Principal struct {
	APIKeyID   string
	UserID     string
	Username   string
	Role       string
	SessionID  string
	AccessJTI  string
	TenantID   string
	ProjectIDs map[string]struct{}
	// Permissions 是权限位集合；模块只读它，不新增权限。
	Permissions map[Permission]struct{}
	// Delegated 标记该身份来自助手委托令牌。
	Delegated bool
}

// HasPermission 判断是否具有给定权限位（nil 集合视为无权限）。
func (p Principal) HasPermission(perm Permission) bool {
	if p.Permissions == nil {
		return false
	}
	_, ok := p.Permissions[perm]
	return ok
}

// Agent 是本轮要执行的助手配置快照，由宿主从系统库读出后填入。
type Agent struct {
	ID            string
	ProjectID     string
	Name          string
	Module        string
	Description   string
	SystemPrompt  string
	ToolIDs       []string
	ModelOverride string
	// Writable 为假时只能执行只读命令（在破坏性确认之前拦截）。
	Writable bool
}

// Message 是历史会话的一轮，供提示词与工具摘要使用。
type Message struct {
	Role          string
	Content       string
	ToolCallsJSON string
}

// LogEvent 是 search_logs 工具返回的一行（字段名与 JSON 契约保持一致）。
type LogEvent struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Level      string    `json:"level"`
	Logger     string    `json:"logger"`
	Message    string    `json:"message"`
	FieldsJSON string    `json:"fields_json,omitempty"`
	RequestID  string    `json:"request_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// LogLevelCount 是 log_level_stats 的一行。
type LogLevelCount struct {
	Level string
	Count int64
}

// LLMSettings 只承载默认模型这一项：模块不需要也不应看到 API Key 或 BaseURL。
type LLMSettings struct {
	ProjectID    string
	DefaultModel string
}

// Logger 是本模块需要的日志出口（元数据层与内容层共用）。
//
// 宿主 observability.Logger 的方法签名与之兼容（Field 是 zap.Field 别名），
// 可直接传入；装配侧不要把 nil *zapLogger 装进非 nil 接口，否则 panic。
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}

// CommandDecision 是一条 simplebase 命令的安全分类结果。
type CommandDecision struct {
	// ReadOnly 表示命令不改变任何资源（事实来源是宿主的 sbcli 只读表）。
	ReadOnly bool
	// Destructive 表示执行前必须取得界面确认。
	Destructive bool
}

// CommandPolicy 是宿主命令语义的唯一入口。
//
// Classify 与 RedactArgv 的规则全部来自宿主的 sbcli / sqlguard：模块内不得
// 复制第二份只读允许表或 SQL 解析器。策略缺失或返回错误时，调用方必须拒绝
// 执行（fail-closed），不能退回「确认后执行」。
type CommandPolicy interface {
	Classify(argv []string, stdin string) (CommandDecision, error)
	// RedactArgv 返回可展示的 argv 副本：敏感值替换为 ***，SQL 只留首关键字。
	RedactArgv(argv []string) []string
}

// SettingsAccess 读取项目级 LLM 默认设置（不含密钥）。
type SettingsAccess interface {
	LLMSettings(ctx context.Context, projectID string) (LLMSettings, error)
}
