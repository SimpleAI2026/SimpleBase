// 文件说明：observer.go — planv5.0 P0 的认证阶段低基数观测
//
// 用法
//
//	type AuthStage（导出类型，同包与上层均可使用）
//	type AuthCacheState（导出类型，同包与上层均可使用）
//	同包调用：AuthObserver → service.go, session.go 等；AuthCacheHit → cache.go, session.go；AuthCacheMiss → cache.go, session.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/auth/observer.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] api_key_repository.go, cache.go, delegation.go, id.go, jwt.go 等 15 个
//	[关键引用] AuthObserver ← service.go, session.go, user_repository.go
//
// 主要符号
//
//	type AuthStage string
//	const AuthStageLockWait
//	const AuthStageUserLoad
//	const AuthStageUserConn
//	const AuthStageUserQuery
//	const AuthStageUserScan
//	... 另有 13 个顶层符号

package auth

import (
	"context"
	"time"
)

// AuthStage 是认证子阶段的低基数标签（可直接作日志/指标 label）。
type AuthStage string

const (
	// AuthStageLockWait 等待 principalMu 的耗时。
	AuthStageLockWait AuthStage = "lock_wait"
	// AuthStageUserLoad 库查用户（GetByID）耗时。
	AuthStageUserLoad AuthStage = "user_load"
	// 以下阶段是 user_load / auth 的子段，不参与总耗时归因。
	AuthStageUserConn   AuthStage = "user_conn"
	AuthStageUserQuery  AuthStage = "user_query"
	AuthStageUserScan   AuthStage = "user_scan"
	AuthStageFlightWait AuthStage = "flight_wait"
	AuthStageFlightLoad AuthStage = "flight_load"
	// AuthStageProjectExpand 展开项目集（PrincipalFromUser）耗时。
	AuthStageProjectExpand AuthStage = "project_expand"
	// AuthStageKeyLoad API Key 未命中缓存时的仓储查询耗时。
	AuthStageKeyLoad AuthStage = "key_load"
	// AuthStageSessionLoad refresh 流程按 hash 查会话耗时。
	AuthStageSessionLoad AuthStage = "session_load"
	// AuthStageSessionUpdate refresh 轮转：作废旧会话（UPDATE）耗时。
	AuthStageSessionUpdate AuthStage = "session_update"
	// AuthStageSessionIssue refresh 轮转：写入新会话 + 签发 token 对耗时。
	AuthStageSessionIssue AuthStage = "session_issue"
)

// AuthCacheState 是缓存命中状态（低基数）。
type AuthCacheState string

const (
	AuthCacheHit  AuthCacheState = "hit"
	AuthCacheMiss AuthCacheState = "miss"
	AuthCacheNone AuthCacheState = "none"
)

// AuthObserver 是认证链路的观测接口（nil-safe：实现方自行判空）。
//
// stage 与 cache 标签均为固定集合；duration 可能为 0（无该阶段）。
// ctx 供实现方关联请求级计时器（如 api.StageTimer）；不得从 ctx 提取
// 任何凭据或高基数字段。
type AuthObserver interface {
	// ObserveAuthStage 记录一个认证子阶段耗时。
	ObserveAuthStage(ctx context.Context, stage AuthStage, d time.Duration)
	// ObserveAuthCache 记录缓存命中状态（hit/miss；无缓存通道用 none）。
	ObserveAuthCache(ctx context.Context, state AuthCacheState)
}

// noopAuthObserver 是缺省空观察者。
type noopAuthObserver struct{}

func (noopAuthObserver) ObserveAuthStage(context.Context, AuthStage, time.Duration) {}
func (noopAuthObserver) ObserveAuthCache(context.Context, AuthCacheState)           {}
