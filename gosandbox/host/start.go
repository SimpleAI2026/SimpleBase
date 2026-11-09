// 文件说明：start.go — gosandbox/host 中 PTYSize、StartSpec 等符号的实现
//
// 用法
//
//	WaitTimeout(p Proc, d time.Duration) bool  // WaitTimeout 等待进程结束或超时，供测试与关停使用
//	type PTYSize（导出类型，同包与上层均可使用）
//	type StartSpec（导出类型，同包与上层均可使用）
//	同包调用：process → cgroup_linux.go, policy.go 等；PTYSize → pty_darwin.go, pty_linux.go 等；Close() → pty_darwin.go, pty_linux.go 等
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/start.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 13 个
//	[关键引用] process ← cgroup_linux.go, policy.go, seatbelt.go
//
// 主要符号
//
//	type PTYSize struct {…}
//	type StartSpec struct {…}
//	type ExitInfo struct {…}
//	type Proc interface {…}
//	WaitTimeout(p Proc, d time.Duration) bool
//	... 另有 10 个顶层符号

package host

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// PTYSize 是伪终端的窗口大小。
type PTYSize struct{ Cols, Rows uint16 }

// StartSpec 描述一个需要保持句柄的进程（交互终端、后台进程、流式输出）。
type StartSpec struct {
	Command Command
	// PTY 为 nil 时用管道（stdout 与 stderr 合并为一路）；非 nil 时分配伪终端。
	PTY *PTYSize
}

// ExitInfo 是进程结束信息。
type ExitInfo struct {
	// ExitCode 是退出码；被信号杀死时为 128+信号号。
	ExitCode int
	// Signaled 为真表示进程被信号终止。
	Signaled bool
}

// Proc 是一个正在运行的沙盒进程。
//
// Output 是单一合并流：PTY 模式下是终端输出，管道模式下是 stdout+stderr。
// 调用方必须持续读取 Output；读得慢会让子进程在写入时阻塞（背压），而不是在内存里堆积。
type Proc interface {
	PID() int
	// Stdin 写入即键盘输入。管道模式下 Close 发送 EOF；PTY 模式下 Close 无效，需发送 Ctrl+D。
	Stdin() io.WriteCloser
	Output() io.Reader
	// Resize 仅 PTY 模式有效，管道模式返回 ErrNotPTY。
	Resize(PTYSize) error
	// Signal 只支持 SIGTERM 与 SIGKILL，发给整个进程组。
	Signal(sig syscall.Signal) error
	// Wait 阻塞到进程结束并回收资源，可多次调用，结果相同。
	Wait() ExitInfo
	// Done 在进程结束后关闭。
	Done() <-chan struct{}
	// Close 终止进程（若仍在运行）并释放 PTY / 管道。调用方用完必须调用；可重复调用。
	// 进程自然结束后不会自动关闭输出端，这样读端能把残留输出读完再收到 EOF。
	Close() error
}

// process 是 Proc 的通用实现，bwrap 与 Seatbelt 共用。
type process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	output io.Reader
	// ptmx 非 nil 表示 PTY 模式。
	ptmx *os.File
	// closers 在进程结束后立即执行（cgroup 清理等，不影响残留输出的读取）。
	closers []func()
	// releasers 由 Close 执行（PTY 主端、管道读端）。
	releasers []func()
	closeOnce sync.Once

	done chan struct{}
	exit ExitInfo
}

func newProcess(cmd *exec.Cmd, stdin io.WriteCloser, output io.Reader, ptmx *os.File, releasers []func(), closers ...func()) *process {
	p := &process{cmd: cmd, stdin: stdin, output: output, ptmx: ptmx, releasers: releasers, closers: closers, done: make(chan struct{})}
	go p.reap()
	return p
}

func (p *process) reap() {
	werr := p.cmd.Wait()
	info := exitInfo(p.cmd.ProcessState, werr)
	p.exit = info
	// 进程结束后，孙进程可能仍持有 PTY；先杀整个进程组再关闭，避免读端永久阻塞。
	killGroup(p.cmd.Process.Pid, syscall.SIGKILL)
	for _, c := range p.closers {
		c()
	}
	close(p.done)
}

func (p *process) PID() int              { return p.cmd.Process.Pid }
func (p *process) Stdin() io.WriteCloser { return p.stdin }
func (p *process) Output() io.Reader     { return p.output }
func (p *process) Done() <-chan struct{} { return p.done }
func (p *process) Wait() ExitInfo        { <-p.done; return p.exit }

// Close 实现 Proc。
func (p *process) Close() error {
	p.closeOnce.Do(func() {
		select {
		case <-p.done:
		default:
			killGroup(p.cmd.Process.Pid, syscall.SIGKILL)
		}
		<-p.done
		for _, r := range p.releasers {
			r()
		}
	})
	return nil
}

func (p *process) Signal(sig syscall.Signal) error {
	if sig != syscall.SIGTERM && sig != syscall.SIGKILL {
		return ErrUnsupportedSignal
	}
	select {
	case <-p.done:
		return nil
	default:
	}
	return killGroup(p.cmd.Process.Pid, sig)
}

// WaitTimeout 等待进程结束或超时，供测试与关停使用。
func WaitTimeout(p Proc, d time.Duration) bool {
	select {
	case <-p.Done():
		return true
	case <-time.After(d):
		return false
	}
}
