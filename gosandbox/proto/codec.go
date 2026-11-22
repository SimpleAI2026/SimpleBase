package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// 编解码错误。
var (
	// ErrFrameTooLarge 表示帧长超过 MaxFrame。
	ErrFrameTooLarge = errors.New("proto: frame too large")
	// ErrTruncated 表示 payload 比声明的短。
	ErrTruncated = errors.New("proto: truncated payload")
	// ErrUnknownKind 表示遇到未知的帧类型（版本不一致）。
	ErrUnknownKind = errors.New("proto: unknown frame kind")
	// ErrFieldTooLong 表示某个字符串字段超过上限。
	ErrFieldTooLong = errors.New("proto: field too long")
)

// FileKind 是目录项的类型。
type FileKind uint8

const (
	FileRegular FileKind = iota + 1
	FileDir
	FileSymlink
	FileOther
)

// Entry 是一个目录项。
type Entry struct {
	Name   string
	Kind   FileKind
	Size   int64
	Mode   uint32
	ModNS  int64
	Target string // 符号链接的目标；其它类型为空
}

// Stats 是沙盒的用量快照。
type Stats struct {
	MemoryBytes    int64
	MemoryPeak     int64
	CPUUsec        int64
	Pids           int64
	OOMKills       int64
	DiskReadBytes  int64
	DiskWriteBytes int64
	// DiskUsedBytes 是沙盒可写部分实际占用的字节数（upper + ws）。
	//
	// 由宿主侧驱动计算并填进 Stats：agent 在沙盒里看到的是 overlay 的
	// 合并视图，分不出「镜像本来就有」与「用户写入」，算不准。
	// 这与 DiskWriteBytes（io 带宽）不是一回事。
	DiskUsedBytes int64
}

// Msg 是一帧的内容。只有 Kind 对应的字段有效；其余为零值。
//
// 每帧只编码它用到的字段（见 Encode 的分支），所以 Output 这种高频帧
// 不会被无关字段撑大。
type Msg struct {
	Kind Kind

	// 进程 id 与文件请求 id 共用；文件请求用 Seq 关联回复。
	ID  string
	Seq uint32

	// Hello
	Version uint16
	Agent   string
	BootNS  int64

	// Ping / Pong
	Nonce uint64

	// Error
	Err string

	// Start
	Argv    []string
	Env     []string
	Cwd     string
	User    string // "uid:gid"；空表示继承 agent
	TTY     bool
	Rows    uint16
	Cols    uint16
	Stdin   bool
	Rlimits []Rlimit

	// Started
	PID int32

	// Output / Input / RelayData
	Stream Stream
	Data   []byte

	// Exited
	Reason ExitReason
	Code   int32
	Sig    int32

	// InputConsumed
	Bytes uint32

	// Signal
	Signal Signal

	// Resize
	// Rows / Cols 复用 Start 的字段

	// 文件
	Path    string
	To      string
	Offset  uint64
	Mode    uint32
	Size    int64
	ModNS   int64
	FileKnd FileKind
	Link    string
	Entries []Entry

	// 观测
	Stats Stats
	Lines []string
	Ports []uint16

	// Relay
	ConnID uint32
	Port   uint16

	// Watch
	Recursive bool

	// Flags 是位标记：半关闭方向等。
	Flags uint8
}

// writer 是 payload 的追加写入器。
type writer struct{ b []byte }

func (w *writer) u8(v uint8)   { w.b = append(w.b, v) }
func (w *writer) u16(v uint16) { w.b = binary.LittleEndian.AppendUint16(w.b, v) }
func (w *writer) u32(v uint32) { w.b = binary.LittleEndian.AppendUint32(w.b, v) }
func (w *writer) u64(v uint64) { w.b = binary.LittleEndian.AppendUint64(w.b, v) }
func (w *writer) i32(v int32)  { w.u32(uint32(v)) }
func (w *writer) i64(v int64)  { w.u64(uint64(v)) }
func (w *writer) raw(p []byte) { w.b = append(w.b, p...) }

