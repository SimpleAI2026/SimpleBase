// Package sandbox 是 microsandbox Cloud 的唯一接入层
//（cloud-agent-sandbox-plan §3/§4/§6）。
//
// 全仓只有本包 import microsandbox SDK。其余包（cloudagent/api/app）
// 依赖本包导出的 Client 与接口约定。
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// DefaultCloudAPIURL 是未配置 api_url 时的 Cloud 端点。
const DefaultCloudAPIURL = "https://api.microsandbox.dev"

// ErrUnavailable 表示云沙盒未启用或 backend 核对失败。
var ErrUnavailable = errors.New("sandbox: cloud sandbox is not available")

// Output 是一次沙盒命令的结果（已截断、已脱敏由调用方负责）。
type Output struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// ThreadSandbox 是旧版 cloudagent 使用的工具接口；保留兼容既有调用。
type ThreadSandbox interface {
	Available() bool
	Exec(ctx context.Context, projectID, threadID, cmd string, args []string) (Output, error)
	Shell(ctx context.Context, projectID, threadID, command string) (Output, error)
	ReadFile(ctx context.Context, projectID, threadID, path string) (string, error)
	WriteFile(ctx context.Context, projectID, threadID, path, content string) error
	ReleaseThread(ctx context.Context, projectID, threadID string) error
}

// Client 实现 Sandbox，经 microsandbox Cloud 执行。
type Client struct {
	cfg config.SandboxConfig

	mu      sync.Mutex
	locks   map[string]*sync.Mutex
	enabled bool
}

// New 写入进程环境并核对 Cloud backend（cloud-agent-sandbox-plan §3.1）。
// 核对失败只关闭沙盒功能并打不含 key 的日志，不返回错误。
func New(cfg config.SandboxConfig, cacheDir string, logger observability.Logger) *Client {
	c := &Client{cfg: normalizeConfig(cfg), locks: map[string]*sync.Mutex{}}
	if !c.cfg.Enabled || strings.TrimSpace(c.cfg.APIKey) == "" {
		if logger != nil && c.cfg.Enabled {
			logger.Warn("sandbox disabled: enabled but api_key is empty")
		}
		return c
	}
	// 第一次调用 SDK 之前写入进程环境。
	_ = os.Setenv("MSB_BACKEND", "cloud")
	_ = os.Setenv("MSB_API_KEY", c.cfg.APIKey)
	if strings.TrimSpace(c.cfg.APIURL) != "" {
		_ = os.Setenv("MSB_API_URL", strings.TrimSpace(c.cfg.APIURL))
	}
	if cacheDir != "" {
		_ = os.Setenv("MSB_HOME", filepath.Join(cacheDir, "microsandbox"))
	}
	info, err := microsandbox.DefaultBackendInfo()
	if err != nil || info.Kind != microsandbox.BackendCloud {
		if logger != nil {
			logger.Warn("sandbox disabled: backend is not cloud (cloud backend required; no local fallback)",
				zap.String("kind", string(info.Kind)))
		}
		return c
	}
	c.enabled = true
	if logger != nil {
		logger.Info("sandbox cloud backend ready")
	}
	return c
}

// normalizeConfig 补齐空缺省值（Validate 已保证 enabled 分支的字段合法）。
func normalizeConfig(cfg config.SandboxConfig) config.SandboxConfig {
	if cfg.Image == "" {
		cfg.Image = "python:3.12-slim"
	}
	if cfg.CPUs == 0 {
		cfg.CPUs = 1
	}
	if cfg.MemoryMiB == 0 {
		cfg.MemoryMiB = 256
	}
	if cfg.MaxDuration == 0 {
		cfg.MaxDuration = 30 * time.Minute
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 5 * time.Minute
	}
	if cfg.ExecTimeout == 0 {
		cfg.ExecTimeout = 30 * time.Second
	}
	if cfg.MaxOutputBytes == 0 {
		cfg.MaxOutputBytes = 65536
	}
	if cfg.MaxFileBytes == 0 {
		cfg.MaxFileBytes = 1 << 20
	}
	if cfg.Network == "" {
		cfg.Network = "none"
	}
	if cfg.Workdir == "" {
		cfg.Workdir = "/workspace"
	}
	return cfg
}

