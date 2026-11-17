package proto

// codec_test.go：协议编解码的往返与边界。
//
//   1. 每种帧往返后字段不变；
//   2. 帧头 len 含 kind，读回时 payload 只剩内容；
//   3. 超长字段、超大帧、截断、未知类型都被拒绝，且不按声明分配内存；
//   4. 大块数据（Output / FsWrite）走裸字节，不经帧复制。

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// round 编码再解码一帧。
func round(t *testing.T, m Msg) Msg {
	t.Helper()
	payload, err := m.Encode()
	if err != nil {
		t.Fatalf("encode %d: %v", m.Kind, err)
	}
	got, err := Decode(m.Kind, payload)
	if err != nil {
		t.Fatalf("decode %d: %v", m.Kind, err)
	}
	if got.Kind != m.Kind {
		t.Fatalf("kind = %d, want %d", got.Kind, m.Kind)
	}
	return got
}

func TestRoundTripProcess(t *testing.T) {
	start := round(t, Msg{Kind: KindStart, ID: "p1", Argv: []string{"/bin/sh", "-c", "echo hi"},
		Env: []string{"PATH=/bin", "HOME=/root"}, Cwd: "/workspace", User: "0:0",
		TTY: true, Stdin: true, Rows: 40, Cols: 120,
		Rlimits: []Rlimit{{Name: "nofile", Cur: 1024, Max: 4096}, {Name: "core", Cur: 0, Max: 0}}})
	if start.ID != "p1" || len(start.Argv) != 3 || start.Argv[2] != "echo hi" {
		t.Fatalf("start = %+v", start)
	}
	if len(start.Env) != 2 || !start.TTY || !start.Stdin || start.Rows != 40 || start.Cols != 120 {
		t.Fatalf("start flags = %+v", start)
	}
	if len(start.Rlimits) != 2 || start.Rlimits[0].Max != 4096 {
		t.Fatalf("rlimits = %+v", start.Rlimits)
	}

	if got := round(t, Msg{Kind: KindStarted, ID: "p1", PID: 42}); got.PID != 42 {
		t.Fatalf("pid = %d", got.PID)
	}
	out := round(t, Msg{Kind: KindOutput, ID: "p1", Stream: StreamStderr, Data: []byte("boom\n")})
	if out.Stream != StreamStderr || string(out.Data) != "boom\n" {
		t.Fatalf("output = %+v", out)
	}
	ex := round(t, Msg{Kind: KindExited, ID: "p1", Reason: ExitSignaled, Sig: 9})
	if ex.Reason != ExitSignaled || ex.Sig != 9 {
		t.Fatalf("exited = %+v", ex)
	}
	if got := round(t, Msg{Kind: KindInputConsumed, ID: "p1", Bytes: 4096}); got.Bytes != 4096 {
		t.Fatalf("consumed = %d", got.Bytes)
	}
	if got := round(t, Msg{Kind: KindSignal, ID: "p1", Signal: 15}); got.Signal != 15 {
		t.Fatalf("signal = %d", got.Signal)
	}
	if got := round(t, Msg{Kind: KindResize, ID: "p1", Rows: 24, Cols: 80}); got.Rows != 24 || got.Cols != 80 {
		t.Fatalf("resize = %+v", got)
	}
}

func TestRoundTripFiles(t *testing.T) {
	st := round(t, Msg{Kind: KindFsStatReply, Seq: 7, FileKnd: FileSymlink, Size: 12, Mode: 0o644, ModNS: 1700000000, Link: "/target"})
	if st.Seq != 7 || st.FileKnd != FileSymlink || st.Link != "/target" || st.ModNS != 1700000000 {
		t.Fatalf("stat = %+v", st)
	}
	ls := round(t, Msg{Kind: KindFsListReply, Seq: 8, Entries: []Entry{
		{Name: "a.txt", Kind: FileRegular, Size: 10, Mode: 0o644, ModNS: 1},
		{Name: "sub", Kind: FileDir, Mode: 0o755, ModNS: 2},
		{Name: "link", Kind: FileSymlink, Target: "../a.txt", ModNS: 3},
	}})
	if len(ls.Entries) != 3 || ls.Entries[0].Name != "a.txt" || ls.Entries[2].Target != "../a.txt" {
		t.Fatalf("list = %+v", ls.Entries)
	}
	rd := round(t, Msg{Kind: KindFsReadReply, Seq: 9, Data: bytes.Repeat([]byte("x"), 1000)})
	if len(rd.Data) != 1000 {
		t.Fatalf("read len = %d", len(rd.Data))
	}
	wr := round(t, Msg{Kind: KindFsWrite, ID: "p1", Path: "/workspace/x", Offset: 5, Size: 4096, Mode: 0o600})
	if wr.Path != "/workspace/x" || wr.Offset != 5 || wr.Size != 4096 || wr.Mode != 0o600 {
		t.Fatalf("write = %+v", wr)
	}
	rn := round(t, Msg{Kind: KindFsRename, ID: "p1", Path: "/a", To: "/b"})
	if rn.Path != "/a" || rn.To != "/b" {
		t.Fatalf("rename = %+v", rn)
	}
}