// str 写 u32 长度前缀的字符串。超长返回错误，不静默截断。
func (w *writer) str(limit int, s string) error {
	if len(s) > limit {
		return fmt.Errorf("%w: %d > %d", ErrFieldTooLong, len(s), limit)
	}
	w.u32(uint32(len(s)))
	w.b = append(w.b, s...)
	return nil
}

func (w *writer) bytes(limit int, p []byte) error {
	if len(p) > limit {
		return fmt.Errorf("%w: %d > %d", ErrFieldTooLong, len(p), limit)
	}
	w.u32(uint32(len(p)))
	w.b = append(w.b, p...)
	return nil
}

func (w *writer) strs(limit int, list []string) error {
	if len(list) > limit {
		return fmt.Errorf("%w: %d items > %d", ErrFieldTooLong, len(list), limit)
	}
	w.u32(uint32(len(list)))
	for _, s := range list {
		if err := w.str(MaxArgLen, s); err != nil {
			return err
		}
	}
	return nil
}

func (w *writer) rlimits(rs []Rlimit) error {
	w.u32(uint32(len(rs)))
	for _, r := range rs {
		if err := w.str(32, r.Name); err != nil {
			return err
		}
		w.u64(r.Cur)
		w.u64(r.Max)
	}
	return nil
}

func (w *writer) entries(es []Entry) error {
	w.u32(uint32(len(es)))
	for _, e := range es {
		if err := w.str(MaxPathLen, e.Name); err != nil {
			return err
		}
		w.u8(uint8(e.Kind))
		w.i64(e.Size)
		w.u32(e.Mode)
		w.i64(e.ModNS)
		if err := w.str(MaxPathLen, e.Target); err != nil {
			return err
		}
	}
	return nil
}

// reader 是 payload 的读取器，越界一律返回 ErrTruncated。
type reader struct {
	b   []byte
	off int
}

func (r *reader) take(n int) ([]byte, error) {
	if n < 0 || r.off+n > len(r.b) {
		return nil, ErrTruncated
	}
	p := r.b[r.off : r.off+n]
	r.off += n
	return p, nil
}

func (r *reader) u8() (uint8, error) {
	p, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return p[0], nil
}

