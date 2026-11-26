package sandboxhttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/hashx"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

// SandboxService 由 app 装配的 Manager 实现；handler 不直接依赖 SDK。
type SandboxService interface {
	Available() bool
	Capabilities() sandbox.Capabilities
	Interactive() bool
	Create(context.Context, string, sandbox.CreateInput, string) (sandbox.Sandbox, error)
	List(context.Context, string, string, string, string, int) ([]sandbox.Sandbox, string, error)
	Get(context.Context, string, string, bool) (sandbox.Sandbox, error)
	GetByID(context.Context, string) (sandbox.Sandbox, error)
	Start(context.Context, string, string) (sandbox.Sandbox, error)
	Stop(context.Context, string, string) (sandbox.Sandbox, error)
	// Pause 冻结沙盒（后端不支持时退回 Stop）；Resume 解冻或启动。
	Pause(context.Context, string, string) (sandbox.Sandbox, error)
	Resume(context.Context, string, string) (sandbox.Sandbox, error)
	// Metrics 读沙盒的资源用量；后端不支持时返回 vm.ErrNotSupported。
	Metrics(context.Context, string, string) (vm.Metrics, error)
	// Logs 读沙盒的历史输出；后端不支持时返回 vm.ErrNotSupported。
	Logs(context.Context, string, string, vm.LogQuery) ([]vm.LogRecord, error)
	// DialPort 连到沙盒内 127.0.0.1:port。
	DialPort(context.Context, string, string, uint16) (io.ReadWriteCloser, error)
	// Watch 监听沙盒内的目录；后端不支持时返回 vm.ErrNotSupported。
	Watch(context.Context, string, string, string, bool) (vm.FileWatch, error)
	// 能力探测：后端不支持时对应端点回答 501，而不是返回空结果。
	CanLogs() bool
	CanWatch() bool
	CanPorts() bool
	// CanSnapshot 报告后端是否支持文件系统快照与 fork。
	CanSnapshot() bool
	// Snapshot 给沙盒拍文件系统快照；Snapshots 列出全部；RemoveSnapshot 删除。
	Snapshot(context.Context, string, string, string) (sandbox.Snapshot, error)
	Snapshots(context.Context) ([]sandbox.Snapshot, error)
	RemoveSnapshot(context.Context, string) error
	// Fork 从快照建一个新沙盒。
	Fork(context.Context, string, string, string, sandbox.CreateInput) (sandbox.Sandbox, error)
	Delete(context.Context, string, string) error
	SetExpiry(context.Context, string, string, int64) (sandbox.Sandbox, error)
	ExtendExpiry(context.Context, string, string, int64) (sandbox.Sandbox, error)
	ReadFile(context.Context, string, string, string) (sandbox.FileContent, error)
	WriteFile(context.Context, string, string, string, []byte) error
	RemoveFile(context.Context, string, string, string) error
	ListDir(context.Context, string, string, string) ([]vm.FileEntry, error)
	StartProc(context.Context, string, string, sandbox.StartSpec) (*sandbox.ProcHandle, error)
	Procs(string) []sandbox.ProcInfo
	FindProc(projectID, sandboxID string, pid int, tag string) (*sandbox.ProcHandle, bool)
	SetEnvs(sandboxID string, env map[string]string)
	Envs(sandboxID string) map[string]string
	ClearEnvs(sandboxID string)
}

// SandboxHandler 是 E2B 兼容 API 的处理器：控制面、envd 数据面与少量控制台专用的 ext 路由。
type SandboxHandler struct {
	svc      SandboxService
	writable bool
	audit    httpkit.AuditService
	store    *systemdb.Store
	usage    interface {
		RecordSandbox(context.Context, string, string, int64) error
	}
	deps       Deps
	signer     *envdTokenSigner
	envdVer    string
	defaultTpl string
	now        func() time.Time
	tickets    *ticketStore
}

// NewSandboxHandler 构造处理器。secret 用于派生 envd 访问令牌的签名密钥。
func NewSandboxHandler(svc SandboxService, writable bool, audit httpkit.AuditService, store *systemdb.Store, usage interface {
	RecordSandbox(context.Context, string, string, int64) error
}, deps Deps) *SandboxHandler {
	ver := deps.Config.Sandbox.E2B.EnvdVersion
	if ver == "" {
		ver = defaultEnvdVersion
	}
	tpl := deps.Config.Sandbox.E2B.DefaultTemplate
	if tpl == "" {
		// E2B 的默认模板别名；templateID 等于它时按实例默认镜像处理。
		tpl = defaultTemplate
	}
	return &SandboxHandler{svc: svc, writable: writable, audit: audit, store: store, usage: usage, deps: deps,
		signer: NewEnvdTokenSigner(deps.Config.Auth.APIKeyHashSecret), envdVer: ver,
		defaultTpl: tpl, now: time.Now, tickets: NewTicketStore()}
}