func TestRoundTripObservability(t *testing.T) {
	s := round(t, Msg{Kind: KindStats, Stats: Stats{MemoryBytes: 1 << 20, MemoryPeak: 2 << 20,
		CPUUsec: 1234, Pids: 3, OOMKills: 1, DiskReadBytes: 4096, DiskWriteBytes: 8192}})
	if s.Stats.MemoryBytes != 1<<20 || s.Stats.OOMKills != 1 || s.Stats.DiskWriteBytes != 8192 {
		t.Fatalf("stats = %+v", s.Stats)
	}
	rel := round(t, Msg{Kind: KindRelay, ConnID: 99, Port: 8080})
	if rel.ConnID != 99 || rel.Port != 8080 {
		t.Fatalf("relay = %+v", rel)
	}
	top := round(t, Msg{Kind: KindTop, Lines: []string{"0\t1\t0\t..."}})
	if len(top.Lines) != 1 {
		t.Fatalf("top = %+v", top)
	}
}

// 回复帧必须带 Seq：宿主按它找回等待中的调用方。
//
// 少了这一字段，文件请求的回复会被当成「不属于任何请求」丢掉，
// 调用方只能等到超时（FsWrite / Ack 全部如此）。
func TestRoundTripRepliesCarrySeq(t *testing.T) {
	if got := round(t, Msg{Kind: KindAck, Seq: 7}); got.Seq != 7 {
		t.Fatalf("ack seq = %d", got.Seq)
	}
	if got := round(t, Msg{Kind: KindFsWriteAck, Seq: 8}); got.Seq != 8 {
		t.Fatalf("write ack seq = %d", got.Seq)
	}
	er := round(t, Msg{Kind: KindError, Seq: 9, Err: "boom"})
	if er.Seq != 9 || er.Err != "boom" {
		t.Fatalf("error = %+v", er)
	}
}

// RelayClose 的 Flags 区分「连不上」与「对端正常关闭」。
func TestRoundTripRelayCloseFlags(t *testing.T) {
	rc := round(t, Msg{Kind: KindRelayClose, ConnID: 3, Flags: RelayConnectFailed})
	if rc.ConnID != 3 || rc.Flags != RelayConnectFailed {
		t.Fatalf("relay close = %+v", rc)
	}
	// 老 agent 只带 ConnID：按「正常关闭」处理，不报错。
	w := &writer{}
	w.u32(5)
	m, err := Decode(KindRelayClose, w.b)
	if err != nil || m.ConnID != 5 || m.Flags != 0 {
		t.Fatalf("old relay close = %+v, %v", m, err)
	}
}

// 帧头 len 含 kind；ReadFrame 读回时 payload 只剩内容。
func TestFrameHeader(t *testing.T) {
	payload := []byte("abc")
	f := Frame(KindPing, payload)
	if got := int(f[0]) | int(f[1])<<8 | int(f[2])<<16 | int(f[3])<<24; got != len(payload)+2 {
		t.Fatalf("len = %d, want %d", got, len(payload)+2)
	}
	var kind Kind
	var buf []byte
	if err := ReadFrame(bytes.NewReader(f), &kind, &buf); err != nil {
		t.Fatal(err)
	}
	if kind != KindPing || string(buf) != "abc" {
		t.Fatalf("kind=%d payload=%q", kind, buf)
	}
}

// 连续多帧读回，payload 缓冲复用。
func TestReadFrameReusesBuffer(t *testing.T) {
	var stream bytes.Buffer
	for i := 0; i < 3; i++ {
		stream.Write(Frame(KindPong, []byte(strings.Repeat("z", 8))))
	}
	var kind Kind
	var buf []byte
	for i := 0; i < 3; i++ {
		if err := ReadFrame(&stream, &kind, &buf); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if len(buf) != 8 {
			t.Fatalf("frame %d len = %d", i, len(buf))
		}
	}
}

