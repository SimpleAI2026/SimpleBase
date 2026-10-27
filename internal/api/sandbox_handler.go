// 文件说明：sandbox_handler.go — internal/api 中 SandboxService、SandboxHandler 等符号的实现
//
// 用法
//
//	NewSandboxHandler(svc SandboxService, writable bool, audit AuditService, store ...
//	(h *SandboxHandler) Create(c echo.Context) error  // Create 对应 POST /v2/sandboxes 与 POST /sandboxes：创建并立即启动
//	(h *SandboxHandler) List(c echo.Context) error  // List 对应 GET /v2/sandboxes 与 GET /sandboxes
//	同包调用：project() → agent_handler.go, agent_schedule_handler.go 等；Get() → collection_meta_handler.go, collection_transfer.go 等；List() → api_keys_handler.go, backup_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sandbox_handler.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/vm, internal/auth, internal/sandbox, internal/systemdb 等
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] project() ← agent_handler.go, agent_schedule_handler.go, api_key_policy.go
//
// 主要符号
//
//	type SandboxService interface {…}
//	type SandboxHandler struct {…}
//	NewSandboxHandler(svc SandboxService, writable bool, audit AuditService, store ...
//	(h *SandboxHandler) Create(c echo.Context) error
//	(h *SandboxHandler) List(c echo.Context) error
//	(h *SandboxHandler) Get(c echo.Context) error
//	... 另有 38 个顶层符号

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/utils/hashx"
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
	audit    AuditService
	store    *systemdb.Store
	usage    interface {
		RecordSandbox(context.Context, string, string, int64) error
	}
	deps       Dependencies
	signer     *envdTokenSigner
	envdVer    string
	defaultTpl string
	now        func() time.Time
	tickets    *ticketStore
}

// NewSandboxHandler 构造处理器。secret 用于派生 envd 访问令牌的签名密钥。
func NewSandboxHandler(svc SandboxService, writable bool, audit AuditService, store *systemdb.Store, usage interface {
	RecordSandbox(context.Context, string, string, int64) error
}, deps Dependencies) *SandboxHandler {
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
		signer: newEnvdTokenSigner(deps.Config.Auth.APIKeyHashSecret), envdVer: ver,
		defaultTpl: tpl, now: time.Now, tickets: newTicketStore()}
}

// project 取 e2bProjectMiddleware 注入的项目。
func (h *SandboxHandler) project(c echo.Context) (string, error) {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return "", echo.NewHTTPError(http.StatusBadRequest, "project context missing")
	}
	return pc.ID, nil
}