func (r *reader) u16() (uint16, error) {
	p, err := r.take(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(p), nil
}

func (r *reader) u32() (uint32, error) {
	p, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(p), nil
}

func (r *reader) u64() (uint64, error) {
	p, err := r.take(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(p), nil
}

func (r *reader) i32() (int32, error) { v, err := r.u32(); return int32(v), err }
func (r *reader) i64() (int64, error) { v, err := r.u64(); return int64(v), err }

// str 读 u32 长度前缀的字符串；长度超过 limit 直接拒绝，不按声明分配。
func (r *reader) str(limit int) (string, error) {
	n, err := r.u32()
	if err != nil {
		return "", err
	}
	if int(n) > limit {
		return "", fmt.Errorf("%w: %d > %d", ErrFieldTooLong, n, limit)
	}
	p, err := r.take(int(n))
	if err != nil {
		return "", err
	}
	return string(p), nil
}

func (r *reader) bytes(limit int) ([]byte, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if int(n) > limit {
		return nil, fmt.Errorf("%w: %d > %d", ErrFieldTooLong, n, limit)
	}
	p, err := r.take(int(n))
	if err != nil {
		return nil, err
	}
	// 复制一份：payload 缓冲会被复用。
	out := make([]byte, len(p))
	copy(out, p)
	return out, nil
}

func (r *reader) strs(limit int) ([]string, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if int(n) > limit {
		return nil, fmt.Errorf("%w: %d items > %d", ErrFieldTooLong, n, limit)
	}
	out := make([]string, 0, min(int(n), 64))
	for i := 0; i < int(n); i++ {
		s, err := r.str(MaxArgLen)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *reader) rlimits() ([]Rlimit, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if n > 64 {
		return nil, fmt.Errorf("%w: %d rlimits", ErrFieldTooLong, n)
	}
	out := make([]Rlimit, 0, n)
	for i := 0; i < int(n); i++ {
		name, err := r.str(32)
		if err != nil {
			return nil, err
		}
		cur, err := r.u64()
		if err != nil {
			return nil, err
		}
		max, err := r.u64()
		if err != nil {
			return nil, err
		}
		out = append(out, Rlimit{Name: name, Cur: cur, Max: max})
	}
	return out, nil
}

func (r *reader) entries() ([]Entry, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if n > 1<<20 {
		return nil, fmt.Errorf("%w: %d entries", ErrFieldTooLong, n)
	}
	out := make([]Entry, 0, min(int(n), 256))
	for i := 0; i < int(n); i++ {
		name, err := r.str(MaxPathLen)
		if err != nil {
			return nil, err
		}
		k, err := r.u8()
		if err != nil {
			return nil, err
		}
		size, err := r.i64()
		if err != nil {
			return nil, err
		}
		mode, err := r.u32()
		if err != nil {
			return nil, err
		}
		mod, err := r.i64()
		if err != nil {
			return nil, err
		}
		target, err := r.str(MaxPathLen)
		if err != nil {
			return nil, err
		}
		out = append(out, Entry{Name: name, Kind: FileKind(k), Size: size, Mode: mode, ModNS: mod, Target: target})
	}
	return out, nil
}

// Frame 给 payload 补上 len 与 kind。
func Frame(kind Kind, payload []byte) []byte {
	out := make([]byte, Header+len(payload))
	binary.LittleEndian.PutUint32(out, uint32(len(payload)+2))
	binary.LittleEndian.PutUint16(out[4:], uint16(kind))
	copy(out[Header:], payload)
	return out
}

// ReadFrame 从流里读一帧。payload 复用调用方的缓冲，避免每帧分配。
// kind 与 payload 都是输出参数；返回 io.EOF 表示对端正常关闭。
func ReadFrame(r io.Reader, kind *Kind, payload *[]byte) error {
	var head [4]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return err
	}
	n := binary.LittleEndian.Uint32(head[:])
	if n < 2 || n > MaxFrame {
		return fmt.Errorf("%w: %d", ErrFrameTooLarge, n)
	}
	buf := (*payload)[:0]
	if cap(buf) < int(n) {
		buf = make([]byte, n)
	} else {
		buf = buf[:n]
	}
	if _, err := io.ReadFull(r, buf); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return ErrTruncated
		}
		return err
	}
	k := binary.LittleEndian.Uint16(buf[:2])
	*kind = Kind(k)
	*payload = buf[:n]
	// 去掉 kind，payload 只留内容。
	copy(buf, buf[2:])
	*payload = buf[:n-2]
	return nil
}