// project 取 e2bProjectMiddleware 注入的项目。
func (h *SandboxHandler) project(c echo.Context) (string, error) {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return "", err
	}
	return pc.ID, nil
}

// requireWrite 校验写权限、实例可写、非系统项目。
func (h *SandboxHandler) requireWrite(c echo.Context) (string, error) {
	p, err := h.project(c)
	if err != nil {
		return "", err
	}
	principal := httpkit.PrincipalOf(c)
	if !principal.HasPermission(auth.DatabaseWrite) {
		return "", auth.ErrForbidden
	}
	if !h.writable {
		return "", godatabase.ErrWriterUnavailable
	}
	if systemdb.IsAdminProject(p) {
		return "", httpkit.NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support sandboxes", "")
	}
	return p, nil
}

// requireRead 校验读权限。
func (h *SandboxHandler) requireRead(c echo.Context) (string, error) {
	p, err := h.project(c)
	if err != nil {
		return "", err
	}
	principal := httpkit.PrincipalOf(c)
	if !principal.HasPermission(auth.DatabaseRead) && !principal.HasPermission(auth.DatabaseWrite) {
		return "", auth.ErrForbidden
	}
	return p, nil
}

func (h *SandboxHandler) record(c echo.Context, p, id, action, detail string) {
	if h.audit == nil {
		return
	}
	httpkit.RecordAudit(c, h.audit, "sandbox."+action, p, "ok", id+" "+detail)
}

// recordExec 记录一次命令执行：审计只记命令的 SHA-256 与元数据，用量按耗时累计。
func (h *SandboxHandler) recordExec(c echo.Context, p, id, action, command string, exit int, dur time.Duration) {
	h.record(c, p, id, action, fmt.Sprintf("sha256=%s exit=%d duration_ms=%d", hashx.SumString(command), exit, dur.Milliseconds()))
	if systemdb.IsAdminProject(p) {
		return
	}
	if h.usage != nil {
		_ = h.usage.RecordSandbox(c.Request().Context(), p, httpkit.RequestID(c), dur.Milliseconds())
	}
	if h.store != nil {
		h.store.RecordMetric(systemdb.MetricSample{ProjectID: p, Name: "sandbox_exec_count", Value: 1})
		h.store.RecordMetric(systemdb.MetricSample{ProjectID: p, Name: "sandbox_exec_duration_ms", Value: float64(dur.Milliseconds())})
	}
}

// Create 对应 POST /v2/sandboxes 与 POST /sandboxes：创建并立即启动。
func (h *SandboxHandler) Create(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return WriteE2BErr(c, err)
	}
	var req e2bNewSandbox
	if err := decodeE2BBody(c, &req); err != nil {
		return WriteE2BErr(c, err)
	}
	if f := req.unsupported(); f != "" {
		return WriteE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "field "+f+" is not supported by this backend"))
	}
	in := sandbox.CreateInput{Metadata: req.Metadata, Start: true}
	if len(req.EnvVars) > 0 {
		// envVars 只在内存里随进程使用（process.Start 时合并）；这里先校验，不落库。
		if err := sandbox.ValidateEnvForE2B(req.EnvVars); err != nil {
			return WriteE2BErr(c, err)
		}
	}
	if req.Timeout != nil {
		in.TimeoutS = *req.Timeout
	}
	principal := httpkit.PrincipalOf(c)
	in.Source = sourceFor(principal)
	// 名称来自 metadata 的保留键 name；其余保留前缀由 Manager 校验时拒绝。
	if n, ok := req.Metadata[metaName]; ok {
		in.Name = n
		md := make(map[string]string, len(req.Metadata))
		for k, v := range req.Metadata {
			if k != metaName {
				md[k] = v
			}
		}
		in.Metadata = md
	}
	if t := strings.TrimSpace(req.TemplateID); t != "" && t != h.defaultTpl && t != h.svc.Capabilities().DefaultImage {
		in.Image = t
	}
	row, err := h.svc.Create(c.Request().Context(), p, in, httpkit.PrincipalOf(c).Actor())
	if err != nil {
		return WriteE2BErr(c, err)
	}
	h.setEnvs(row.ID, req.EnvVars)
	h.record(c, p, row.ID, "create", "")
	return c.JSON(http.StatusCreated, h.created(p, row, true))
}

