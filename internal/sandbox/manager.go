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

func (m *Manager) markActive(ctx context.Context, r *Sandbox, reset bool) error {
	now := m.now().UTC()
	changed := false
	if reset || r.StartedAt == nil || r.Status == "expired" {
		r.StartedAt = &now
		expires := now.Add(time.Duration(r.MaxDurationS) * time.Second)
		r.ExpiresAt = &expires
		changed = true
	}
	if r.Status != "running" {
		r.Status = "running"
		changed = true
	}
	if r.LastActiveAt == nil || now.Sub(*r.LastActiveAt) >= 30*time.Second {
		r.LastActiveAt = &now
		changed = true
	}
	if changed {
		return m.store.UpdateSandbox(ctx, *r)
	}
	return nil
}

func (m *Manager) Start(ctx context.Context, projectID, id string) (r Sandbox, err error) {
	begin := m.now()
	defer func() { m.logOp("start", projectID, id, begin, err, zap.String("status", r.Status)) }()
	err = m.withLock(ctx, id, func() error {
		var err error
		r, err = m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if err = m.driver.Ensure(ctx, m.target(r)); err != nil {
			m.logger.Warn("sandbox driver ensure failed", zap.String("sandbox_id", id),
				zap.String("vm_name", r.CloudName), zap.String("image", r.Image), zap.Error(err))
			return fmt.Errorf("%w: start", vm.ErrBackend)
		}
		return m.markActive(ctx, &r, r.Status == "expired")
	})
	return r, err
}

func (m *Manager) Stop(ctx context.Context, projectID, id string) (r Sandbox, err error) {
	begin := m.now()
	defer func() { m.logOp("stop", projectID, id, begin, err) }()
	err = m.withLock(ctx, id, func() error {
		var err error
		r, err = m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		// 暂停没有内存快照：进程随之丢失，先关掉会话与进程，避免留下指向已停止工作区的句柄。
		m.closeSandboxProcs(id)
		// 冻结中的沙盒要先解冻，否则 Stop 只是写标记，进程仍冻在 cgroup 里。
		if r.Status == "paused" {
			if pz, ok := m.driver.(vm.Pauser); ok {
				_ = pz.Resume(ctx, m.target(r))
			}
		}
		if r.Status != "pending" {
			if err = m.driver.Stop(ctx, r.CloudName); err != nil {
				m.logger.Warn("sandbox driver stop failed", zap.String("sandbox_id", id),
					zap.String("vm_name", r.CloudName), zap.Error(err))
				return fmt.Errorf("%w: stop", vm.ErrBackend)
			}
		}
		r.Status = "stopped"
		return m.store.UpdateSandbox(ctx, r)
	})
	return r, err
}

// Pause 冻结沙盒：进程与内存保留，恢复是毫秒级。
//
// 驱动实现了 vm.Pauser 时用 cgroup.freeze；否则退回 Stop（进程丢失），
// 并在返回值里如实反映状态，调用方据 Capabilities.Pause 决定是否暴露该能力。
func (m *Manager) Pause(ctx context.Context, projectID, id string) (r Sandbox, err error) {
	begin := m.now()
	defer func() { m.logOp("pause", projectID, id, begin, err, zap.String("status", r.Status)) }()
	err = m.withLock(ctx, id, func() error {
		var err error
		r, err = m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Status == "paused" {
			return ErrAlreadyPaused
		}
		p, ok := m.driver.(vm.Pauser)
		freezing := ok && m.CanPause()
		if !freezing {
			// 没有冻结能力：退回 Stop，语义与旧行为一致。
			m.closeSandboxProcs(id)
			if r.Status != "pending" {
				if err = m.driver.Stop(ctx, r.CloudName); err != nil {
					return fmt.Errorf("%w: pause", vm.ErrBackend)
				}
			}
			r.Status = "stopped"
			return m.store.UpdateSandbox(ctx, r)
		}
		if err = p.Pause(ctx, r.CloudName); err != nil {
			m.logger.Warn("sandbox driver pause failed", zap.String("sandbox_id", id),
				zap.String("vm_name", r.CloudName), zap.Error(err))
			return fmt.Errorf("%w: pause", vm.ErrBackend)
		}
		r.Status = "paused"
		return m.store.UpdateSandbox(ctx, r)
	})
	return r, err
}

