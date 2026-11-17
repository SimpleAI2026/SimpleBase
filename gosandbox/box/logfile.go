// 文件说明：logfile.go — 进程输出落盘与回放（Docker local 驱动的记录格式）
//
// 记录格式：`[len u32][stream u8][time ns u64][data][len u32]`（大端）。
// len = 9 + len(data)，尾部再写一次 len，让 `--tail` 能从文件末尾往后走，
// 不必从头扫。
//
// 为什么用二进制而不是 JSON 行：输出是任意字节（含 \n、NUL），
// 加长度前缀才能既二进制安全又支持从尾部倒读。这与 kiri 的 logfile.rs
// 和 Docker 的 local 驱动一致。
//
// 轮转：单文件 10 MiB × 3 份。<容器>.log / .1 / .2。
//
// 用法
//
//	w, err := Open(dir, "p1", Options{})
//	w.Write(StreamStdout, []byte("hello\n"))
//	w.Close()
//	r, err := Read(dir, "p1", TailOptions{Lines: 100, Follow: false})
//
// 调用架构
//
//	[上游] gosandbox/vm/box（把 agent 的 Output 落盘）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/logfile.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] session.go, handle.go
//
// 主要符号
//
//	type Options struct {…}
//	type Record struct {…}
//	type Log struct {…}
//	Open(dir, name string, opt Options) (*Log, error)
//	(l *Log) Write(stream Stream, data []byte) error
//	(l *Log) Close() error
//	Tail(dir, name string, opt TailOptions) ([]Record, error)

package box

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 日志参数。
const (
	// DefaultMaxSize 是单文件上限：10 MiB（kiri 的默认值）。
	DefaultMaxSize = 10 << 20
	// DefaultMaxFiles 是保留的份数（含当前文件）。
	DefaultMaxFiles = 3
	// MaxLine 是单条记录的数据上限：更长的输出被切开，与 Docker 一致。
	MaxLine = 16 << 10
	// maxRecord 是任何记录的最大长度；超过说明文件被写坏了。
	maxRecord = MaxLine + 17
	// recordOverhead 是每条记录的固定开销：两个 len(4+4) + stream(1) + time(8)。
	recordOverhead = 4 + 1 + 8 + 4
	// flushBytes 是缓冲阈值。
	flushBytes = 32 << 10
	// flushAfter 是最长缓冲时间。
	flushAfter = time.Second
)

// Stream 区分 stdout 与 stderr（与协议一致）。
type Stream = uint8

const (
	// StreamStdout 是标准输出。
	StreamStdout Stream = 1
	// StreamStderr 是标准错误。
	StreamStderr Stream = 2
)

// ErrLogCorrupt 表示日志文件被写坏（记录长度自相矛盾）。
var ErrLogCorrupt = errors.New("box: log file is corrupt")

// Options 是写入参数。
type Options struct {
	// MaxSize 是单文件上限；0 取 DefaultMaxSize。
	MaxSize int64
	// MaxFiles 是保留份数；0 取 DefaultMaxFiles。
	MaxFiles int
}

// effective 补齐默认值。
func (o Options) effective() Options {
	if o.MaxSize <= 0 {
		o.MaxSize = DefaultMaxSize
	}
	if o.MaxFiles <= 0 {
		o.MaxFiles = DefaultMaxFiles
	}
	return o
}

// Record 是一条输出记录。
type Record struct {
	Stream Stream
	// Time 是到达时间。
	Time time.Time
	Data []byte
}

// Log 是某个进程的日志写入器。
type Log struct {
	path string
	opt  Options

	mu   sync.Mutex
	f    *os.File
	size int64
	// buf 是待写缓冲：攒到阈值或超时才落盘。
	buf       []byte
	lastFlush time.Time
	closed    bool
}

// Open 打开（或创建）一个进程的日志。
func Open(dir, name string, opt Options) (*Log, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	opt = opt.effective()
	path := filepath.Join(dir, name+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &Log{path: path, opt: opt, f: f, size: st.Size(), lastFlush: time.Now()}, nil
}

// Path 返回当前文件路径。
func (l *Log) Path() string { return l.path }

// Write 写一条记录。超过 MaxLine 的数据被切成多条。
func (l *Log) Write(stream Stream, data []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return errors.New("box: log is closed")
	}
	now := time.Now().UnixNano()
	for len(data) > 0 {
		n := len(data)
		if n > MaxLine {
			n = MaxLine
		}
		l.appendRecord(stream, now, data[:n])
		data = data[n:]
	}
	// 缓冲够大或够久就落盘。
	if len(l.buf) >= flushBytes || time.Since(l.lastFlush) >= flushAfter {
		return l.flushLocked()
	}
	return nil
}

// appendRecord 把一条记录追加进缓冲。
func (l *Log) appendRecord(stream Stream, ts int64, data []byte) {
	total := uint32(recordOverhead + len(data))
	var head [13]byte
	binary.BigEndian.PutUint32(head[0:4], total)
	head[4] = stream
	binary.BigEndian.PutUint64(head[5:13], uint64(ts))
	l.buf = append(l.buf, head[:]...)
	l.buf = append(l.buf, data...)
	var tail [4]byte
	binary.BigEndian.PutUint32(tail[:], total)
	l.buf = append(l.buf, tail[:]...)
}