// Get 对应 GET /sandboxes/{id}。envdAccessToken 只给有写权限的调用方：
// 否则只读 Key 能通过详情拿到可写的数据面令牌。
func (h *SandboxHandler) Get(c echo.Context) error {
	p, err := h.requireRead(c)
	if err != nil {
		return WriteE2BErr(c, err)
	}
	row, err := h.svc.Get(c.Request().Context(), p, c.Param("sandboxID"), c.QueryParam("refresh") == "1")
	if err != nil {
		return WriteE2BErr(c, err)
	}
	return c.JSON(http.StatusOK, h.detail(c, row))
}

// Delete 对应 DELETE /sandboxes/{id}。
func (h *SandboxHandler) Delete(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return WriteE2BErr(c, err)
	}
	id := c.Param("sandboxID")
	if err := h.svc.Delete(c.Request().Context(), p, id); err != nil {
		if errors.Is(err, vm.ErrBackend) {
			// 用户的删除请求仍成功；未删除的工作区留在资源表供 reaper 重试。
			h.record(c, p, id, "delete", "backend_removal_pending")
			h.clearEnvs(id)
			return c.NoContent(http.StatusNoContent)
		}
		return WriteE2BErr(c, err)
	}
	h.clearEnvs(id)
	h.record(c, p, id, "delete", "")
	return c.NoContent(http.StatusNoContent)
}

// SetTimeout 对应 POST /sandboxes/{id}/timeout：从调用时刻重新计时。
func (h *SandboxHandler) SetTimeout(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return WriteE2BErr(c, err)
	}
	var req struct {
		Timeout *int64 `json:"timeout"`
	}
	if err := decodeE2BBody(c, &req); err != nil {
		return WriteE2BErr(c, err)
	}
	if req.Timeout == nil || *req.Timeout < 0 {
		return WriteE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "timeout is required and must be >= 0"))
	}
	row, err := h.svc.SetExpiry(c.Request().Context(), p, c.Param("sandboxID"), *req.Timeout)
	if err != nil {
		return WriteE2BErr(c, err)
	}
	h.record(c, p, row.ID, "timeout", "")
	return c.NoContent(http.StatusNoContent)
}

// Pause 对应 POST /sandboxes/{id}/pause。
//
// 后端支持真冻结（Capabilities.Pause）时用 cgroup.freeze：进程与内存保留，
// resume 是毫秒级；否则退回 Stop，进程丢失（没有内存快照）。
func (h *SandboxHandler) Pause(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return WriteE2BErr(c, err)
	}
	id := c.Param("sandboxID")
	cur, err := h.svc.Get(c.Request().Context(), p, id, false)
	if err != nil {
		return WriteE2BErr(c, err)
	}
	if stateOf(cur.Status) == "paused" {
		return WriteE2BErr(c, httpkit.NewAPIError(http.StatusConflict, "sandbox_already_paused", "sandbox is already paused", ""))
	}
	if _, err := h.svc.Pause(c.Request().Context(), p, id); err != nil {
		return WriteE2BErr(c, err)
	}
	h.record(c, p, id, "pause", "")
	return c.NoContent(http.StatusNoContent)
}

// Connect 对应 POST /v2/sandboxes/{id}/connect、POST /sandboxes/{id}/connect 与 POST /sandboxes/{id}/resume。
// 已暂停 → 启动（201）；已运行 → 只延长超时（200，TTL 只延长不缩短）。
func (h *SandboxHandler) Connect(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return WriteE2BErr(c, err)
	}
	var req struct {
		Timeout *int64 `json:"timeout"`
	}
	if err := decodeE2BBody(c, &req); err != nil {
		return WriteE2BErr(c, err)
	}
	timeout := int64(300)
	if req.Timeout != nil {
		timeout = *req.Timeout
	}
	if timeout < 0 {
		return WriteE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "timeout must be >= 0"))
	}
	id := c.Param("sandboxID")
	ctx := c.Request().Context()
	cur, err := h.svc.Get(ctx, p, id, false)
	if err != nil {
		return WriteE2BErr(c, err)
	}
	status := http.StatusOK
	var row sandbox.Sandbox
	if stateOf(cur.Status) == "paused" {
		status = http.StatusCreated
		if _, err = h.svc.Resume(ctx, p, id); err != nil {
			return WriteE2BErr(c, err)
		}
		if row, err = h.svc.SetExpiry(ctx, p, id, timeout); err != nil {
			return WriteE2BErr(c, err)
		}
	} else {
		if row, err = h.svc.ExtendExpiry(ctx, p, id, timeout); err != nil {
			return WriteE2BErr(c, err)
		}
	}
	h.record(c, p, id, "connect", "")
	return c.JSON(status, h.created(p, row, true))
}