// Resume 解冻沙盒。已停止（或从未启动）时等价于 Start。
func (m *Manager) Resume(ctx context.Context, projectID, id string) (r Sandbox, err error) {
	begin := m.now()
	defer func() { m.logOp("resume", projectID, id, begin, err, zap.String("status", r.Status)) }()
	err = m.withLock(ctx, id, func() error {
		var err error
		r, err = m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Status != "paused" {
			// 不是冻结状态：按 Start 处理（幂等）。
			if err = m.driver.Ensure(ctx, m.target(r)); err != nil {
				return fmt.Errorf("%w: resume", vm.ErrBackend)
			}
			return m.markActive(ctx, &r, r.Status == "expired")
		}
		if p, ok := m.driver.(vm.Pauser); ok {
			if err = p.Resume(ctx, m.target(r)); err != nil {
				m.logger.Warn("sandbox driver resume failed", zap.String("sandbox_id", id),
					zap.String("vm_name", r.CloudName), zap.Error(err))
				return fmt.Errorf("%w: resume", vm.ErrBackend)
			}
		}
		return m.markActive(ctx, &r, false)
	})
	return r, err
}

// Metrics 读沙盒的资源用量。驱动不支持时返回 vm.ErrNotSupported。
func (m *Manager) Metrics(ctx context.Context, projectID, id string) (vm.Metrics, error) {
	if !m.Available() {
		return vm.Metrics{}, vm.ErrUnavailable
	}
	r, err := m.Get(ctx, projectID, id, false)
	if err != nil {
		return vm.Metrics{}, err
	}
	meter, ok := m.driver.(vm.Meter)
	if !ok {
		return vm.Metrics{}, fmt.Errorf("%w: metrics", vm.ErrNotSupported)
	}
	return meter.Metrics(ctx, r.CloudName)
}

// CanPause 报告驱动是否支持真冻结（而不是退回 Stop）。
func (m *Manager) CanPause() bool {
	if !m.Available() {
		return false
	}
	if _, ok := m.driver.(vm.Pauser); !ok {
		return false
	}
	if f, ok := m.driver.(vm.Freezer); ok {
		return f.CanFreeze()
	}
	return true
}

// CanMetrics 报告驱动是否能提供资源用量。
func (m *Manager) CanMetrics() bool {
	if !m.Available() {
		return false
	}
	_, ok := m.driver.(vm.Meter)
	return ok
}

func (m *Manager) Delete(ctx context.Context, projectID, id string) (err error) {
	begin := m.now()
	defer func() { m.logOp("delete", projectID, id, begin, err) }()
	return m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		m.closeSandboxProcs(id)
		// 即使尚为 pending，也可能是驱动首次启动后、落库前失败；Remove 必须幂等。
		if err := m.driver.Remove(ctx, r.CloudName); err != nil {
			m.logger.Error("sandbox removal failed, kept for reaper retry", zap.String("sandbox_id", id),
				zap.String("vm_name", r.CloudName), zap.Error(err))
			// 不隐藏失败：保留元数据供 reaper 重试，避免 VM 泄漏且仍然计费。
			r.LastError = removalFailedMarker
			if updateErr := m.store.UpdateSandbox(ctx, r); updateErr != nil {
				return updateErr
			}
			return fmt.Errorf("%w: remove", vm.ErrBackend)
		}
		return m.store.SoftDeleteSandbox(ctx, projectID, id, "")
	})
}

func (m *Manager) validExec(in ExecInput) (vm.ExecSpec, error) {
	if (strings.TrimSpace(in.Command) == "") == (strings.TrimSpace(in.Cmd) == "") {
		return vm.ExecSpec{}, ErrInvalidSpec
	}
	if len(in.Command) > 4096 || len(in.Cmd) > 1024 || len(in.Args) > 128 {
		return vm.ExecSpec{}, ErrInvalidSpec
	}
	for _, arg := range in.Args {
		if len(arg) > 4096 || strings.ContainsRune(arg, 0) {
			return vm.ExecSpec{}, ErrInvalidSpec
		}
	}
	if err := validateEnv(in.Env); err != nil {
		return vm.ExecSpec{}, err
	}
	cwd := in.Cwd
	if cwd == "" {
		cwd = m.workdir()
	}
	var err error
	cwd, err = vm.CleanPath(m.workdir(), cwd, true)
	if err != nil {
		return vm.ExecSpec{}, err
	}
	timeout := time.Duration(in.TimeoutS) * time.Second
	if timeout == 0 {
		timeout = m.cfg.ExecTimeout
	}
	max := m.cfg.ExecTimeoutMax
	if max <= 0 {
		max = 300 * time.Second
	}
	if timeout < time.Second || timeout > max {
		return vm.ExecSpec{}, fmt.Errorf("%w: timeout_s", ErrInvalidSpec)
	}
	return vm.ExecSpec{Shell: in.Command, Cmd: in.Cmd, Args: in.Args, Cwd: cwd, Env: in.Env, Timeout: timeout}, nil
}