// requireWrite 校验写权限、实例可写、非系统项目。
func (h *SandboxHandler) requireWrite(c echo.Context) (string, error) {
	p, err := h.project(c)
	if err != nil {
		return "", err
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	if !principal.HasPermission(auth.DatabaseWrite) {
		return "", auth.ErrForbidden
	}
	if !h.writable {
		return "", echo.NewHTTPError(http.StatusServiceUnavailable, "instance is read-only")
	}
	if systemdb.IsAdminProject(p) {
		return "", NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support sandboxes", "")
	}
	return p, nil
}

// requireRead 校验读权限。
func (h *SandboxHandler) requireRead(c echo.Context) (string, error) {
	p, err := h.project(c)
	if err != nil {
		return "", err
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	if !principal.HasPermission(auth.DatabaseRead) && !principal.HasPermission(auth.DatabaseWrite) {
		return "", auth.ErrForbidden
	}
	return p, nil
}

func (h *SandboxHandler) record(c echo.Context, p, id, action, detail string) {
	if h.audit == nil {
		return
	}
	_ = h.audit.Record(c.Request().Context(), AuditEvent{ProjectID: p, PrincipalID: actorID(c), Kind: "sandbox." + action,
		RequestID: RequestIDFromContext(c.Request().Context()), Status: "ok", Detail: id + " " + detail})
}

// recordExec 记录一次命令执行：审计只记命令的 SHA-256 与元数据，用量按耗时累计。
func (h *SandboxHandler) recordExec(c echo.Context, p, id, action, command string, exit int, dur time.Duration) {
	h.record(c, p, id, action, fmt.Sprintf("sha256=%s exit=%d duration_ms=%d", hashx.SumString(command), exit, dur.Milliseconds()))
	if systemdb.IsAdminProject(p) {
		return
	}
	if h.usage != nil {
		_ = h.usage.RecordSandbox(c.Request().Context(), p, RequestIDFromContext(c.Request().Context()), dur.Milliseconds())
	}
	if h.store != nil {
		h.store.RecordMetric(systemdb.MetricSample{ProjectID: p, Name: "sandbox_exec_count", Value: 1})
		h.store.RecordMetric(systemdb.MetricSample{ProjectID: p, Name: "sandbox_exec_duration_ms", Value: float64(dur.Milliseconds())})
	}
}

// ---- E2B 控制面 DTO ----

// e2bSandbox 对应 E2B 的 Sandbox 响应（创建 / connect）。
type e2bSandbox struct {
	TemplateID      string  `json:"templateID"`
	SandboxID       string  `json:"sandboxID"`
	Alias           string  `json:"alias,omitempty"`
	ClientID        string  `json:"clientID"`
	EnvdVersion     string  `json:"envdVersion"`
	EnvdAccessToken string  `json:"envdAccessToken,omitempty"`
	Domain          *string `json:"domain"`
	XSimpleBase     *e2bExt `json:"x_simplebase,omitempty"`
}

// e2bExt 是对标准响应的唯一加法：E2B 没有的信息（五态原值、来源、错误原因、最近活跃）。SDK 会忽略未知字段。
type e2bExt struct {
	Status       string     `json:"status"`
	Source       string     `json:"source"`
	LastError    string     `json:"lastError,omitempty"`
	LastActiveAt *time.Time `json:"lastActiveAt,omitempty"`
	IdleTimeoutS int64      `json:"idleTimeoutS"`
}

// e2bListed 对应 ListedSandbox。
type e2bListed struct {
	TemplateID  string            `json:"templateID"`
	Alias       string            `json:"alias,omitempty"`
	SandboxID   string            `json:"sandboxID"`
	ClientID    string            `json:"clientID"`
	StartedAt   string            `json:"startedAt"`
	EndAt       string            `json:"endAt"`
	CPUCount    int               `json:"cpuCount"`
	MemoryMB    int               `json:"memoryMB"`
	DiskSizeMB  int               `json:"diskSizeMB"`
	Metadata    map[string]string `json:"metadata"`
	State       string            `json:"state"`
	EnvdVersion string            `json:"envdVersion"`
	XSimpleBase *e2bExt           `json:"x_simplebase,omitempty"`
}

// e2bDetail 对应 SandboxDetail。
type e2bDetail struct {
	e2bListed
	EnvdAccessToken     string         `json:"envdAccessToken,omitempty"`
	AllowInternetAccess bool           `json:"allowInternetAccess"`
	Domain              *string        `json:"domain"`
	Network             map[string]any `json:"network"`
	Lifecycle           map[string]any `json:"lifecycle"`
}

// e2bNewSandbox 对应 NewSandbox / NewSandboxV2 的请求体。
// 不支持的字段用 json.RawMessage 接住，以便明确返回 400 而不是静默忽略。
type e2bNewSandbox struct {
	TemplateID          string            `json:"templateID"`
	Timeout             *int64            `json:"timeout"`
	AutoPause           *bool             `json:"autoPause"`
	AutoPauseMemory     *bool             `json:"autoPauseMemory"`
	AutoResume          json.RawMessage   `json:"autoResume"`
	Secure              *bool             `json:"secure"`
	AllowInternetAccess *bool             `json:"allow_internet_access"`
	Network             json.RawMessage   `json:"network"`
	Metadata            map[string]string `json:"metadata"`
	EnvVars             map[string]string `json:"envVars"`
	MCP                 json.RawMessage   `json:"mcp"`
	IAM                 json.RawMessage   `json:"iam"`
	VolumeMounts        json.RawMessage   `json:"volumeMounts"`
}

func isNullOrEmpty(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "" || s == "null" || s == "{}" || s == "[]"
}

// unsupported 返回请求里第一个我们明确不支持的字段名；没有则空。
func (r e2bNewSandbox) unsupported() string {
	switch {
	case r.AllowInternetAccess != nil && *r.AllowInternetAccess:
		return "allow_internet_access"
	case !isNullOrEmpty(r.Network):
		return "network"
	case !isNullOrEmpty(r.IAM):
		return "iam"
	case !isNullOrEmpty(r.VolumeMounts):
		return "volumeMounts"
	case !isNullOrEmpty(r.MCP):
		return "mcp"
	case r.AutoPause != nil && *r.AutoPause:
		return "autoPause"
	case !isNullOrEmpty(r.AutoResume) && !strings.Contains(string(r.AutoResume), `"enabled":false`):
		return "autoResume"
	}
	return ""
}

// defaultTemplate 是 E2B 的默认模板别名，对应实例配置的 sandbox.image。
const defaultTemplate = "base"

// 保留的 metadata 键：name 用于展示，simplebase.source 由服务端按主体注入。
const (
	metaName   = "name"
	metaSource = sandbox.ReservedMetadataPrefix + "source"
)

// sourceFor 按主体决定沙盒来源：登录态 → console，委托令牌 → agent，API Key → api。
func sourceFor(p auth.Principal) string {
	switch {
	case p.Delegated:
		return "agent"
	case p.UserID != "":
		return "console"
	}
	return "api"
}

// stateOf 把五态映射成 E2B 的二态。
func stateOf(status string) string {
	switch status {
	case "pending", "running":
		return "running"
	}
	return "paused"
}

func (h *SandboxHandler) alias(r sandbox.Sandbox) string {
	if r.Image == h.svc.Capabilities().DefaultImage && h.defaultTpl != "" {
		return h.defaultTpl
	}
	return r.Image
}

func (h *SandboxHandler) metadataOf(r sandbox.Sandbox) map[string]string {
	out := map[string]string{metaName: r.Name, metaSource: r.Source}
	for k, v := range r.Metadata {
		out[k] = v
	}
	return out
}

func rfc3339(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (h *SandboxHandler) listed(r sandbox.Sandbox) e2bListed {
	started := rfc3339(r.StartedAt)
	if started == "" {
		started = r.CreatedAt.UTC().Format(time.RFC3339)
	}
	return e2bListed{TemplateID: r.Image, Alias: h.alias(r), SandboxID: r.ID, ClientID: r.ID,
		StartedAt: started, EndAt: rfc3339(r.ExpiresAt), CPUCount: r.CPUs, MemoryMB: r.MemoryMiB, DiskSizeMB: 0,
		Metadata: h.metadataOf(r), State: stateOf(r.Status), EnvdVersion: h.envdVer, XSimpleBase: ext(r)}
}

func ext(r sandbox.Sandbox) *e2bExt {
	return &e2bExt{Status: r.Status, Source: r.Source, LastError: r.LastError, LastActiveAt: r.LastActiveAt, IdleTimeoutS: r.IdleTimeoutS}
}

// token 为沙盒签发 envd 令牌；到期时间取沙盒到期时间，缺省 1 小时。
func (h *SandboxHandler) token(projectID string, r sandbox.Sandbox) string {
	exp := h.now().Add(time.Hour)
	if r.ExpiresAt != nil {
		exp = *r.ExpiresAt
	}
	return h.signer.Issue(projectID, r.ID, exp)
}

func (h *SandboxHandler) created(projectID string, r sandbox.Sandbox, withToken bool) e2bSandbox {
	out := e2bSandbox{TemplateID: r.Image, SandboxID: r.ID, Alias: h.alias(r), ClientID: r.ID, EnvdVersion: h.envdVer, XSimpleBase: ext(r)}
	if withToken {
		out.EnvdAccessToken = h.token(projectID, r)
	}
	return out
}

func decodeE2BBody(c echo.Context, dst any) error {
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, 1<<20))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot read request body")
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	return nil
}

// ---- 控制面 ----

// Create 对应 POST /v2/sandboxes 与 POST /sandboxes：创建并立即启动。
func (h *SandboxHandler) Create(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	var req e2bNewSandbox
	if err := decodeE2BBody(c, &req); err != nil {
		return writeE2BErr(c, err)
	}
	if f := req.unsupported(); f != "" {
		return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "field "+f+" is not supported by this backend"))
	}
	in := sandbox.CreateInput{Metadata: req.Metadata, Start: true}
	if len(req.EnvVars) > 0 {
		// envVars 只在内存里随进程使用（process.Start 时合并）；这里先校验，不落库。
		if err := sandbox.ValidateEnvForE2B(req.EnvVars); err != nil {
			return writeE2BErr(c, err)
		}
	}
	if req.Timeout != nil {
		in.TimeoutS = *req.Timeout
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
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
	row, err := h.svc.Create(c.Request().Context(), p, in, actorID(c))
	if err != nil {
		return writeE2BErr(c, err)
	}
	h.setEnvs(row.ID, req.EnvVars)
	h.record(c, p, row.ID, "create", "")
	return c.JSON(http.StatusCreated, h.created(p, row, true))
}

