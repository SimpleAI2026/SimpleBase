// 文件说明：codec_contract_test.go — gosandbox/proto 的单元测试（TestWireContracts、TestWireNonzeroFields 等）
//
// 用法
//
//	go test ./gosandbox/proto -run 'TestWireContracts|TestWireNonzeroFields|TestDecodePartialAndOwnedData'
//
// 调用架构
//
//	[上游] gosandbox/agent, gosandbox/box, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/proto/codec_contract_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] codec.go, codec_test.go, frame.go, message.go, proto.go
//
// 主要符号
//
//	TestWireContracts(t *testing.T)
//	TestWireNonzeroFields(t *testing.T)
//	TestDecodePartialAndOwnedData(t *testing.T)

package proto

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestWireContracts(t *testing.T) {
	lengths := []int{14, 8, 8, 0, 8, 4, 29, 8, 9, 17, 8, 8, 8, 8, 8, 12, 29, 12, 8, 24, 8, 28, 12, 12, 16, 4, 9, 4, 8, 8, 64, 8, 8, 8, 8, 6, 8, 5, 5}
	if len(lengths) != int(KindRelayClose) {
		t.Fatal("missing wire fixture")
	}
	for i, size := range lengths {
		kind := Kind(i + 1)
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			want := make([]byte, size)
			got, err := (&Msg{Kind: kind}).Encode()
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("wire = %x, %v; want %x", got, err, want)
			}
			decoded, err := Decode(kind, want)
			if err != nil {
				t.Fatal(err)
			}
			extra, err := Decode(kind, append(want, 0xff))
			if err != nil || !reflect.DeepEqual(decoded, extra) {
				t.Fatalf("trailing bytes changed result: %+v, %v", extra, err)
			}
			for n := 0; n < size; n++ {
				partial, err := Decode(kind, want[:n])
				if !errors.Is(err, ErrTruncated) || partial.Kind != kind {
					t.Fatalf("prefix %d: %+v, %v", n, partial, err)
				}
			}
		})
	}
}

func TestWireNonzeroFields(t *testing.T) {
	cases := []struct {
		msg  Msg
		wire string
	}{
		{Msg{Kind: KindHello, Version: 2, Agent: "a", BootNS: 3}, "020001000000610300000000000000"},
		{Msg{Kind: KindOutput, ID: "p", Stream: StreamStderr, Data: []byte("x")}, "0100000070020100000078"},
		{Msg{Kind: KindFsWrite, Seq: 1, ID: "p", Path: "x", Offset: 2, Size: 3, Mode: 4}, "010000000100000070010000007802000000000000000300000004000000"},
		{Msg{Kind: KindRelay, ConnID: 1, Port: 8080}, "01000000901f"},
		{Msg{Kind: KindExited, ID: "p", Reason: ExitCode, Code: 2, Sig: 3, Err: "x"}, "01000000700102000000030000000100000078"},
	}
	for _, tc := range cases {
		want, err := hex.DecodeString(tc.wire)
		if err != nil {
			t.Fatal(err)
		}
		got, err := tc.msg.Encode()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("kind %d: %x, %v; want %x", tc.msg.Kind, got, err, want)
		}
		decoded, err := Decode(tc.msg.Kind, want)
		if err != nil || !reflect.DeepEqual(decoded, tc.msg) {
			t.Fatalf("decode: %+v, %v", decoded, err)
		}
	}
}

func TestDecodePartialAndOwnedData(t *testing.T) {
	m, err := Decode(KindFsWrite, []byte{7, 0, 0, 0, 1, 0, 0, 0, 'p'})
	if !errors.Is(err, ErrTruncated) || m.Seq != 7 || m.ID != "p" || m.Path != "" {
		t.Fatalf("partial: %+v, %v", m, err)
	}
	for _, data := range [][]byte{nil, []byte("value")} {
		payload, err := (&Msg{Kind: KindInput, ID: "p", Data: data}).Encode()
		if err != nil {
			t.Fatal(err)
		}
		m, err := Decode(KindInput, payload)
		if err != nil || m.Data == nil || !bytes.Equal(m.Data, data) {
			t.Fatalf("data: %+v, %v", m, err)
		}
		clear(payload)
		if !bytes.Equal(m.Data, data) {
			t.Fatal("decoded data aliases payload")
		}
	}
}
