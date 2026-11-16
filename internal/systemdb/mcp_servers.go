// mcp_servers.go — sys_mcp_servers 的读写（项目级外部 MCP Server 配置）。
//
// 凭据口径与 sys_llm_settings.api_key 一致：HeadersJSON 只服务端内部使用，
// 对外必须脱敏（只回 has_credential），禁止记日志。
//
// tools_policy_json 是权限判定的唯一事实来源：Server 自报的 annotations
// 只作配置向导的默认建议，未标记的工具一律按「写」处理（planv6.2 §4.2-1）。

package systemdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MCP 传输类型。
const (
	MCPTransportHTTP = "http"
)

// MCPServer 是 sys_mcp_servers 一行。
type MCPServer struct {
	ID        string
	ProjectID string
	Name      string
	Transport string
	URL       string
	// HeadersJSON 是静态 Header（含凭据），禁止对外回显。
	HeadersJSON string
	TimeoutS    int
	Enabled     bool
	// ToolsPolicyJSON 是每工具的 read_only / destructive 标记。
	ToolsPolicyJSON string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// MCPToolPolicy 是单个工具的权限标记。
type MCPToolPolicy struct {
	ReadOnly    bool `json:"read_only"`
	Destructive bool `json:"destructive"`
}

const mcpServerColumns = `id, project_id, name, transport, url, headers_json, timeout_s, enabled, tools_policy_json, created_at, updated_at`

// CreateMCPServer 写入一个 Server 配置。
func (s *Store) CreateMCPServer(ctx context.Context, m MCPServer) (MCPServer, error) {
	if s == nil || s.db == nil {
		return MCPServer{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.Transport == "" {
		m.Transport = MCPTransportHTTP
	}
	if m.HeadersJSON == "" {
		m.HeadersJSON = "{}"
	}
	if m.ToolsPolicyJSON == "" {
		m.ToolsPolicyJSON = "{}"
	}
	if m.TimeoutS <= 0 {
		m.TimeoutS = 60
	}
	m.CreatedAt = now
	m.UpdatedAt = now
	enabled := int64(0)
	if m.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sys_mcp_servers(id, project_id, name, transport, url, headers_json, timeout_s, enabled, tools_policy_json, created_at, updated_at, archived_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		m.ID, m.ProjectID, m.Name, m.Transport, m.URL, m.HeadersJSON, int64(m.TimeoutS), enabled, m.ToolsPolicyJSON, now, now)
	if err != nil {
		return MCPServer{}, err
	}
	s.notifyWrite(ctx)
	return m, nil
}

// ListMCPServers 列出项目内未归档的 Server。
func (s *Store) ListMCPServers(ctx context.Context, projectID string) ([]MCPServer, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+mcpServerColumns+` FROM sys_mcp_servers
		 WHERE project_id = ? AND archived_at IS NULL ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MCPServer, 0)
	for rows.Next() {
		m, err := scanMCPServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMCPServer 按 ID 取未归档 Server。
func (s *Store) GetMCPServer(ctx context.Context, projectID, id string) (MCPServer, error) {
	if s == nil || s.db == nil {
		return MCPServer{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+mcpServerColumns+` FROM sys_mcp_servers
		 WHERE id = ? AND project_id = ? AND archived_at IS NULL`, id, projectID)
	m, err := scanMCPServer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return MCPServer{}, sql.ErrNoRows
	}
	return m, err
}

// GetMCPServerByName 按名称取未归档 Server（Agent 的 mcp: 引用用的是名称）。
func (s *Store) GetMCPServerByName(ctx context.Context, projectID, name string) (MCPServer, error) {
	if s == nil || s.db == nil {
		return MCPServer{}, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+mcpServerColumns+` FROM sys_mcp_servers
		 WHERE project_id = ? AND name = ? AND archived_at IS NULL`, projectID, name)
	m, err := scanMCPServer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return MCPServer{}, sql.ErrNoRows
	}
	return m, err
}

// UpdateMCPServer 更新可变字段。
func (s *Store) UpdateMCPServer(ctx context.Context, m MCPServer) (MCPServer, error) {
	if s == nil || s.db == nil {
		return MCPServer{}, ErrUnavailable
	}
	now := time.Now().UTC()
	m.UpdatedAt = now
	enabled := int64(0)
	if m.Enabled {
		enabled = 1
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_mcp_servers SET name=?, transport=?, url=?, headers_json=?, timeout_s=?, enabled=?, tools_policy_json=?, updated_at=?
		 WHERE id=? AND project_id=? AND archived_at IS NULL`,
		m.Name, m.Transport, m.URL, m.HeadersJSON, int64(m.TimeoutS), enabled, m.ToolsPolicyJSON, now, m.ID, m.ProjectID)
	if err != nil {
		return MCPServer{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return MCPServer{}, sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return m, nil
}

// ArchiveMCPServer 软删。
func (s *Store) ArchiveMCPServer(ctx context.Context, projectID, id string) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE sys_mcp_servers SET archived_at=?, updated_at=? WHERE id=? AND project_id=? AND archived_at IS NULL`,
		now, now, id, projectID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	s.notifyWrite(ctx)
	return nil
}

// CountMCPServers 返回项目内未归档数量（配额校验用）。
func (s *Store) CountMCPServers(ctx context.Context, projectID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, ErrUnavailable
	}
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_mcp_servers WHERE project_id = ? AND archived_at IS NULL`, projectID).Scan(&n)
	return int(n), err
}

// ParseMCPToolPolicies 解析 tools_policy_json。
// 解析失败返回空表：调用方按「未标记 = 写」处理，属于 fail-closed。
func ParseMCPToolPolicies(raw string) map[string]MCPToolPolicy {
	out := map[string]MCPToolPolicy{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// ParseMCPHeaders 解析 headers_json（含凭据，禁止记日志）。
func ParseMCPHeaders(raw string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func scanMCPServer(sc rowScanner) (MCPServer, error) {
	var m MCPServer
	var enabled int64
	var timeout int64
	if err := sc.Scan(&m.ID, &m.ProjectID, &m.Name, &m.Transport, &m.URL, &m.HeadersJSON,
		&timeout, &enabled, &m.ToolsPolicyJSON, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return MCPServer{}, err
	}
	m.TimeoutS = int(timeout)
	m.Enabled = enabled != 0
	return m, nil
}
