// 文件说明：doctor_test.go — 部署后自检报告测试（§8.3.6）
package app

import (
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
)

func TestDoctorReportDevBaseline(t *testing.T) {
	cfg := config.Config{
		Instance: config.InstanceConfig{Profile: config.ProfileDevelopment},
		Auth:     config.AuthConfig{APIKeyHashSecret: "k", SessionSecret: "explicit-secret", SessionSecretExplicit: true},
	}
	r := RunDoctor(cfg)
	if !r.OK() {
		t.Fatalf("dev baseline should not fail: %s", r.Text())
	}
	if !strings.Contains(r.Text(), "result: PASS") {
		t.Fatal("text output missing PASS")
	}
	if !strings.Contains(r.JSON(), `"profile": "development"`) {
		t.Fatal("json output missing profile")
	}
}

func TestDoctorReportProductionGaps(t *testing.T) {
	// production 且缺关键项 → fail。
	cfg := config.Config{
		Instance: config.InstanceConfig{Profile: config.ProfileProduction},
		Auth:     config.AuthConfig{APIKeyHashSecret: "k"},
	}
	r := RunDoctor(cfg)
	if r.OK() {
		t.Fatalf("production without bootstrap password must fail: %s", r.Text())
	}
	hasFail := false
	for _, c := range r.Checks {
		if c.Status == DoctorFail {
			hasFail = true
		}
	}
	if !hasFail {
		t.Fatal("expected at least one fail check")
	}

	// 补齐后通过。
	cfg.Auth.BootstrapPassword = "a-strong-enough-bootstrap-password"
	cfg.Observability.MetricsToken = "tok"
	cfg.HTTP.TrustedProxies = []string{"10.0.0.5/32"}
	cfg.Limits.RateLimit.Enabled = true
	cfg.Limits.RateLimit.Rules = []config.RateLimitRule{{Match: "/v1/**", Max: 100, Window: time.Minute, Key: config.RateLimitKeyPrincipal}}
	cfg.Backup = config.BackupConfig{Enabled: true, Schedule: "0 4 * * *", KeepLast: 1, MaxPerProject: 2, MissedPolicy: config.MissedPolicyCatchup}
	r = RunDoctor(cfg)
	if !r.OK() {
		t.Fatalf("complete production should pass: %s", r.Text())
	}
}

func TestDoctorReportTrustedProxyWildcard(t *testing.T) {
	cfg := config.Config{
		Instance: config.InstanceConfig{Profile: config.ProfileProduction},
		Auth:     config.AuthConfig{APIKeyHashSecret: "k", BootstrapPassword: "a-strong-enough-bootstrap-password", SessionSecret: "s"},
		Observability: config.ObservabilityConfig{MetricsToken: "tok"},
		HTTP:     config.HTTPConfig{TrustedProxies: []string{"0.0.0.0/0"}},
	}
	r := RunDoctor(cfg)
	if r.OK() {
		t.Fatal("0.0.0.0/0 must fail doctor")
	}
}
