// 文件说明：proc.go — internal/sandbox 中 ProcTerminal、ProcCommand 等符号的实现
//
// 用法
//
//	(h *ProcHandle) Touch()  // Touch 记录一次活动，供空闲超时与续期使用
//	(h *ProcHandle) Close() error  // Close 终止进程并从进程表移除，可重复调用
//	同包调用：Close() → manager.go, output_hub.go；procTable → manager.go；newProcTable() → manager.go
//
// 调用架构
//
//	[上游] internal/api, internal/app, internal/sandbox/sshd
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sandbox/proc.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/vm
//	[同包] errors.go, manager.go, output_hub.go, paths.go, pty.go
//	[关键引用] Close() ← manager.go, output_hub.go
//
// 主要符号
//
//	const ProcTerminal
//	const ProcCommand
//	type ProcInfo struct {…}
//	type ProcHandle struct {…}
//	(h *ProcHandle) Touch()
//	(h *ProcHandle) Close() error
//	... 另有 20 个顶层符号

package sandbox

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// 进程种类。
const (
	// ProcTerminal 是控制台 / SSH 的交互终端会话，受会话数上限约束、空闲超时断开。
	ProcTerminal = "terminal"
	// ProcCommand 是 E2B process.Start 启动的命令（前台流式或后台），按 pid / tag 寻址。
	ProcCommand = "command"
)

// ProcInfo 是进程表里一项的只读视图。
type ProcInfo struct {
	PID     int
	Tag     string
	Kind    string
	Started time.Time
}

// ProcHandle 是 Manager 登记的交互进程。调用方通过它读写，用完必须 Close。
type ProcHandle struct {
	vm.Proc
	ID        uint64
	SandboxID string
	ProjectID string
	Tag       string
	Kind      string
	Started   time.Time

	m *Manager
	// hub 是进程输出的唯一读取者，向所有订阅者广播（见 output_hub.go）。
	hub       *outputHub
	activity  atomic.Int64 // 最近一次输入输出的 unix nano
	closeOnce sync.Once
	cancel    context.CancelFunc
}

// Touch 记录一次活动，供空闲超时与续期使用。
func (h *ProcHandle) Touch() { h.activity.Store(h.m.now().UnixNano()) }

// Close 终止进程并从进程表移除，可重复调用。
func (h *ProcHandle) Close() error {
	h.closeOnce.Do(func() {
		h.cancel()
		_ = h.Proc.Close()
		h.m.procs.remove(h)
	})
	return nil
}

// procTable 是进程内的会话 / 进程登记表。单写实例，不落库。
type procTable struct {
	mu   sync.Mutex
	next uint64
	by   map[string]map[uint64]*ProcHandle // sandboxID -> id -> handle
}

func newProcTable() *procTable { return &procTable{by: map[string]map[uint64]*ProcHandle{}} }

func (t *procTable) add(h *ProcHandle) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.next++
	h.ID = t.next
	if t.by[h.SandboxID] == nil {
		t.by[h.SandboxID] = map[uint64]*ProcHandle{}
	}
	t.by[h.SandboxID][h.ID] = h
}

func (t *procTable) remove(h *ProcHandle) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if m := t.by[h.SandboxID]; m != nil {
		delete(m, h.ID)
		if len(m) == 0 {
			delete(t.by, h.SandboxID)
		}
	}
}

func (t *procTable) countKind(sandboxID, kind string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, h := range t.by[sandboxID] {
		if h.Kind == kind {
			n++
		}
	}
	return n
}

