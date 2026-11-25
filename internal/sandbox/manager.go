// 文件说明：manager.go — 项目级沙盒 Manager：类型定义、创建 / 查询 / 续期与元数据校验
//
// 用法
//
//	Sandbox、Store、CreateInput（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] internal/api, internal/agentbridge, internal/app
//	         ▼
//	[本文件] internal/sandbox/manager.go
//	         ▼
//	[下游] gosandbox/vm
//	[同包] errors.go, paths.go, proc.go
//
// 主要符号
//
//	type Sandbox struct {…}
//	type Store interface {…}
//	type CreateInput struct {…}
//	type ExecInput struct {…}
//	type ExecOutput struct {…}
//	type FileContent struct {…}
//	type Capabilities struct {…}
//	type Manager struct {…}
//	... 另有 20 个顶层符号

package sandbox

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	pagelimit "github.com/linkxzhou/SimpleBase/godatabase/limit"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// Sandbox 是项目级沙盒的持久元数据，命令和文件内容不入库。
type Sandbox struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"project_id"`
	Name         string     `json:"name"`
	CloudName    string     `json:"cloud_name"` // 驱动侧的 VM 名（历史字段名；local 后端下是工作区目录名）
	Source       string     `json:"source"`
	ThreadID     string     `json:"thread_id,omitempty"`
	Image        string     `json:"image"`
	CPUs         int        `json:"cpus"`
	MemoryMiB    int        `json:"memory_mib"`
	Network      string     `json:"network"`
	IdleTimeoutS int64      `json:"idle_timeout_s"`
	MaxDurationS int64      `json:"max_duration_s"`
	Status       string     `json:"status"`
	LastError    string     `json:"last_error,omitempty"`
	CreatedBy    string     `json:"created_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at"`
	LastActiveAt *time.Time `json:"last_active_at"`
	ExpiresAt    *time.Time `json:"expires_at"`
	DeletedAt    *time.Time `json:"-"`
	// Metadata 是用户自定义键值（E2B metadata）；不含 name 与 simplebase.* 保留键。
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Store 由 app 适配 systemdb.Store，隔离业务层与 SQL。
type Store interface {
	CreateSandbox(context.Context, Sandbox) (Sandbox, error)
	GetSandbox(context.Context, string, string) (Sandbox, error)
	// GetSandboxByID 不带项目条件，只供「请求里没有项目」的入口（SSH 登录）使用，调用方必须再校验访问权。
	GetSandboxByID(context.Context, string) (Sandbox, error)
	GetSandboxByName(context.Context, string, string) (Sandbox, error)
	GetSandboxByThread(context.Context, string, string) (Sandbox, error)
	ListSandboxes(context.Context, string, string, string, string, int) ([]Sandbox, string, error)
	CountSandboxes(context.Context, string) (int, error)
	UpdateSandbox(context.Context, Sandbox) error
	SoftDeleteSandbox(context.Context, string, string, string) error
	// ListExpiredSandboxes 取已过期的行（reaper 用）。
	ListExpiredSandboxes(context.Context, time.Time, int) ([]Sandbox, error)
	// ListIdleRunningSandboxes 取运行中且最后活动早于给定时刻的行（空闲冻结用）。
	ListIdleRunningSandboxes(context.Context, time.Time, int) ([]Sandbox, error)
	ListStaleRunSandboxes(context.Context, time.Time, int) ([]Sandbox, error)
	ListFailedSandboxRemovals(context.Context, int) ([]Sandbox, error)
	PurgeDeletedSandboxes(context.Context, time.Time) (int64, error)
}

// CreateInput 描述新沙盒，零值采用实例默认配置。
//
// E2B 创建体里没有 CPU / 内存 / 网络：规格由实例配置统一决定，网络恒为关闭。
type CreateInput struct {
	Name  string
	Image string
	// TimeoutS 是存活时间（秒），从创建 / 启动时刻起算；0 取实例默认，超过 max_duration 时截断。
	TimeoutS int64
	// Metadata 是用户自定义键值，不含保留键。
	Metadata map[string]string
	// Start 为真时创建后立即启动（E2B 创建即运行）。
	Start    bool
	Source   string
	ThreadID string
}

type ExecInput struct {
	Command  string            `json:"command"`
	Cmd      string            `json:"cmd"`
	Args     []string          `json:"args"`
	Cwd      string            `json:"cwd"`
	Env      map[string]string `json:"env"`
	TimeoutS int64             `json:"timeout_s"`
}

type ExecOutput struct {
	ExitCode        int    `json:"exit_code"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	TimedOut        bool   `json:"timed_out"`
	DurationMs      int64  `json:"duration_ms"`
	Status          string `json:"status"`
	SandboxID       string `json:"sandbox_id,omitempty"`
}

