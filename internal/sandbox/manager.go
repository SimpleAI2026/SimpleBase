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
	"github.com/linkxzhou/SimpleBase/internal/config"
)

// Sandbox 是项目级沙盒的持久元数据，命令和文件内容不入库。
type Sandbox struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"project_id"`
	Name         string     `json:"name"`
	CloudName    string     `json:"cloud_name"`
	Source       string     `json:"source"`
	ThreadID     string     `json:"thread_id,omitempty"`
	Image        string     `json:"image"`
	CPUs         int        `json:"cpus"`
	MemoryMiB    int        `json:"memory_mib"`
	Network      string     `json:"network"`
	IdleTimeoutS int64      `json:"idle_timeout_s"`
	MaxDurationS int64      `json:"max_duration_s"`
	Status       string     `json:"status"`
	Reused       bool       `json:"-"` // 内存幂等表命中，HTTP 返回 200
	LastError    string     `json:"last_error,omitempty"`
	CreatedBy    string     `json:"created_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at"`
	LastActiveAt *time.Time `json:"last_active_at"`
	ExpiresAt    *time.Time `json:"expires_at"`
	DeletedAt    *time.Time `json:"-"`
}

// Store 由 app 适配 systemdb.Store，隔离业务层与 SQL。
type Store interface {
	CreateSandbox(context.Context, Sandbox) (Sandbox, error)
	GetSandbox(context.Context, string, string) (Sandbox, error)
	GetSandboxByName(context.Context, string, string) (Sandbox, error)
	GetSandboxByThread(context.Context, string, string) (Sandbox, error)
	ListSandboxes(context.Context, string, string, string, string, int) ([]Sandbox, string, error)
	CountSandboxes(context.Context, string) (int, error)
	UpdateSandbox(context.Context, Sandbox) error
	SoftDeleteSandbox(context.Context, string, string, string) error
	ListExpiredSandboxes(context.Context, time.Time, int) ([]Sandbox, error)
	ListStaleRunSandboxes(context.Context, time.Time, int) ([]Sandbox, error)
	ListFailedSandboxRemovals(context.Context, int) ([]Sandbox, error)
	PurgeDeletedSandboxes(context.Context, time.Time) (int64, error)
}

// CreateInput 描述新沙盒，零值采用实例默认配置。
type CreateInput struct {
	Name           string `json:"name"`
	Image          string `json:"image"`
	CPUs           int    `json:"cpus"`
	MemoryMiB      int    `json:"memory_mib"`
	Network        string `json:"network"`
	IdleTimeoutS   int64  `json:"idle_timeout_s"`
	Start          bool   `json:"start"`
	IdempotencyKey string `json:"-"`
	Source         string `json:"-"`
	ThreadID       string `json:"-"`
}

type UpdateInput struct {
	Name         *string `json:"name"`
	IdleTimeoutS *int64  `json:"idle_timeout_s"`
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

type RunFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type RunInput struct {
	Image    string            `json:"image"`
	Files    []RunFile         `json:"files"`
	Command  string            `json:"command"`
	Cmd      string            `json:"cmd"`
	Args     []string          `json:"args"`
	Cwd      string            `json:"cwd"`
	Env      map[string]string `json:"env"`
	TimeoutS int64             `json:"timeout_s"`
	Keep     bool              `json:"keep"`
}

func (in RunInput) execInput() ExecInput {
	return ExecInput{Command: in.Command, Cmd: in.Cmd, Args: in.Args, Cwd: in.Cwd, Env: in.Env, TimeoutS: in.TimeoutS}
}

type Capabilities struct {
	Available       bool     `json:"available"`
	Backend         string   `json:"backend"`
	Images          []string `json:"images"`
	DefaultImage    string   `json:"default_image"`
	CPUsMax         int      `json:"cpus_max"`
	MemoryMiBMax    int      `json:"memory_mib_max"`
	ExecTimeoutMaxS int64    `json:"exec_timeout_max_s"`
	MaxFileBytes    int      `json:"max_file_bytes"`
	MaxOutputBytes  int      `json:"max_output_bytes"`
	MaxPerProject   int      `json:"max_per_project"`
	NetworkOptions  []string `json:"network_options"`
}

type idemEntry struct {
	id    string
	until time.Time
}

// Manager 管理 VM 的元数据、串行访问、懒启动及清理。
type Manager struct {
	cfg    config.SandboxConfig
	store  Store
	driver Driver
	now    func() time.Time
	mu     sync.Mutex
	locks  map[string]chan struct{}
	idem   map[string]idemEntry
	// lockWait 是同一 VM 锁的最长等待时间（§7.3 ErrBusy），测试可缩短。
	lockWait time.Duration
}

const defaultLockWait = 5 * time.Second

func NewManager(cfg config.SandboxConfig, store Store, driver Driver) *Manager {
	return &Manager{cfg: cfg, store: store, driver: driver, now: time.Now,
		locks: make(map[string]chan struct{}), idem: make(map[string]idemEntry), lockWait: defaultLockWait}
}

func (m *Manager) Available() bool {
	return m != nil && m.cfg.Enabled && m.store != nil && m.driver != nil
}

func (m *Manager) Capabilities() Capabilities {
	c := Capabilities{Available: m.Available(), Backend: m.cfg.EffectiveBackend(), DefaultImage: m.cfg.Image,
		CPUsMax: 4, MemoryMiBMax: 4096, ExecTimeoutMaxS: int64(m.cfg.ExecTimeoutMax.Seconds()),
		MaxFileBytes: m.cfg.MaxFileBytes, MaxOutputBytes: m.cfg.MaxOutputBytes, MaxPerProject: m.cfg.MaxPerProject,
		Images: append([]string(nil), m.cfg.Images...), NetworkOptions: []string{"none"}}
	if len(c.Images) == 0 {
		c.Images = []string{m.cfg.Image}
	}
	if m.cfg.Network == "public" {
		c.NetworkOptions = append(c.NetworkOptions, "public")
	}
	return c
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
		return ctx.Err()
	case <-timer.C:
		return ErrBusy
	}
}

// withProjectLock 串行项目内的计数、名称校验与创建，单实例防止并发超限。
func (m *Manager) withProjectLock(ctx context.Context, projectID string, fn func() error) error {
	return m.withLock(ctx, "project:"+projectID, fn)
}

func (m *Manager) Create(ctx context.Context, projectID string, in CreateInput, actor string) (Sandbox, error) {
	if !m.Available() {
		return Sandbox{}, ErrUnavailable
	}
	if len(in.IdempotencyKey) > 128 || strings.ContainsAny(in.IdempotencyKey, "\r\n") {
		return Sandbox{}, fmt.Errorf("%w: idempotency key", ErrInvalidSpec)
	}
	var result Sandbox
	err := m.withProjectLock(ctx, projectID, func() error {
		if in.IdempotencyKey != "" {
			key := projectID + ":" + in.IdempotencyKey
			m.mu.Lock()
			entry, ok := m.idem[key]
			m.mu.Unlock()
			if ok && m.now().Before(entry.until) {
				if old, err := m.store.GetSandbox(ctx, projectID, entry.id); err == nil {
					result = old
					result.Reused = true
					return nil
				}
			}
		}
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
		if in.CPUs == 0 {
			in.CPUs = m.cfg.CPUs
		}
		if in.MemoryMiB == 0 {
			in.MemoryMiB = m.cfg.MemoryMiB
		}
		if in.CPUs < 1 || in.CPUs > 4 || in.MemoryMiB < 128 || in.MemoryMiB > 4096 {
			return fmt.Errorf("%w: resources exceed limits", ErrInvalidSpec)
		}
		if in.Network == "" {
			in.Network = "none"
		}
		if in.Network != "none" && (in.Network != "public" || m.cfg.Network != "public") {
			return fmt.Errorf("%w: network not allowed", ErrInvalidSpec)
		}
		if in.IdleTimeoutS == 0 {
			in.IdleTimeoutS = int64(m.cfg.IdleTimeout.Seconds())
		}
		if in.IdleTimeoutS < 1 || in.IdleTimeoutS > int64(m.cfg.MaxDuration.Seconds()) {
			return fmt.Errorf("%w: idle_timeout_s", ErrInvalidSpec)
		}
		id := uuid.NewString()
		cloudName := CloudName(id)
		if in.Source == "agent" {
			cloudName = ThreadCloudName(in.ThreadID)
		}
		source := in.Source
		if source == "" {
			source = "api"
		}
		result, err = m.store.CreateSandbox(ctx, Sandbox{ID: id, ProjectID: projectID, Name: in.Name,
			CloudName: cloudName, Source: source, ThreadID: in.ThreadID,
			Image: in.Image, CPUs: in.CPUs, MemoryMiB: in.MemoryMiB, Network: in.Network,
			IdleTimeoutS: in.IdleTimeoutS, MaxDurationS: int64(m.cfg.MaxDuration.Seconds()),
			Status: "pending", CreatedBy: actor, CreatedAt: m.now().UTC()})
		if err != nil {
			return err
		}
		if in.IdempotencyKey != "" {
			m.mu.Lock()
			if len(m.idem) >= 1024 {
				for k, e := range m.idem {
					if m.now().After(e.until) {
						delete(m.idem, k)
					}
				}
				if len(m.idem) >= 1024 {
					for k := range m.idem {
						delete(m.idem, k)
						break
					}
				}
			}
			m.idem[projectID+":"+in.IdempotencyKey] = idemEntry{id: result.ID, until: m.now().Add(24 * time.Hour)}
			m.mu.Unlock()
		}
		return nil
	})
	if err != nil {
		return Sandbox{}, err
	}
	if in.Start && result.Status == "pending" {
		return m.Start(ctx, projectID, result.ID)
	}
	return result, nil
}

// List 按创建时间倒序分页；cursor 为上一页返回的 next，空表示第一页。
func (m *Manager) List(ctx context.Context, projectID, status, source, cursor string, limit int) ([]Sandbox, string, error) {
	if !m.Available() {
		return nil, "", ErrUnavailable
	}
	if len(cursor) > 128 {
		return nil, "", fmt.Errorf("%w: cursor", ErrInvalidSpec)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	return m.store.ListSandboxes(ctx, projectID, status, source, cursor, limit)
}

func (m *Manager) Get(ctx context.Context, projectID, id string, refresh bool) (Sandbox, error) {
	if !m.Available() {
		return Sandbox{}, ErrUnavailable
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
	vm, err := m.driver.Status(ctx, r.CloudName)
	if err != nil {
		return Sandbox{}, fmt.Errorf("%w: status", ErrBackend)
	}
	next := r.Status
	switch vm {
	case VMRunning:
		next = "running"
	case VMStopped:
		next = "stopped"
	case VMAbsent:
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

func (m *Manager) Update(ctx context.Context, projectID, id string, in UpdateInput) (Sandbox, error) {
	var result Sandbox
	err := m.withProjectLock(ctx, projectID, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if in.Name != nil {
			if !validName(*in.Name) {
				return ErrInvalidSpec
			}
			if *in.Name != r.Name {
				if _, err := m.store.GetSandboxByName(ctx, projectID, *in.Name); err == nil {
					return ErrNameConflict
				} else if !errors.Is(err, sql.ErrNoRows) {
					return err
				}
			}
			r.Name = *in.Name
		}
		if in.IdleTimeoutS != nil {
			if *in.IdleTimeoutS < 1 || *in.IdleTimeoutS > r.MaxDurationS {
				return ErrInvalidSpec
			}
			r.IdleTimeoutS = *in.IdleTimeoutS
		}
		if err := m.store.UpdateSandbox(ctx, r); err != nil {
			return err
		}
		result = r
		return nil
	})
	return result, err
}

func (m *Manager) target(r Sandbox) Target {
	return Target{Name: r.CloudName, Spec: Spec{Image: r.Image, CPUs: r.CPUs, MemoryMiB: r.MemoryMiB,
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

func (m *Manager) Start(ctx context.Context, projectID, id string) (Sandbox, error) {
	var r Sandbox
	err := m.withLock(ctx, id, func() error {
		var err error
		r, err = m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if err = m.driver.Ensure(ctx, m.target(r)); err != nil {
			return fmt.Errorf("%w: start", ErrBackend)
		}
		return m.markActive(ctx, &r, r.Status == "expired")
	})
	return r, err
}

func (m *Manager) Stop(ctx context.Context, projectID, id string) (Sandbox, error) {
	var r Sandbox
	err := m.withLock(ctx, id, func() error {
		var err error
		r, err = m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		if r.Status != "pending" {
			if err = m.driver.Stop(ctx, r.CloudName); err != nil {
				return fmt.Errorf("%w: stop", ErrBackend)
			}
		}
		r.Status = "stopped"
		return m.store.UpdateSandbox(ctx, r)
	})
	return r, err
}

func (m *Manager) Delete(ctx context.Context, projectID, id string) error {
	return m.withLock(ctx, id, func() error {
		r, err := m.Get(ctx, projectID, id, false)
		if err != nil {
			return err
		}
		// 即使尚为 pending，也可能是驱动首次启动后、落库前失败；Remove 必须幂等。
		if err := m.driver.Remove(ctx, r.CloudName); err != nil {
			// 不隐藏失败：保留元数据供 reaper 重试，避免 VM 泄漏且仍然计费。
			r.LastError = "cloud removal failed"
			if updateErr := m.store.UpdateSandbox(ctx, r); updateErr != nil {
				return updateErr
			}
			return fmt.Errorf("%w: remove", ErrBackend)
		}
		return m.store.SoftDeleteSandbox(ctx, projectID, id, "")
	})
}

func (m *Manager) validExec(in ExecInput) (ExecSpec, error) {
	if (strings.TrimSpace(in.Command) == "") == (strings.TrimSpace(in.Cmd) == "") {
		return ExecSpec{}, ErrInvalidSpec
	}
	if len(in.Command) > 4096 || len(in.Cmd) > 1024 || len(in.Args) > 128 {
		return ExecSpec{}, ErrInvalidSpec
	}
	for _, arg := range in.Args {
		if len(arg) > 4096 || strings.ContainsRune(arg, 0) {
			return ExecSpec{}, ErrInvalidSpec
		}
	}
	if err := validateEnv(in.Env); err != nil {
		return ExecSpec{}, err
	}
	cwd := in.Cwd
	if cwd == "" {
		cwd = m.workdir()
	}
	var err error
	cwd, err = cleanPath(m.workdir(), cwd, true)
	if err != nil {
		return ExecSpec{}, err
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
		return ExecSpec{}, fmt.Errorf("%w: timeout_s", ErrInvalidSpec)
	}
	return ExecSpec{Shell: in.Command, Cmd: in.Cmd, Args: in.Args, Cwd: cwd, Env: in.Env, Timeout: timeout}, nil
}

// Exec 同步执行；非零退出与超时为成功 HTTP 结果（§7.2）。
func (m *Manager) Exec(ctx context.Context, projectID, id string, in ExecInput) (ExecOutput, error) {
	var out ExecOutput
	spec, err := m.validExec(in)
	if err != nil {
		return out, err
	}
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
		start := m.now()
		result, err := m.driver.Exec(ctx, m.target(r), spec)
		if err != nil {
			return fmt.Errorf("%w: exec", ErrBackend)
		}
		if err = m.markActive(ctx, &r, false); err != nil {
			return err
		}
		out.Stdout, out.StdoutTruncated = truncateUTF8(result.Stdout, m.cfg.MaxOutputBytes)
		out.Stderr, out.StderrTruncated = truncateUTF8(result.Stderr, m.cfg.MaxOutputBytes)
		out.ExitCode = result.ExitCode
		out.TimedOut = result.TimedOut
		out.DurationMs = m.now().Sub(start).Milliseconds()
		out.Status = r.Status
		return nil
	})
	return out, err
}

func (m *Manager) ReadFile(ctx context.Context, projectID, id, p string) (FileContent, error) {
	var result FileContent
	clean, err := cleanPath(m.workdir(), p, false)
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
			if errors.Is(err, ErrFileNotFound) {
				return err
			}
			return fmt.Errorf("%w: read", ErrBackend)
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
	clean, err := cleanPath(m.workdir(), p, false)
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
			return fmt.Errorf("%w: write", ErrBackend)
		}
		return m.markActive(ctx, &r, false)
	})
}

func (m *Manager) RemoveFile(ctx context.Context, projectID, id, p string) error {
	clean, err := cleanPath(m.workdir(), p, false)
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
			return fmt.Errorf("%w: remove file", ErrBackend)
		}
		return m.markActive(ctx, &r, false)
	})
}

func (m *Manager) ListDir(ctx context.Context, projectID, id, p string) ([]FileEntry, error) {
	if p == "" {
		p = m.workdir()
	}
	clean, err := cleanPath(m.workdir(), p, true)
	if err != nil {
		return nil, err
	}
	var entries []FileEntry
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
			return fmt.Errorf("%w: list", ErrBackend)
		}
		if len(entries) > 500 {
			entries = entries[:500]
		}
		return m.markActive(ctx, &r, false)
	})
	return entries, err
}

// RunOnce 总文件预算 1 MiB / 20 个；keep=false 即使执行失败也释放 VM。
func (m *Manager) RunOnce(ctx context.Context, projectID string, in RunInput, actor string) (out ExecOutput, err error) {
	if len(in.Files) > 20 {
		return out, ErrFileTooLarge
	}
	bytes := 0
	for _, file := range in.Files {
		if _, e := cleanPath(m.workdir(), file.Path, false); e != nil {
			return out, e
		}
		bytes += len(file.Content)
	}
	if m.cfg.MaxFileBytes > 0 && bytes > m.cfg.MaxFileBytes {
		return out, ErrFileTooLarge
	}
	if _, err = m.validExec(in.execInput()); err != nil {
		return out, err
	}
	source := "run"
	if in.Keep {
		// 保留的调试资源不能被一次性任务 reaper 回收。
		source = "api"
	}
	r, err := m.Create(ctx, projectID, CreateInput{Image: in.Image, Source: source}, actor)
	if err != nil {
		return out, err
	}
	if !in.Keep {
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if cleanupErr := m.Delete(cleanupCtx, projectID, r.ID); cleanupErr != nil && err == nil {
				err = cleanupErr // 不把可能仍在计费的 VM 伪装成已清理
			}
		}()
	}
	for _, file := range in.Files {
		if err = m.WriteFile(ctx, projectID, r.ID, file.Path, []byte(file.Content)); err != nil {
			return out, err
		}
	}
	out, err = m.Exec(ctx, projectID, r.ID, in.execInput())
	if in.Keep {
		out.SandboxID = r.ID
	}
	return out, err
}

// EnsureForThread 将 v3 thread VM 挂到项目资源表；按 thread 幂等。
func (m *Manager) EnsureForThread(ctx context.Context, projectID, threadID string) (Sandbox, error) {
	if !m.Available() {
		return Sandbox{}, ErrUnavailable
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
			Name: "agent-" + strings.ReplaceAll(threadID, "-", ""), CloudName: ThreadCloudName(threadID),
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
	var out ExecOutput
	spec, err := m.validExec(in)
	if err != nil {
		return out, err
	}
	err = m.withLock(ctx, r.ID, func() error {
		r, err = m.Get(ctx, projectID, r.ID, false)
		if err != nil {
			return err
		}
		start := m.now()
		result, e := m.driver.Exec(ctx, m.target(r), spec)
		if e != nil {
			return fmt.Errorf("%w: exec", ErrBackend)
		}
		if e = m.markActive(ctx, &r, false); e != nil {
			return e
		}
		out.Stdout, out.StdoutTruncated = truncateUTF8(result.Stdout, m.cfg.MaxOutputBytes)
		out.Stderr, out.StderrTruncated = truncateUTF8(result.Stderr, m.cfg.MaxOutputBytes)
		out.ExitCode = result.ExitCode
		out.TimedOut = result.TimedOut
		out.DurationMs = m.now().Sub(start).Milliseconds()
		out.Status = r.Status
		return nil
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
	clean, err := cleanPath(m.workdir(), p, false)
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
			return fmt.Errorf("%w: write", ErrBackend)
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
		// 兼容 v3 创建但尚未录入 v4 元数据的 VM。
		return m.driver.Remove(ctx, ThreadCloudName(threadID))
	}
	if err != nil {
		return err
	}
	return m.Delete(ctx, projectID, r.ID)
}

// ReapOnce 每轮只处理至多 100 行，先清理遗留 VM，再清过期状态和软删行。
func (m *Manager) ReapOnce(ctx context.Context) error {
	if !m.Available() {
		return nil
	}
	now := m.now()
	failed, err := m.store.ListFailedSandboxRemovals(ctx, 100)
	if err != nil {
		return err
	}
	for _, r := range failed {
		if ctx.Err() != nil {
			return ctx.Err()
		}
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
		_ = m.Delete(ctx, r.ProjectID, r.ID)
	}
	expired, err := m.store.ListExpiredSandboxes(ctx, now, 100)
	if err != nil {
		return err
	}
	for _, r := range expired {
		r.Status = "expired"
		if err = m.store.UpdateSandbox(ctx, r); err != nil {
			return err
		}
	}
	_, err = m.store.PurgeDeletedSandboxes(ctx, now.Add(-7*24*time.Hour))
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
				_ = m.ReapOnce(ctx)
			}
		}
	}()
}

func (m *Manager) Close() error { return nil }