// List 对应 GET /v2/sandboxes 与 GET /sandboxes。
func (h *SandboxHandler) List(c echo.Context) error {
	p, err := h.requireRead(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	q := c.QueryParams()
	limit := 100
	if v := q.Get("limit"); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil || n < 1 || n > 100 {
			return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "limit must be 1-100"))
		}
		limit = n
	}
	wantMeta, err := parseMetadataFilter(q.Get("metadata"))
	if err != nil {
		return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "invalid metadata filter"))
	}
	states := map[string]bool{}
	for _, s := range q["state"] {
		for _, part := range strings.Split(s, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if part != "running" && part != "paused" {
				return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "state must be running or paused"))
			}
			states[part] = true
		}
	}
	var startedAfter time.Time
	if v := q.Get("startedAfter"); v != "" {
		t, perr := time.Parse(time.RFC3339, v)
		if perr != nil {
			return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "startedAfter must be RFC3339"))
		}
		startedAfter = t
	}
	order := q.Get("order")
	if order != "" && order != "asc" && order != "desc" {
		return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "order must be asc or desc"))
	}

	// 项目内沙盒数量有上限（≤100），翻完整个项目后在内存里过滤 / 排序 / 分页，
	// 这样 metadata、state、startedAfter 的组合都不依赖库内索引。
	var all []sandbox.Sandbox
	cursor := ""
	for {
		page, next, lerr := h.svc.List(c.Request().Context(), p, "", "", cursor, 100)
		if lerr != nil {
			return writeE2BErr(c, lerr)
		}
		all = append(all, page...)
		if next == "" || len(all) > 1000 {
			break
		}
		cursor = next
	}
	var kept []sandbox.Sandbox
	running := 0
	for _, r := range all {
		if len(states) > 0 && !states[stateOf(r.Status)] {
			continue
		}
		if !startedAfter.IsZero() && !startOf(r).After(startedAfter) {
			continue
		}
		if !matchMetadata(h.metadataOf(r), wantMeta) {
			continue
		}
		kept = append(kept, r)
		if stateOf(r.Status) == "running" {
			running++
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := startOf(kept[i]), startOf(kept[j])
		if !a.Equal(b) {
			if order == "asc" {
				return a.Before(b)
			}
			return a.After(b)
		}
		if order == "asc" {
			return kept[i].ID < kept[j].ID
		}
		return kept[i].ID > kept[j].ID
	})
	// nextToken 是上一页最后一项的 ID：在排好序的结果里定位它的下一项。
	if tok := q.Get("nextToken"); tok != "" {
		idx := -1
		for i, r := range kept {
			if r.ID == tok {
				idx = i
				break
			}
		}
		if idx < 0 {
			return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "invalid nextToken"))
		}
		kept = kept[idx+1:]
	}
	next := ""
	if len(kept) > limit {
		kept = kept[:limit]
		next = kept[limit-1].ID
	}
	out := make([]e2bListed, 0, len(kept))
	for _, r := range kept {
		out = append(out, h.listed(r))
	}
	c.Response().Header().Set("X-Total-Running", strconv.Itoa(running))
	if next != "" {
		c.Response().Header().Set("X-Next-Token", next)
	}
	return c.JSON(http.StatusOK, out)
}

