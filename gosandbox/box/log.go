// 文件说明：log.go — gosandbox/box 的内部实现（logSink、newLogSink 等）
//
// 用法
//
//	同包调用：logSink → session.go；newLogSink() → logfile_test.go, session.go
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/log.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] backpressure_test.go, copytree.go, deadline.go, egress_e2e_test.go, files.go 等 26 个
//	[关键引用] logSink ← session.go；newLogSink ← logfile_test.go, session.go
//
// 主要符号
//
//	type logSink struct{…}
//	newLogSink(dir string, opt Options, warn func(string, ...any)) *logSink
//	(s *logSink) open(name string) *Log
//	(s *logSink) write(l *Log, stream Stream, data []byte)
//	(s *logSink) close(name string)
//	(s *logSink) closeAll()
//	(s *Session) Logs(procs []string, opt TailOptions) ([]Record, error)
//	(s *Session) logNames() ([]string, error)
//	... 另有 1 个顶层符号

package box

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// logSink 管理一个沙盒下所有进程的日志文件。
//
// 每个进程一个 Log（<procID>.log），轮转与格式见 logfile.go。
// 写入失败只记一次：日志坏了不该让沙盒不可用。
type logSink struct {
	dir string

	mu      sync.Mutex
	logs    map[string]*Log
	options Options
	// failed 表示已经报过写入失败，避免刷屏。
	failed bool
	warn   func(msg string, kv ...any)
}

// newLogSink 建一个日志池；dir 为空表示不落盘（只保留实时流）。
func newLogSink(dir string, opt Options, warn func(string, ...any)) *logSink {
	if warn == nil {
		warn = func(string, ...any) {}
	}
	return &logSink{dir: dir, logs: map[string]*Log{}, options: opt, warn: warn}
}

// open 打开（或复用）一个进程的日志。dir 为空时返回 nil。
func (s *logSink) open(name string) *Log {
	if s == nil || s.dir == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.logs[name]; ok {
		return l
	}
	l, err := Open(s.dir, name, s.options)
	if err != nil {
		if !s.failed {
			s.failed = true
			s.warn("cannot open sandbox log; output is not persisted", "dir", s.dir, "error", err.Error())
		}
		return nil
	}
	s.logs[name] = l
	return l
}

// write 往日志里写一块输出；失败只警告一次。
//
// 按行切开再落盘：一条记录就是一行，`TailOptions.Lines` 才是
// `docker logs --tail N` 的语义（N 行，而不是 N 次 read 的合并块）。
// 输出块里通常有好几行，整块写进去会让「最后 2 行」退回成整块。
func (s *logSink) write(l *Log, stream Stream, data []byte) {
	if l == nil {
		return
	}
	for len(data) > 0 {
		n := len(data)
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			n = i + 1
		}
		if err := l.Write(stream, data[:n]); err != nil {
			s.mu.Lock()
			first := !s.failed
			s.failed = true
			s.mu.Unlock()
			if first {
				s.warn("sandbox log write failed; output may be incomplete", "error", err.Error())
			}
			return
		}
		data = data[n:]
	}
}

// close 关闭一个进程的日志。
func (s *logSink) close(name string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	l := s.logs[name]
	delete(s.logs, name)
	s.mu.Unlock()
	if l != nil {
		_ = l.Close()
	}
}

// closeAll 关闭全部日志（Session.Close 时）。
func (s *logSink) closeAll() {
	if s == nil {
		return
	}
	s.mu.Lock()
	logs := s.logs
	s.logs = map[string]*Log{}
	s.mu.Unlock()
	for _, l := range logs {
		_ = l.Close()
	}
}

// Logs 读沙盒的日志。
//
// procs 为空表示全部进程（按名字排序）；否则只读给定的几个。
// 多个进程的记录按时间合并后返回，这样 `docker logs` 那样的「一条流」语义
// 对单进程也成立。
func (s *Session) Logs(procs []string, opt TailOptions) ([]Record, error) {
	if s.sinks == nil || s.sinks.dir == "" {
		return nil, errors.New("box: this session does not keep logs")
	}
	names := procs
	if len(names) == 0 {
		var err error
		names, err = s.logNames()
		if err != nil {
			return nil, err
		}
	}
	var all []Record
	for _, name := range names {
		recs, err := Tail(s.sinks.dir, name, opt)
		if err != nil {
			return nil, err
		}
		all = append(all, recs...)
	}
	// 多个进程时按时间合并。
	if len(names) > 1 {
		sort.SliceStable(all, func(i, j int) bool { return all[i].Time.Before(all[j].Time) })
		if opt.Lines > 0 && len(all) > opt.Lines {
			all = all[len(all)-opt.Lines:]
		}
	}
	return all, nil
}

// logNames 返回日志目录下所有进程名（去掉 .log 后缀）。
func (s *Session) logNames() ([]string, error) {
	entries, err := os.ReadDir(s.sinks.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".log" {
			continue
		}
		base := name[:len(name)-len(".log")]
		if seen[base] {
			continue
		}
		seen[base] = true
		out = append(out, base)
	}
	sort.Strings(out)
	return out, nil
}

// ProcIDs 返回当前仍在运行的进程 id（按创建顺序）。
func (s *Session) ProcIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.handles))
	for id := range s.handles {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
