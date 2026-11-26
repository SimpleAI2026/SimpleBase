// 文件说明：codec_golden_test.go — gosandbox/proto 的全 Kind 字节快照与错误文本契约测试
//
// 用法
//
//	go test ./proto -run 'TestWireGoldenRoundTrip|TestFieldTooLongErrors'
//
// 调用架构
//
//	[上游] 重构前的协议特征测试
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/proto/codec_golden_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] codec.go, codec_contract_test.go, codec_test.go, frame.go, message.go, proto.go
//
// 主要符号
//
//	TestWireGoldenRoundTrip(t *testing.T)
//	TestFieldTooLongErrors(t *testing.T)

package proto

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestWireGoldenRoundTrip(t *testing.T) {
	cases := []struct {
		msg  Msg
		wire string
	}{
		{Msg{Kind: KindHello, Version: Version, Agent: "agent", BootNS: 0x0102030405060708}, "0300050000006167656e740807060504030201"},
		{Msg{Kind: KindPing, Nonce: 0x0102030405060708}, "0807060504030201"},
		{Msg{Kind: KindPong, Nonce: 0x1112131415161718}, "1817161514131211"},
		{Msg{Kind: KindShutdown}, ""},
		{Msg{Kind: KindError, Seq: 5, Err: "boom"}, "0500000004000000626f6f6d"},
		{Msg{Kind: KindAck, Seq: 6}, "06000000"},
		{Msg{Kind: KindStart, ID: "proc", Argv: []string{"/bin/sh", "-c", "echo hi"}, Env: []string{"PATH=/bin", "HOME=/root"}, Cwd: "/workspace", User: "1000:1000", TTY: true, Stdin: true, Rows: 40, Cols: 120, Rlimits: []Rlimit{{Name: "nofile", Cur: 1024, Max: 4096}}}, "0400000070726f6303000000070000002f62696e2f7368020000002d63070000006563686f2068690200000009000000504154483d2f62696e0a000000484f4d453d2f726f6f740a0000002f776f726b737061636509000000313030303a31303030032800780001000000060000006e6f66696c6500040000000000000010000000000000"},
		{Msg{Kind: KindStarted, ID: "proc", PID: 42}, "0400000070726f632a000000"},
		{Msg{Kind: KindOutput, ID: "proc", Stream: StreamStderr, Data: []byte{0, 1, 0xfe, 0xff}}, "0400000070726f6302040000000001feff"},
		{Msg{Kind: KindExited, ID: "proc", Reason: ExitSignaled, Code: 2, Sig: 9, Err: "killed"}, "0400000070726f63020200000009000000060000006b696c6c6564"},
		{Msg{Kind: KindInput, ID: "proc", Data: []byte("input")}, "0400000070726f6305000000696e707574"},
		{Msg{Kind: KindInputConsumed, ID: "proc", Bytes: 4096}, "0400000070726f6300100000"},
		{Msg{Kind: KindCloseStdin, Seq: 7, ID: "proc"}, "070000000400000070726f63"},
		{Msg{Kind: KindResize, ID: "proc", Rows: 24, Cols: 80}, "0400000070726f6318005000"},
		{Msg{Kind: KindSignal, ID: "proc", Signal: 15}, "0400000070726f630f000000"},
		{Msg{Kind: KindFsStat, Seq: 8, ID: "fs", Path: "/workspace/file"}, "080000000200000066730f0000002f776f726b73706163652f66696c65"},
		{Msg{Kind: KindFsStatReply, Seq: 8, FileKnd: FileSymlink, Size: 12, Mode: 0o644, ModNS: 1700000000, Link: "/target"}, "08000000030c00000000000000a401000000f1536500000000070000002f746172676574"},
		{Msg{Kind: KindFsList, Seq: 9, ID: "fs", Path: "/workspace"}, "090000000200000066730a0000002f776f726b7370616365"},
		{Msg{Kind: KindFsListReply, Seq: 9, Entries: []Entry{{Name: "file", Kind: FileRegular, Size: 10, Mode: 0o644, ModNS: 11}, {Name: "link", Kind: FileSymlink, Size: 12, Mode: 0o777, ModNS: 13, Target: "file"}}}, "09000000020000000400000066696c65010a00000000000000a40100000b0000000000000000000000040000006c696e6b030c00000000000000ff0100000d000000000000000400000066696c65"},
		{Msg{Kind: KindFsRead, Seq: 10, ID: "fs", Path: "/workspace/file", Offset: 2, Size: 4096}, "0a0000000200000066730f0000002f776f726b73706163652f66696c65020000000000000000100000"},
		{Msg{Kind: KindFsReadReply, Seq: 10, Data: []byte{0, 2, 4, 8}}, "0a0000000400000000020408"},
		{Msg{Kind: KindFsWrite, Seq: 11, ID: "fs", Path: "/workspace/file", Offset: 3, Size: 4096, Mode: 0o600}, "0b0000000200000066730f0000002f776f726b73706163652f66696c6503000000000000000010000080010000"},
		{Msg{Kind: KindFsRemove, Seq: 12, ID: "fs", Path: "/workspace/file"}, "0c0000000200000066730f0000002f776f726b73706163652f66696c65"},
		{Msg{Kind: KindFsMkdir, Seq: 13, ID: "fs", Path: "/workspace/dir"}, "0d0000000200000066730e0000002f776f726b73706163652f646972"},
		{Msg{Kind: KindFsRename, Seq: 14, ID: "fs", Path: "/workspace/old", To: "/workspace/new"}, "0e0000000200000066730e0000002f776f726b73706163652f6f6c640e0000002f776f726b73706163652f6e6577"},
		{Msg{Kind: KindFsWriteAck, Seq: 11}, "0b000000"},
		{Msg{Kind: KindWatch, ID: "watch", Path: "/workspace", Recursive: true}, "0500000077617463680a0000002f776f726b737061636501"},
		{Msg{Kind: KindUnwatch, ID: "watch"}, "050000007761746368"},
		{Msg{Kind: KindWatchEvent, ID: "watch", Entries: []Entry{{Name: "file", Kind: FileRegular, Size: 10, Mode: 0o644, ModNS: 11}}}, "050000007761746368010000000400000066696c65010a00000000000000a40100000b0000000000000000000000"},
		{Msg{Kind: KindStatsQuery, Seq: 15, ID: "stats"}, "0f000000050000007374617473"},
		{Msg{Kind: KindStats, Stats: Stats{MemoryBytes: 1, MemoryPeak: 2, CPUUsec: 3, Pids: 4, OOMKills: 5, DiskReadBytes: 6, DiskWriteBytes: 7, DiskUsedBytes: 8}}, "01000000000000000200000000000000030000000000000004000000000000000500000000000000060000000000000007000000000000000800000000000000"},
		{Msg{Kind: KindTopQuery, Seq: 16, ID: "top"}, "1000000003000000746f70"},
		{Msg{Kind: KindTop, Seq: 16, Lines: []string{"first", "second"}}, "1000000002000000050000006669727374060000007365636f6e64"},
		{Msg{Kind: KindListenQuery, Seq: 17, ID: "listen"}, "11000000060000006c697374656e"},
		{Msg{Kind: KindListening, Seq: 17, Ports: []uint16{80, 443, 8080}}, "11000000030000005000bb01901f"},
		{Msg{Kind: KindRelay, ConnID: 18, Port: 8080}, "12000000901f"},
		{Msg{Kind: KindRelayData, ConnID: 18, Data: []byte("relay")}, "120000000500000072656c6179"},
		{Msg{Kind: KindRelayHalfClose, ConnID: 18, Flags: 1}, "1200000001"},
		{Msg{Kind: KindRelayClose, ConnID: 18, Flags: RelayConnectFailed}, "1200000001"},
	}
	if len(cases) != int(KindRelayClose) {
		t.Fatalf("fixtures = %d, want %d", len(cases), KindRelayClose)
	}
	for i, tc := range cases {
		kind := Kind(i + 1)
		t.Run(fmt.Sprintf("kind_%02d", kind), func(t *testing.T) {
			if tc.msg.Kind != kind {
				t.Fatalf("fixture kind = %d, want %d", tc.msg.Kind, kind)
			}
			got, err := tc.msg.Encode()
			if err != nil {
				t.Fatal(err)
			}
			want, err := hex.DecodeString(tc.wire)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("wire = %x, want %x", got, want)
			}
			decoded, err := Decode(kind, want)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded, tc.msg) {
				t.Fatalf("round trip = %+v, want %+v", decoded, tc.msg)
			}
		})
	}
}