type FileContent struct {
	Content   []byte
	Truncated bool
}

type Capabilities struct {
	Available       bool     `json:"available"`
	Backend         string   `json:"backend"`
	Images          []string `json:"images"`
	DefaultImage    string   `json:"default_image"`
	CPUs            int      `json:"cpus"`
	MemoryMiB       int      `json:"memory_mib"`
	TimeoutDefaultS int64    `json:"timeout_default_s"`
	TimeoutMaxS     int64    `json:"timeout_max_s"`
	ExecTimeoutMaxS int64    `json:"exec_timeout_max_s"`
	MaxFileBytes    int      `json:"max_file_bytes"`
	MaxOutputBytes  int      `json:"max_output_bytes"`
	MaxPerProject   int      `json:"max_per_project"`
	// Terminal 为真表示后端能提供交互进程（PTY / 后台进程）。
	Terminal bool `json:"terminal"`
	// Pause 为真表示 pause 是真冻结（进程与内存保留），而不是退回 stop。
	Pause bool `json:"pause"`
	// Metrics 为真表示能读沙盒的资源用量。
	Metrics bool `json:"metrics"`
	// Network 是出网档位："none" / "egress"。egress 用独立 netns + pasta 出网，
	// 元数据与私网由沙盒内的黑洞路由拦住；不做域名白名单。
	Network string `json:"network"`
	// DiskLimitMiB 是每沙盒可写部分的上限；0 = 不限。超限冻结而不是杀。
	DiskLimitMiB int64 `json:"disk_limit_mib"`
}

// Manager 管理沙盒的元数据、串行访问、懒启动及清理；执行交给 vm.Driver（local / fake）。
type Manager struct {
	cfg    config.SandboxConfig
	store  Store
	driver vm.Driver
	now    func() time.Time
	mu     sync.Mutex
	locks  map[string]chan struct{}
	// lockWait 是同一 VM 锁的最长等待时间（§7.3 ErrBusy），测试可缩短。
	lockWait time.Duration
	logger   Logger
	// procs 登记每个沙盒当前的交互会话与后台进程（内存，进程内）。
	procs *procTable
	// envs 是创建时指定的 envVars，只存内存（见 SetEnvs）。
	envMu sync.Mutex
	envs  map[string]map[string]string
}

// Logger 是 Manager 需要的最小日志接口；observability.Logger 与 *zap.Logger 均满足。
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}

const defaultLockWait = 5 * time.Second

// removalFailedMarker 是「驱动删除失败、等待 reaper 重试」的 last_error 标记。
// 它是持久化数据，并被 systemdb.ListFailedSandboxRemovals 按字面量匹配；
// 历史行里已有该值，不要改文案。
const removalFailedMarker = "cloud removal failed"

func NewManager(cfg config.SandboxConfig, store Store, driver vm.Driver) *Manager {
	return &Manager{cfg: cfg, store: store, driver: driver, now: time.Now,
		locks: make(map[string]chan struct{}), lockWait: defaultLockWait,
		logger: zap.NewNop(), procs: newProcTable(), envs: make(map[string]map[string]string)}
}

// SetLogger 注入日志器；nil 恢复为空操作。日志只含 ID、状态、耗时与错误，不含命令、环境变量与文件内容。
func (m *Manager) SetLogger(l Logger) {
	if l == nil {
		l = zap.NewNop()
	}
	m.logger = l
}