// Available 报告云沙盒是否启用。
func (c *Client) Available() bool {
	return c != nil && c.enabled
}

// Close 释放锁表；无外部资源（VM 由 Cloud 回收）。
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.locks = map[string]*sync.Mutex{}
	return nil
}

// threadLock 返回该沙盒名的进程内串行锁（cloud-agent-sandbox-plan §4）。
func (c *Client) threadLock(name string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, ok := c.locks[name]
	if !ok {
		m = &sync.Mutex{}
		c.locks[name] = m
	}
	return m
}

// SandboxName 由 threadID 生成 Cloud 沙盒名（cloud-agent-sandbox-plan §6）。
// sb- + 32 位十六进制，远小于 128 字节上限。
func SandboxName(threadID string) string {
	return "sb-" + strings.ReplaceAll(strings.ToLower(strings.TrimSpace(threadID)), "-", "")
}

func (c *Client) networkPolicy() *microsandbox.NetworkConfig {
	if strings.EqualFold(strings.TrimSpace(c.cfg.Network), "public") {
		return microsandbox.NetworkPolicy.FromProfiles(microsandbox.NetworkProfilePublic)
	}
	return microsandbox.NetworkPolicy.None()
}

// open 连接或创建该 thread 的具名 Cloud 沙盒（cloud-agent-sandbox-plan §3.2）。
func (c *Client) open(ctx context.Context, projectID, threadID string) (*microsandbox.Sandbox, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	return microsandbox.ConnectOrCreateSandbox(ctx, SandboxName(threadID),
		microsandbox.WithImage(c.cfg.Image),
		microsandbox.WithCPUs(uint8(c.cfg.CPUs)),
		microsandbox.WithMemory(uint32(c.cfg.MemoryMiB)),
		microsandbox.WithWorkdir(c.cfg.Workdir),
		microsandbox.WithMaxDuration(c.cfg.MaxDuration),
		microsandbox.WithIdleTimeout(c.cfg.IdleTimeout),
		microsandbox.WithNetwork(c.networkPolicy()),
		microsandbox.WithLabels(map[string]string{
			"simplebase": "1",
			"project_id": projectID,
			"thread_id":  threadID,
		}),
	)
}

// Exec 在沙盒内运行命令（不经 shell）。非零退出码不是 Go error。
func (c *Client) Exec(ctx context.Context, projectID, threadID, cmd string, args []string) (Output, error) {
	lock := c.threadLock(SandboxName(threadID))
	lock.Lock()
	defer lock.Unlock()
	sb, err := c.open(ctx, projectID, threadID)
	if err != nil {
		return Output{}, err
	}
	defer sb.Close() // 释放句柄；不 Stop，VM 靠 idle/max duration 回收
	out, err := sb.Exec(ctx, cmd, args,
		microsandbox.WithExecTimeout(c.cfg.ExecTimeout),
		microsandbox.WithExecCwd(c.cfg.Workdir))
	if err != nil {
		return Output{}, wrapExecErr(err)
	}
	return c.truncateOutput(out.Stdout(), out.Stderr(), out.ExitCode()), nil
}

// Shell 在沙盒内以 /bin/sh -c 执行 command。
func (c *Client) Shell(ctx context.Context, projectID, threadID, command string) (Output, error) {
	lock := c.threadLock(SandboxName(threadID))
	lock.Lock()
	defer lock.Unlock()
	sb, err := c.open(ctx, projectID, threadID)
	if err != nil {
		return Output{}, err
	}
	defer sb.Close()
	out, err := sb.Shell(ctx, command,
		microsandbox.WithExecTimeout(c.cfg.ExecTimeout),
		microsandbox.WithExecCwd(c.cfg.Workdir))
	if err != nil {
		return Output{}, wrapExecErr(err)
	}
	return c.truncateOutput(out.Stdout(), out.Stderr(), out.ExitCode()), nil
}

