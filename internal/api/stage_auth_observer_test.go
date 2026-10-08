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

	obs.ObserveAuthStage(timer.ctx, auth.AuthStageLockWait, 1500*time.Microsecond)
	obs.ObserveAuthStage(timer.ctx, auth.AuthStageUserLoad, 800*time.Microsecond)
	obs.ObserveAuthStage(timer.ctx, auth.AuthStageProjectExpand, 200*time.Microsecond)
	obs.ObserveAuthStage(timer.ctx, auth.AuthStageSessionLoad, 50*time.Microsecond)
	obs.ObserveAuthStage(timer.ctx, auth.AuthStageKeyLoad, 30*time.Microsecond)
	obs.ObserveAuthCache(timer.ctx, auth.AuthCacheMiss)

	spent, _ := timer.Snapshot()
	if spent[StageAuthLock] != 1500*time.Microsecond {
		t.Fatalf("lock_wait=%v", spent[StageAuthLock])
	}
	if spent[StageAuthUser] != 800*time.Microsecond {
		t.Fatalf("user_load=%v", spent[StageAuthUser])
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

	obs.ObserveAuthCache(timer.ctx, auth.AuthCacheHit)
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
		case StageAuthLock, StageAuthUser, StageAuthProject, StageAuthSession, StageAuthKey, StageSystemDB:
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