func startOf(r sandbox.Sandbox) time.Time {
	if r.StartedAt != nil && !r.StartedAt.IsZero() {
		return *r.StartedAt
	}
	return r.CreatedAt
}

// parseMetadataFilter 解析 "k=v&k2=v2"（键值各自 URL 编码）。
func parseMetadataFilter(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, errors.New("missing =")
		}
		dk, err := url.QueryUnescape(k)
		if err != nil {
			return nil, err
		}
		dv, err := url.QueryUnescape(v)
		if err != nil {
			return nil, err
		}
		out[dk] = dv
	}
	return out, nil
}

func matchMetadata(have, want map[string]string) bool {
	for k, v := range want {
		if got, ok := have[k]; !ok || got != v {
			return false
		}
	}
	return true
}

// Get 对应 GET /sandboxes/{id}。envdAccessToken 只给有写权限的调用方：
// 否则只读 Key 能通过详情拿到可写的数据面令牌。
func (h *SandboxHandler) Get(c echo.Context) error {
	p, err := h.requireRead(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	row, err := h.svc.Get(c.Request().Context(), p, c.Param("sandboxID"), c.QueryParam("refresh") == "1")
	if err != nil {
		return writeE2BErr(c, err)
	}
	d := e2bDetail{e2bListed: h.listed(row), AllowInternetAccess: false,
		Network:   map[string]any{"allowPublicTraffic": false},
		Lifecycle: map[string]any{"onTimeout": "kill", "autoResume": false}}
	if principal, _ := PrincipalFromContext(c.Request().Context()); principal.HasPermission(auth.DatabaseWrite) && h.writable {
		d.EnvdAccessToken = h.token(p, row)
	}
	return c.JSON(http.StatusOK, d)
}

// Delete 对应 DELETE /sandboxes/{id}。
func (h *SandboxHandler) Delete(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	id := c.Param("sandboxID")
	if err := h.svc.Delete(c.Request().Context(), p, id); err != nil {
		if errors.Is(err, vm.ErrBackend) {
			// 用户的删除请求仍成功；未删除的工作区留在资源表供 reaper 重试。
			h.record(c, p, id, "delete", "backend_removal_pending")
			h.clearEnvs(id)
			return c.NoContent(http.StatusNoContent)
		}
		return writeE2BErr(c, err)
	}
	h.clearEnvs(id)
	h.record(c, p, id, "delete", "")
	return c.NoContent(http.StatusNoContent)
}

// SetTimeout 对应 POST /sandboxes/{id}/timeout：从调用时刻重新计时。
func (h *SandboxHandler) SetTimeout(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	var req struct {
		Timeout *int64 `json:"timeout"`
	}
	if err := decodeE2BBody(c, &req); err != nil {
		return writeE2BErr(c, err)
	}
	if req.Timeout == nil || *req.Timeout < 0 {
		return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "timeout is required and must be >= 0"))
	}
	row, err := h.svc.SetExpiry(c.Request().Context(), p, c.Param("sandboxID"), *req.Timeout)
	if err != nil {
		return writeE2BErr(c, err)
	}
	h.record(c, p, row.ID, "timeout", "")
	return c.NoContent(http.StatusNoContent)
}

