// 文件说明：doctor.go — 部署后自检报告（production-readiness-plan §8.3.6）
//
// 用法
//
//	report := app.DoctorReport(cfg)
//	report.Text() / report.JSON() / report.OK()
//
// 纯配置层检查：不启动 HTTP、不触碰系统库；
// 检查项：profile / 密钥域 / 引导口令 / 观测收口 / trusted_proxies / 磁盘水位 / 备份窗口。
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/alerts"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

// DoctorCheck 是单条检查结果。
type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | fail
	Detail string `json:"detail"`
}

// DoctorCheckStatus 取值。
const (
	DoctorOK   = "ok"
	DoctorWarn = "warn"
	DoctorFail = "fail"
)

// DoctorReport 是 doctor 的完整输出。
type DoctorReport struct {
	Profile string        `json:"profile"`
	Time    time.Time     `json:"time"`
	Checks  []DoctorCheck `json:"checks"`
}

// OK 全部检查非 fail 时返回 true。
func (r DoctorReport) OK() bool {
	for _, c := range r.Checks {
		if c.Status == DoctorFail {
			return false
		}
	}
	return true
}

// Text 人类可读输出。
func (r DoctorReport) Text() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "simplebase doctor  profile=%s  time=%s\n", r.Profile, r.Time.Format(time.RFC3339))
	for _, c := range r.Checks {
		mark := "✓"
		if c.Status == DoctorWarn {
			mark = "!"
		} else if c.Status == DoctorFail {
			mark = "x"
		}
		fmt.Fprintf(&b, "  [%s] %-24s %s\n", mark, c.Name, c.Detail)
	}
	if r.OK() {
		b.WriteString("result: PASS\n")
	} else {
		b.WriteString("result: FAIL\n")
	}
	return b.String()
}

// JSON 机器可读输出。
func (r DoctorReport) JSON() string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

// RunDoctor 生成部署后自检报告（§8.3.6）。
func RunDoctor(cfg config.Config) DoctorReport {
	r := DoctorReport{Profile: cfg.Instance.Profile, Time: time.Now().UTC()}
	add := func(name, status, detail string) {
		r.Checks = append(r.Checks, DoctorCheck{Name: name, Status: status, Detail: detail})
	}

	// 1. profile。
	profileStatus := DoctorWarn
	profileDetail := fmt.Sprintf("instance.profile=%q (production 启用安全默认值)", cfg.Instance.Profile)
	if cfg.IsProduction() {
		profileStatus = DoctorOK
	}
	add("profile", profileStatus, profileDetail)

	// 2. 密钥域：session secret 来源。
	sessionSrc := "inherited（由 API Key 盐派生；轮换 Key 盐会使登录态失效）"
	if cfg.Auth.SessionSecretExplicit {
		sessionSrc = "explicit（SIMPLEBASE_AUTH_SESSION_SECRET）"
	}
	add("session-secret", DoctorOK, fmt.Sprintf("来源 %s；长度 %d", sessionSrc, len(cfg.Auth.SessionSecret)))

	// 3. 引导口令（production 必须注入）。
	bootLen := len(strings.TrimSpace(cfg.Auth.BootstrapPassword))
	if cfg.IsProduction() {
		if bootLen == 0 {
			add("bootstrap-password", DoctorFail, "production 必须注入 SIMPLEBASE_BOOTSTRAP_PASSWORD")
		} else if bootLen < 16 {
			add("bootstrap-password", DoctorFail, "引导口令长度不足 16")
		} else {
			add("bootstrap-password", DoctorOK, "已注入（长度满足）")
		}
	} else if bootLen == 0 {
		add("bootstrap-password", DoctorWarn, "未注入：使用内置默认口令（仅限 development）")
	} else {
		add("bootstrap-password", DoctorOK, "已注入")
	}

	// 4. 观测收口。
	if cfg.Observability.MetricsAddress != "" {
		add("observability", DoctorOK, fmt.Sprintf("/metrics 独立监听 %s（业务端口 404）", cfg.Observability.MetricsAddress))
	} else if cfg.Observability.MetricsToken != "" {
		add("observability", DoctorOK, "/metrics 由 Bearer token 保护")
	} else if cfg.IsProduction() {
		add("observability", DoctorFail, "production 必须配置 metrics_address 或 SIMPLEBASE_METRICS_TOKEN")
	} else {
		add("observability", DoctorWarn, "/metrics 在业务端口开放（development 默认）")
	}

	// 5. trusted_proxies。
	if len(cfg.HTTP.TrustedProxies) == 0 {
		add("trusted-proxies", DoctorWarn, "未配置：X-Forwarded-For 不被信任（直连场景正确；有反代需配置）")
	} else {
		bad := false
		for _, cidr := range cfg.HTTP.TrustedProxies {
			if cidr == "0.0.0.0/0" || cidr == "::/0" {
				bad = true
			}
		}
		if bad {
			add("trusted-proxies", DoctorFail, "包含 0.0.0.0/0 或 ::/0（信任任意来源的 XFF）")
		} else {
			add("trusted-proxies", DoctorOK, strings.Join(cfg.HTTP.TrustedProxies, ","))
		}
	}

	// 6. 磁盘水位。
	if cfg.Storage.DataDir != "" {
		ratio, err := alerts.DiskFreeRatio(cfg.Storage.DataDir)
		switch {
		case err != nil:
			add("disk", DoctorWarn, fmt.Sprintf("无法读取 %s：%v", cfg.Storage.DataDir, err))
		case ratio < 0.10:
			add("disk", DoctorFail, fmt.Sprintf("%s 可用空间 %.1f%%（< 10%%）", cfg.Storage.DataDir, ratio*100))
		case ratio < 0.20:
			add("disk", DoctorWarn, fmt.Sprintf("%s 可用空间 %.1f%%（< 20%%）", cfg.Storage.DataDir, ratio*100))
		default:
			add("disk", DoctorOK, fmt.Sprintf("%s 可用空间 %.1f%%", cfg.Storage.DataDir, ratio*100))
		}
	} else {
		add("disk", DoctorWarn, "storage.data_dir 未配置，跳过磁盘检查")
	}

	// 7. 备份窗口。
	if cfg.Backup.Enabled {
		add("backup", DoctorOK, fmt.Sprintf("schedule=%q missed_policy=%s keep_last=%d",
			cfg.Backup.Schedule, cfg.Backup.EffectiveMissedPolicy(), cfg.Backup.KeepLast))
	} else if cfg.IsProduction() {
		add("backup", DoctorFail, "production 下备份默认应开启（显式 backup.enabled=false 已跳过）")
	} else {
		add("backup", DoctorWarn, "备份未启用（development 可接受）")
	}

	// 8. 限流。
	if cfg.Limits.RateLimit.Enabled {
		add("rate-limit", DoctorOK, fmt.Sprintf("%d 条规则 / max_buckets=%d", len(cfg.Limits.RateLimit.Rules), cfg.Limits.RateLimit.MaxBuckets))
	} else {
		add("rate-limit", DoctorWarn, "限流未启用")
	}
	return r
}