// logOp 统一记录一次生命周期操作的结果；失败用 Warn 并带上底层错误。
func (m *Manager) logOp(op, projectID, id string, start time.Time, err error, fields ...zap.Field) {
	fs := append([]zap.Field{zap.String("op", op), zap.String("project_id", projectID),
		zap.String("sandbox_id", id), zap.Duration("duration", m.now().Sub(start))}, fields...)
	if err != nil {
		m.logger.Warn("sandbox operation failed", append(fs, zap.Error(err))...)
		return
	}
	m.logger.Info("sandbox operation ok", fs...)
}

func (m *Manager) Available() bool {
	return m != nil && m.cfg.Enabled && m.store != nil && m.driver != nil
}

func (m *Manager) Capabilities() Capabilities {
	c := Capabilities{Available: m.Available(), Backend: m.cfg.EffectiveBackend(), DefaultImage: m.cfg.Image,
		CPUs: m.cfg.CPUs, MemoryMiB: m.cfg.MemoryMiB,
		TimeoutDefaultS: int64(m.cfg.IdleTimeout.Seconds()), TimeoutMaxS: int64(m.cfg.MaxDuration.Seconds()),
		ExecTimeoutMaxS: int64(m.cfg.ExecTimeoutMax.Seconds()),
		MaxFileBytes:    m.cfg.MaxFileBytes, MaxOutputBytes: m.cfg.MaxOutputBytes, MaxPerProject: m.cfg.MaxPerProject,
		Images: append([]string(nil), m.cfg.Images...), Terminal: m.Interactive(),
		Pause: m.CanPause(), Metrics: m.CanMetrics(),
		Network:      m.networkMode(),
		DiskLimitMiB: m.cfg.DiskLimitMiB}
	if len(c.Images) == 0 {
		c.Images = []string{m.cfg.Image}
	}
	return c
}

// networkMode 返回生效的出网档位（空按 none）。
func (m *Manager) networkMode() string {
	if strings.EqualFold(strings.TrimSpace(m.cfg.Network), "") {
		return "none"
	}
	return strutil.Normalize(m.cfg.Network)
}

// Interactive 报告后端是否实现了 vm.Interactive，且配置没有关掉终端。
func (m *Manager) Interactive() bool {
	if !m.Available() || !m.cfg.Terminal.Enabled {
		return false
	}
	_, ok := m.driver.(vm.Interactive)
	return ok
}

// withLock 同一 VM 的操作串行，5s 后返回 ErrBusy；ctx 取消立即返回。
func (m *Manager) withLock(ctx context.Context, name string, fn func() error) error {
	m.mu.Lock()
	ch := m.locks[name]
	if ch == nil {
		ch = make(chan struct{}, 1)
		m.locks[name] = ch
	}
	wait := m.lockWait
	m.mu.Unlock()
	if wait <= 0 {
		wait = defaultLockWait
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case ch <- struct{}{}:
		defer func() { <-ch }()
		return fn()
	case <-ctx.Done():
		m.logger.Warn("sandbox lock wait canceled", zap.String("key", name), zap.Error(ctx.Err()))
		return ctx.Err()
	case <-timer.C:
		m.logger.Warn("sandbox lock wait timeout", zap.String("key", name), zap.Duration("wait", wait))
		return ErrBusy
	}
}

// withProjectLock 串行项目内的计数、名称校验与创建，单实例防止并发超限。
func (m *Manager) withProjectLock(ctx context.Context, projectID string, fn func() error) error {
	return m.withLock(ctx, "project:"+projectID, fn)
}

