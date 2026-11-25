// 文件说明：service.go — Agent 协作上下文的应用服务（Service：构造、能力、生命周期）
//
// 用法
//
//	svc := agentrun.NewService(cfg, agentrun.FromStore(store), runtime, confirms)   // app 装配（runtime 可为 nil）
//	svc.Converge(ctx)          // 启动收敛：把上一进程残留的 running 轮次落 interrupted
//	svc.BeginShutdown()        // 关停第 0 步（非阻塞）：拒绝新建、取消在途 run / 轮次
//	svc.Wait(ctx)              // 关停收尾：等全部收尾写库完成；超时返回 ctx.Err()
//	svc.Capabilities()         // 入口判定用的能力快照（LLM / MCP / CLI 技能 / 沙盒 / 确认窗口）
//
// 调用架构
//
//	[上游] internal/app（构造、Converge、BeginShutdown / Wait）、internal/api（用例调用）、
//	       cronjob → agentbridge.GroupCronRunner（群定时）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/service.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/{cloudagent,group,plan}, internal/agentbridge, internal/systemdb（行类型）
//	[同包] ports.go, run.go, run_events.go, confirm.go, turn.go, cron.go, group_engine.go
//
// 主要符号
//
//	type Config struct {…}
//	type Capabilities struct {…}
//	type Service struct {…}
//	NewService(cfg Config, ports Ports, runtime *cloudagent.Runtime, confirms *ConfirmHub) *Service
//	NewServiceWithStore(cfg Config, store SystemStore, runtime *cloudagent.Runtime, confirms *ConfirmHub) *Service
//	(s *Service) Capabilities() Capabilities
//	... 另有 9 个顶层符号
//
// 归属（internal-ddd-hexagonal-implementation-plan §3）：Service 持有根 ctx，全部 run / 轮次从根 ctx 派生，
// 生命周期归 app 管理（D6）：BeginShutdown 取消、Wait 等收尾。应用服务不认识 HTTP / Echo，
// 只返回领域错误；授权判定与协议编解码留在入口。

package agentrun

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
)

// Config 是入口无法从运行时反查的实例能力。
type Config struct {
	// SkillsCLI：实例是否提供 CLI 技能（KV / 云函数 / 定时模块的助手依赖它）。
	SkillsCLI bool
	// Sandbox：云沙盒是否可用（沙盒模块的助手依赖它）。
	Sandbox bool
}

// Capabilities 是入口做 503 / 能力判定用的只读快照，全部取自同一个 Service。
type Capabilities struct {
	LLMConfigured  bool
	MCPConfigured  bool
	LLMSettings    bool // 是否装配了项目模型设置读取（ListAgentModels 的默认模型）
	SkillsCLI      bool
	Sandbox        bool
	ConfirmTimeout time.Duration
	Stopping       bool
}

// Service 是 Agent 协作上下文的应用服务：固定助手会话的 run、协作群轮次、群定时、审批与事件环。
type Service struct {
	cfg      Config
	ports    Ports
	runtime  *cloudagent.Runtime
	events   *runEventHub
	confirms *ConfirmHub

	root   context.Context
	cancel context.CancelFunc
	// wg 登记直接 run（ExecuteRun）；群轮次的执行循环由各项目引擎拥有，经 WaitIdle 等待。
	wg       sync.WaitGroup
	stopping atomic.Bool

	// groupEngines 按项目隔离群引擎，并共享各请求的轮次登记 / 事件 broker。
	groupEngines sync.Map // projectID → *group.Engine
}

// NewService 构造应用服务。runtime 可为 nil（LLM 未启用）；confirms 由 app 先构造并注入
// Runtime.Confirm 与 RuntimeProvider.Confirm，这里同时持有其生命周期（nil 时自建）。
func NewService(cfg Config, ports Ports, runtime *cloudagent.Runtime, confirms *ConfirmHub) *Service {
	root, cancel := context.WithCancel(context.Background())
	if confirms == nil {
		confirms = NewConfirmHub()
	}
	return &Service{
		cfg: cfg, ports: ports, runtime: runtime, events: newRunEventHub(), confirms: confirms,
		root: root, cancel: cancel,
	}
}

// NewServiceWithStore 是装配与测试共用的便捷构造：同一个仓储实现填充全部存储端口。
func NewServiceWithStore(cfg Config, store SystemStore, runtime *cloudagent.Runtime, confirms *ConfirmHub) *Service {
	return NewService(cfg, FromStore(store), runtime, confirms)
}

// Capabilities 返回入口判定用的能力快照。
func (s *Service) Capabilities() Capabilities {
	caps := Capabilities{SkillsCLI: s.cfg.SkillsCLI, Sandbox: s.cfg.Sandbox, ConfirmTimeout: s.confirmTimeout(), Stopping: s.stopping.Load()}
	if provider := s.provider(); provider != nil && provider.Settings != nil {
		caps.LLMSettings = true
	}
	if s.runtime != nil {
		caps.LLMConfigured = s.runtime.LLM != nil
		caps.MCPConfigured = s.runtime.MCP != nil
	}
	return caps
}

