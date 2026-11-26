// 文件说明：codec.go — gosandbox/proto 的 payload 编解码：writer / reader 原语，以及按 Kind 分发的 Encode / Decode
//
// 用法
//
//	(m *Msg) Encode() ([]byte, error)             // 按 m.Kind 生成 payload（不含 len 与 kind）
//	Decode(kind Kind, payload []byte) (Msg, error) // 按 kind 解析 payload
//	同包调用：writer → codec_test.go, codec_golden_test.go
//
// 调用架构
//
//	[上游] gosandbox/agent, gosandbox/box, gosandbox/vm/box（成帧见 frame.go 的 Frame / ReadFrame / Send）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] Encode / Decode：只按 Kind 分发，不含字段读写
//	         │
//	         ▼
//	每种布局一对 encodeX / decodeX（同布局的帧共用一对），依次调用 writer / reader 原语
//	[下游] 仅标准库
//	[同包] codec_contract_test.go, codec_golden_test.go, codec_test.go, frame.go, message.go, proto.go
//	[关键引用] writer ← codec_test.go, codec_golden_test.go；Decode ← codec_contract_test.go, codec_golden_test.go, codec_test.go
//
// 主要符号
//
//	Encode、Decode
//	encodeX / decodeX：同一种布局的编码与解码相邻，分组与顺序同 proto.go 的 Kind
//	writer、reader 及其方法（u8 … u64 / i32 / i64 / str / bytes / strs / rlimits / entries）
//	ErrFrameTooLarge / ErrTruncated / ErrUnknownKind / ErrFieldTooLong
//
// 字段顺序、长度上限与解码失败时的错误文本都是 wire 契约：由 codec_golden_test.go 的全 Kind 字节快照
// 和 codec_contract_test.go 锁定；各布局的注释按「类型 字段」列出，str / bytes / strs 括号里是上限。

package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
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

// Encode 生成 payload（不含 len 与 kind）。这里只按 Kind 分发到对应布局的 encodeX；
// 只有该帧用到的字段才进 payload：Output 这种高频帧因此不会被无关字段撑大。
func (m *Msg) Encode() ([]byte, error) {
	switch m.Kind {
	// 会话
	case KindHello:
		return encodeHello(m)
	case KindPing, KindPong:
		return encodePing(m)
	case KindShutdown:
		// 无字段。仍返回非 nil 的空切片。
		return []byte{}, nil
	case KindError:
		return encodeError(m)
	case KindAck, KindFsWriteAck:
		return encodeAck(m)

	// 进程
	case KindStart:
		return encodeStart(m)
	case KindStarted:
		return encodeStarted(m)
	case KindOutput:
		return encodeOutput(m)
	case KindExited:
		return encodeExited(m)
	case KindInput:
		return encodeInput(m)
	case KindInputConsumed:
		return encodeInputConsumed(m)
	case KindCloseStdin, KindStatsQuery, KindTopQuery, KindListenQuery:
		return encodeRequest(m)
	case KindResize:
		return encodeResize(m)
	case KindSignal:
		return encodeSignal(m)

	// 文件
	case KindFsStat, KindFsList, KindFsRemove, KindFsMkdir:
		return encodePathRequest(m)
	case KindFsStatReply:
		return encodeFsStatReply(m)
	case KindFsListReply:
		return encodeFsListReply(m)
	case KindFsRead:
		return encodeFsRead(m)
	case KindFsReadReply:
		return encodeFsReadReply(m)
	case KindFsWrite:
		return encodeFsWrite(m)
	case KindFsRename:
		return encodeFsRename(m)

	// 目录监听
	case KindWatch:
		return encodeWatch(m)
	case KindUnwatch:
		return encodeUnwatch(m)
	case KindWatchEvent:
		return encodeWatchEvent(m)

	// 观测
	case KindStats:
		return encodeStats(m)
	case KindTop:
		return encodeTop(m)
	case KindListening:
		return encodeListening(m)

	// 中继
	case KindRelay:
		return encodeRelay(m)
	case KindRelayData:
		return encodeRelayData(m)
	case KindRelayHalfClose:
		return encodeRelayHalfClose(m)
	case KindRelayClose:
		return encodeRelayClose(m)
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownKind, m.Kind)
	}
}