func (m *Manager) Create(ctx context.Context, projectID string, in CreateInput, actor string) (Sandbox, error) {
	if !m.Available() {
		return Sandbox{}, vm.ErrUnavailable
	}
	if err := validateMetadata(in.Metadata); err != nil {
		return Sandbox{}, err
	}
	var result Sandbox
	err := m.withProjectLock(ctx, projectID, func() error {
		if in.Name == "" {
			var b [3]byte
			if _, err := rand.Read(b[:]); err != nil {
				return err
			}
			in.Name = "sbx-" + hex.EncodeToString(b[:])
		}
		if !validName(in.Name) {
			return fmt.Errorf("%w: name", ErrInvalidSpec)
		}
		if old, err := m.store.GetSandboxByName(ctx, projectID, in.Name); err == nil && old.ID != "" {
			return ErrNameConflict
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		count, err := m.store.CountSandboxes(ctx, projectID)
		if err != nil {
			return err
		}
		max := m.cfg.MaxPerProject
		if max <= 0 {
			max = 5
		}
		if count >= max {
			return ErrLimitExceeded
		}
		if in.Image == "" {
			in.Image = m.cfg.Image
		}
		allowed := false
		images := m.cfg.Images
		if len(images) == 0 {
			images = []string{m.cfg.Image}
		}
		for _, img := range images {
			if img == in.Image {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("%w: image is not allowed", ErrInvalidSpec)
		}
		idle, err := m.clampTimeout(in.TimeoutS)
		if err != nil {
			return err
		}
		id := uuid.NewString()
		source := in.Source
		if source == "" {
			source = "api"
		}
		result, err = m.store.CreateSandbox(ctx, Sandbox{ID: id, ProjectID: projectID, Name: in.Name,
			CloudName: CloudName(id), Source: source, ThreadID: in.ThreadID,
			Image: in.Image, CPUs: m.cfg.CPUs, MemoryMiB: m.cfg.MemoryMiB, Network: "none",
			IdleTimeoutS: idle, MaxDurationS: int64(m.cfg.MaxDuration.Seconds()),
			Status: "pending", CreatedBy: actor, CreatedAt: m.now().UTC(), Metadata: in.Metadata})
		return err
	})
	if err != nil {
		m.logger.Warn("sandbox create failed", zap.String("project_id", projectID), zap.Error(err))
		return Sandbox{}, err
	}
	m.logger.Info("sandbox created", zap.String("project_id", projectID), zap.String("sandbox_id", result.ID),
		zap.String("source", result.Source), zap.String("image", result.Image), zap.Bool("start", in.Start))
	if in.Start && result.Status == "pending" {
		return m.Start(ctx, projectID, result.ID)
	}
	return result, nil
}

// clampTimeout 把请求的存活秒数归一：0 取实例默认；超过 max_duration 时截断（E2B 语义）；负数非法。
func (m *Manager) clampTimeout(s int64) (int64, error) {
	if s < 0 {
		return 0, fmt.Errorf("%w: timeout", ErrInvalidSpec)
	}
	max := int64(m.cfg.MaxDuration.Seconds())
	if s == 0 {
		s = int64(m.cfg.IdleTimeout.Seconds())
	}
	if max > 0 && s > max {
		s = max
	}
	if s < 1 {
		s = 1
	}
	return s, nil
}

// List 按创建时间倒序分页；cursor 为上一页返回的 next，空表示第一页。
func (m *Manager) List(ctx context.Context, projectID, status, source, cursor string, limit int) ([]Sandbox, string, error) {
	if !m.Available() {
		return nil, "", vm.ErrUnavailable
	}
	if len(cursor) > 128 {
		return nil, "", fmt.Errorf("%w: cursor", ErrInvalidSpec)
	}
	limit = pagelimit.Clamp(limit, 50, 100)
	return m.store.ListSandboxes(ctx, projectID, status, source, cursor, limit)
}

// GetByID 按沙盒 ID 全局取一行，不带项目条件。只供请求里没有项目的入口（SSH 登录）使用，
// 调用方取到后必须再校验主体对返回行 ProjectID 的访问权，且对外不得区分「不存在」与「无权限」。
func (m *Manager) GetByID(ctx context.Context, id string) (Sandbox, error) {
	if !m.Available() {
		return Sandbox{}, vm.ErrUnavailable
	}
	r, err := m.store.GetSandboxByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Sandbox{}, ErrNotFound
	}
	return r, err
}

// SetExpiry 从调用时刻起重新计时（E2B POST /sandboxes/{id}/timeout 语义）：
// 新的到期时间 = now + timeoutS，超过 max_duration 时截断。只对运行中的沙盒有意义。
func (m *Manager) SetExpiry(ctx context.Context, projectID, id string, timeoutS int64) (Sandbox, error) {
	if timeoutS < 0 {
		return Sandbox{}, fmt.Errorf("%w: timeout", ErrInvalidSpec)
	}
	var out Sandbox
	err := m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Status == "expired" {
			return ErrGone
		}
		t, err := m.clampTimeout(timeoutS)
		if err != nil {
			return err
		}
		now := m.now().UTC()
		exp := now.Add(time.Duration(t) * time.Second)
		r.ExpiresAt = &exp
		if r.StartedAt == nil {
			r.StartedAt = &now
		}
		if err := m.store.UpdateSandbox(ctx, r); err != nil {
			return err
		}
		out = r
		return nil
	})
	return out, err
}