// BeginShutdown 进入停机（幂等、非阻塞）：之后拒绝新建 run / 轮次 / 群定时（ErrStopping），
// 取消根 ctx 让在途直接 run 收尾，并让各项目引擎取消有执行循环的轮次（停在
// waiting_human / awaiting_plan_confirm 的轮次不受影响，仍由下次启动的 Converge 收敛）。
// 调用方随后用 Wait 等全部收尾写库完成。
func (s *Service) BeginShutdown() {
	if !s.stopping.CompareAndSwap(false, true) {
		return
	}
	s.cancel()
	s.groupEngines.Range(func(_, value any) bool {
		value.(*group.Engine).BeginShutdown()
		return true
	})
}

// Wait 等待全部在途工作收尾落库：先等群引擎的执行循环（其中包含群任务的 run），
// 再等直接 run；超时返回 ctx.Err()，调用方记录告警后不阻塞后续关停。
func (s *Service) Wait(ctx context.Context) error {
	if err := s.waitEngines(ctx); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close 是 BeginShutdown + Wait 的组合（测试与非 App 调用方使用）。
func (s *Service) Close(ctx context.Context) error {
	s.BeginShutdown()
	return s.Wait(ctx)
}

// waitEngines 逐个等待项目引擎空闲；停机后才创建的引擎由 groupEngineFor 自行停机。
func (s *Service) waitEngines(ctx context.Context) error {
	var firstErr error
	s.groupEngines.Range(func(_, value any) bool {
		if err := value.(*group.Engine).WaitIdle(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr == nil
	})
	return firstErr
}

// ActiveTurns 返回仍有执行循环的轮次数（关停超时时的诊断信息）。
func (s *Service) ActiveTurns() map[string]int {
	out := map[string]int{}
	s.groupEngines.Range(func(key, value any) bool {
		out[key.(string)] = len(value.(*group.Engine).ActiveTurns())
		return true
	})
	return out
}

// Converge 启动收敛：只在启动时运行一次，避免把后续新建的在途轮次误判为僵尸。
// 把上一进程残留的 running 轮次交给各项目引擎落 interrupted（不自动重跑）。
func (s *Service) Converge(ctx context.Context) {
	if s.ports.Collab == nil || s.ports.Collab.CollabSchemaReady(ctx) != nil {
		return
	}
	turns, err := s.ports.Collab.ListRunningAgentTurns(ctx, time.Now().UTC())
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for _, turn := range turns {
		if seen[turn.ProjectID] {
			continue
		}
		seen[turn.ProjectID] = true
		if engine, err := s.groupEngineFor(turn.ProjectID); err == nil {
			_ = engine.Converge(ctx)
		}
	}
}

// provider 返回宿主 RuntimeProvider；运行时未装配或 Provider 不是宿主实现时为 nil。
func (s *Service) provider() *agentbridge.RuntimeProvider {
	if s.runtime == nil {
		return nil
	}
	provider, _ := s.runtime.Provider.(*agentbridge.RuntimeProvider)
	return provider
}

// DefaultModel 返回项目模型设置里的默认模型；未装配设置读取或读取失败时返回空串。
func (s *Service) DefaultModel(ctx context.Context, projectID string) string {
	if provider := s.provider(); provider != nil && provider.Settings != nil {
		if st, err := provider.Settings.LLMSettings(ctx, projectID); err == nil {
			return st.DefaultModel
		}
	}
	return ""
}

// ReleaseThreadSandbox 释放会话占用的云沙盒（会话归档时调用）；无沙盒时为空操作，失败原样返回。
func (s *Service) ReleaseThreadSandbox(ctx context.Context, projectID, threadID string) error {
	if provider := s.provider(); provider != nil && provider.Sandbox != nil && provider.Sandbox.Available() {
		return provider.Sandbox.ReleaseThread(ctx, projectID, threadID)
	}
	return nil
}

// IsRunActive 报告 run 是否仍在本进程执行（列表接口据此过滤重启后残留的 running 行）。
func (s *Service) IsRunActive(runID string) bool {
	return s.runtime != nil && s.runtime.IsRunActive(runID)
}

// llmConfigured 报告模型服务是否可用。
func (s *Service) llmConfigured() bool { return s.runtime != nil && s.runtime.LLM != nil }

// confirmTimeout 返回审批等待窗口；未配置时用 2m（与 config.example.yaml 默认一致）。
func (s *Service) confirmTimeout() time.Duration {
	if s.runtime != nil && s.runtime.ConfirmTimeout > 0 {
		return s.runtime.ConfirmTimeout
	}
	return 2 * time.Minute
}