func TestFieldTooLongErrors(t *testing.T) {
	encodeCases := []struct {
		name string
		msg  Msg
		want string
	}{
		{"agent", Msg{Kind: KindHello, Agent: strings.Repeat("a", 65)}, "proto: field too long: 65 > 64"},
		{"path", Msg{Kind: KindFsStat, Path: strings.Repeat("p", MaxPathLen+1)}, "proto: field too long: 4097 > 4096"},
		{"error", Msg{Kind: KindError, Err: strings.Repeat("e", MaxArgLen+1)}, "proto: field too long: 65537 > 65536"},
		{"bytes", Msg{Kind: KindOutput, Data: make([]byte, MaxOutputChunk+1)}, "proto: field too long: 65537 > 65536"},
		{"argument count", Msg{Kind: KindStart, Argv: make([]string, MaxArgs+1)}, "proto: field too long: 4097 items > 4096"},
		{"argument", Msg{Kind: KindStart, Argv: []string{strings.Repeat("a", MaxArgLen+1)}}, "proto: field too long: 65537 > 65536"},
		{"rlimit name", Msg{Kind: KindStart, Rlimits: []Rlimit{{Name: strings.Repeat("r", 33)}}}, "proto: field too long: 33 > 32"},
		{"entry name", Msg{Kind: KindFsListReply, Entries: []Entry{{Name: strings.Repeat("n", MaxPathLen+1)}}}, "proto: field too long: 4097 > 4096"},
	}
	for _, tc := range encodeCases {
		t.Run("encode/"+tc.name, func(t *testing.T) {
			if _, err := tc.msg.Encode(); err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	decodeCases := []struct {
		name string
		kind Kind
		make func() []byte
		want string
	}{
		{"string", KindFsStat, func() []byte {
			w := &writer{}
			w.u32(1)
			w.u32(MaxPathLen + 1)
			return w.b
		}, "proto: field too long: 4097 > 4096"},
		{"bytes", KindOutput, func() []byte {
			w := &writer{}
			w.str(MaxPathLen, "p")
			w.u8(uint8(StreamStdout))
			w.u32(MaxOutputChunk + 1)
			return w.b
		}, "proto: field too long: 65537 > 65536"},
		{"items", KindTop, func() []byte {
			w := &writer{}
			w.u32(1)
			w.u32(MaxArgs + 1)
			return w.b
		}, "proto: field too long: 4097 items > 4096"},
		{"rlimits", KindStart, func() []byte {
			w := &writer{}
			w.str(MaxPathLen, "p")
			w.strs(MaxArgs, nil)
			w.strs(MaxEnv, nil)
			w.str(MaxPathLen, "")
			w.str(64, "")
			w.u8(0)
			w.u16(0)
			w.u16(0)
			w.u32(65)
			return w.b
		}, "proto: field too long: 65 rlimits"},
		{"entries", KindFsListReply, func() []byte {
			w := &writer{}
			w.u32(1)
			w.u32(1<<20 + 1)
			return w.b
		}, "proto: field too long: 1048577 entries"},
		{"ports", KindListening, func() []byte {
			w := &writer{}
			w.u32(1)
			w.u32(1<<16 + 1)
			return w.b
		}, "proto: field too long: 65537 ports"},
	}
	for _, tc := range decodeCases {
		t.Run("decode/"+tc.name, func(t *testing.T) {
			if _, err := Decode(tc.kind, tc.make()); err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