// Pause 对应 POST /sandboxes/{id}/pause：保留文件，进程丢失（没有内存快照）。
func (h *SandboxHandler) Pause(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	id := c.Param("sandboxID")
	cur, err := h.svc.Get(c.Request().Context(), p, id, false)
	if err != nil {
		return writeE2BErr(c, err)
	}
	if stateOf(cur.Status) == "paused" {
		return writeE2BErr(c, NewAPIError(http.StatusConflict, "sandbox_already_paused", "sandbox is already paused", ""))
	}
	if _, err := h.svc.Stop(c.Request().Context(), p, id); err != nil {
		return writeE2BErr(c, err)
	}
	h.record(c, p, id, "pause", "")
	return c.NoContent(http.StatusNoContent)
}

// Connect 对应 POST /v2/sandboxes/{id}/connect、POST /sandboxes/{id}/connect 与 POST /sandboxes/{id}/resume。
// 已暂停 → 启动（201）；已运行 → 只延长超时（200，TTL 只延长不缩短）。
func (h *SandboxHandler) Connect(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	var req struct {
		Timeout *int64 `json:"timeout"`
	}
	if err := decodeE2BBody(c, &req); err != nil {
		return writeE2BErr(c, err)
	}
	timeout := int64(300)
	if req.Timeout != nil {
		timeout = *req.Timeout
	}
	if timeout < 0 {
		return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "timeout must be >= 0"))
	}
	id := c.Param("sandboxID")
	ctx := c.Request().Context()
	cur, err := h.svc.Get(ctx, p, id, false)
	if err != nil {
		return writeE2BErr(c, err)
	}
	status := http.StatusOK
	var row sandbox.Sandbox
	if stateOf(cur.Status) == "paused" {
		status = http.StatusCreated
		if _, err = h.svc.Start(ctx, p, id); err != nil {
			return writeE2BErr(c, err)
		}
		if row, err = h.svc.SetExpiry(ctx, p, id, timeout); err != nil {
			return writeE2BErr(c, err)
		}
	} else {
		if row, err = h.svc.ExtendExpiry(ctx, p, id, timeout); err != nil {
			return writeE2BErr(c, err)
		}
	}
	h.record(c, p, id, "connect", "")
	return c.JSON(status, h.created(p, row, true))
}

// Unsupported 对应 E2B 有而这里没有的控制面端点（fork、snapshots、logs、metrics 等）。
func (h *SandboxHandler) Unsupported(c echo.Context) error {
	return c.JSON(http.StatusNotImplemented, e2bErrorBody{Code: http.StatusNotImplemented,
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
		return writeE2BErr(c, err)
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
		"envd_version": h.envdVer,
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
	return writeE2BErr(c, vm.ErrUnavailable)
}

// notFound 是 /e2b/** 下未注册路径的 E2B 形状 404。
func (h *SandboxHandler) notFound(c echo.Context) error {
	return c.JSON(http.StatusNotFound, e2bErrorBody{Code: http.StatusNotFound, Message: "not found", ErrorCode: "not_found"})
}