// flushLocked 把缓冲写进文件，必要时轮转。调用方持锁。
func (l *Log) flushLocked() error {
	if len(l.buf) == 0 {
		l.lastFlush = time.Now()
		return nil
	}
	// 写入会超上限：先轮转，再写。
	if l.size+int64(len(l.buf)) > l.opt.MaxSize {
		if err := l.rotateLocked(); err != nil {
			return err
		}
	}
	n, err := l.f.Write(l.buf)
	l.size += int64(n)
	l.buf = l.buf[:0]
	l.lastFlush = time.Now()
	return err
}

// rotateLocked 轮转：.log → .1 → .2，最老的一份删掉。
func (l *Log) rotateLocked() error {
	if err := l.f.Close(); err != nil {
		return err
	}
	// 从最老的开始往后挪。
	for i := l.opt.MaxFiles - 1; i >= 1; i-- {
		from := l.path
		if i > 1 {
			from = fmt.Sprintf("%s.%d", l.path, i-1)
		}
		to := fmt.Sprintf("%s.%d", l.path, i)
		if _, err := os.Stat(from); errors.Is(err, os.ErrNotExist) {
			continue
		}
		_ = os.Remove(to)
		if err := os.Rename(from, to); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	l.f = f
	l.size = 0
	return nil
}

// Flush 立刻落盘。
func (l *Log) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.flushLocked()
}

// Close 落盘并关闭。
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if err := l.flushLocked(); err != nil {
		l.f.Close()
		return err
	}
	return l.f.Close()
}

// TailOptions 是回放参数。
type TailOptions struct {
	// Lines 是最后的多少行（一条记录一行）；0 表示全部。
	//
	// 会话落盘时按行切记录（见 logSink.write），所以这里与
	// `docker logs --tail N` 同语义：N 是行数。
	Lines int
	// Since 是起始时间；零值表示不限。
	Since time.Time
	// Streams 是只保留的流；空表示全部。
	Streams []Stream
}

// Tail 读回一个进程的日志（按时间顺序）。
//
// 实现从最新的文件读到最老的，凑够 Lines 为止；小文件直接全读。
// 不做真正的「从尾部倒走」优化：轮转文件总共 30 MiB，全读也就几十毫秒，
// 而倒读的边界情况（记录被轮转切开）很难测透。
func Tail(dir, name string, opt TailOptions) ([]Record, error) {
	files := logFiles(dir, name)
	var all []Record
	// 从最老的一份开始读，保证顺序。
	for i := len(files) - 1; i >= 0; i-- {
		recs, err := readLogFile(files[i])
		if err != nil {
			return nil, err
		}
		all = append(all, recs...)
	}
	// 时间过滤。
	if !opt.Since.IsZero() {
		cut := opt.Since.UnixNano()
		kept := all[:0]
		for _, r := range all {
			if r.Time.UnixNano() >= cut {
				kept = append(kept, r)
			}
		}
		all = kept
	}
	// 流过滤。
	if len(opt.Streams) > 0 {
		allow := map[Stream]bool{}
		for _, s := range opt.Streams {
			allow[s] = true
		}
		kept := all[:0]
		for _, r := range all {
			if allow[r.Stream] {
				kept = append(kept, r)
			}
		}
		all = kept
	}
	// 只留最后 N 条。
	if opt.Lines > 0 && len(all) > opt.Lines {
		all = all[len(all)-opt.Lines:]
	}
	return all, nil
}

// logFiles 返回一个进程的日志文件，按从新到旧排列。
func logFiles(dir, name string) []string {
	base := filepath.Join(dir, name+".log")
	out := []string{}
	if _, err := os.Stat(base); err == nil {
		out = append(out, base)
	}
	// .1 / .2 …：按序号升序，序号越大越旧。
	var rotated []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, name+".log.") {
			continue
		}
		if _, err := strconv.Atoi(strings.TrimPrefix(n, name+".log.")); err != nil {
			continue
		}
		rotated = append(rotated, filepath.Join(dir, n))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(rotated)))
	return append(out, rotated...)
}

// readLogFile 读一个日志文件的全部记录。
func readLogFile(path string) ([]Record, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Record
	for off := 0; off < len(body); {
		if off+recordOverhead > len(body) {
			// 尾部有半条记录（进程被杀时可能发生）：忽略而不是报错，
			// 否则一次崩溃会让整份日志读不出来。
			break
		}
		total := binary.BigEndian.Uint32(body[off : off+4])
		if total < recordOverhead || total > maxRecord {
			return nil, fmt.Errorf("%w: record length %d at offset %d", ErrLogCorrupt, total, off)
		}
		end := off + int(total)
		if end > len(body) {
			break
		}
		stream := body[off+4]
		ts := int64(binary.BigEndian.Uint64(body[off+5 : off+13]))
		// 数据段是 [off+13, off+total-4)：首尾各一个长度、一个流、一个时间戳。
		// total 是整条记录的长度（含头尾），所以数据段可能为空（空输出）。
		dataStart, dataEnd := off+13, off+int(total)-4
		if dataEnd < dataStart {
			dataEnd = dataStart
		}
		data := body[dataStart:dataEnd]
		out = append(out, Record{Stream: stream, Time: time.Unix(0, ts), Data: data})
		off = end
	}
	return out, nil
}

// Reader 返回一个把所有记录按时间顺序展开的 io.Reader（stdout 与 stderr 合并）。
func Reader(recs []Record) io.Reader {
	var buf []byte
	for _, r := range recs {
		buf = append(buf, r.Data...)
	}
	return strings.NewReader(string(buf))
}
