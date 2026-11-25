// 文件说明：ports.go — Agent 协作应用服务消费的窄端口（消费方定义；*systemdb.Store 隐式满足）
//
// 用法
//
//	type RunStore interface {…}         // run 行与消息
//	type ApprovalStore interface {…}    // 审批队列
//	type ThreadStore interface {…}      // 会话、工作区与协作行
//	type CollabStore interface {…}      // 轮次 / 任务 / 事件（群引擎仓储适配器的全部依赖）
//	type TagStore / AgentCatalog / GroupCronStore / Quota / Audit interface {…}
//	type SystemStore interface {…}      // 仅供电线使用的联合接口（NewServiceWithStore）
//	type Ports struct {…}               // NewService 的显式端口集合
//
// 调用架构
//
//	[上游] internal/app（装配时注入 *systemdb.Store / usage.Service / audit.Service）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/ports.go
//	         │ 依赖
//	         ▼
//	[下游] internal/systemdb（仅行类型与方法签名）, internal/audit
//	[同包] service.go
//
// 主要符号
//
//	type Ports struct {…}
//	type RunStore interface {…}
//	type ApprovalStore interface {…}
//	type ThreadStore interface {…}
//	... 另有 8 个顶层符号
//
// 约定（internal-ddd-hexagonal-implementation-plan §3.2，样板同 cronjob.Store / functionrun.Runner）：
// 接口在消费方定义，签名直接使用 systemdb 行类型，app 注入实现；不复制 DTO、不建万能仓储、
// 不把 *sql.DB / lease 泄露为契约。方法清单由 P0 的 grep 脚本核对（`store\.[A-Z]\w*\(` 于迁出文件）。
// 轮次 / 任务的状态迁移只经 gocloudagent/group 引擎 → 本包的 group_store.go，应用服务不另写状态机。

package agentrun

