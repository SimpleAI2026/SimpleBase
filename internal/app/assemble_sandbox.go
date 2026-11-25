// 文件说明：sandbox 装配阶段：assembleSandbox / 终端桥 / SSH 认证 / 驱动选项（原 app.go 拆出）
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/image"
	"github.com/linkxzhou/SimpleBase/gosandbox/netx"
	"github.com/linkxzhou/SimpleBase/gosandbox/snapshot"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	boxvm "github.com/linkxzhou/SimpleBase/gosandbox/vm/box"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm/fake"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm/local"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/api"
	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/sandbox/sshd"
	"go.uber.org/zap"
)

func (a *App) assembleSandbox(ctx context.Context, cfg config.Config, logger observability.Logger) *api.TerminalBridge {
	if cfg.Sandbox.Enabled && a.systemStore != nil {
		var driver vm.Driver
		driverLog := sandboxDriverLog(logger)
		switch cfg.Sandbox.EffectiveBackend() {
		case config.SandboxBackendFake:
			driver = fake.New()
		case config.SandboxBackendLocal:
			localDrv, err := local.New(ctx, sandboxLocalOptions(cfg, driverLog))
			if err == nil {
				driver = localDrv
			} else if logger != nil {
				// 隔离不可用时不退回裸执行：云沙盒功能整体不可用，其余服务照常启动。
				logger.Warn("sandbox disabled: local isolation unavailable (no unisolated fallback)", zap.Error(err))
			}
		case config.SandboxBackendBox:
			boxDrv, store, boxErr := newBoxDriver(ctx, cfg, driverLog)
			if boxErr == nil {
				driver = boxDrv
				a.imageStore = store
			} else if logger != nil {
				logger.Warn("sandbox disabled: box backend unavailable (no unisolated fallback)", zap.Error(boxErr))
			}
		}
		if driver != nil {
			a.sandboxMgr = sandbox.NewManager(cfg.Sandbox, sandboxStoreAdapter{s: a.systemStore}, driver)
			if logger != nil {
				a.sandboxMgr.SetLogger(logger)
				logger.Info("sandbox manager ready", zap.String("backend", cfg.Sandbox.EffectiveBackend()),
					zap.String("image", cfg.Sandbox.Image), zap.Int("max_per_project", cfg.Sandbox.MaxPerProject),
					zap.Duration("idle_timeout", cfg.Sandbox.IdleTimeout), zap.Duration("reap_interval", cfg.Sandbox.ReapInterval))
			}
			a.registerCloser(a.sandboxMgr)
			a.sandboxMgr.StartReaper(a.ensureSchedulerCtx())
		}
	}
	if a.sandboxMgr != nil && a.sandboxMgr.Available() && a.sandboxMgr.Interactive() {
		return a.setupSandboxTerminal(cfg, logger)
	}
	return nil
}

// sandboxAdapter 只适配 gocloudagent 的线程工具接口，业务状态统一由 Manager 管理。
type sandboxAdapter struct{ m *sandbox.Manager }

func loopbackBase(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	host, port, err := splitHostPortLoose(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + host + ":" + port
}

func splitHostPortLoose(addr string) (string, string, error) {
	if strings.HasPrefix(addr, "[") {
		end := strings.LastIndex(addr, "]")
		if end < 0 || end+1 >= len(addr) || addr[end+1] != ':' {
			return "", "", errors.New("bad addr")
		}
		return addr[:end+1], addr[end+2:], nil
	}
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return "", "", errors.New("bad addr")
	}
	return addr[:i], addr[i+1:], nil
}

func (s *sandboxAdapter) Available() bool { return s.m.Available() }

func (s *sandboxAdapter) Exec(ctx context.Context, projectID, threadID, cmd string, args []string) (agentbridge.SandboxOutput, error) {
	out, err := s.m.ExecForThread(ctx, projectID, threadID, sandbox.ExecInput{Cmd: cmd, Args: args})
	return agentbridge.SandboxOutput{Stdout: out.Stdout, Stderr: out.Stderr, ExitCode: out.ExitCode}, err
}

func (s *sandboxAdapter) Shell(ctx context.Context, projectID, threadID, command string) (agentbridge.SandboxOutput, error) {
	out, err := s.m.ExecForThread(ctx, projectID, threadID, sandbox.ExecInput{Command: command})
	return agentbridge.SandboxOutput{Stdout: out.Stdout, Stderr: out.Stderr, ExitCode: out.ExitCode}, err
}

