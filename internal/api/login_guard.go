// 文件说明：login_guard.go — 三维度登录防护（§8.1.6：ip+username / ip / username 失败计数软锁定）
//
// 用法
//
//	g := newLoginGuard(opts)
//	verdict := g.allow(ip, username)   // 认证尝试前调用
//	g.recordSuccess(username)          // 登录成功后调用（清零失败计数）
//	g.recordFailure(ip, username)      // 登录失败后调用
//
//	软锁定：15 分钟内同用户名失败 10 次 → 锁 15 分钟（429 login_locked）。
//	软锁定没有 IP 豁免通道：锁定期间该用户名一律 429，直到窗口过期。
package api

import (
	"strings"
	"sync"
	"time"
)

// loginGuardOpts 三维度参数（§8.1.6 表）。
type loginGuardOpts struct {
	// PairMax / PairWindowDur：ip+username 维度（现有行为 10/min）。
	PairMax       int
	PairWindowDur time.Duration
	// IPMax / IPWindowDur：ip 维度，挡「固定密码换用户名」撞库（30/min）。
	IPMax       int
	IPWindowDur time.Duration
	// FailThreshold / FailWindow / LockDuration：用户名失败计数软锁定。
	FailThreshold int
	FailWindow    time.Duration
	LockDuration  time.Duration
}

func defaultLoginGuardOpts() loginGuardOpts {
	return loginGuardOpts{
		PairMax:       10,
		PairWindowDur: time.Minute,
		IPMax:         30,
		IPWindowDur:   time.Minute,
		FailThreshold: 10,
		FailWindow:    15 * time.Minute,
		LockDuration:  15 * time.Minute,
	}
}

// loginVerdict 是 allow 的结论。
type loginVerdict int

const (
	loginAllowed loginVerdict = iota
	loginRateLimited
	loginSoftLocked
)

type loginFailure struct {
	count    int
	lastFail time.Time
	lockedAt time.Time
}

// loginGuard 进程内登录防护；零外部依赖，停机即清零（爆破者的等待成本也归零，
// 但软锁定窗口内的新进程仍会从第一次失败重新计数——文档已写明取舍）。
type loginGuard struct {
	opts loginGuardOpts

	mu sync.Mutex
	// pair / ip 维度复用 loginLimiter 的窗口计数。
	pair *loginLimiter
	ip   *loginLimiter
	// failures 按用户名（小写）记录失败计数与锁定。
	failures map[string]*loginFailure
	now      func() time.Time
}

func newLoginGuard(opts loginGuardOpts) *loginGuard {
	if opts.PairMax <= 0 || opts.PairWindowDur <= 0 || opts.IPMax <= 0 || opts.IPWindowDur <= 0 ||
		opts.FailThreshold <= 0 || opts.FailWindow <= 0 || opts.LockDuration <= 0 {
		opts = defaultLoginGuardOpts()
	}
	return &loginGuard{
		opts:     opts,
		pair:     newLoginLimiter(opts.PairMax, opts.PairWindowDur),
		ip:       newLoginLimiter(opts.IPMax, opts.IPWindowDur),
		failures: map[string]*loginFailure{},
		now:      time.Now,
	}
}

func normalizeLoginUsername(u string) string { return strings.ToLower(strings.TrimSpace(u)) }

// allow 在认证尝试前调用：三维度依次判定。
func (g *loginGuard) allow(ip, username string) loginVerdict {
	uname := normalizeLoginUsername(username)
	g.mu.Lock()
	_, locked := g.checkLockLocked(uname)
	g.mu.Unlock()

	if locked {
		return loginSoftLocked
	}
	if !g.pair.allow(ip + "/" + uname) {
		return loginRateLimited
	}
	if !g.ip.allow(ip) {
		return loginRateLimited
	}
	return loginAllowed
}

// checkLockLocked 返回用户名是否处于软锁定窗口（调用方需持锁）。
func (g *loginGuard) checkLockLocked(uname string) (loginVerdict, bool) {
	f, ok := g.failures[uname]
	if !ok {
		return loginAllowed, false
	}
	now := g.now()
	// 失败窗口整体滑出后清零。
	if now.Sub(f.lastFail) > g.opts.FailWindow && f.lockedAt.IsZero() {
		delete(g.failures, uname)
		return loginAllowed, false
	}
	if !f.lockedAt.IsZero() {
		if now.Sub(f.lockedAt) >= g.opts.LockDuration {
			delete(g.failures, uname)
			return loginAllowed, false
		}
		return loginSoftLocked, true
	}
	return loginAllowed, false
}

// recordFailure 登录失败后调用：失败计数 +1，达到阈值进入软锁定。
func (g *loginGuard) recordFailure(ip, username string) {
	uname := normalizeLoginUsername(username)
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	f := g.failures[uname]
	if f == nil || now.Sub(f.lastFail) > g.opts.FailWindow {
		f = &loginFailure{}
		g.failures[uname] = f
	}
	f.count++
	f.lastFail = now
	if f.count >= g.opts.FailThreshold && f.lockedAt.IsZero() {
		f.lockedAt = now
	}
}

// recordSuccess 登录成功后调用：清零失败计数。
func (g *loginGuard) recordSuccess(username string) {
	uname := normalizeLoginUsername(username)
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.failures, uname)
}
