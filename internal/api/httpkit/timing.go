// 文件说明：timing.go — 请求分段计时（api-db-perf-validation-plan §2.1）：Stage 枚举与 StageTimer
//
// 用法
//
//	NewStageTimer(ctx) / StageTimerFrom(ctx)      // 中间件绑定、handler 取用（没有计时器时返回 nil，调用方判空）
//	(t *StageTimer) Observe(stage, d) / Scoped(stage) / Settle(stage) / SetMeta / Snapshot
//	PerfStages / StageDurationsJSON               // 阶段顺序与日志序列化
//	PerfStageMiddleware(recorder, logger)          // 分段计时中间件（放在 requestID 中间件之后）
//
// 调用架构
//
//	[上游] internal/api（perf 中间件、SQL / KV / 数据库 handler、认证 handler）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/httpkit/timing.go
//	         │ 依赖
//	         ▼
//	[下游] echo, go.uber.org/zap
//	[同包] errors.go
//
// 主要符号
//
//	type Stage string
//	type StageTimer struct {…}
//	type StageScope struct {…}
//	NewStageTimer(ctx context.Context) *StageTimer
//	StageTimerFrom(ctx context.Context) *StageTimer
//	... 另有 14 个顶层符号
//
// StageTimer 本身与传输无关；perf 中间件只依赖 Echo 与两个最小接口，也在这里。
// 认证子阶段观察者与带计时的 SQL lease 包装留在 api（它们依赖 auth / lease）。

package httpkit

import (
	"context"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// Stage 是请求链路的一个命名阶段。取值固定（低基数，可直接作 Prometheus label）。
type Stage string

const (
	// StageTotal 端到端总耗时（由中间件测量，含未归类时间）。
	StageTotal Stage = "total"
	// StageAuth 认证（JWT 校验 / API Key 查询）。
	StageAuth Stage = "auth"
	// StageProject project 解析（ResolveProjectTenant，走系统库）。
	StageProject Stage = "project_resolve"
	// StageDecode body 解码 + 校验 + sqlguard。
	StageDecode Stage = "decode_validate"
	// StageCatalog catalog GetDatabase / GetKVDatabase 系统库查询。
	StageCatalog Stage = "catalog_lookup"
	// StageSemaphore 并发槽位获取（MaxConcurrentQueries）。
	StageSemaphore Stage = "semaphore"
	// StageAcquire registry Acquire（热）/ WriteGate / 冷开 factory.Open。
	StageAcquire Stage = "acquire"
	// StageDBQueue 单连接排队（*sql.DB 内部等待），由 timedLease 记录。
	StageDBQueue Stage = "db_conn_queue"
	// StageDBExec SQL 执行 + 行扫描（godatabase.Query/Execute 内部）。
	StageDBExec Stage = "db_exec"
	// StageDBCommit KV 自管事务的 commit 部分（写放大分析；SQL 路径并入 db_exec）。
	StageDBCommit Stage = "db_commit"
	// StageSerialize 结果 JSON 安全转换与响应序列化。
	StageSerialize Stage = "serialize"
	// StageOnWrite 写后 onWrite 回调（CatalogSyncer.MarkDirty / sync_on_commit）。
	StageOnWrite Stage = "on_write"
	// StageUnattributed 归因不到任何段的剩余时间（total - 其余段之和）。
	StageUnattributed Stage = "unattributed"

	// —— 认证子阶段（planv5.0 §4 P0.1）：只细分 StageAuth，不与其重叠双算 ——
	// StageAuthLock 等 待认证锁（principalMu / apiKeyCache）的耗时；
	// StageAuthUser 为库查用户，StageAuthProjectSet 为展开项目集，
	// StageAuthSession 为 refresh 按 hash 查会话，StageAuthKey 为 API Key 仓储查询。
	// 这些子阶段包含在 StageAuth 总耗时内，归因时不得再计入 attributed。
	StageAuthLock       Stage = "auth_lock_wait"
	StageAuthUser       Stage = "auth_user_load"
	StageAuthUserConn   Stage = "auth_user_conn"
	StageAuthUserQuery  Stage = "auth_user_query"
	StageAuthUserScan   Stage = "auth_user_scan"
	StageAuthFlightWait Stage = "auth_flight_wait"
	StageAuthFlightLoad Stage = "auth_flight_load"
	StageAuthProject    Stage = "auth_project_expand"
	StageAuthSession    Stage = "auth_session_load"
	StageAuthKey        Stage = "auth_key_load"
	// StageAuthSessionUpdate / StageAuthSessionIssue 细分 refresh 轮转：
	// 前者为作废旧会话的 UPDATE，后者为新会话写入 + token 签发。
	StageAuthSessionUpdate Stage = "auth_session_update"
	StageAuthSessionIssue  Stage = "auth_session_issue"
	// StageSystemDB 系统库直连读（systemdb.Store 关键读入口）的细分段；
	// 不计入 PerfStages（与 db_exec 口径不同：未经 registry lease）。
	StageSystemDB Stage = "systemdb_read"
)

// PerfStages 是除 total/unattributed 外的全部阶段，顺序即链路顺序。
var PerfStages = []Stage{
	StageAuth, StageProject, StageDecode, StageCatalog, StageSemaphore,
	StageAcquire, StageDBQueue, StageDBExec, StageDBCommit, StageSerialize, StageOnWrite,
}

// StageTimer 在单个请求内累计各阶段耗时。并发安全（读多写少，mutex 足够）。
type StageTimer struct {
	mu     sync.Mutex
	spent  map[Stage]time.Duration
	ctx    context.Context
	extra  map[string]int64 // rows / status 等非耗时元数据
	scopes map[Stage]*StageScope
	start  time.Time
}

type stageTimerKey struct{}

// NewStageTimer 绑定到 ctx；后续 StageTimerFrom 可取出。
func NewStageTimer(ctx context.Context) *StageTimer {
	t := &StageTimer{
		spent: map[Stage]time.Duration{},
		extra: map[string]int64{},
		start: time.Now(),
		ctx:   ctx,
	}
	t.ctx = t.WithContext(ctx)
	return t
}

// WithContext 返回携带本 timer 的 context（供 adapter/handler 深层取用）。
func (t *StageTimer) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, stageTimerKey{}, t)
}

