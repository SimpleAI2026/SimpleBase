// observer.go 实现 planv5.0 P0 的认证阶段低基数观测。
//
// SessionService.PrincipalFromClaims 曾在 principalMu 内做库查询（潜在锁
// convoy），P0 先不改行为，只把认证拆成可归因的子阶段：
//   - auth_lock_wait：等待 principalMu 的耗时（锁 convoy 的直接证据）；
//   - auth_cache_hit / auth_cache_miss：缓存命中状态；
//   - auth_user_load：GetByID 查用户耗时；
//   - auth_project_expand：PrincipalFromUser 展开项目集耗时。
//
// 观察者是可选依赖（nil-safe），由 api 装配处注入 StageTimer 桥接；
// 只记录时长与命中状态等低基数标签，禁止输出 token/用户名/SQL。
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
	AuthCacheHit    AuthCacheState = "hit"
	AuthCacheMiss   AuthCacheState = "miss"
	AuthCacheNone   AuthCacheState = "none"
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

// authObservers 组合多个观察者（装配处用于同时写 StageTimer 与指标）。
type authObservers []AuthObserver

func (os authObservers) ObserveAuthStage(ctx context.Context, stage AuthStage, d time.Duration) {
	for _, o := range os {
		if o != nil {
			o.ObserveAuthStage(ctx, stage, d)
		}
	}
}

func (os authObservers) ObserveAuthCache(ctx context.Context, state AuthCacheState) {
	for _, o := range os {
		if o != nil {
			o.ObserveAuthCache(ctx, state)
		}
	}
}

// noopAuthObserver 是缺省空观察者。
type noopAuthObserver struct{}

func (noopAuthObserver) ObserveAuthStage(context.Context, AuthStage, time.Duration) {}
func (noopAuthObserver) ObserveAuthCache(context.Context, AuthCacheState)           {}
