// 文件说明：routes_e2b.go — 云沙盒 E2B 兼容 API 路由挂载
//
// 用法
//
//	mountE2BRoutes(e, deps)  // 必须在 web.Register 之前调用
//
// 调用架构
//
//	NewRouter（router.go）→ NewRouter → mountE2BRoutes → SandboxHandler
//	权限闸门统一经 routeGuards（routes_guards.go），禁止在路由块里直接写 auth.Require。
//
// 主要符号
//
//	func mountE2BRoutes(e *echo.Echo, deps Dependencies)

package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// mountE2BRoutes 挂载云沙盒的 E2B 兼容 API。这是沙盒唯一的 HTTP 面，旧的
// /v1/projects/:projectID/sandboxes/** 已整体删除，不保留任何兼容层。
//
//   - 控制面：/e2b/sandboxes、/e2b/v2/sandboxes/**（X-API-Key 或 Bearer 鉴权）；
//   - envd 数据面：根路径（SDK 拼 /files 会丢掉基址里的路径前缀，所以必须在根）与
//     /e2b/envd/**（控制台用）两处挂同一组 handler；
//   - ext：/e2b/ext/**，E2B 没有对应物但控制台需要（能力查询、终端）。
//
// 必须在 web.Register（SPA GET fallback）之前调用。
func mountE2BRoutes(e *echo.Echo, deps Dependencies) {
	sh := NewSandboxHandler(deps.Sandbox, deps.Config.Instance.Writable, deps.Audit, deps.System, deps.SandboxUsage, deps)
	authMW := timedAuthMiddleware(deps.authMW())
	cred := e2bCredentialMiddleware()
	proj := e2bProjectMiddleware(deps)
	available := deps.Sandbox != nil && deps.Sandbox.Available()

	// ext：capabilities 无条件挂载，沙盒不可用时给出原因，控制台据此展示空态。
	ext := e.Group(e2bPrefix+"/ext", cred, authMW, proj)
	ext.GET("/capabilities", sh.Capabilities)
	// 镜像管理：E2B 没有对应物，控制台与 CLI 用。
	ext.GET("/images", sh.Images)
	ext.POST("/images/pull", sh.ImagePull)
	ext.DELETE("/images", sh.ImageRemove)
	if !available {
		// 其余 E2B 路径统一返回 503，而不是落进 SPA fallback。
		e.Any(e2bPrefix+"/*", sh.unavailable)
		return
	}

	// ---- 控制面 ----
	cp := e.Group(e2bPrefix, cred, authMW, proj)
	both := func(method, path string, h echo.HandlerFunc) {
		cp.Add(method, path, h)
		cp.Add(method, "/v2"+path, h)
	}
	both(http.MethodPost, "/sandboxes", sh.Create)
	both(http.MethodGet, "/sandboxes", sh.List)
	cp.GET("/sandboxes/:sandboxID", sh.Get)
	cp.DELETE("/sandboxes/:sandboxID", sh.Delete)
	cp.POST("/sandboxes/:sandboxID/timeout", sh.SetTimeout)
	cp.POST("/sandboxes/:sandboxID/pause", sh.Pause)
	cp.POST("/sandboxes/:sandboxID/resume", sh.Connect)
	both(http.MethodPost, "/sandboxes/:sandboxID/connect", sh.Connect)
	// 有真实实现的端点：metrics（cgroup）、logs（进程输出落盘）、host（入站中继）。
	both(http.MethodGet, "/sandboxes/:sandboxID/metrics", sh.Metrics)
	both(http.MethodGet, "/sandboxes/:sandboxID/logs", sh.Logs)
	// 只注册 GET 与 POST：Any 会为每个 HTTP 方法各登记一条路由，
	// 让路由清单膨胀，也会把 OPTIONS / TRACE 这类方法暴露出去。
	// 保持 GET/POST 的 v1 → v2 注册顺序，避免 OpenAPI 顺序变化。
	cp.GET("/sandboxes/:sandboxID/host/:port", sh.HostProxy)
	cp.POST("/sandboxes/:sandboxID/host/:port", sh.HostProxy)
	cp.GET("/v2/sandboxes/:sandboxID/host/:port", sh.HostProxy)
	cp.POST("/v2/sandboxes/:sandboxID/host/:port", sh.HostProxy)
	// 快照与 fork：文件系统层（响应里 processes=false）。快照 create/list 交错注册，
	// 为保持既有顺序不合并这两组；其余相邻对通过 both 登记。
	cp.POST("/sandboxes/:sandboxID/snapshots", sh.SnapshotCreate)
	cp.GET("/sandboxes/:sandboxID/snapshots", sh.SnapshotList)
	cp.POST("/v2/sandboxes/:sandboxID/snapshots", sh.SnapshotCreate)
	cp.GET("/v2/sandboxes/:sandboxID/snapshots", sh.SnapshotList)
	both(http.MethodPost, "/sandboxes/:sandboxID/fork", sh.Fork)
	both(http.MethodDelete, "/snapshots/:snapshotID", sh.SnapshotRemove)
	both(http.MethodPost, "/snapshots/:snapshotID/fork", sh.SnapshotFork)
	for _, u := range []string{"network", "refreshes"} {
		cp.Any("/sandboxes/:sandboxID/"+u, sh.Unsupported)
		cp.Any("/v2/sandboxes/:sandboxID/"+u, sh.Unsupported)
	}

	// ---- ext：终端 ----
	if deps.Terminal != nil && deps.Sandbox.Interactive() {
		ext.POST("/sandboxes/:sandboxID/terminal-tickets", sh.TerminalTicket)
		// WebSocket 升级不带 Authorization，靠一次性票据 + 同源校验，所以不经过 authMW。
		e.GET(e2bPrefix+"/ext/sandboxes/:sandboxID/terminal", sh.Terminal)
	}

	// ---- envd 数据面：同一组 handler 挂两处 ----
	envdAuth := sh.envdAuth(func(next echo.HandlerFunc) echo.HandlerFunc { return cred(authMW(next)) })
	mountEnvd := func(g *echo.Group) {
		g.GET("/health", sh.Health)
		g.GET("/envs", sh.Envs)
		g.GET("/metrics", sh.EnvdMetrics)
		g.GET("/files", sh.FilesGet)
		g.POST("/files", sh.FilesPost)
		fs := "/filesystem.Filesystem/"
		g.POST(fs+"Stat", sh.FsStat)
		g.POST(fs+"ListDir", sh.FsListDir)
		g.POST(fs+"MakeDir", sh.FsMakeDir)
		g.POST(fs+"Move", sh.FsMove)
		g.POST(fs+"Remove", sh.FsRemove)
		// 目录监听：走真实的 inotify（Linux）；其它平台按 501 返回。
		g.POST(fs+"WatchDir", sh.FsWatchDir)
		g.POST(fs+"CreateWatcher", sh.FsCreateWatcher)
		g.POST(fs+"GetWatcherEvents", sh.FsGetWatcherEvents)
		g.POST(fs+"RemoveWatcher", sh.FsRemoveWatcher)
		pr := "/process.Process/"
		g.POST(pr+"Start", sh.ProcStart)
		g.POST(pr+"Connect", sh.ProcConnect)
		g.POST(pr+"List", sh.ProcList)
		g.POST(pr+"SendInput", sh.ProcSendInput)
		g.POST(pr+"SendSignal", sh.ProcSendSignal)
		g.POST(pr+"CloseStdin", sh.ProcCloseStdin)
		g.POST(pr+"Update", sh.ProcUpdate)
		g.POST(pr+"StreamInput", sh.ProcStreamInput)
	}
	mountEnvd(e.Group("", envdAuth))
	mountEnvd(e.Group(e2bEnvdPrefix, envdAuth))
	// /e2b/** 下其余未注册的路径：E2B 形状的 404（而不是 SPA 的 index.html）。
	e.Any(e2bPrefix+"/*", sh.notFound)
}