// StageTimerFrom 取出 ctx 上的 timer；未启用返回 nil。
func StageTimerFrom(ctx context.Context) *StageTimer {
	if ctx == nil {
		return nil
	}
	if t, ok := ctx.Value(stageTimerKey{}).(*StageTimer); ok {
		return t
	}
	return nil
}

// Observe 在 stage 上累加一段耗时。nil-safe。
func (t *StageTimer) Observe(stage Stage, d time.Duration) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.spent[stage] += d
	t.mu.Unlock()
}

// SetMeta 记录非耗时元数据（rows / status 等），仅用于日志行。nil-safe。
func (t *StageTimer) SetMeta(key string, v int64) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.extra[key] = v
	t.mu.Unlock()
}

// Meta 读取元数据。
func (t *StageTimer) Meta(key string) int64 {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.extra[key]
}

// Snapshot 返回各阶段累计耗时副本与元数据副本。
func (t *StageTimer) Snapshot() (map[Stage]time.Duration, map[string]int64) {
	if t == nil {
		return nil, nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	spent := make(map[Stage]time.Duration, len(t.spent))
	for k, v := range t.spent {
		spent[k] = v
	}
	extra := make(map[string]int64, len(t.extra))
	for k, v := range t.extra {
		extra[k] = v
	}
	return spent, extra
}

// StageScope 返回一个在作用域结束时自动把耗时记入 stage 的守卫。
// timer 为 nil 时返回 nil（调用方对 nil 调用 Done 仍安全）。
func (t *StageTimer) StageScope(stage Stage) *StageScope {
	if t == nil {
		return nil
	}
	return &StageScope{t: t, stage: stage, start: time.Now()}
}

// StageScope 是一次性计时守卫。Done 幂等；nil-safe。
type StageScope struct {
	t     *StageTimer
	stage Stage
	start time.Time
	done  bool
}

func (s *StageScope) Done() {
	if s == nil || s.done {
		return
	}
	s.done = true
	s.t.Observe(s.stage, time.Since(s.start))
}

// Scoped 返回作用域守卫并登记到 timer 上（供中间件在进入 next 前提前结算）。
// 与 StageScope 的区别：scope 存在 timer 内，跨中间件边界可见，按请求隔离。
func (t *StageTimer) Scoped(stage Stage) *StageScope {
	if t == nil {
		return nil
	}
	s := &StageScope{t: t, stage: stage, start: time.Now()}
	t.mu.Lock()
	if t.scopes == nil {
		t.scopes = map[Stage]*StageScope{}
	}
	t.scopes[stage] = s
	t.mu.Unlock()
	return s
}

// Settle 提前结算登记在 timer 上的指定段（幂等）：已结算或不存在时 no-op。
// 用于 auth 这类「中间件内部再放行 next」的段边界。
func (t *StageTimer) Settle(stage Stage) {
	if t == nil {
		return
	}
	t.mu.Lock()
	s := t.scopes[stage]
	delete(t.scopes, stage)
	t.mu.Unlock()
	s.Done()
}

// stageTimingRecorder 是 perfStageMiddleware 输出端的最小接口。
// stage 用 string 而非 api.Stage，使 *observability.Metrics 直接满足
// （保持 observability 不依赖 api 类型）。
// StageDurationsJSON 把各段耗时转为毫秒浮点（日志行用）。
// P0 修正（planv5.0 §4 P0.2）：Duration.Milliseconds() 会把 <1ms 截断成 0，
// 造成「无开销」假象；改用 float64 毫秒保留亚毫秒精度。
func StageDurationsJSON(spent map[Stage]time.Duration) map[string]float64 {
	out := make(map[string]float64, len(spent))
	for k, v := range spent {
		out[string(k)] = float64(v.Microseconds()) / 1000.0
	}
	return out
}

// loggerLike 抽象 zap 风格 Debug，避免中间件依赖具体 logger 实现。

// StartedAt 返回计时器创建时刻（端到端总耗时的起点）。
func (t *StageTimer) StartedAt() time.Time { return t.start }

type StageTimingRecorder interface {
	ObserveStage(route string, stage string, seconds float64)
}

// PerfStageMiddleware 按 plan §2.1 装配分段计时中间件。
//
// recorder 可为 nil（仅日志输出）；logger 可为 nil（仅指标输出）。
// 放在 requestIDMiddleware 之后，使日志行携带 request_id。
func PerfStageMiddleware(recorder StageTimingRecorder, logger LoggerLike) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			timer := NewStageTimer(c.Request().Context())
			c.SetRequest(c.Request().WithContext(timer.WithContext(c.Request().Context())))

			err := next(c)

			total := time.Since(timer.StartedAt())
			spent, extra := timer.Snapshot()
			spent[StageTotal] = total

			// 未归因时间 = total - 已归因段之和；为负时（外部注入的段耗时
			// 大于真实墙钟）记 0，保证账户行存在且不为负。
			var attributed time.Duration
			for _, st := range PerfStages {
				attributed += spent[st]
			}
			rest := total - attributed
			if rest < 0 {
				rest = 0
			}
			spent[StageUnattributed] = rest

			route := c.Path()
			if recorder != nil {
				for st, d := range spent {
					recorder.ObserveStage(route, string(st), d.Seconds())
				}
			}
			if logger != nil {
				logger.Debug("perf_stage",
					zap.String("request_id", RequestID(c)),
					zap.String("route", route),
					zap.String("method", c.Request().Method),
					zap.Duration("total", total),
					zap.Any("stages", StageDurationsJSON(spent)),
					zap.Any("meta", extra),
				)
			}
			return err
		}
	}
}

type LoggerLike interface {
	Debug(msg string, fields ...zap.Field)
}

// timedAuthMiddleware 包装 auth 中间件，把认证耗时计入 StageAuth。
// auth 包禁止反向依赖 api 包（internal/AGENTS.md），因此在装配处计时。
//
// 计时边界（§7.2 M1 修正）：inner(next) 会同步执行后续整条链路，
// 因此把 next 替换为「先结束 auth 计时再放行」的包装函数，
// 保证 auth 段只含认证本身（JWT 校验 / API Key 查询），
// 不吞掉 project/handler/DB 的耗时；认证失败由外层 Done 兜底（幂等）。