func (m *Manager) execSandbox(ctx context.Context, r Sandbox, spec vm.ExecSpec, threadID string) (ExecOutput, error) {
	var out ExecOutput
	start := m.now()
	result, err := m.driver.Exec(ctx, m.target(r), spec)
	if err != nil {
		fields := []zap.Field{zap.String("sandbox_id", r.ID), zap.String("vm_name", r.CloudName), zap.Error(err)}
		if threadID != "" {
			fields = append(fields, zap.String("thread_id", threadID))
		}
		m.logger.Warn("sandbox driver exec failed", fields...)
		return out, fmt.Errorf("%w: exec", vm.ErrBackend)
	}
	fields := []zap.Field{zap.String("sandbox_id", r.ID), zap.Int("exit_code", result.ExitCode),
		zap.Bool("timed_out", result.TimedOut), zap.Duration("duration", m.now().Sub(start)),
		zap.Int("stdout_bytes", len(result.Stdout)), zap.Int("stderr_bytes", len(result.Stderr))}
	m.logger.Info("sandbox exec done", fields...)
	if err := m.markActive(ctx, &r, false); err != nil {
		return out, err
	}
	out.Stdout, out.StdoutTruncated = vm.TruncateUTF8(result.Stdout, m.cfg.MaxOutputBytes)
	out.Stderr, out.StderrTruncated = vm.TruncateUTF8(result.Stderr, m.cfg.MaxOutputBytes)
	out.ExitCode = result.ExitCode
	out.TimedOut = result.TimedOut
	out.DurationMs = m.now().Sub(start).Milliseconds()
	out.Status = r.Status
	return out, nil
}

// Exec 同步执行；非零退出与超时为成功 HTTP 结果（§7.2）。
func (m *Manager) Exec(ctx context.Context, projectID, id string, in ExecInput) (ExecOutput, error) {
	spec, err := m.validExec(in)
	if err != nil {
		return ExecOutput{}, err
	}
	var out ExecOutput
	err = m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Source == "agent" {
			return ErrBusy
		}
		if r.Status == "expired" {
			return ErrGone
		}
		out, err = m.execSandbox(ctx, r, spec, "")
		return err
	})
	return out, err
}

func (m *Manager) ReadFile(ctx context.Context, projectID, id, p string) (FileContent, error) {
	var result FileContent
	clean, err := vm.CleanPath(m.workdir(), p, false)
	if err != nil {
		return result, err
	}
	err = m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Status == "expired" {
			return ErrGone
		}
		data, err := m.driver.ReadFile(ctx, m.target(r), clean)
		if err != nil {
			if errors.Is(err, vm.ErrFileNotFound) {
				return err
			}
			m.logger.Warn("sandbox driver read failed", zap.String("sandbox_id", id), zap.String("path", clean), zap.Error(err))
			return fmt.Errorf("%w: read", vm.ErrBackend)
		}
		if err = m.markActive(ctx, &r, false); err != nil {
			return err
		}
		max := m.cfg.MaxFileBytes
		if max > 0 && len(data) > max {
			result.Truncated = true
			data = data[:max]
		}
		result.Content = data
		return nil
	})
	return result, err
}

func (m *Manager) WriteFile(ctx context.Context, projectID, id, p string, data []byte) error {
	clean, err := vm.CleanPath(m.workdir(), p, false)
	if err != nil {
		return err
	}
	if m.cfg.MaxFileBytes > 0 && len(data) > m.cfg.MaxFileBytes {
		return ErrFileTooLarge
	}
	return m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Source == "agent" {
			return ErrBusy
		}
		if r.Status == "expired" {
			return ErrGone
		}
		if err = m.driver.WriteFile(ctx, m.target(r), clean, data); err != nil {
			m.logger.Warn("sandbox driver write failed", zap.String("sandbox_id", id), zap.String("path", clean), zap.Error(err))
			return fmt.Errorf("%w: write", vm.ErrBackend)
		}
		return m.markActive(ctx, &r, false)
	})
}

