// 文件说明：types.go — 无密钥 DTO：Principal / Agent / Message / LogEvent / LLMSettings

package cloudagent

import (
	"time"
)

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
