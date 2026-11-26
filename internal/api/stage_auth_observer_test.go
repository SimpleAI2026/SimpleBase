// 文件说明：stage_auth_observer_test.go — internal/api 的单元测试（TestStageAuthObserverWritesSubStages、TestStageAuthObserverNoopWithoutTimer 等）
//
// 用法
//
//	go test ./internal/api -run 'TestStageAuthObserverWritesSubStages|TestStageAuthObserverNoopWithoutTimer|TestStageAuthSubStagesExcludedFromAttribution|TestStageDurationsFloatPrecision'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/stage_auth_observer_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestStageAuthObserverWritesSubStages(t *testing.T)
//	TestStageAuthObserverNoopWithoutTimer(t *testing.T)
//	TestStageAuthSubStagesExcludedFromAttribution(t *testing.T)
//	TestStageDurationsFloatPrecision(t *testing.T)

// stage_auth_observer_test.go 验证 planv5.0 §4 P0.1 的认证子阶段桥接：
// auth.AuthObserver 事件写入请求级 StageTimer，子阶段不与 StageAuth 双算。
package api

import (
	"context"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
)

func TestStageAuthObserverWritesSubStages(t *testing.T) {
	timer := newStageTimer(context.Background())
	obs := newStageAuthObserver()

	obs.ObserveAuthStage(timer.WithContext(context.Background()), auth.AuthStageLockWait, 1500*time.Microsecond)
	obs.ObserveAuthStage(timer.WithContext(context.Background()), auth.AuthStageUserLoad, 800*time.Microsecond)
	obs.ObserveAuthStage(timer.WithContext(context.Background()), auth.AuthStageUserConn, 100*time.Microsecond)
	obs.ObserveAuthStage(timer.WithContext(context.Background()), auth.AuthStageUserQuery, 200*time.Microsecond)
	obs.ObserveAuthStage(timer.WithContext(context.Background()), auth.AuthStageUserScan, 300*time.Microsecond)
	obs.ObserveAuthStage(timer.WithContext(context.Background()), auth.AuthStageFlightWait, 400*time.Microsecond)
	obs.ObserveAuthStage(timer.WithContext(context.Background()), auth.AuthStageFlightLoad, 500*time.Microsecond)
	obs.ObserveAuthStage(timer.WithContext(context.Background()), auth.AuthStageProjectExpand, 200*time.Microsecond)
	obs.ObserveAuthStage(timer.WithContext(context.Background()), auth.AuthStageSessionLoad, 50*time.Microsecond)
	obs.ObserveAuthStage(timer.WithContext(context.Background()), auth.AuthStageKeyLoad, 30*time.Microsecond)
	obs.ObserveAuthCache(timer.WithContext(context.Background()), auth.AuthCacheMiss)

	spent, _ := timer.Snapshot()
	if spent[StageAuthLock] != 1500*time.Microsecond {
		t.Fatalf("lock_wait=%v", spent[StageAuthLock])
	}
	if spent[StageAuthUser] != 800*time.Microsecond {
		t.Fatalf("user_load=%v", spent[StageAuthUser])
	}
	for stage, expected := range map[Stage]time.Duration{
		StageAuthUserConn: 100 * time.Microsecond, StageAuthUserQuery: 200 * time.Microsecond,
		StageAuthUserScan: 300 * time.Microsecond, StageAuthFlightWait: 400 * time.Microsecond,
		StageAuthFlightLoad: 500 * time.Microsecond,
	} {
		if spent[stage] != expected {
			t.Fatalf("%s=%v want %v", stage, spent[stage], expected)
		}
	}
	if spent[StageAuthProject] != 200*time.Microsecond {
		t.Fatalf("project_expand=%v", spent[StageAuthProject])
	}
	if spent[StageAuthSession] != 50*time.Microsecond {
		t.Fatalf("session_load=%v", spent[StageAuthSession])
	}
	if spent[StageAuthKey] != 30*time.Microsecond {
		t.Fatalf("key_load=%v", spent[StageAuthKey])
	}
	if timer.Meta("auth_cache") != 0 {
		t.Fatalf("miss meta=%d", timer.Meta("auth_cache"))
	}

	obs.ObserveAuthCache(timer.WithContext(context.Background()), auth.AuthCacheHit)
	if timer.Meta("auth_cache") != 1 {
		t.Fatalf("hit meta=%d", timer.Meta("auth_cache"))
	}
}

func TestStageAuthObserverNoopWithoutTimer(t *testing.T) {
	// 分段计时关闭（ctx 无 timer）：观察者静默丢弃，不 panic。
	obs := newStageAuthObserver()
	obs.ObserveAuthStage(context.Background(), auth.AuthStageLockWait, time.Second)
	obs.ObserveAuthCache(context.Background(), auth.AuthCacheHit)
}

func TestStageAuthSubStagesExcludedFromAttribution(t *testing.T) {
	// 子阶段不得进入 perfStages 归因求和（否则与 StageAuth 双算）。
	for _, st := range perfStages {
		switch st {
		case StageAuthLock, StageAuthUser, StageAuthUserConn, StageAuthUserQuery, StageAuthUserScan,
			StageAuthFlightWait, StageAuthFlightLoad, StageAuthProject, StageAuthSession, StageAuthKey, StageSystemDB:
			t.Fatalf("sub-stage %s must not be in perfStages", st)
		}
	}
}

func TestStageDurationsFloatPrecision(t *testing.T) {
	// P0.2：亚毫秒段不得截断为 0。
	spent := map[Stage]time.Duration{
		StageCatalog: 700 * time.Microsecond,
		StageAuth:    1981 * time.Millisecond,
	}
	out := stageDurationsJSON(spent)
	if out[string(StageCatalog)] <= 0 {
		t.Fatalf("sub-ms stage truncated to %v", out[string(StageCatalog)])
	}
	if out[string(StageAuth)] < 1980 || out[string(StageAuth)] > 1982 {
		t.Fatalf("auth ms wrong: %v", out[string(StageAuth)])
	}
}