import (
	"context"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// Ports 是 NewService 需要的全部外部依赖；存储端口通常由同一个 *systemdb.Store 满足。
type Ports struct {
	Runs      RunStore
	Approvals ApprovalStore
	Threads   ThreadStore
	Collab    CollabStore
	Tags      TagStore
	Agents    AgentCatalog
	Cron      GroupCronStore
	// Quota 为 nil 表示不检查配额；Audit 为 nil 表示不记审计。
	Quota Quota
	Audit Audit
}

// RunStore 是 run 行与会话消息的仓储。
type RunStore interface {
	CreateAgentRun(ctx context.Context, r systemdb.AgentRun) (systemdb.AgentRun, error)
	GetAgentRun(ctx context.Context, projectID, id string) (systemdb.AgentRun, error)
	UpdateAgentRunStatus(ctx context.Context, projectID, id, status, errMsg string, started, finished *time.Time) error
	UpdateAgentRunMetrics(ctx context.Context, projectID, id string, r systemdb.AgentRun) error
	ListAgentRuns(ctx context.Context, projectID, threadID string, limit int) ([]systemdb.AgentRun, error)
	AppendAgentMessage(ctx context.Context, msg systemdb.AgentMessage) (systemdb.AgentMessage, error)
	ListAgentMessages(ctx context.Context, projectID, threadID string, limit int) ([]systemdb.AgentMessage, error)
	RecordAgentToolSpans(projectID, traceID, toolCardsJSON string) error
}

// ApprovalStore 是工具审批队列。
type ApprovalStore interface {
	CreateAgentApproval(ctx context.Context, a systemdb.AgentApproval) (systemdb.AgentApproval, error)
	GetPendingAgentApproval(ctx context.Context, projectID, runID, callID string) (systemdb.AgentApproval, error)
	ResolveAgentApproval(ctx context.Context, projectID, id, status, resolvedBy string) error
	CancelRunApprovals(ctx context.Context, projectID, runID string) error
}

// ThreadStore 是会话、工作区与协作行的仓储。
type ThreadStore interface {
	GetAgentThread(ctx context.Context, projectID, id string) (systemdb.AgentThread, error)
	CreateAgentThread(ctx context.Context, t systemdb.AgentThread) (systemdb.AgentThread, error)
	CreateAgentThreadWithWorkspace(ctx context.Context, t systemdb.AgentThread, ws systemdb.AgentThreadWorkspace) (systemdb.AgentThread, error)
	ArchiveAgentThread(ctx context.Context, projectID, id string) error
	GetAgentThreadWorkspace(ctx context.Context, projectID, threadID string) (systemdb.AgentThreadWorkspace, error)
	GetAgentThreadCollab(ctx context.Context, projectID, threadID string) (systemdb.AgentThreadCollab, error)
	CreateAgentThreadCollab(ctx context.Context, c systemdb.AgentThreadCollab) (systemdb.AgentThreadCollab, error)
}

// CollabStore 是群引擎仓储适配器（group_store.go）与会话轮次相关用例的全部依赖。
// HTTP 快照只读用到的方法（ListAgentTurns / LastAgentTurnSeq / ListHumanAgentTasks …）不在此列，入口自带读端口。
type CollabStore interface {
	CollabSchemaReady(ctx context.Context) error
	CreateAgentTurn(ctx context.Context, t systemdb.AgentTurn) (systemdb.AgentTurn, bool, error)
	GetAgentTurn(ctx context.Context, projectID, turnID string) (systemdb.AgentTurn, error)
	UpdateAgentTurnStatus(ctx context.Context, projectID, turnID, status, routeJSON, errCode string, expectedVersion int64) error
	ClearAgentTurnError(ctx context.Context, projectID, turnID string) error
	ClaimAgentTurn(ctx context.Context, projectID, threadID, turnID string) error
	ReleaseAgentTurn(ctx context.Context, projectID, threadID, turnID string) error
	ListRunningAgentTurns(ctx context.Context, cutoff time.Time) ([]systemdb.AgentTurn, error)
	LastAgentTurnSeq(ctx context.Context, projectID, turnID string) (int64, error)
	ListAgentTasks(ctx context.Context, projectID, turnID string) ([]systemdb.AgentTask, error)
	ReplaceAgentTasks(ctx context.Context, projectID, turnID string, tasks []systemdb.AgentTask) error
	GetAgentTask(ctx context.Context, projectID, taskID string) (systemdb.AgentTask, error)
	UpdateAgentTask(ctx context.Context, t systemdb.AgentTask, expectedVersion int64) (systemdb.AgentTask, error)
	AppendAgentTaskRun(ctx context.Context, r systemdb.AgentTaskRun) error
	AppendAgentMessageMeta(ctx context.Context, m systemdb.AgentMessageMeta) error
	AppendAgentTurnEventAtomic(ctx context.Context, ev systemdb.AgentTurnEvent) (systemdb.AgentTurnEvent, error)
	ListAgentTurnEvents(ctx context.Context, projectID, turnID string, after int64, limit int) ([]systemdb.AgentTurnEvent, error)
}

// TagStore 是工作台标签的读取（预置群的默认值回退在 tags.go）。
type TagStore interface {
	GetAgentTagRow(ctx context.Context, projectID, id string) (systemdb.AgentTag, error)
}

// AgentCatalog 是助手目录。
type AgentCatalog interface {
	GetCloudAgent(ctx context.Context, projectID, id string) (systemdb.CloudAgent, error)
	ListCloudAgents(ctx context.Context, projectID string) ([]systemdb.CloudAgent, error)
	SeedDefaultCloudAgentsWithCapabilities(ctx context.Context, projectID string, sandboxAvailable, skillsCLI bool) error
}

// GroupCronStore 是群定时任务的触发去重与绑定回写。
type GroupCronStore interface {
	ClaimCronFire(ctx context.Context, projectID, jobID, fireKey string) (bool, error)
	CompleteCronFire(ctx context.Context, projectID, jobID, fireKey, turnID, threadID string) error
	UpsertAgentGroupCron(ctx context.Context, c systemdb.AgentGroupCron) (systemdb.AgentGroupCron, error)
}

// Quota 是模型调用配额检查；*usage.Service 满足。
type Quota interface {
	CheckQuota(ctx context.Context, projectID, kind string) error
}

// Audit 是审计写入（只记元数据）；*audit.Service 满足。
type Audit interface {
	Record(ctx context.Context, e audit.Event) error
}

// SystemStore 是全部存储端口的联合，**只供电线**（NewServiceWithStore）把同一个仓储实现一次性填进 Ports；
// 业务代码不得持有本接口。
type SystemStore interface {
	RunStore
	ApprovalStore
	ThreadStore
	CollabStore
	TagStore
	AgentCatalog
	GroupCronStore
}

// FromStore 用同一个仓储实现填充全部存储端口（app 装配与测试共用）。
func FromStore(store SystemStore) Ports {
	return Ports{
		Runs: store, Approvals: store, Threads: store, Collab: store,
		Tags: store, Agents: store, Cron: store,
	}
}
