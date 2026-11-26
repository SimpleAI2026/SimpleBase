package iamhttp

// login_guard_test.go：三维度登录防护的用例（自 internal/api/production_entry_test.go 随实现迁入）。

import (
	"testing"
	"time"
)

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

func TestLoginGuardDimensions(t *testing.T) {
	base := time.Unix(1000, 0)
	g := newLoginGuard(defaultLoginGuardOpts()).WithClock(func() time.Time { return base })

	// 同 IP 换用户名撞库：ip 维度 30/min。
	for i := 0; i < 30; i++ {
		if g.allow("1.1.1.1", "victim-"+itoaTest(i)) != loginAllowed {
			t.Fatalf("attempt %d should pass pair dimension", i)
		}
	}
	if v := g.allow("1.1.1.1", "another-user"); v != loginRateLimited {
		t.Fatalf("ip dimension should throttle credential stuffing, got %v", v)
	}

	// 多 IP 爆破单用户名：失败计数 → 软锁定。
	g2 := newLoginGuard(defaultLoginGuardOpts()).WithClock(func() time.Time { return base })
	for i := 0; i < 10; i++ {
		if g2.allow("10.0.0."+itoaTest(i), "admin") != loginAllowed {
			t.Fatalf("distributed attempt %d should pass rate dimensions", i)
		}
		g2.recordFailure("10.0.0."+itoaTest(i), "admin")
	}
	if v := g2.allow("10.9.9.9", "admin"); v != loginSoftLocked {
		t.Fatalf("username should be soft-locked, got %v", v)
	}

	// 软锁定没有 IP 豁免：即便该用户名刚在别的 IP 成功登录过，锁定后仍拒绝。
	g3 := newLoginGuard(defaultLoginGuardOpts()).WithClock(func() time.Time { return base })
	g3.recordSuccess("boss")
	for i := 0; i < 10; i++ {
		g3.recordFailure("10.0.0."+itoaTest(i), "boss")
	}
	if v := g3.allow("5.5.5.5", "boss"); v != loginSoftLocked {
		t.Fatalf("soft-locked username has no IP exemption, got %v", v)
	}

	// 锁定过期自动解除。
	g4 := newLoginGuard(defaultLoginGuardOpts())
	t0 := base
	g4.WithClock(func() time.Time { return t0 })
	for i := 0; i < 10; i++ {
		g4.recordFailure("10.0.0."+itoaTest(i), "late")
	}
	if g4.allow("8.8.8.8", "late") != loginSoftLocked {
		t.Fatal("should be locked")
	}
	t0 = base.Add(16 * time.Minute)
	if g4.allow("8.8.8.8", "late") != loginAllowed {
		t.Fatal("lock should expire after LockDuration")
	}

	// 成功登录清零失败计数。
	g5 := newLoginGuard(defaultLoginGuardOpts()).WithClock(func() time.Time { return base })
	for i := 0; i < 9; i++ {
		g5.recordFailure("10.0.0."+itoaTest(i), "recover")
	}
	g5.recordSuccess("recover")
	g5.recordFailure("9.9.9.9", "recover")
	if g5.allow("9.9.9.9", "recover") != loginAllowed {
		t.Fatal("success must reset failure count")
	}
}

func (g *loginGuard) WithClock(now func() time.Time) *loginGuard {
	g.now = now
	g.pair.now = now
	g.ip.now = now
	return g
}