// 对端正常关闭返回 io.EOF，而不是把半帧当数据。
func TestReadFrameEOF(t *testing.T) {
	var kind Kind
	var buf []byte
	if err := ReadFrame(bytes.NewReader(nil), &kind, &buf); !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v", err)
	}
}

// 声明长度超过 MaxFrame 直接拒绝，不按声明分配。
func TestReadFrameRejectsOversized(t *testing.T) {
	f := Frame(KindPing, nil)
	f[0], f[1], f[2], f[3] = 0xff, 0xff, 0xff, 0xff
	var kind Kind
	var buf []byte
	if err := ReadFrame(bytes.NewReader(f), &kind, &buf); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("err = %v", err)
	}
	// 正常的 1 MiB 帧是允许的（边界值）。
	big := Frame(KindPong, make([]byte, MaxFrame-2))
	if err := ReadFrame(bytes.NewReader(big), &kind, &buf); err != nil {
		t.Fatalf("max frame must be accepted: %v", err)
	}
}

// 帧被截断：返回 ErrTruncated。
func TestReadFrameTruncated(t *testing.T) {
	f := Frame(KindPing, []byte("0123456789"))
	var kind Kind
	var buf []byte
	if err := ReadFrame(bytes.NewReader(f[:len(f)-3]), &kind, &buf); !errors.Is(err, ErrTruncated) {
		t.Fatalf("err = %v", err)
	}
}

// 超长字段在编码时就被拒绝，而不是生成一帧让对端拒绝。
func TestEncodeRejectsLongFields(t *testing.T) {
	long := strings.Repeat("a", MaxPathLen+1)
	if _, err := (&Msg{Kind: KindFsStat, ID: "p", Path: long}).Encode(); !errors.Is(err, ErrFieldTooLong) {
		t.Fatalf("err = %v", err)
	}
	many := make([]string, MaxArgs+1)
	if _, err := (&Msg{Kind: KindStart, ID: "p", Argv: many}).Encode(); !errors.Is(err, ErrFieldTooLong) {
		t.Fatalf("argv: %v", err)
	}
	// Output 超过 MaxOutputChunk 也要拒绝：否则帧会超过 MaxFrame。
	if _, err := (&Msg{Kind: KindOutput, ID: "p", Data: make([]byte, MaxOutputChunk+1)}).Encode(); !errors.Is(err, ErrFieldTooLong) {
		t.Fatalf("output: %v", err)
	}
}

// 构造的帧声明了超长字符串：解码按上限拒绝，不按声明分配。
func TestDecodeRejectsLongString(t *testing.T) {
	w := &writer{}
	w.u32(7)       // Seq
	w.u32(1 << 20) // 声明 1 MiB 的字符串，但只有 3 字节
	w.raw([]byte("abc"))
	if _, err := Decode(KindFsStat, w.b); !errors.Is(err, ErrFieldTooLong) {
		t.Fatalf("err = %v", err)
	}
}

// 截断的 payload 解不出来，且不 panic。
func TestDecodeTruncated(t *testing.T) {
	payload, err := (&Msg{Kind: KindFsListReply, Seq: 1, Entries: []Entry{{Name: "a", Kind: FileRegular}}}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 1, 4, len(payload) - 1} {
		if _, err := Decode(KindFsListReply, payload[:n]); err == nil {
			t.Errorf("truncated payload[%d] must fail", n)
		}
	}
}

// 未知的帧类型在编码与解码两侧都拒绝（版本不一致时快速失败）。
func TestUnknownKind(t *testing.T) {
	if _, err := (&Msg{Kind: Kind(9999)}).Encode(); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("encode err = %v", err)
	}
	if _, err := Decode(Kind(9999), nil); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("decode err = %v", err)
	}
}

// Send 写出的帧能被 ReadFrame 读回。
func TestSend(t *testing.T) {
	var buf bytes.Buffer
	if err := Send(&buf, &Msg{Kind: KindHello, Version: Version, Agent: "test", BootNS: 42}); err != nil {
		t.Fatal(err)
	}
	var kind Kind
	var payload []byte
	if err := ReadFrame(&buf, &kind, &payload); err != nil {
		t.Fatal(err)
	}
	got, err := Decode(kind, payload)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != Version || got.Agent != "test" || got.BootNS != 42 {
		t.Fatalf("hello = %+v", got)
	}
}
