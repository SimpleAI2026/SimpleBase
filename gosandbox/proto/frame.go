// 文件说明：frame.go — gosandbox/proto 中 Frame、ReadFrame 等符号的实现
//
// 用法
//
//	Frame(kind Kind, payload []byte) []byte  // Frame 给 payload 补上 len 与 kind
//	ReadFrame(r io.Reader, kind *Kind, payload *[]byte) error  // ReadFrame 从流里读一帧
//	Send(w io.Writer, m *Msg) error  // Send 编码并写出一帧
//
// 调用架构
//
//	[上游] gosandbox/agent, gosandbox/box, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/proto/frame.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] codec.go, codec_contract_test.go, codec_test.go, message.go, proto.go
//	[关键引用] Frame ← codec_test.go；ReadFrame ← codec_test.go；Send ← codec_test.go
//
// 主要符号
//
//	Frame(kind Kind, payload []byte) []byte
//	ReadFrame(r io.Reader, kind *Kind, payload *[]byte) error
//	Send(w io.Writer, m *Msg) error

package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

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