// Encode 生成 payload（不含 len 与 kind）。字段按 Kind 分支编码，
// 只有该帧用到的字段才进 payload：Output 这种高频帧因此不会被无关字段撑大。
func (m *Msg) Encode() ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	switch m.Kind {
	case KindHello:
		w.u16(m.Version)
		if err := w.str(64, m.Agent); err != nil {
			return nil, err
		}
		w.i64(m.BootNS)
	case KindPing, KindPong:
		w.u64(m.Nonce)
	case KindShutdown:
		// 无字段。
	case KindAck, KindFsWriteAck:
		// 回复必须带回请求的 Seq：宿主按它找回等待的调用方。
		w.u32(m.Seq)
	case KindInputConsumed:
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		w.u32(m.Bytes)
	case KindError:
		// Seq 为 0 表示这是不针对某个请求的错误（协议错误、未知帧等）。
		w.u32(m.Seq)
		if err := w.str(MaxArgLen, m.Err); err != nil {
			return nil, err
		}
	case KindStart:
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		if err := w.strs(MaxArgs, m.Argv); err != nil {
			return nil, err
		}
		if err := w.strs(MaxEnv, m.Env); err != nil {
			return nil, err
		}
		if err := w.str(MaxPathLen, m.Cwd); err != nil {
			return nil, err
		}
		if err := w.str(64, m.User); err != nil {
			return nil, err
		}
		var flags uint8
		if m.TTY {
			flags |= 1
		}
		if m.Stdin {
			flags |= 2
		}
		w.u8(flags)
		w.u16(m.Rows)
		w.u16(m.Cols)
		if err := w.rlimits(m.Rlimits); err != nil {
			return nil, err
		}
	case KindStarted:
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		w.i32(m.PID)
	case KindOutput:
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		w.u8(uint8(m.Stream))
		if err := w.bytes(MaxOutputChunk, m.Data); err != nil {
			return nil, err
		}
	case KindExited:
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		w.u8(uint8(m.Reason))
		w.i32(m.Code)
		w.i32(m.Sig)
		// Err 是可选说明（例如「请求的 uid 在沙盒里无法映射」）。
		// 空串编码为 0 长度，老版本读到 0 长度按空处理，帧仍然合法。
		if err := w.str(MaxPathLen, m.Err); err != nil {
			return nil, err
		}
	case KindInput:
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		if err := w.bytes(MaxOutputChunk, m.Data); err != nil {
			return nil, err
		}
	case KindCloseStdin, KindFsStat, KindFsList, KindFsRead, KindFsRemove, KindFsMkdir, KindStatsQuery, KindTopQuery, KindListenQuery:
		// Seq 是宿主的请求序号：文件与端口查询的回复都要原样带回它。
		w.u32(m.Seq)
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		if m.Kind == KindFsStat || m.Kind == KindFsList || m.Kind == KindFsRemove || m.Kind == KindFsMkdir {
			if err := w.str(MaxPathLen, m.Path); err != nil {
				return nil, err
			}
		}
		if m.Kind == KindFsRead {
			if err := w.str(MaxPathLen, m.Path); err != nil {
				return nil, err
			}
			w.u64(m.Offset)
			w.u32(uint32(m.Size))
		}
	case KindFsWrite:
		w.u32(m.Seq)
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		if err := w.str(MaxPathLen, m.Path); err != nil {
			return nil, err
		}
		w.u64(m.Offset)
		w.u32(uint32(m.Size))
		w.u32(m.Mode)
	case KindFsRename:
		w.u32(m.Seq)
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		if err := w.str(MaxPathLen, m.Path); err != nil {
			return nil, err
		}
		if err := w.str(MaxPathLen, m.To); err != nil {
			return nil, err
		}
	case KindResize:
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		w.u16(m.Rows)
		w.u16(m.Cols)
	case KindSignal:
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		w.i32(int32(m.Signal))
	case KindFsStatReply:
		w.u32(m.Seq)
		w.u8(uint8(m.FileKnd))
		w.i64(m.Size)
		w.u32(m.Mode)
		w.i64(m.ModNS)
		if err := w.str(MaxPathLen, m.Link); err != nil {
			return nil, err
		}
	case KindFsListReply:
		w.u32(m.Seq)
		if err := w.entries(m.Entries); err != nil {
			return nil, err
		}
	case KindFsReadReply:
		w.u32(m.Seq)
		if err := w.bytes(MaxFrame, m.Data); err != nil {
			return nil, err
		}
	case KindStats:
		for _, v := range []int64{m.Stats.MemoryBytes, m.Stats.MemoryPeak, m.Stats.CPUUsec,
			m.Stats.Pids, m.Stats.OOMKills, m.Stats.DiskReadBytes, m.Stats.DiskWriteBytes,
			m.Stats.DiskUsedBytes} {
			w.i64(v)
		}
	case KindTop:
		// 与 Listening 一致：带上请求的 Seq，宿主才能把回复交回调用方。
		w.u32(m.Seq)
		if err := w.strs(MaxArgs, m.Lines); err != nil {
			return nil, err
		}
	case KindListening:
		w.u32(m.Seq)
		w.u32(uint32(len(m.Ports)))
		for _, p := range m.Ports {
			w.u16(p)
		}
	case KindRelay:
		w.u32(m.ConnID)
		w.u16(m.Port)
	case KindRelayData:
		w.u32(m.ConnID)
		if err := w.bytes(MaxOutputChunk, m.Data); err != nil {
			return nil, err
		}
	case KindRelayHalfClose:
		// 方向：0 = 关读（对端别再发数据给我），1 = 关写（我不再发数据）。
		w.u32(m.ConnID)
		w.u8(m.Flags)
	case KindRelayClose:
		// Flags 区分「连不上」（RelayConnectFailed）与「对端正常关闭」。
		w.u32(m.ConnID)
		w.u8(m.Flags)
	case KindWatch:
		// ID 是监听句柄；Path 是要看的目录；Recursive 决定是否下探。
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		if err := w.str(MaxPathLen, m.Path); err != nil {
			return nil, err
		}
		var flags uint8
		if m.Recursive {
			flags |= 1
		}
		w.u8(flags)
	case KindUnwatch:
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
	case KindWatchEvent:
		if err := w.str(MaxPathLen, m.ID); err != nil {
			return nil, err
		}
		if err := w.entries(m.Entries); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownKind, m.Kind)
	}
	return w.b, nil
}