func (m *Manager) RemoveFile(ctx context.Context, projectID, id, p string) error {
	clean, err := vm.CleanPath(m.workdir(), p, false)
	if err != nil {
		return err
	}
	return m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Source == "agent" {
			return ErrBusy
		}
		if r.Status == "expired" {
			return ErrGone
		}
		if err = m.driver.RemoveFile(ctx, m.target(r), clean); err != nil {
			m.logger.Warn("sandbox driver remove file failed", zap.String("sandbox_id", id), zap.String("path", clean), zap.Error(err))
			return fmt.Errorf("%w: remove file", vm.ErrBackend)
		}
		return m.markActive(ctx, &r, false)
	})
}

func (m *Manager) ListDir(ctx context.Context, projectID, id, p string) ([]vm.FileEntry, error) {
	if p == "" {
		p = m.workdir()
	}
	clean, err := vm.CleanPath(m.workdir(), p, true)
	if err != nil {
		return nil, err
	}
	var entries []vm.FileEntry
	err = m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Status == "expired" {
			return ErrGone
		}
		entries, err = m.driver.ListDir(ctx, m.target(r), clean)
		if err != nil {
			m.logger.Warn("sandbox driver list failed", zap.String("sandbox_id", id), zap.String("path", clean), zap.Error(err))
			return fmt.Errorf("%w: list", vm.ErrBackend)
		}
		if len(entries) > 500 {
			entries = entries[:500]
		}
		return m.markActive(ctx, &r, false)
	})
	return entries, err
}

