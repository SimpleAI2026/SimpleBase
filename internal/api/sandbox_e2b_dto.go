// 文件说明：sandbox_e2b_dto.go — E2B 兼容 API 的控制面 DTO 与映射（内部模型 ↔ E2B 响应）
//
// 用法
//
//	同包调用：created() / listed() / detail() / token() → sandbox_handler.go, sandbox_list.go；
//	decodeE2BBody() → sandbox_handler.go 的 Create / Connect / SetTimeout
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sandbox_e2b_dto.go
//	         │ 依赖
//	         ▼
//	[下游] internal/sandbox, internal/auth
//	[同包] sandbox_handler.go, sandbox_list.go
//	[关键引用] created ← sandbox_handler.go（Create / Connect）；listed ← sandbox_list.go
//
// 主要符号
//
//	type e2bSandbox / e2bExt / e2bListed / e2bDetail / e2bNewSandbox struct {…}
//	const defaultTemplate；const metaName / metaSource
//	sourceFor(...) / stateOf(...) / rfc3339(...) / ext(...) / isNullOrEmpty(...)
//	(*SandboxHandler).alias / metadataOf / listed / token / created / detail
//	decodeE2BBody(c echo.Context, dst any) error

package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

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

// detail 组装一个沙盒详情响应（含 envd 令牌，仅对可写主体）。
//
// 抽出来是因为 fork 也要返回同样的形状：两处各拼一遍迟早会漂移。
func (h *SandboxHandler) detail(c echo.Context, row sandbox.Sandbox) e2bDetail {
	d := e2bDetail{e2bListed: h.listed(row), AllowInternetAccess: false,
		Network:   map[string]any{"allowPublicTraffic": false},
		Lifecycle: map[string]any{"onTimeout": "kill", "autoResume": false}}
	if principal := principalOf(c); principal.HasPermission(auth.DatabaseWrite) && h.writable {
		d.EnvdAccessToken = h.token(row.ProjectID, row)
	}
	return d
}