// Decode 按 kind 解析 payload。这里只按 Kind 分发到对应布局的 decodeX；
// 出错时返回的 Msg 保留出错前已经解出的字段，Kind 始终等于入参。
func Decode(kind Kind, payload []byte) (Msg, error) {
	switch kind {
	// 会话
	case KindHello:
		return decodeHello(kind, payload)
	case KindPing, KindPong:
		return decodePing(kind, payload)
	case KindShutdown:
		// 无字段。
		return Msg{Kind: kind}, nil
	case KindError:
		return decodeError(kind, payload)
	case KindAck, KindFsWriteAck:
		return decodeAck(kind, payload)

	// 进程
	case KindStart:
		return decodeStart(kind, payload)
	case KindStarted:
		return decodeStarted(kind, payload)
	case KindOutput:
		return decodeOutput(kind, payload)
	case KindExited:
		return decodeExited(kind, payload)
	case KindInput:
		return decodeInput(kind, payload)
	case KindInputConsumed:
		return decodeInputConsumed(kind, payload)
	case KindCloseStdin, KindStatsQuery, KindTopQuery, KindListenQuery:
		return decodeRequest(kind, payload)
	case KindResize:
		return decodeResize(kind, payload)
	case KindSignal:
		return decodeSignal(kind, payload)

	// 文件
	case KindFsStat, KindFsList, KindFsRemove, KindFsMkdir:
		return decodePathRequest(kind, payload)
	case KindFsStatReply:
		return decodeFsStatReply(kind, payload)
	case KindFsListReply:
		return decodeFsListReply(kind, payload)
	case KindFsRead:
		return decodeFsRead(kind, payload)
	case KindFsReadReply:
		return decodeFsReadReply(kind, payload)
	case KindFsWrite:
		return decodeFsWrite(kind, payload)
	case KindFsRename:
		return decodeFsRename(kind, payload)

	// 目录监听
	case KindWatch:
		return decodeWatch(kind, payload)
	case KindUnwatch:
		return decodeUnwatch(kind, payload)
	case KindWatchEvent:
		return decodeWatchEvent(kind, payload)

	// 观测
	case KindStats:
		return decodeStats(kind, payload)
	case KindTop:
		return decodeTop(kind, payload)
	case KindListening:
		return decodeListening(kind, payload)

	// 中继
	case KindRelay:
		return decodeRelay(kind, payload)
	case KindRelayData:
		return decodeRelayData(kind, payload)
	case KindRelayHalfClose:
		return decodeRelayHalfClose(kind, payload)
	case KindRelayClose:
		return decodeRelayClose(kind, payload)
	default:
		return Msg{Kind: kind}, fmt.Errorf("%w: %d", ErrUnknownKind, kind)
	}
}

// ---- 会话 ----

// Hello：u16 Version · str(64) Agent · i64 BootNS。
func encodeHello(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u16(m.Version)
	if err := w.str(64, m.Agent); err != nil {
		return nil, err
	}
	w.i64(m.BootNS)
	return w.b, nil
}

func decodeHello(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.Version, err = r.u16(); err != nil {
		return m, err
	}
	if m.Agent, err = r.str(64); err != nil {
		return m, err
	}
	m.BootNS, err = r.i64()
	return m, err
}

// Ping 与 Pong 同布局：u64 Nonce。
func encodePing(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u64(m.Nonce)
	return w.b, nil
}

func decodePing(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	m.Nonce, err = r.u64()
	return m, err
}

// Error：u32 Seq · str(MaxArgLen) Err。
// Seq 为 0 表示这是不针对某个请求的错误（协议错误、未知帧等）。
func encodeError(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.Seq)
	if err := w.str(MaxArgLen, m.Err); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodeError(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.Seq, err = r.u32(); err != nil {
		return m, err
	}
	m.Err, err = r.str(MaxArgLen)
	return m, err
}