// ExtendExpiry 只延长不缩短（E2B connect 语义）。返回更新后的行。
func (m *Manager) ExtendExpiry(ctx context.Context, projectID, id string, timeoutS int64) (Sandbox, error) {
	r, err := m.Get(ctx, projectID, id, false)
	if err != nil {
		return Sandbox{}, err
	}
	t, err := m.clampTimeout(timeoutS)
	if err != nil {
		return Sandbox{}, err
	}
	want := m.now().UTC().Add(time.Duration(t) * time.Second)
	if r.ExpiresAt != nil && !want.After(*r.ExpiresAt) {
		return r, nil
	}
	return m.SetExpiry(ctx, projectID, id, timeoutS)
}

func (m *Manager) Get(ctx context.Context, projectID, id string, refresh bool) (Sandbox, error) {
	if !m.Available() {
		return Sandbox{}, vm.ErrUnavailable
	}
	r, err := m.store.GetSandbox(ctx, projectID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Sandbox{}, ErrNotFound
	}
	if err != nil {
		return Sandbox{}, err
	}
	if !refresh || r.Status == "pending" {
		return r, nil
	}
	// paused 是「冻结」状态：工作区还在，驱动侧看起来仍是 running，
	// 不能按驱动状态覆盖，否则一次 Get 就把暂停抹掉了。
	if r.Status == "paused" {
		return r, nil
	}
	kind, err := m.driver.Status(ctx, r.CloudName)
	if err != nil {
		m.logger.Warn("sandbox driver status failed", zap.String("sandbox_id", id),
			zap.String("vm_name", r.CloudName), zap.Error(err))
		return Sandbox{}, fmt.Errorf("%w: status", vm.ErrBackend)
	}
	next := r.Status
	switch kind {
	case vm.VMRunning:
		next = "running"
	case vm.VMStopped:
		next = "stopped"
	case vm.VMAbsent:
		next = "expired"
	}
	if next != r.Status {
		r.Status = next
		if err := m.store.UpdateSandbox(ctx, r); err != nil {
			return Sandbox{}, err
		}
	}
	return r, nil
}

// validateMetadata 校验 E2B metadata：数量与长度有上限，键值不含控制字符，
// 且不得使用 SimpleBase 保留的 simplebase. 前缀与 name 键。
func validateMetadata(md map[string]string) error {
	if len(md) > 32 {
		return fmt.Errorf("%w: metadata has more than 32 entries", ErrInvalidSpec)
	}
	for k, v := range md {
		if k == "" || len(k) > 128 || len(v) > 1024 {
			return fmt.Errorf("%w: metadata key or value too long", ErrInvalidSpec)
		}
		if strings.HasPrefix(k, ReservedMetadataPrefix) || k == "name" {
			return fmt.Errorf("%w: metadata key %q is reserved", ErrInvalidSpec, k)
		}
		for _, r := range k + v {
			if r < 0x20 || r == 0x7f {
				return fmt.Errorf("%w: metadata contains control characters", ErrInvalidSpec)
			}
		}
	}
	return nil
}

// ReservedMetadataPrefix 是服务端保留的 metadata 键前缀（simplebase.source 等）。
const ReservedMetadataPrefix = "simplebase."

func (m *Manager) target(r Sandbox) vm.Target {
	return vm.Target{Name: r.CloudName, Spec: vm.Spec{Image: r.Image, CPUs: r.CPUs, MemoryMiB: r.MemoryMiB,
		Network: r.Network, Workdir: m.workdir(), IdleTimeout: time.Duration(r.IdleTimeoutS) * time.Second,
		MaxDuration: time.Duration(r.MaxDurationS) * time.Second,
		Labels:      map[string]string{"simplebase": "1", "project_id": r.ProjectID, "sandbox_id": r.ID}}}
}
func (m *Manager) workdir() string {
	if m.cfg.Workdir != "" {
		return m.cfg.Workdir
	}
	return "/workspace"
}