// ReadFile 读取沙盒 /workspace 下的文件，返回内容（超出上限时截断）。
func (c *Client) ReadFile(ctx context.Context, projectID, threadID, path string) (string, error) {
	clean, err := c.validPath(path)
	if err != nil {
		return "", err
	}
	lock := c.threadLock(SandboxName(threadID))
	lock.Lock()
	defer lock.Unlock()
	sb, err := c.open(ctx, projectID, threadID)
	if err != nil {
		return "", err
	}
	defer sb.Close()
	text, err := sb.FS().ReadString(ctx, clean)
	if err != nil {
		return "", fmt.Errorf("sandbox: read file: %w", err)
	}
	if len(text) > c.cfg.MaxFileBytes {
		return text[:c.cfg.MaxFileBytes], nil
	}
	return text, nil
}

// WriteFile 写入沙盒 /workspace 下的文件；单文件不超过 max_file_bytes。
func (c *Client) WriteFile(ctx context.Context, projectID, threadID, path, content string) error {
	clean, err := c.validPath(path)
	if err != nil {
		return err
	}
	if len(content) > c.cfg.MaxFileBytes {
		return fmt.Errorf("sandbox: file exceeds max_file_bytes (%d bytes)", c.cfg.MaxFileBytes)
	}
	lock := c.threadLock(SandboxName(threadID))
	lock.Lock()
	defer lock.Unlock()
	sb, err := c.open(ctx, projectID, threadID)
	if err != nil {
		return err
	}
	defer sb.Close()
	if err := sb.FS().WriteString(ctx, clean, content); err != nil {
		return fmt.Errorf("sandbox: write file: %w", err)
	}
	return nil
}

// ReleaseThread 在删除 thread 时停止并移除 Cloud 沙盒（cloud-agent-sandbox-plan §3.2）。
// 失败仅返回错误供上层记日志，不阻塞 thread 的软删。
func (c *Client) ReleaseThread(ctx context.Context, projectID, threadID string) error {
	if !c.Available() {
		return nil
	}
	name := SandboxName(threadID)
	h, err := microsandbox.GetSandbox(ctx, name)
	if err != nil {
		// 不存在视为已释放。
		return nil
	}
	if h.Status() == microsandbox.SandboxStatusRunning {
		if err := h.Stop(ctx); err != nil {
			return fmt.Errorf("sandbox: stop %s: %w", name, err)
		}
	}
	if err := h.Remove(ctx); err != nil {
		return fmt.Errorf("sandbox: remove %s: %w", name, err)
	}
	return nil
}

// validPath 校验路径必须落在 workdir（/workspace）下（cloud-agent-sandbox-plan §6）。
// 拒绝空、相对、含 ..、非前缀路径。
func (c *Client) validPath(path string) (string, error) {
	workdir := c.cfg.Workdir
	p := strings.TrimSpace(path)
	if p == "" {
		return "", errors.New("sandbox: path is required")
	}
	if !filepath.IsAbs(p) {
		return "", errors.New("sandbox: path must be absolute")
	}
	clean := filepath.Clean(p)
	if clean != workdir && !strings.HasPrefix(clean, workdir+"/") {
		return "", fmt.Errorf("sandbox: path must be under %s", workdir)
	}
	if clean == workdir {
		return "", fmt.Errorf("sandbox: path must be a file under %s", workdir)
	}
	return clean, nil
}

// truncateOutput 把 stdout/stderr 截到 max_output_bytes。
func (c *Client) truncateOutput(stdout, stderr string, exitCode int) Output {
	return Output{
		Stdout:   truncateBytes(stdout, c.cfg.MaxOutputBytes),
		Stderr:   truncateBytes(stderr, c.cfg.MaxOutputBytes),
		ExitCode: exitCode,
	}
}

func truncateBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

// wrapExecErr 把 SDK 传输错误归一；超时用固定句子（cloud-agent-sandbox-plan §7.1）。
func wrapExecErr(err error) error {
	if err == nil {
		return nil
	}
	if microsandbox.IsKind(err, microsandbox.ErrExecTimeout) {
		return errors.New("sandbox: 命令超过执行时限")
	}
	return fmt.Errorf("sandbox: exec failed: %w", err)
}
