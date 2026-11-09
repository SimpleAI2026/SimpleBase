package gosdk

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// 云沙盒只有 E2B 兼容 API（/e2b/**）。这里封装其控制面；沙盒内的命令与文件（envd 数据面）
// 请使用官方 e2b SDK，把 E2B_API_URL / E2B_DOMAIN 指向本服务（见 docs/automation/sandbox-api.md）。

// SandboxExt 是 SimpleBase 附加在标准响应上的扩展信息（E2B 没有的字段）。
type SandboxExt struct {
	Status       string `json:"status"`
	Source       string `json:"source"`
	LastError    string `json:"lastError,omitempty"`
	LastActiveAt string `json:"lastActiveAt,omitempty"`
	IdleTimeoutS int64  `json:"idleTimeoutS"`
}

// SandboxInfo 对应 E2B 的 ListedSandbox / Sandbox。envdAccessToken 有意不暴露：
// 它是可写的数据面令牌，不应出现在命令行输出里。
type SandboxInfo struct {
	ID         string            `json:"sandboxID"`
	TemplateID string            `json:"templateID"`
	Alias      string            `json:"alias,omitempty"`
	State      string            `json:"state,omitempty"`
	StartedAt  string            `json:"startedAt,omitempty"`
	EndAt      string            `json:"endAt,omitempty"`
	CPUCount   int               `json:"cpuCount,omitempty"`
	MemoryMB   int               `json:"memoryMB,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Ext        *SandboxExt       `json:"x_simplebase,omitempty"`
}

// SandboxCapabilities 对应 GET /e2b/ext/capabilities。
type SandboxCapabilities struct {
	Available     bool     `json:"available"`
	Reason        string   `json:"reason,omitempty"`
	Backend       string   `json:"backend"`
	Images        []string `json:"images"`
	DefaultImage  string   `json:"default_image,omitempty"`
	MaxPerProject int      `json:"max_per_project,omitempty"`
	MaxFileBytes  int      `json:"max_file_bytes,omitempty"`
	Terminal      bool     `json:"terminal"`
}

// CreateSandboxInput 对应 E2B 的 NewSandbox。Name 是便利字段，写入 metadata["name"]。
type CreateSandboxInput struct {
	Name       string
	TemplateID string
	// TimeoutS 为沙盒存活秒数；0 使用服务端默认。
	TimeoutS int64
	Metadata map[string]string
}

func (c *Client) e2bPath(id, suffix string) string {
	p := "/e2b/sandboxes"
	if id != "" {
		p += "/" + url.PathEscape(id)
	}
	return p + suffix
}

// SandboxCapabilities 查询沙盒是否可用及其限额；不可用时 Available=false 并带 Reason。
func (c *Client) SandboxCapabilities(ctx context.Context) (*SandboxCapabilities, error) {
	var out SandboxCapabilities
	err := c.requestJSON(ctx, http.MethodGet, "/e2b/ext/capabilities", nil, nil, &out)
	return &out, err
}

// ListSandboxes 返回项目内的沙盒（项目沙盒数有上限，一页即可覆盖）。state 可为 running / paused。
func (c *Client) ListSandboxes(ctx context.Context, state string, limit int) ([]SandboxInfo, error) {
	q := url.Values{}
	if state != "" {
		q.Set("state", state)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out []SandboxInfo
	if err := c.requestJSON(ctx, http.MethodGet, "/e2b/v2/sandboxes", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateSandbox 创建并启动沙盒。
func (c *Client) CreateSandbox(ctx context.Context, in CreateSandboxInput) (*SandboxInfo, error) {
	md := make(map[string]string, len(in.Metadata)+1)
	for k, v := range in.Metadata {
		md[k] = v
	}
	if in.Name != "" {
		md["name"] = in.Name
	}
	body := map[string]any{}
	if in.TemplateID != "" {
		body["templateID"] = in.TemplateID
	}
	if in.TimeoutS > 0 {
		body["timeout"] = in.TimeoutS
	}
	if len(md) > 0 {
		body["metadata"] = md
	}
	var out SandboxInfo
	err := c.requestJSON(ctx, http.MethodPost, "/e2b/v2/sandboxes", nil, body, &out)
	return &out, err
}

// GetSandbox 返回单个沙盒详情。
func (c *Client) GetSandbox(ctx context.Context, id string) (*SandboxInfo, error) {
	var out SandboxInfo
	err := c.requestJSON(ctx, http.MethodGet, c.e2bPath(id, ""), nil, nil, &out)
	return &out, err
}

// DeleteSandbox 删除沙盒及其工作区。
func (c *Client) DeleteSandbox(ctx context.Context, id string) error {
	return c.requestJSON(ctx, http.MethodDelete, c.e2bPath(id, ""), nil, nil, nil)
}

// ConnectSandbox 启动已暂停的沙盒，或延长运行中沙盒的存活时间（E2B connect 语义）。
func (c *Client) ConnectSandbox(ctx context.Context, id string, timeoutS int64) (*SandboxInfo, error) {
	body := map[string]any{}
	if timeoutS > 0 {
		body["timeout"] = timeoutS
	}
	var out SandboxInfo
	err := c.requestJSON(ctx, http.MethodPost, "/e2b/v2/sandboxes/"+url.PathEscape(id)+"/connect", nil, body, &out)
	return &out, err
}

// PauseSandbox 暂停沙盒：保留工作区文件，不保存进程与内存。
func (c *Client) PauseSandbox(ctx context.Context, id string) error {
	return c.requestJSON(ctx, http.MethodPost, c.e2bPath(id, "/pause"), nil, nil, nil)
}
