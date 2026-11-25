// 文件说明：types.go — 通用 Plan 能力的数据结构（planv6.8 §13.4/§13.5）
//
// plan 包与宿主无关：只描述「目标、分工、确认、进度」，由 Validate 按 Constraints 校验，
// 由 Planner 调模型产出，由 Store 持久化。群聊调度（group 包）、本地 CLI、云端宿主共用。
//
// 文件分工：
//   - types.go      Decision / Dispatch / Confirm / Step / Progress / Constraints / Limits
//   - validate.go   Validate：首错返回，错误可用 errors.Is 归类
//   - schema.go     plan / explore / reflect 工具的 JSON Schema 与参数解码
//   - tool.go       eino 工具实现（plan / explore / reflect）
//   - planner.go    Planner：强制模型以 plan 工具调用输出分工（进度归约在 group 包）
//   - store.go      Store 接口、MemoryStore、FileStore（planning-with-files 布局）

package plan

// 动作。
const (
	ActionCreate   = "create"
	ActionUpdate   = "update"
	ActionReplan   = "replan"
	ActionComplete = "complete"
)

// 复杂度。
const (
	ComplexitySimple  = "simple"
	ComplexityComplex = "complex"
)

// 目标类型。
const (
	TargetAgent = "agent"
	TargetHuman = "human"
)

// 风险等级（destructive 强制计划确认）。
const (
	RiskRead        = "read"
	RiskWrite       = "write"
	RiskDestructive = "destructive"
)

// 步骤状态。
const (
	StepPending    = "pending"
	StepInProgress = "in_progress"
	StepDone       = "done"
	StepBlocked    = "blocked"
	StepSkipped    = "skipped"
)

// Dispatch 是一次 @：交给谁、发什么、依赖谁。
type Dispatch struct {
	Ref            string   `json:"ref"`
	TargetKind     string   `json:"target_kind"`
	TargetID       string   `json:"target_id"`
	Message        string   `json:"message"`
	ExpectedOutput string   `json:"expected_output,omitempty"`
	DependsOn      []string `json:"depends_on,omitempty"`
	ContextFrom    []string `json:"context_from,omitempty"`
	Risk           string   `json:"risk,omitempty"`
	StepID         string   `json:"step_id,omitempty"`
}

// Confirm 描述执行前是否需要人确认整份计划。
type Confirm struct {
	Required bool     `json:"required"`
	Reason   string   `json:"reason,omitempty"`
	Question string   `json:"question,omitempty"`
	Options  []string `json:"options,omitempty"`
	// Editable 为 nil 时按 true 处理（允许编辑 message / 删除 dispatch）。
	Editable *bool `json:"editable,omitempty"`
}

// CanEdit 报告确认时是否允许编辑。
func (c Confirm) CanEdit() bool { return c.Editable == nil || *c.Editable }

// Step 是复杂计划的里程碑。
type Step struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Evidence string `json:"evidence,omitempty"`
}

// Progress 是进度摘要（群头部与进度条展示）。
type Progress struct {
	Percent int    `json:"percent"`
	Summary string `json:"summary,omitempty"`
	Next    string `json:"next,omitempty"`
}

// Decision 是 plan 工具的一次完整输出（也是持久化形状）。
type Decision struct {
	Action      string     `json:"action"`
	Goal        string     `json:"goal,omitempty"`
	Complexity  string     `json:"complexity,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	Dispatches  []Dispatch `json:"dispatches,omitempty"`
	Confirm     Confirm    `json:"confirm"`
	Steps       []Step     `json:"steps,omitempty"`
	Progress    Progress   `json:"progress"`
	NeedSummary bool       `json:"need_summary,omitempty"`
	Findings    []string   `json:"findings,omitempty"`
	Unknowns    []string   `json:"unknowns,omitempty"`
	// Source 记录决策来源：model / rule / edited（宿主写入，不由模型提供）。
	Source string `json:"source,omitempty"`
}

// 决策来源。
const (
	SourceModel  = "model"
	SourceRule   = "rule"
	SourceEdited = "edited"
)

// Candidate 是 Planner 可见的一个助手（不含密钥与完整工具 schema）。
type Candidate struct {
	ID          string   `json:"agent_id"`
	Name        string   `json:"name"`
	Module      string   `json:"module,omitempty"`
	Description string   `json:"description,omitempty"`
	Skills      []string `json:"skills,omitempty"`
}

// Limits 是任务图上限。
type Limits struct {
	MaxDispatches int
	MaxDepth      int
	MaxMessage    int
}

// DefaultLimits 返回默认上限：8 个 dispatch、依赖深度 4、单条消息 2000 字。
func DefaultLimits() Limits { return Limits{MaxDispatches: 8, MaxDepth: 4, MaxMessage: 2000} }

func (l Limits) normalized() Limits {
	d := DefaultLimits()
	if l.MaxDispatches <= 0 || l.MaxDispatches > 32 {
		l.MaxDispatches = d.MaxDispatches
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = d.MaxDepth
	}
	if l.MaxMessage <= 0 {
		l.MaxMessage = d.MaxMessage
	}
	return l
}

// Constraints 是宿主对一次决策的硬约束。
type Constraints struct {
	// Members 是可 @ 的助手 ID。
	Members []string
	// HumanID 非空表示可以把任务交给该人。
	HumanID string
	// HumanDisabled 为真表示群策略禁止人工任务（与 HumanID 为空区分错误码）。
	HumanDisabled bool
	// Mentioned 是用户显式 @ 的助手：每个都必须至少出现一次。
	Mentioned []string
	// MentionedHuman 为真表示用户显式 @ 了人：必须包含一个 human dispatch。
	MentionedHuman bool
	Limits         Limits
}

// ConfirmPolicy 是宿主强制的确认策略（只能加严，不能放宽 destructive）。
type ConfirmPolicy struct {
	// Mode：auto（默认，模型建议 ∪ 规则）/ always / never（只忽略模型建议）。
	Mode string
	// MinDispatches 达到该数量时强制确认；0 取 3，负数关闭该规则。
	MinDispatches int
	// MinDepth 依赖深度达到该值时强制确认；0 取 2，负数关闭该规则。
	MinDepth int
}

// 确认模式。
const (
	ConfirmAuto   = "auto"
	ConfirmAlways = "always"
	ConfirmNever  = "never"
)