func (t *procTable) list(sandboxID string) []*ProcHandle {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]*ProcHandle, 0, len(t.by[sandboxID]))
	for _, h := range t.by[sandboxID] {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// all 返回全部句柄的快照（reaper 与关停使用）。
func (t *procTable) all() []*ProcHandle {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []*ProcHandle
	for _, m := range t.by {
		for _, h := range m {
			out = append(out, h)
		}
	}
	return out
}

// closeSandbox 关闭某个沙盒的全部会话与进程，返回关闭个数。
func (m *Manager) closeSandboxProcs(sandboxID string) int {
	hs := m.procs.list(sandboxID)
	for _, h := range hs {
		_ = h.Close()
	}
	return len(hs)
}

// SetEnvs 记录沙盒创建时指定的 envVars。只在内存里随沙盒保存，重启丢失：
// 用户可能在里面放密钥，不落系统库。process.Start 时与请求自带的 envs 合并（请求优先）。
func (m *Manager) SetEnvs(sandboxID string, env map[string]string) {
	m.envMu.Lock()
	defer m.envMu.Unlock()
	if len(env) == 0 {
		delete(m.envs, sandboxID)
		return
	}
	cp := make(map[string]string, len(env))
	for k, v := range env {
		cp[k] = v
	}
	m.envs[sandboxID] = cp
}

// Envs 返回沙盒的默认环境变量副本。
func (m *Manager) Envs(sandboxID string) map[string]string {
	m.envMu.Lock()
	defer m.envMu.Unlock()
	out := make(map[string]string, len(m.envs[sandboxID]))
	for k, v := range m.envs[sandboxID] {
		out[k] = v
	}
	return out
}

// ClearEnvs 删除沙盒的默认环境变量（沙盒删除时调用）。
func (m *Manager) ClearEnvs(sandboxID string) { m.SetEnvs(sandboxID, nil) }

// StartSpec 描述要启动的进程。Shell / Cmd 都为空表示交互 shell（仅 PTY 终端用）。
type StartSpec struct {
	Kind  string // ProcTerminal | ProcCommand
	Tag   string
	Shell string
	Cmd   string
	Args  []string
	Cwd   string
	Env   map[string]string
	PTY   *vm.PTYSize
}

// StartProc 在沙盒里启动一个保持句柄的进程（终端 / E2B process.Start）。
//
// 只在校验与 Ensure 时持有沙盒锁，拿到进程后立即释放；否则长会话会占住锁，
// 让同一沙盒的文件操作 5 秒后全部 ErrBusy。
func (m *Manager) StartProc(ctx context.Context, projectID, id string, spec StartSpec) (*ProcHandle, error) {
	if !m.Available() {
		return nil, vm.ErrUnavailable
	}
	inter, ok := m.driver.(vm.Interactive)
	if !ok || !m.cfg.Terminal.Enabled {
		return nil, vm.ErrNotSupported
	}
	kind := spec.Kind
	if kind == "" {
		kind = ProcCommand
	}
	if len(spec.Args) > 128 || len(spec.Shell) > 65536 || len(spec.Cmd) > 1024 {
		return nil, fmt.Errorf("%w: command too large", ErrInvalidSpec)
	}
	for _, a := range spec.Args {
		if len(a) > 4096 || strings.ContainsRune(a, 0) {
			return nil, fmt.Errorf("%w: argument", ErrInvalidSpec)
		}
	}
	if err := validateEnv(spec.Env); err != nil {
		return nil, err
	}
	// 沙盒级默认环境变量打底，请求自带的覆盖它们。
	if base := m.Envs(id); len(base) > 0 {
		for k, v := range spec.Env {
			base[k] = v
		}
		spec.Env = base
	}
	cwd := spec.Cwd
	if cwd == "" {
		cwd = m.workdir()
	}
	cwd, err := cleanPath(m.workdir(), cwd, true)
	if err != nil {
		return nil, err
	}
	var h *ProcHandle
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
		if kind == ProcTerminal {
			if m.procs.countKind(id, ProcTerminal) >= m.cfg.Terminal.EffectiveSessionsPerSandbox() {
				return ErrLimitExceeded
			}
		}
		if spec.Tag != "" {
			for _, o := range m.procs.list(id) {
				if o.Tag == spec.Tag {
					return fmt.Errorf("%w: tag already in use", ErrInvalidSpec)
				}
			}
		}
		// 进程自己的 ctx：不继承请求 ctx（请求结束进程不应被杀），由 Close / 沙盒停止终止。
		pctx, cancel := context.WithCancel(context.Background())
		p, serr := inter.Start(pctx, m.target(r), vm.StartSpec{Cmd: spec.Cmd, Args: spec.Args, Shell: spec.Shell,
			Env: spec.Env, Cwd: cwd, PTY: spec.PTY})
		if serr != nil {
			cancel()
			m.logger.Warn("sandbox driver start failed", zap.String("sandbox_id", id), zap.Error(serr))
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%w: start", vm.ErrBackend)
		}
		h = &ProcHandle{Proc: p, SandboxID: id, ProjectID: projectID, Tag: spec.Tag, Kind: kind,
			Started: m.now(), m: m, cancel: cancel, hub: newOutputHub(p.Output())}
		h.Touch()
		m.procs.add(h)
		if err := m.markActive(ctx, &r, false); err != nil {
			_ = h.Close()
			h = nil
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 进程自然退出时自动从表中移除。
	go func(h *ProcHandle) {
		<-h.Proc.Done()
		_ = h.Close()
	}(h)
	m.logger.Info("sandbox process started", zap.String("sandbox_id", id), zap.String("kind", kind), zap.Bool("pty", spec.PTY != nil))
	return h, nil
}

// Procs 返回沙盒当前的进程（E2B process.List）。
func (m *Manager) Procs(sandboxID string) []ProcInfo {
	hs := m.procs.list(sandboxID)
	out := make([]ProcInfo, 0, len(hs))
	for _, h := range hs {
		if h.Kind != ProcCommand {
			continue
		}
		out = append(out, ProcInfo{PID: h.PID(), Tag: h.Tag, Kind: h.Kind, Started: h.Started})
	}
	return out
}

// FindProc 按 pid 或 tag 找沙盒内的命令进程（E2B process.Connect / SendInput / SendSignal）。
func (m *Manager) FindProc(projectID, sandboxID string, pid int, tag string) (*ProcHandle, bool) {
	for _, h := range m.procs.list(sandboxID) {
		if h.ProjectID != projectID || h.Kind != ProcCommand {
			continue
		}
		if (tag != "" && h.Tag == tag) || (tag == "" && pid != 0 && h.PID() == pid) {
			return h, true
		}
	}
	return nil, false
}

// KillProc 终止进程：SIGTERM 或 SIGKILL。
func (m *Manager) KillProc(h *ProcHandle, sig syscall.Signal) error { return h.Signal(sig) }

// Subscribe 订阅进程输出。fromStart 为真时先收到缓冲里的历史输出（Connect 到已运行的进程），
// 否则只收订阅之后的新数据。进程输出只有 hub 一个读取者，所以多个订阅者互不抢数据。
// 用完必须 Close。
func (h *ProcHandle) Subscribe(fromStart bool) *Subscription { return h.hub.Subscribe(fromStart) }

// CopyOutput 把进程输出（含缓冲里的历史）持续复制到 w，直到进程输出结束；每次读到数据刷新活动时间。
func (h *ProcHandle) CopyOutput(w io.Writer) error {
	sub := h.Subscribe(true)
	defer sub.Close()
	buf := make([]byte, 32*1024)
	for {
		n, err := sub.Read(buf)
		if n > 0 {
			h.Touch()
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// reapProcs 关闭空闲超时的终端会话，并为仍有活动的进程续期沙盒。由 reaper 每轮调用。
func (m *Manager) reapProcs(ctx context.Context) {
	idle := m.cfg.Terminal.EffectiveIdleTimeout()
	now := m.now()
	touched := map[string]bool{}
	for _, h := range m.procs.all() {
		last := time.Unix(0, h.activity.Load())
		if h.Kind == ProcTerminal && now.Sub(last) > idle {
			m.logger.Info("sandbox terminal idle, closing", zap.String("sandbox_id", h.SandboxID))
			_ = h.Close()
			continue
		}
		if touched[h.SandboxID] {
			continue
		}
		touched[h.SandboxID] = true
		if r, err := m.Get(ctx, h.ProjectID, h.SandboxID, false); err == nil {
			_ = m.markActive(ctx, &r, false)
		}
	}
}

// ProcCount 返回沙盒当前的进程 / 会话总数（测试与诊断用）。
func (m *Manager) ProcCount(sandboxID string) int { return len(m.procs.list(sandboxID)) }