func (s *sandboxAdapter) ReadFile(ctx context.Context, projectID, threadID, path string) (string, error) {
	file, err := s.m.ReadFileForThread(ctx, projectID, threadID, path)
	return string(file.Content), err
}

func (s *sandboxAdapter) WriteFile(ctx context.Context, projectID, threadID, path, content string) error {
	return s.m.WriteFileForThread(ctx, projectID, threadID, path, []byte(content))
}

func (s *sandboxAdapter) ReleaseThread(ctx context.Context, projectID, threadID string) error {
	return s.m.ReleaseThread(ctx, projectID, threadID)
}

// sandboxLocalOptions 把配置转成本机沙盒驱动的显式选项，并把服务自己的数据
// （data_dir、缓存目录、配置文件与 .env）列为对沙盒内命令不可读的路径。
func sandboxLocalOptions(cfg config.Config, log vm.LogFunc) local.Options {
	root := cfg.Sandbox.Root
	if root == "" {
		root = filepath.Join(cfg.Storage.DataDir, "sandboxes")
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	opts := local.Options{
		Root: root, Workdir: cfg.Sandbox.Workdir,
		MaxFileBytes: cfg.Sandbox.MaxFileBytes, MaxOutputBytes: cfg.Sandbox.MaxOutputBytes,
		PidsMax: cfg.Sandbox.PidsMax, BwrapPath: cfg.Sandbox.BwrapPath, HelperPath: cfg.Sandbox.HelperPath,
		Log: log,
	}
	opts.ProtectDirs = []string{cfg.Storage.DataDir, cfg.Database.CacheDir}
	cfgPath := os.Getenv("SIMPLEBASE_CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	opts.ProtectFiles = []string{cfgPath, ".env"}
	return opts
}

// newBoxDriver 建常驻沙盒驱动；配置了镜像目录时同时接上镜像来源。
//
// 镜像来源是可选的：不配 images_dir 时只支持 image=host（宿主根只读），
// 与 local 后端的行为一致。这样默认部署不需要任何 registry 依赖。
func newBoxDriver(ctx context.Context, cfg config.Config, log vm.LogFunc) (vm.Driver, *image.Store, error) {
	opts := sandboxBoxOptions(cfg, log)
	var store *image.Store
	if dir := cfg.Sandbox.ImagesDir; dir != "" {
		st, err := image.Open(dir)
		if err != nil {
			return nil, nil, err
		}
		store = st
		opts.Images = boxvm.NewImageSource(image.NewPuller(store))
	}
	// 快照与镜像共用一个根：两者都是「沙盒的磁盘状态」，分开放会让
	// 清理与备份要记住两个目录。
	if dir := cfg.Sandbox.Root; dir != "" {
		snaps, err := snapshot.Open(dir)
		if err != nil {
			return nil, nil, err
		}
		opts.Snapshots = snaps
	}
	drv, err := boxvm.New(ctx, opts)
	if err != nil {
		return nil, nil, err
	}
	return drv, store, nil
}

// sandboxBoxOptions 把配置转成常驻沙盒驱动的选项。
//
// box 后端在沙盒里跑一个 PID 1 agent，宿主文件系统不可见，所以 seccomp 用
// Docker 默认档（允许本地 socket），而不是 host 档。
func sandboxBoxOptions(cfg config.Config, log vm.LogFunc) boxvm.Options {
	root := cfg.Sandbox.Root
	if root == "" {
		root = filepath.Join(cfg.Storage.DataDir, "sandboxes")
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	opts := boxvm.Options{
		Root: root, Workdir: cfg.Sandbox.Workdir,
		Helper: cfg.Sandbox.HelperPath, Bwrap: cfg.Sandbox.BwrapPath,
		MaxFileBytes: cfg.Sandbox.MaxFileBytes, MaxOutputBytes: cfg.Sandbox.MaxOutputBytes,
		PidsMax: cfg.Sandbox.PidsMax, MemoryMiB: cfg.Sandbox.MemoryMiB,
		Seccomp: "default",
		Log:     log,
	}
	// 让沙盒活过服务重启（升级、崩溃）。
	opts.Supervise = cfg.Sandbox.Supervise
	// 磁盘限额：MiB → 字节。0 表示不限。
	if cfg.Sandbox.DiskLimitMiB > 0 {
		opts.DiskLimitBytes = cfg.Sandbox.DiskLimitMiB << 20
	}
	// 出网：默认 none。档位字符串非法时退到 none，让沙盒仍能跑但不联网——
	// 配置校验已挡掉非法值，这里是第二道防线（别让坏配置变成「意外联网」）。
	if mode, err := netx.ParseMode(cfg.Sandbox.Network); err == nil {
		opts.Network = netx.Policy{Mode: mode}
	}
	return opts
}

// sandboxDriverLog 把 gosandbox 的 vm.LogFunc 适配到 observability.Logger；logger 为 nil 时返回 nil（不输出）。
func sandboxDriverLog(l observability.Logger) vm.LogFunc {
	if l == nil {
		return nil
	}
	return func(level, msg string, kv ...any) {
		fields := make([]zap.Field, 0, len(kv)/2+1)
		for i := 0; i+1 < len(kv); i += 2 {
			key, ok := kv[i].(string)
			if !ok {
				continue
			}
			fields = append(fields, zap.Any(key, kv[i+1]))
		}
		switch level {
		case "debug":
			l.Debug(msg, fields...)
		case "warn":
			l.Warn(msg, fields...)
		case "error":
			l.Error(msg, fields...)
		default:
			l.Info(msg, fields...)
		}
	}
}

// setupSandboxTerminal 装配浏览器终端与（可选的）外部 SSH 端口。失败只关闭终端，不影响沙盒的其它能力。
//
// 返回的 TerminalBridge 里 Server 先是 nil：SSH 服务端在 buildHTTPServers 中、路由树之前由
// bindSandboxSSHAuth 挂上。因此这里只准备主机密钥与指纹。
func (a *App) setupSandboxTerminal(cfg config.Config, logger observability.Logger) *api.TerminalBridge {
	keyPath := cfg.Sandbox.SSH.HostKeyPath
	if keyPath == "" {
		keyPath = filepath.Join(cfg.Storage.DataDir, "sandbox_ssh_host_ed25519")
	}
	signer, err := sshd.LoadOrCreateHostKey(keyPath)
	if err != nil {
		if logger != nil {
			logger.Warn("sandbox terminal disabled: host key unavailable", zap.Error(err))
		}
		return nil
	}
	a.sshHostKey = signer
	return &api.TerminalBridge{Fingerprint: sshd.Fingerprint(signer), SSHListen: strings.TrimSpace(cfg.Sandbox.SSH.Listen)}
}

// bindSandboxSSHAuth 构造 SSH 服务端并按需开放外部端口，返回服务端供 TerminalBridge 使用。
// 认证器用同一份 deps 另建一个 SandboxHandler（只走票据校验与项目沙盒查找，不挂路由）。
func (a *App) bindSandboxSSHAuth(t *api.TerminalBridge, deps api.Dependencies) *sshd.Server {
	h := api.NewSandboxHandler(deps.Sandbox, deps.Config.Instance.Writable, deps.Audit, deps.System, deps.SandboxUsage, deps)
	audit := func(ctx context.Context, id sshd.Identity, kind string, d time.Duration, exit int) {
		if a.auditSvc == nil {
			return
		}
		_ = a.auditSvc.Record(ctx, audit.Event{ProjectID: id.ProjectID, PrincipalID: id.Actor, Kind: "sandbox.terminal." + kind,
			Status: "ok", Detail: fmt.Sprintf("%s duration_ms=%d exit=%d", id.SandboxID, d.Milliseconds(), exit)})
	}
	srv, err := sshd.New(sshd.Options{HostKey: a.sshHostKey, Authenticator: api.NewSSHAuthenticator(h), Sessions: a.sandboxMgr, Audit: audit})
	if err != nil {
		if a.logger != nil {
			a.logger.Warn("sandbox terminal disabled", zap.Error(err))
		}
		return nil
	}
	a.registerCloser(closerFunc(srv.Close))
	if t.SSHListen != "" {
		l, lerr := net.Listen("tcp", t.SSHListen)
		if lerr != nil {
			a.logger.Warn("sandbox ssh listener failed; the browser terminal still works", zap.String("listen", t.SSHListen), zap.Error(lerr))
			t.SSHListen = ""
		} else {
			go func() { _ = srv.Serve(l) }()
			a.logger.Info("sandbox ssh listening", zap.String("listen", t.SSHListen), zap.String("host_key", t.Fingerprint))
		}
	}
	return srv
}