// Ack 与 FsWriteAck 同布局：u32 Seq。
// 回复必须带回请求的 Seq：宿主按它找回等待的调用方。
func encodeAck(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.Seq)
	return w.b, nil
}

func decodeAck(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	m.Seq, err = r.u32()
	return m, err
}

// ---- 进程 ----

// Start：str(MaxPathLen) ID · strs(MaxArgs) Argv · strs(MaxEnv) Env · str(MaxPathLen) Cwd · str(64) User ·
// u8 flags（bit0 TTY，bit1 Stdin）· u16 Rows · u16 Cols · rlimits。
func encodeStart(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
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
	return w.b, nil
}

func decodeStart(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
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
	return m, err
}

// Started：str(MaxPathLen) ID · i32 PID。
func encodeStarted(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	if err := w.str(MaxPathLen, m.ID); err != nil {
		return nil, err
	}
	w.i32(m.PID)
	return w.b, nil
}

func decodeStarted(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.ID, err = r.str(MaxPathLen); err != nil {
		return m, err
	}
	m.PID, err = r.i32()
	return m, err
}

// Output：str(MaxPathLen) ID · u8 Stream · bytes(MaxOutputChunk) Data。
func encodeOutput(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	if err := w.str(MaxPathLen, m.ID); err != nil {
		return nil, err
	}
	w.u8(uint8(m.Stream))
	if err := w.bytes(MaxOutputChunk, m.Data); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodeOutput(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.ID, err = r.str(MaxPathLen); err != nil {
		return m, err
	}
	var s uint8
	if s, err = r.u8(); err != nil {
		return m, err
	}
	m.Stream = Stream(s)
	m.Data, err = r.bytes(MaxOutputChunk)
	return m, err
}

// Exited：str(MaxPathLen) ID · u8 Reason · i32 Code · i32 Sig · str(MaxPathLen) Err。
func encodeExited(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	if err := w.str(MaxPathLen, m.ID); err != nil {
		return nil, err
	}
	w.u8(uint8(m.Reason))
	w.i32(m.Code)
	w.i32(m.Sig)
	// 无说明时也编码零长度字段，截断不能当作空串。
	if err := w.str(MaxPathLen, m.Err); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodeExited(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
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
	m.Err, err = r.str(MaxPathLen)
	return m, err
}

// Input：str(MaxPathLen) ID · bytes(MaxOutputChunk) Data。
func encodeInput(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	if err := w.str(MaxPathLen, m.ID); err != nil {
		return nil, err
	}
	if err := w.bytes(MaxOutputChunk, m.Data); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodeInput(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.ID, err = r.str(MaxPathLen); err != nil {
		return m, err
	}
	m.Data, err = r.bytes(MaxOutputChunk)
	return m, err
}

// InputConsumed：str(MaxPathLen) ID · u32 Bytes。
func encodeInputConsumed(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	if err := w.str(MaxPathLen, m.ID); err != nil {
		return nil, err
	}
	w.u32(m.Bytes)
	return w.b, nil
}

func decodeInputConsumed(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.ID, err = r.str(MaxPathLen); err != nil {
		return m, err
	}
	m.Bytes, err = r.u32()
	return m, err
}

// 只有请求头的帧：CloseStdin、StatsQuery、TopQuery、ListenQuery。布局：u32 Seq · str(MaxPathLen) ID。
// Seq 是宿主的请求序号：文件与端口查询的回复都要原样带回它。
func encodeRequest(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.Seq)
	if err := w.str(MaxPathLen, m.ID); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodeRequest(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.Seq, err = r.u32(); err != nil {
		return m, err
	}
	m.ID, err = r.str(MaxPathLen)
	return m, err
}

// Resize：str(MaxPathLen) ID · u16 Rows · u16 Cols。
func encodeResize(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	if err := w.str(MaxPathLen, m.ID); err != nil {
		return nil, err
	}
	w.u16(m.Rows)
	w.u16(m.Cols)
	return w.b, nil
}

func decodeResize(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.ID, err = r.str(MaxPathLen); err != nil {
		return m, err
	}
	if m.Rows, err = r.u16(); err != nil {
		return m, err
	}
	m.Cols, err = r.u16()
	return m, err
}

// Signal：str(MaxPathLen) ID · i32 Signal。
func encodeSignal(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	if err := w.str(MaxPathLen, m.ID); err != nil {
		return nil, err
	}
	w.i32(int32(m.Signal))
	return w.b, nil
}

func decodeSignal(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.ID, err = r.str(MaxPathLen); err != nil {
		return m, err
	}
	var sig int32
	if sig, err = r.i32(); err != nil {
		return m, err
	}
	m.Signal = Signal(sig)
	return m, nil
}

// ---- 文件 ----

// 带一个路径的请求：FsStat、FsList、FsRemove、FsMkdir。布局：u32 Seq · str(MaxPathLen) ID · str(MaxPathLen) Path。
func encodePathRequest(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.Seq)
	if err := w.str(MaxPathLen, m.ID); err != nil {
		return nil, err
	}
	if err := w.str(MaxPathLen, m.Path); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodePathRequest(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.Seq, err = r.u32(); err != nil {
		return m, err
	}
	if m.ID, err = r.str(MaxPathLen); err != nil {
		return m, err
	}
	m.Path, err = r.str(MaxPathLen)
	return m, err
}

// FsStatReply：u32 Seq · u8 FileKnd · i64 Size · u32 Mode · i64 ModNS · str(MaxPathLen) Link。
func encodeFsStatReply(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.Seq)
	w.u8(uint8(m.FileKnd))
	w.i64(m.Size)
	w.u32(m.Mode)
	w.i64(m.ModNS)
	if err := w.str(MaxPathLen, m.Link); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodeFsStatReply(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
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
	return m, err
}

// FsListReply：u32 Seq · entries。
func encodeFsListReply(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.Seq)
	if err := w.entries(m.Entries); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodeFsListReply(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.Seq, err = r.u32(); err != nil {
		return m, err
	}
	m.Entries, err = r.entries()
	return m, err
}

// FsRead：u32 Seq · str(MaxPathLen) ID · str(MaxPathLen) Path · u64 Offset · u32 Size。
func encodeFsRead(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.Seq)
	if err := w.str(MaxPathLen, m.ID); err != nil {
		return nil, err
	}
	if err := w.str(MaxPathLen, m.Path); err != nil {
		return nil, err
	}
	w.u64(m.Offset)
	w.u32(uint32(m.Size))
	return w.b, nil
}

func decodeFsRead(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
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
	return m, nil
}

// FsReadReply：u32 Seq · bytes(MaxFrame) Data。
func encodeFsReadReply(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.Seq)
	if err := w.bytes(MaxFrame, m.Data); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodeFsReadReply(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.Seq, err = r.u32(); err != nil {
		return m, err
	}
	m.Data, err = r.bytes(MaxFrame)
	return m, err
}

// FsWrite：u32 Seq · str(MaxPathLen) ID · str(MaxPathLen) Path · u64 Offset · u32 Size · u32 Mode。
func encodeFsWrite(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
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
	return w.b, nil
}

func decodeFsWrite(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
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
	return m, err
}

// FsRename：u32 Seq · str(MaxPathLen) ID · str(MaxPathLen) Path · str(MaxPathLen) To。
func encodeFsRename(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
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
	return w.b, nil
}

func decodeFsRename(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
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
	return m, err
}

// ---- 目录监听 ----

// Watch：str(MaxPathLen) ID · str(MaxPathLen) Path · u8 flags（bit0 Recursive）。
// ID 是监听句柄；Path 是要看的目录；Recursive 决定是否下探。
func encodeWatch(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
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
	return w.b, nil
}

func decodeWatch(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
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
	return m, nil
}

// Unwatch：str(MaxPathLen) ID。
func encodeUnwatch(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	if err := w.str(MaxPathLen, m.ID); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodeUnwatch(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	m.ID, err = r.str(MaxPathLen)
	return m, err
}

// WatchEvent：str(MaxPathLen) ID · entries。
func encodeWatchEvent(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	if err := w.str(MaxPathLen, m.ID); err != nil {
		return nil, err
	}
	if err := w.entries(m.Entries); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodeWatchEvent(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.ID, err = r.str(MaxPathLen); err != nil {
		return m, err
	}
	m.Entries, err = r.entries()
	return m, err
}

// ---- 观测 ----

// Stats：8 × i64，依次是 MemoryBytes · MemoryPeak · CPUUsec · Pids · OOMKills · DiskReadBytes · DiskWriteBytes · DiskUsedBytes。
func encodeStats(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	for _, v := range []int64{m.Stats.MemoryBytes, m.Stats.MemoryPeak, m.Stats.CPUUsec,
		m.Stats.Pids, m.Stats.OOMKills, m.Stats.DiskReadBytes, m.Stats.DiskWriteBytes,
		m.Stats.DiskUsedBytes} {
		w.i64(v)
	}
	return w.b, nil
}

func decodeStats(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	fields := []*int64{&m.Stats.MemoryBytes, &m.Stats.MemoryPeak, &m.Stats.CPUUsec,
		&m.Stats.Pids, &m.Stats.OOMKills, &m.Stats.DiskReadBytes, &m.Stats.DiskWriteBytes,
		&m.Stats.DiskUsedBytes}
	for _, p := range fields {
		if *p, err = r.i64(); err != nil {
			return m, err
		}
	}
	return m, nil
}

// Top：u32 Seq · strs(MaxArgs) Lines。
// 与 Listening 一致：带上请求的 Seq，宿主才能把回复交回调用方。
func encodeTop(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.Seq)
	if err := w.strs(MaxArgs, m.Lines); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodeTop(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.Seq, err = r.u32(); err != nil {
		return m, err
	}
	m.Lines, err = r.strs(MaxArgs)
	return m, err
}

// Listening：u32 Seq · u32 N · N × u16 Port（N 上限 1<<16）。
func encodeListening(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.Seq)
	w.u32(uint32(len(m.Ports)))
	for _, p := range m.Ports {
		w.u16(p)
	}
	return w.b, nil
}

func decodeListening(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
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
	return m, nil
}

// ---- 中继 ----

// Relay：u32 ConnID · u16 Port。
func encodeRelay(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.ConnID)
	w.u16(m.Port)
	return w.b, nil
}

func decodeRelay(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.ConnID, err = r.u32(); err != nil {
		return m, err
	}
	m.Port, err = r.u16()
	return m, err
}

// RelayData：u32 ConnID · bytes(MaxOutputChunk) Data。
func encodeRelayData(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.ConnID)
	if err := w.bytes(MaxOutputChunk, m.Data); err != nil {
		return nil, err
	}
	return w.b, nil
}

func decodeRelayData(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.ConnID, err = r.u32(); err != nil {
		return m, err
	}
	m.Data, err = r.bytes(MaxOutputChunk)
	return m, err
}

// RelayHalfClose：u32 ConnID · u8 Flags。
// 方向：0 = 关读（对端别再发数据给我），1 = 关写（我不再发数据）。
func encodeRelayHalfClose(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.ConnID)
	w.u8(m.Flags)
	return w.b, nil
}

func decodeRelayHalfClose(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.ConnID, err = r.u32(); err != nil {
		return m, err
	}
	m.Flags, err = r.u8()
	return m, err
}

// RelayClose：u32 ConnID · u8 Flags。
// Flags 区分「连不上」（RelayConnectFailed）与「对端正常关闭」。
func encodeRelayClose(m *Msg) ([]byte, error) {
	w := &writer{b: make([]byte, 0, 64)}
	w.u32(m.ConnID)
	w.u8(m.Flags)
	return w.b, nil
}

func decodeRelayClose(kind Kind, payload []byte) (Msg, error) {
	m := Msg{Kind: kind}
	r := &reader{b: payload}
	var err error
	if m.ConnID, err = r.u32(); err != nil {
		return m, err
	}
	m.Flags, err = r.u8()
	return m, err
}
