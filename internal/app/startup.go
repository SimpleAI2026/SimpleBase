// 文件说明：startup.go — 启动横幅与启动阶段计时日志
//
// 用法
//
//	logStartupBanner(logger, cfg)   // 启动首行：构建信息 + 生效配置摘要
//	newStartupClock(logger) *startupClock
//	(c *startupClock) step(name string, fields ...zap.Field)  // 记录单阶段耗时
//	(a *App) step(name string, fields ...zap.Field)           // 带 nil 保护的包装
//
// 调用架构
//
//	[上游] internal/app（NewWithRegistry / assembleDeps / Start / Shutdown）
//	         │ 调用本文件
//	         ▼
//	[下游] internal/config, internal/observability
//
// 设计目标：部署（尤其是容器）里「日志太少看不到进度」时，用一组固定行
// 覆盖「二进制是什么版本 → 配置从哪来 → 每个装配阶段用了多久 → 监听在哪」。
// 所有字段只含非密钥信息（密钥布尔化，见 config.Config.Redacted）。
package app

import (
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// buildInfo 汇总二进制自身的构建信息（-ldflags 未注入时退化为 dev）。
type buildInfo struct {
	Version   string
	Revision  string
	BuildTime string
	Dirty     bool
}

// readBuildInfo 从 runtime/debug 读取 Go 模块与 VCS 信息。
// 二进制被 `go build`（无 -ldflags）构建时也能拿到 vcs.revision，便于排查
// 「镜像里到底是哪次提交」。
func readBuildInfo() buildInfo {
	out := buildInfo{Version: "dev"}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return out
	}
	if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		out.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			out.Revision = s.Value
		case "vcs.time":
			out.BuildTime = s.Value
		case "vcs.modified":
			out.Dirty = s.Value == "true"
		}
	}
	return out
}

// logSitesMode 输出站点访问方式（planv6.7 §1.1）。
// base_domain 已配置而 subdomain=false 时给出兼容告警：升级前依赖子域名的部署需显式开启。
func logSitesMode(logger observability.Logger, cfg config.Config) {
	if logger == nil {
		return
	}
	s := cfg.Sites
	if s.Mode() == config.SiteModePath {
		logger.Info("sites mode=path", zap.String("mode", config.SiteModePath), zap.String("prefix", s.EffectivePathPrefix()))
		if strings.TrimSpace(s.BaseDomain) != "" {
			logger.Warn("sites.base_domain is set but SIMPLEBASE_SITES_SUBDOMAIN is not 1; subdomain routing is disabled, using path mode",
				zap.String("base_domain", s.BaseDomain))
		}
		return
	}
	logger.Info("sites mode=subdomain", zap.String("mode", config.SiteModeSubdomain), zap.String("domain", s.BaseDomain))
}

// logStartupBanner 在构造任何依赖之前输出第一行日志。
// 这一行的作用是让「进程起来了但没日志」可区分：只要有它，就说明配置已加载、
// 日志级别解析成功。
func logStartupBanner(logger observability.Logger, cfg config.Config) {
	if logger == nil {
		return
	}
	bi := readBuildInfo()
	cwd, _ := os.Getwd()
	// 容器里 /simplebase 由 ENTRYPOINT 启动时 cwd 通常是 /；config.yaml 是相对 cwd 解析的，
	// 「配置文件没生效」八成是 cwd 变了，所以必须打出来。
	logger.Info("simplebase starting",
		zap.String("version", bi.Version),
		zap.String("revision", bi.Revision),
		zap.Bool("dirty", bi.Dirty),
		zap.String("build_time", bi.BuildTime),
		zap.Int("pid", os.Getpid()),
		zap.Int("ppid", os.Getppid()),
		zap.String("cwd", cwd),
		zap.String("go_version", runtime.Version()),
		zap.String("goos", runtime.GOOS),
		zap.String("goarch", runtime.GOARCH),
		zap.Int("num_cpu", runtime.NumCPU()),
		zap.Int("gomaxprocs", runtime.GOMAXPROCS(0)),
		zap.String("instance_id", cfg.Instance.ID),
		zap.String("profile", cfg.ProfileName()),
		zap.Bool("writable", cfg.Instance.Writable),
		zap.String("log_level", cfg.Observability.LogLevel),
		zap.String("log_format", cfg.Observability.LogFormat),
		zap.String("log_output", cfg.Observability.LogOutput),
		zap.String("config_path", config.ConfigFilePath()),
		// 只输出被设置的变量名，绝不输出值（密钥、AK/SK 都在这里）。
		zap.Strings("env_overrides", setSimpleBaseEnv()),
	)
	// 级别写错时 zap 会静默退化为 info，这里显式提示，避免「我明明配了 debug」。
	if !config.ValidLogLevel(cfg.Observability.LogLevel) {
		logger.Warn("unknown observability.log_level, falling back to info",
			zap.String("log_level", cfg.Observability.LogLevel),
			zap.String("allowed", "debug|info|warn|error"),
		)
	}
}

// setSimpleBaseEnv 返回已设置的 SIMPLEBASE_* 变量名（按字典序）。
// 只返回名字不返回值：这是「环境变量到底有没有生效」的证据，同时不泄漏密钥。
func setSimpleBaseEnv() []string {
	var names []string
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		if name := kv[:i]; strings.HasPrefix(name, "SIMPLEBASE_") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// startupClock 记录启动阶段耗时：每行带该阶段耗时与累计耗时。
type startupClock struct {
	logger observability.Logger
	start  time.Time
	last   time.Time
}

func newStartupClock(logger observability.Logger) *startupClock {
	now := time.Now()
	return &startupClock{logger: logger, start: now, last: now}
}

// step 输出一行启动阶段日志（级别 Info，保证 info 档位也可见）。
func (c *startupClock) step(name string, fields ...zap.Field) {
	if c == nil || c.logger == nil {
		return
	}
	now := time.Now()
	fields = append(fields,
		zap.Int64("elapsed_ms", now.Sub(c.last).Milliseconds()),
		zap.Int64("total_ms", now.Sub(c.start).Milliseconds()),
	)
	c.last = now
	c.logger.Info("startup step: "+name, fields...)
}

// step 是 App 上的包装：未初始化 clock（测试直接构造 App）时不做事。
func (a *App) step(name string, fields ...zap.Field) {
	if a == nil {
		return
	}
	a.clock.step(name, fields...)
}
