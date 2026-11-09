package auth

import (
	"testing"
	"time"
)

// E 批次：Remember 的 exp 参数参与作废记录的保留时间计算。
// 修复前：RevokeRun 固定按 31 分钟保留，exp 参数被丢弃（_ = exp）。
func TestDelegationRegistry_RevokeRetentionUsesExp(t *testing.T) {
	reg := NewDelegationRegistry()
	now := time.Now()
	// 两个 run 各持一个 jti：一个 exp 很近，一个 exp 很远。
	reg.Remember("run-a", "jti-a", now.Add(2*time.Minute))
	reg.Remember("run-b", "jti-b", now.Add(30*time.Minute))

	reg.RevokeRun("run-a")
	reg.RevokeRun("run-b")

	// 作废即刻生效。
	if !reg.Revoked("jti-a", now.Add(time.Second)) {
		t.Error("jti-a should be revoked immediately after RevokeRun")
	}
	if !reg.Revoked("jti-b", now.Add(time.Second)) {
		t.Error("jti-b should be revoked immediately after RevokeRun")
	}

	// 短 exp 的记录在 exp+1min 之后被惰性清理（此时该 jti 的令牌也已过期，
	// 记录清理不影响安全性——VerifyJWT 在 exp 处已拒绝）。
	if reg.Revoked("jti-a", now.Add(3*time.Minute+time.Second)) {
		t.Error("jti-a record should be dropped after exp+1min")
	}
	// 长 exp 的记录仍在保留期内（30min exp → 31min 内保留）。
	if !reg.Revoked("jti-b", now.Add(3*time.Minute)) {
		t.Error("jti-b record should still be retained at 3min")
	}
	// 到达 exp+1min 之后同样清理。
	if reg.Revoked("jti-b", now.Add(31*time.Minute+time.Second)) {
		t.Error("jti-b record should be dropped after exp+1min")
	}
}

// exp 为零值时回退到固定 31 分钟保留，兼容旧调用方。
func TestDelegationRegistry_ZeroExpFallsBack(t *testing.T) {
	reg := NewDelegationRegistry()
	now := time.Now()
	reg.Remember("run-c", "jti-c", time.Time{})
	reg.RevokeRun("run-c")
	if !reg.Revoked("jti-c", now.Add(30*time.Minute)) {
		t.Error("zero-exp record should be retained for fallback duration")
	}
	if reg.Revoked("jti-c", now.Add(31*time.Minute+time.Second)) {
		t.Error("zero-exp record should be dropped after fallback duration")
	}
}

// RevokeRun 后再次 Remember 同一 run 的新 jti 仍可作废（多次签发场景）。
func TestDelegationRegistry_ReIssueAfterRevoke(t *testing.T) {
	reg := NewDelegationRegistry()
	now := time.Now()
	reg.Remember("run-d", "jti-d1", now.Add(10*time.Minute))
	reg.RevokeRun("run-d")
	if !reg.Revoked("jti-d1", now.Add(time.Second)) {
		t.Fatal("jti-d1 should be revoked")
	}
	reg.Remember("run-d", "jti-d2", now.Add(10*time.Minute))
	if reg.Revoked("jti-d2", now.Add(time.Second)) {
		t.Error("new jti must not be revoked before RevokeRun")
	}
	reg.RevokeRun("run-d")
	if !reg.Revoked("jti-d2", now.Add(time.Second)) {
		t.Error("jti-d2 should be revoked after second RevokeRun")
	}
}