// EnsureForThread 为 agent thread 取一个沙盒（按 thread 幂等）。
func (m *Manager) EnsureForThread(ctx context.Context, projectID, threadID string) (Sandbox, error) {
	if !m.Available() {
		return Sandbox{}, vm.ErrUnavailable
	}
	var r Sandbox
	err := m.withProjectLock(ctx, projectID, func() error {
		var err error
		r, err = m.store.GetSandboxByThread(ctx, projectID, threadID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
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
		id := uuid.NewString()
		r, err = m.store.CreateSandbox(ctx, Sandbox{ID: id, ProjectID: projectID,
			Name: "agent-" + strings.ReplaceAll(threadID, "-", ""), CloudName: CloudName(id),
			Source: "agent", ThreadID: threadID, Image: m.cfg.Image, CPUs: m.cfg.CPUs, MemoryMiB: m.cfg.MemoryMiB,
			Network: m.cfg.Network, IdleTimeoutS: int64(m.cfg.IdleTimeout.Seconds()),
			MaxDurationS: int64(m.cfg.MaxDuration.Seconds()), Status: "pending", CreatedAt: m.now().UTC()})
		return err
	})
	return r, err
}

func (m *Manager) ExecForThread(ctx context.Context, projectID, threadID string, in ExecInput) (ExecOutput, error) {
	r, err := m.EnsureForThread(ctx, projectID, threadID)
	if err != nil {
		return ExecOutput{}, err
	}
	spec, err := m.validExec(in)
	if err != nil {
		return ExecOutput{}, err
	}
	var out ExecOutput
	err = m.withLock(ctx, r.ID, func() error {
		r, err = m.Get(ctx, projectID, r.ID, false)
		if err != nil {
			return err
		}
		out, err = m.execSandbox(ctx, r, spec, threadID)
		return err
	})
	return out, err
}

func (m *Manager) ReadFileForThread(ctx context.Context, projectID, threadID, p string) (FileContent, error) {
	r, err := m.EnsureForThread(ctx, projectID, threadID)
	if err != nil {
		return FileContent{}, err
	}
	return m.ReadFile(ctx, projectID, r.ID, p)
}
func (m *Manager) WriteFileForThread(ctx context.Context, projectID, threadID, p string, data []byte) error {
	r, err := m.EnsureForThread(ctx, projectID, threadID)
	if err != nil {
		return err
	}
	clean, err := vm.CleanPath(m.workdir(), p, false)
	if err != nil {
		return err
	}
	if m.cfg.MaxFileBytes > 0 && len(data) > m.cfg.MaxFileBytes {
		return ErrFileTooLarge
	}
	return m.withLock(ctx, r.ID, func() error {
		r, err = m.Get(ctx, projectID, r.ID, false)
		if err != nil {
			return err
		}
		if e := m.driver.WriteFile(ctx, m.target(r), clean, data); e != nil {
			m.logger.Warn("sandbox driver write failed", zap.String("sandbox_id", r.ID), zap.String("path", clean),
				zap.String("thread_id", threadID), zap.Error(e))
			return fmt.Errorf("%w: write", vm.ErrBackend)
		}
		return m.markActive(ctx, &r, false)
	})
}
func (m *Manager) ReleaseThread(ctx context.Context, projectID, threadID string) error {
	if !m.Available() {
		return nil
	}
	r, err := m.store.GetSandboxByThread(ctx, projectID, threadID)
	if errors.Is(err, sql.ErrNoRows) {
		// 没有元数据行 = 这个 thread 没有沙盒，无需清理。
		return nil
	}
	if err != nil {
		return err
	}
	return m.Delete(ctx, projectID, r.ID)
}

// freezeIdle 把空闲超过 FreezeAfter 的运行中沙盒冻结（cgroup.freeze）。
//
// 冻结保留内存与进程，恢复是毫秒级；这是密度手段，不是回收手段：
// 真正的回收仍由 ListExpiredSandboxes + 软删负责。驱动不支持 Pauser 时直接返回。
func (m *Manager) freezeIdle(ctx context.Context, now time.Time) {
	if m.cfg.FreezeAfter <= 0 {
		return
	}
	if _, ok := m.driver.(vm.Pauser); !ok {
		return
	}
	idle, err := m.store.ListIdleRunningSandboxes(ctx, now.Add(-m.cfg.FreezeAfter), 100)
	if err != nil {
		m.logger.Warn("reaper listing idle sandboxes failed", zap.Error(err))
		return
	}
	for _, r := range idle {
		if ctx.Err() != nil {
			return
		}
		// 有活跃进程（终端 / 后台命令）的沙盒不算空闲：进程自己会刷 activity。
		if len(m.procs.list(r.ID)) > 0 {
			continue
		}
		if _, err := m.Pause(ctx, r.ProjectID, r.ID); err != nil {
			m.logger.Warn("sandbox idle freeze failed", zap.String("sandbox_id", r.ID), zap.Error(err))
			continue
		}
		m.logger.Info("sandbox frozen while idle", zap.String("sandbox_id", r.ID),
			zap.Duration("freeze_after", m.cfg.FreezeAfter))
	}
}

// ReapOnce 每轮只处理至多 100 行，先清理遗留 VM，再清过期状态和软删行。
func (m *Manager) ReapOnce(ctx context.Context) error {
	if !m.Available() {
		return nil
	}
	now := m.now()
	m.reapProcs(ctx)
	failed, err := m.store.ListFailedSandboxRemovals(ctx, 100)
	if err != nil {
		return err
	}
	for _, r := range failed {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.logger.Info("reaper retrying failed removal", zap.String("sandbox_id", r.ID))
		_ = m.Delete(ctx, r.ProjectID, r.ID)
	}
	stale, err := m.store.ListStaleRunSandboxes(ctx, now.Add(-m.cfg.ExecTimeoutMax-10*time.Minute), 100)
	if err != nil {
		return err
	}
	for _, r := range stale {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.logger.Info("reaper deleting stale run sandbox", zap.String("sandbox_id", r.ID))
		_ = m.Delete(ctx, r.ProjectID, r.ID)
	}
	m.freezeIdle(ctx, now)
	expired, err := m.store.ListExpiredSandboxes(ctx, now, 100)
	if err != nil {
		return err
	}
	for _, r := range expired {
		m.logger.Info("reaper marking sandbox expired", zap.String("sandbox_id", r.ID))
		m.closeSandboxProcs(r.ID)
		r.Status = "expired"
		if err = m.store.UpdateSandbox(ctx, r); err != nil {
			return err
		}
	}
	purged, err := m.store.PurgeDeletedSandboxes(ctx, now.Add(-7*24*time.Hour))
	if len(failed)+len(stale)+len(expired) > 0 || purged > 0 {
		m.logger.Info("reaper round done", zap.Int("retry_removal", len(failed)), zap.Int("stale_run", len(stale)),
			zap.Int("expired", len(expired)), zap.Int64("purged", purged))
	}
	return err
}

// StartReaper 启动单一低频清理协程；ctx 取消时退出。
func (m *Manager) StartReaper(ctx context.Context) {
	if !m.Available() || m.cfg.ReapInterval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(m.cfg.ReapInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.ReapOnce(ctx); err != nil && ctx.Err() == nil {
					m.logger.Warn("sandbox reaper round failed", zap.Error(err))
				}
			}
		}
	}()
	m.logger.Info("sandbox reaper started", zap.Duration("interval", m.cfg.ReapInterval))
}

// Close 关闭全部会话与进程。
func (m *Manager) Close() error {
	if m == nil || m.procs == nil {
		return nil
	}
	for _, h := range m.procs.all() {
		_ = h.Close()
	}
	return nil
}
