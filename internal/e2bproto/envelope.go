// 文件说明：envelope.go — E2B envd 数据面所需的线协议：Connect 的 envelope 帧，以及
//
// 用法
//
//	WriteFrame(w io.Writer, msg any) error  // WriteFrame 写一个数据 envelope：1 字节标志 + 4 字节大端长度 + JSON 消息
//	WriteEndStream(w io.Writer, err *Error) error  // WriteEndStream 写流结束 envelope
//	同包调用：Error() → messages.go, messages.go
//
// 调用架构
//
//	[上游] internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/e2bproto/envelope.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] messages.go
//	[关键引用] Error() ← messages.go
//
// 主要符号
//
//	const ContentTypeUnary
//	const ContentTypeStream
//	const MaxFrameBytes
//	var ErrFrameTooLarge
//	var ErrCompressed
//	WriteFrame(w io.Writer, msg any) error
//	... 另有 24 个顶层符号

package e2bproto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

// Connect 的 content-type。流式 RPC 用 application/connect+json，一元 RPC 用 application/json。
const (
	ContentTypeUnary  = "application/json"
	ContentTypeStream = "application/connect+json"
)

// envelope 标志位。
const (
	flagCompressed = 0x01
	flagEndStream  = 0x02
)

// MaxFrameBytes 是单个 envelope 的上限。stdin 数据按块发送，64 KiB 足够，留出 JSON 与 base64 的膨胀。
const MaxFrameBytes = 1 << 20

// ErrFrameTooLarge 表示 envelope 长度超过上限。
var ErrFrameTooLarge = errors.New("e2bproto: envelope too large")

// ErrCompressed 表示收到了压缩的 envelope；我们不在请求里声明支持压缩，也不解压。
var ErrCompressed = errors.New("e2bproto: compressed envelope not supported")

// WriteFrame 写一个数据 envelope：1 字节标志 + 4 字节大端长度 + JSON 消息。
func WriteFrame(w io.Writer, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return writeRaw(w, 0, body)
}

// WriteEndStream 写流结束 envelope。err 为 nil 表示正常结束，否则带上 Connect 错误。
func WriteEndStream(w io.Writer, err *Error) error {
	end := struct {
		Error    *Error         `json:"error,omitempty"`
		Metadata map[string]any `json:"metadata,omitempty"`
	}{Error: err}
	body, mErr := json.Marshal(end)
	if mErr != nil {
		return mErr
	}
	return writeRaw(w, flagEndStream, body)
}

func writeRaw(w io.Writer, flags byte, body []byte) error {
	if len(body) > MaxFrameBytes {
		return ErrFrameTooLarge
	}
	var hdr [5]byte
	hdr[0] = flags
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(body)))
	buf := make([]byte, 0, 5+len(body))
	buf = append(buf, hdr[:]...)
	buf = append(buf, body...)
	_, err := w.Write(buf)
	return err
}

// Frame 是读到的一个 envelope。
type Frame struct {
	EndStream bool
	Body      []byte
}

// ReadFrame 读下一个 envelope。流正常结束返回 io.EOF。
func ReadFrame(r io.Reader) (Frame, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	if hdr[0]&flagCompressed != 0 {
		return Frame{}, ErrCompressed
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > MaxFrameBytes {
		return Frame{}, ErrFrameTooLarge
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return Frame{}, err
	}
	return Frame{EndStream: hdr[0]&flagEndStream != 0, Body: body}, nil
}

// DecodeFrame 读一个数据 envelope 并解成 v；读到 end-stream 返回 io.EOF（其中的错误体被忽略）。
func DecodeFrame(r io.Reader, v any) error {
	f, err := ReadFrame(r)
	if err != nil {
		return err
	}
	if f.EndStream {
		return io.EOF
	}
	return decodeStrict(f.Body, v)
}

// DecodeUnary 解一元请求体。空体视为空消息（Connect 允许省略全零消息）。
func DecodeUnary(body []byte, v any) error {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil
	}
	return decodeStrict(body, v)
}

// decodeStrict 解码时忽略未知字段：SDK 版本升级后可能带新字段，不应因此失败。
func decodeStrict(body []byte, v any) error {
	return json.Unmarshal(body, v)
}

// Error 是 Connect 的错误体。
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Connect 错误码及其 HTTP 状态映射（用于一元 RPC 的 HTTP 响应）。
const (
	CodeInvalidArgument    = "invalid_argument"
	CodeNotFound           = "not_found"
	CodeAlreadyExists      = "already_exists"
	CodePermissionDenied   = "permission_denied"
	CodeResourceExhausted  = "resource_exhausted"
	CodeFailedPrecondition = "failed_precondition"
	CodeUnimplemented      = "unimplemented"
	CodeInternal           = "internal"
	CodeUnavailable        = "unavailable"
	CodeUnauthenticated    = "unauthenticated"
	CodeCanceled           = "canceled"
	CodeDeadlineExceeded   = "deadline_exceeded"
)

// HTTPStatus 返回一元 RPC 失败时应使用的 HTTP 状态码（Connect 协议规范的映射）。
func HTTPStatus(code string) int {
	switch code {
	case CodeInvalidArgument:
		return 400
	case CodeUnauthenticated:
		return 401
	case CodePermissionDenied:
		return 403
	case CodeNotFound:
		return 404
	case CodeAlreadyExists:
		return 409
	case CodeFailedPrecondition:
		return 400
	case CodeResourceExhausted:
		return 429
	case CodeCanceled:
		return 499
	case CodeDeadlineExceeded:
		return 504
	case CodeUnimplemented:
		return 501
	case CodeUnavailable:
		return 503
	default:
		return 500
	}
}