// Decode 按 kind 解析 payload。
func Decode(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	switch kind {
	case KindHello:
		if m.Version, err = r.u16(); err != nil {
			return m, err
		}
		if m.Agent, err = r.str(64); err != nil {
			return m, err
		}
		m.BootNS, err = r.i64()
	case KindPing, KindPong:
		m.Nonce, err = r.u64()
	case KindShutdown:
	case KindAck, KindFsWriteAck:
		m.Seq, err = r.u32()
	case KindInputConsumed:
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		m.Bytes, err = r.u32()
	case KindError:
		if m.Seq, err = r.u32(); err != nil {
			return m, err
		}
		m.Err, err = r.str(MaxArgLen)
	case KindStart:
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		if m.Argv, err = r.strs(MaxArgs); err != nil {
			return m, err
		}
		if m.Env, err = r.strs(MaxEnv); err != nil {
			return m, err
		}
		if m.Cwd, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		if m.User, err = r.str(64); err != nil {
			return m, err
		}
		var flags uint8
		if flags, err = r.u8(); err != nil {
			return m, err
		}
		m.TTY, m.Stdin = flags&1 != 0, flags&2 != 0
		if m.Rows, err = r.u16(); err != nil {
			return m, err
		}
		if m.Cols, err = r.u16(); err != nil {
			return m, err
		}
		m.Rlimits, err = r.rlimits()
	case KindStarted:
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		m.PID, err = r.i32()
	case KindOutput:
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		var s uint8
		if s, err = r.u8(); err != nil {
			return m, err
		}
		m.Stream = Stream(s)
		m.Data, err = r.bytes(MaxOutputChunk)
	case KindExited:
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		var reason uint8
		if reason, err = r.u8(); err != nil {
			return m, err
		}
		m.Reason = ExitReason(reason)
		if m.Code, err = r.i32(); err != nil {
			return m, err
		}
		if m.Sig, err = r.i32(); err != nil {
			return m, err
		}
		// 说明字段按可选处理：老 agent 不带它，读到截断就当空串。
		if m.Err, err = r.str(MaxPathLen); err != nil {
			if errors.Is(err, ErrTruncated) {
				err = nil
				m.Err = ""
			}
			return m, err
		}
	case KindInput:
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		m.Data, err = r.bytes(MaxOutputChunk)
	case KindCloseStdin, KindFsStat, KindFsList, KindFsRead, KindFsRemove, KindFsMkdir, KindStatsQuery, KindTopQuery, KindListenQuery:
		if m.Seq, err = r.u32(); err != nil {
			return m, err
		}
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		if kind == KindFsStat || kind == KindFsList || kind == KindFsRemove || kind == KindFsMkdir {
			if m.Path, err = r.str(MaxPathLen); err != nil {
				return m, err
			}
		}
		if kind == KindFsRead {
			if m.Path, err = r.str(MaxPathLen); err != nil {
				return m, err
			}
			if m.Offset, err = r.u64(); err != nil {
				return m, err
			}
			var n uint32
			if n, err = r.u32(); err != nil {
				return m, err
			}
			m.Size = int64(n)
		}
	case KindFsWrite:
		if m.Seq, err = r.u32(); err != nil {
			return m, err
		}
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		if m.Path, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		if m.Offset, err = r.u64(); err != nil {
			return m, err
		}
		var n uint32
		if n, err = r.u32(); err != nil {
			return m, err
		}
		m.Size = int64(n)
		m.Mode, err = r.u32()
	case KindFsRename:
		if m.Seq, err = r.u32(); err != nil {
			return m, err
		}
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		if m.Path, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		m.To, err = r.str(MaxPathLen)
	case KindResize:
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		if m.Rows, err = r.u16(); err != nil {
			return m, err
		}
		m.Cols, err = r.u16()
	case KindSignal:
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		var sig int32
		if sig, err = r.i32(); err != nil {
			return m, err
		}
		m.Signal = Signal(sig)
	case KindFsStatReply:
		if m.Seq, err = r.u32(); err != nil {
			return m, err
		}
		var k uint8
		if k, err = r.u8(); err != nil {
			return m, err
		}
		m.FileKnd = FileKind(k)
		if m.Size, err = r.i64(); err != nil {
			return m, err
		}
		if m.Mode, err = r.u32(); err != nil {
			return m, err
		}
		if m.ModNS, err = r.i64(); err != nil {
			return m, err
		}
		m.Link, err = r.str(MaxPathLen)
	case KindFsListReply:
		if m.Seq, err = r.u32(); err != nil {
			return m, err
		}
		m.Entries, err = r.entries()
	case KindFsReadReply:
		if m.Seq, err = r.u32(); err != nil {
			return m, err
		}
		m.Data, err = r.bytes(MaxFrame)
	case KindStats:
		fields := []*int64{&m.Stats.MemoryBytes, &m.Stats.MemoryPeak, &m.Stats.CPUUsec,
			&m.Stats.Pids, &m.Stats.OOMKills, &m.Stats.DiskReadBytes, &m.Stats.DiskWriteBytes,
			&m.Stats.DiskUsedBytes}
		for _, p := range fields {
			// 尾部字段缺失不算错：老 agent 发的是短帧，新增字段按 0 处理。
			// 这样协议加字段不必升 Version，两侧可以不同步上线。
			if *p, err = r.i64(); err != nil {
				if errors.Is(err, ErrTruncated) {
					err = nil
					break
				}
				return m, err
			}
		}
	case KindTop:
		if m.Seq, err = r.u32(); err != nil {
			return m, err
		}
		m.Lines, err = r.strs(MaxArgs)
	case KindListening:
		if m.Seq, err = r.u32(); err != nil {
			return m, err
		}
		var n uint32
		if n, err = r.u32(); err != nil {
			return m, err
		}
		if n > 1<<16 {
			return m, fmt.Errorf("%w: %d ports", ErrFieldTooLong, n)
		}
		m.Ports = make([]uint16, 0, min(int(n), 64))
		for i := 0; i < int(n); i++ {
			v, perr := r.u16()
			if perr != nil {
				return m, perr
			}
			m.Ports = append(m.Ports, v)
		}
	case KindRelay:
		if m.ConnID, err = r.u32(); err != nil {
			return m, err
		}
		m.Port, err = r.u16()
	case KindRelayData:
		if m.ConnID, err = r.u32(); err != nil {
			return m, err
		}
		m.Data, err = r.bytes(MaxOutputChunk)
	case KindRelayHalfClose:
		if m.ConnID, err = r.u32(); err != nil {
			return m, err
		}
		m.Flags, err = r.u8()
	case KindRelayClose:
		if m.ConnID, err = r.u32(); err != nil {
			return m, err
		}
		// Flags 是后加的字段：读到截断按「正常关闭」处理，老 agent 也能接。
		if m.Flags, err = r.u8(); err != nil {
			if errors.Is(err, ErrTruncated) {
				err = nil
				m.Flags = 0
			}
			return m, err
		}
	case KindWatch:
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		if m.Path, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		var flags uint8
		if flags, err = r.u8(); err != nil {
			return m, err
		}
		m.Recursive = flags&1 != 0
	case KindUnwatch:
		m.ID, err = r.str(MaxPathLen)
	case KindWatchEvent:
		if m.ID, err = r.str(MaxPathLen); err != nil {
			return m, err
		}
		m.Entries, err = r.entries()
	default:
		return m, fmt.Errorf("%w: %d", ErrUnknownKind, kind)
	}
	return m, err
}

// Send 编码并写出一帧。调用方必须保证同一时刻只有一个 goroutine 在写，
// 否则帧会交错：Supervisor 与 agent 都用一个 writer goroutine 串行化。
func Send(w io.Writer, m *Msg) error {
	payload, err := m.Encode()
	if err != nil {
		return err
	}
	_, err = w.Write(Frame(m.Kind, payload))
	return err
}