// Metrics 对应 GET /sandboxes/{id}/metrics。
//
// 后端没有 cgroup 指标时返回 501（unsupported_feature），而不是报 0：
// 报 0 会让调用方以为沙盒真的没占内存。
func (h *SandboxHandler) Metrics(c echo.Context) error {
	p, err := h.requireRead(c)
	if err != nil {
		return WriteE2BErr(c, err)
	}
	m, err := h.svc.Metrics(c.Request().Context(), p, c.Param("sandboxID"))
	if err != nil {
		if errors.Is(err, vm.ErrNotSupported) {
			return h.Unsupported(c)
		}
		return WriteE2BErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"memory_bytes":      m.MemoryBytes,
		"memory_peak_bytes": m.MemoryPeakBytes,
		"cpu_usec":          m.CPUUsec,
		"pids":              m.Pids,
		"oom_kills":         m.OOMKills,
		"disk_read_bytes":   m.DiskReadBytes,
		"disk_write_bytes":  m.DiskWriteBytes,
		// disk_used_bytes 是「现在占了多少盘」（upper + ws），不是 io 带宽。
		// 配合 sandbox.disk_limit_mib 可以判断离冻结还有多远。
		"disk_used_bytes": m.DiskUsedBytes,
		"backend":         m.Backend,
	})
}

// Unsupported 对应 E2B 有而这里没有的控制面端点（fork、snapshots、logs、metrics 等）。
func (h *SandboxHandler) Unsupported(c echo.Context) error {
	return c.JSON(http.StatusNotImplemented, E2BErrorBody{Code: http.StatusNotImplemented,
		Message: "this E2B endpoint is not supported by SimpleBase", ErrorCode: "unsupported_feature"})
}

func (h *SandboxHandler) setEnvs(id string, env map[string]string) {
	if len(env) > 0 {
		h.svc.SetEnvs(id, env)
	}
}

func (h *SandboxHandler) clearEnvs(id string) { h.svc.ClearEnvs(id) }

// Capabilities 对应 GET /e2b/ext/capabilities（非 E2B 标准，仅控制台使用）。
// 沙盒不可用时仍返回 available:false 与原因，控制台据此展示空态。
func (h *SandboxHandler) Capabilities(c echo.Context) error {
	if _, err := h.requireRead(c); err != nil {
		return WriteE2BErr(c, err)
	}
	out := map[string]any{"available": false, "backend": "local", "images": []string{}, "terminal": false}
	if h.svc == nil || !h.svc.Available() {
		return c.JSON(http.StatusOK, out)
	}
	caps := h.svc.Capabilities()
	body := map[string]any{
		"available": caps.Available, "backend": caps.Backend, "images": caps.Images,
		"default_image": caps.DefaultImage, "default_template": h.defaultTpl,
		"cpus": caps.CPUs, "memory_mib": caps.MemoryMiB,
		"timeout_default_s": caps.TimeoutDefaultS, "timeout_max_s": caps.TimeoutMaxS,
		"max_file_bytes": caps.MaxFileBytes, "max_output_bytes": caps.MaxOutputBytes,
		"max_per_project": caps.MaxPerProject, "terminal": caps.Terminal && h.deps.Terminal != nil,
		// pause 为真表示真冻结（cgroup.freeze）；false 时 pause 退回 stop（进程丢失）。
		"pause": caps.Pause, "pause_freezes": caps.Pause,
		// metrics 为真表示 GET /sandboxes/{id}/metrics 有真实数据。
		"metrics":      caps.Metrics,
		"envd_version": h.envdVer,
		// 出网档位："none" / "egress"。egress 不做地址或域名过滤。
		"network":        caps.Network,
		"disk_limit_mib": caps.DiskLimitMiB,
	}
	if t := h.deps.Terminal; t != nil && t.SSHListen != "" {
		body["ssh"] = map[string]any{"enabled": true, "listen": t.SSHListen, "host_key_fingerprint": t.Fingerprint}
	} else if t != nil {
		body["ssh"] = map[string]any{"enabled": false, "host_key_fingerprint": t.Fingerprint}
	}
	return c.JSON(http.StatusOK, body)
}

// unavailable 在沙盒不可用时回答 E2B 路径：503，而不是落进 SPA fallback。
func (h *SandboxHandler) unavailable(c echo.Context) error {
	return WriteE2BErr(c, vm.ErrUnavailable)
}

// notFound 是 /e2b/** 下未注册路径的 E2B 形状 404。
func (h *SandboxHandler) notFound(c echo.Context) error {
	return c.JSON(http.StatusNotFound, E2BErrorBody{Code: http.StatusNotFound, Message: "not found", ErrorCode: "not_found"})
}
