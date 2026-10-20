package e2bproto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestFrameRoundTripAndEndStream(t *testing.T) {
	var buf bytes.Buffer
	pid := uint32(7)
	if err := WriteFrame(&buf, StartResponse{Event: ProcessEvent{Start: &StartEvent{PID: pid}}}); err != nil {
		t.Fatal(err)
	}
	if err := WriteFrame(&buf, StartResponse{Event: ProcessEvent{Data: &DataEvent{Stdout: Bytes("hi\n")}}}); err != nil {
		t.Fatal(err)
	}
	if err := WriteEndStream(&buf, nil); err != nil {
		t.Fatal(err)
	}
	// 帧头：标志 + 大端长度。
	raw := buf.Bytes()
	if raw[0] != 0 || int(binary.BigEndian.Uint32(raw[1:5])) == 0 {
		t.Fatalf("bad header % x", raw[:5])
	}

	var first StartResponse
	if err := DecodeFrame(&buf, &first); err != nil || first.Event.Start == nil || first.Event.Start.PID != 7 {
		t.Fatalf("first = %+v err=%v", first, err)
	}
	var second StartResponse
	if err := DecodeFrame(&buf, &second); err != nil || string(second.Event.Data.Stdout) != "hi\n" {
		t.Fatalf("second = %+v err=%v", second, err)
	}
	var third StartResponse
	if err := DecodeFrame(&buf, &third); !errors.Is(err, io.EOF) {
		t.Fatalf("end-stream must surface as io.EOF, got %v", err)
	}
}

// bytes 在 JSON 里必须是 base64，而不是数字数组。
func TestBytesAreBase64(t *testing.T) {
	b, _ := json.Marshal(DataEvent{Stdout: Bytes("abc")})
	if string(b) != `{"stdout":"YWJj"}` {
		t.Fatalf("json = %s", b)
	}
	var d DataEvent
	for _, in := range []string{`{"stdout":"YWJj"}`, `{"stdout":"YWJj="}`} {
		if err := json.Unmarshal([]byte(in), &d); err != nil && in == `{"stdout":"YWJj"}` {
			t.Fatal(err)
		}
	}
	if string(d.Stdout) != "abc" {
		t.Fatalf("decoded = %q", d.Stdout)
	}
	if err := json.Unmarshal([]byte(`{"stdout":"!!!"}`), &d); err == nil {
		t.Fatal("invalid base64 must be rejected")
	}
	// 无填充写法也要接受。
	var raw DataEvent
	if err := json.Unmarshal([]byte(`{"stdout":"YWI"}`), &raw); err != nil || string(raw.Stdout) != "ab" {
		t.Fatalf("raw base64: %q %v", raw.Stdout, err)
	}
}

// int64 在 JSON 里是字符串；同时接受数字写法。
func TestInt64Encoding(t *testing.T) {
	b, _ := json.Marshal(EntryInfo{Name: "a", Type: FileTypeFile, Size: 1 << 40})
	if !strings.Contains(string(b), `"size":"1099511627776"`) {
		t.Fatalf("size must be a string: %s", b)
	}
	var e EntryInfo
	if err := json.Unmarshal([]byte(`{"size":"12"}`), &e); err != nil || e.Size != 12 {
		t.Fatalf("string size: %+v %v", e, err)
	}
	if err := json.Unmarshal([]byte(`{"size":34}`), &e); err != nil || e.Size != 34 {
		t.Fatalf("numeric size: %+v %v", e, err)
	}
	if err := json.Unmarshal([]byte(`{"size":"x"}`), &e); err == nil {
		t.Fatal("non-numeric size must fail")
	}
}

func TestSignalAcceptsNameAndNumber(t *testing.T) {
	cases := map[string]Signal{`9`: SignalSIGKILL, `15`: SignalSIGTERM, `"SIGNAL_SIGKILL"`: SignalSIGKILL,
		`"SIGNAL_SIGTERM"`: SignalSIGTERM, `"SIGNAL_UNSPECIFIED"`: SignalUnspecified}
	for in, want := range cases {
		var req SendSignalRequest
		if err := json.Unmarshal([]byte(`{"signal":`+in+`}`), &req); err != nil || req.Signal != want {
			t.Errorf("%s -> %v err=%v", in, req.Signal, err)
		}
	}
	var req SendSignalRequest
	if err := json.Unmarshal([]byte(`{"signal":"SIGNAL_BOGUS"}`), &req); err == nil {
		t.Fatal("unknown signal name must fail")
	}
}

func TestFrameLimits(t *testing.T) {
	// 过大的帧在读取时被拒绝，而不是先分配。
	var hdr [5]byte
	binary.BigEndian.PutUint32(hdr[1:], MaxFrameBytes+1)
	if _, err := ReadFrame(bytes.NewReader(hdr[:])); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversize read: %v", err)
	}
	if err := WriteFrame(io.Discard, strings.Repeat("a", MaxFrameBytes+1)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversize write: %v", err)
	}
	// 压缩帧不支持。
	hdr[0] = 0x01
	binary.BigEndian.PutUint32(hdr[1:], 0)
	if _, err := ReadFrame(bytes.NewReader(hdr[:])); !errors.Is(err, ErrCompressed) {
		t.Fatalf("compressed: %v", err)
	}
	// 截断的帧体是 ErrUnexpectedEOF，不是静默成功。
	binary.BigEndian.PutUint32(hdr[1:], 10)
	hdr[0] = 0
	trunc := append(hdr[:], []byte("abc")...)
	if _, err := ReadFrame(bytes.NewReader(trunc)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated body: %v", err)
	}
	if _, err := ReadFrame(bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
		t.Fatalf("empty: %v", err)
	}
}

func TestEndStreamCarriesError(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteEndStream(&buf, &Error{Code: CodeNotFound, Message: "no such process"}); err != nil {
		t.Fatal(err)
	}
	f, err := ReadFrame(&buf)
	if err != nil || !f.EndStream {
		t.Fatalf("frame = %+v err=%v", f, err)
	}
	var end struct {
		Error *Error `json:"error"`
	}
	if err := json.Unmarshal(f.Body, &end); err != nil || end.Error == nil || end.Error.Code != CodeNotFound {
		t.Fatalf("end = %s err=%v", f.Body, err)
	}
}

func TestDecodeUnaryAndUnknownFields(t *testing.T) {
	var r ListDirRequest
	if err := DecodeUnary(nil, &r); err != nil || r.Path != "" {
		t.Fatalf("empty body: %+v %v", r, err)
	}
	if err := DecodeUnary([]byte(` {"path":"/workspace","depth":2,"future":true} `), &r); err != nil || r.Path != "/workspace" || r.Depth != 2 {
		t.Fatalf("unknown fields must be ignored: %+v %v", r, err)
	}
	if err := DecodeUnary([]byte(`{`), &r); err == nil {
		t.Fatal("malformed JSON must fail")
	}
}

func TestHTTPStatusMapping(t *testing.T) {
	want := map[string]int{CodeInvalidArgument: 400, CodeUnauthenticated: 401, CodePermissionDenied: 403,
		CodeNotFound: 404, CodeAlreadyExists: 409, CodeResourceExhausted: 429, CodeUnimplemented: 501,
		CodeUnavailable: 503, CodeInternal: 500, "weird": 500}
	for code, status := range want {
		if got := HTTPStatus(code); got != status {
			t.Errorf("%s -> %d, want %d", code, got, status)
		}
	}
}

func TestPermStringAndTimestamp(t *testing.T) {
	if got := PermString(0o644, false); got != "-rw-r--r--" {
		t.Fatalf("perm = %q", got)
	}
	if got := PermString(0o755, true); got != "drwxr-xr-x" {
		t.Fatalf("dir perm = %q", got)
	}
	if FormatTimestamp(time.Time{}) != "" {
		t.Fatal("zero time must be omitted")
	}
	if got := FormatTimestamp(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)); got != "2026-01-02T03:04:05Z" {
		t.Fatalf("timestamp = %q", got)
	}
}

func TestStartRequestShape(t *testing.T) {
	// SDK 发来的典型 JSON：cwd、tag、pty 都是可选。
	in := `{"process":{"cmd":"/bin/bash","args":["-l","-c","ls"],"envs":{"A":"1"},"cwd":"/workspace"},"pty":{"size":{"cols":80,"rows":24}},"tag":"t1","stdin":true}`
	var r StartRequest
	if err := DecodeUnary([]byte(in), &r); err != nil {
		t.Fatal(err)
	}
	if r.Process.Cmd != "/bin/bash" || len(r.Process.Args) != 3 || r.Process.Envs["A"] != "1" ||
		r.Process.Cwd == nil || *r.Process.Cwd != "/workspace" || r.PTY == nil || r.PTY.Size.Cols != 80 ||
		r.Tag == nil || *r.Tag != "t1" || r.Stdin == nil || !*r.Stdin {
		t.Fatalf("parsed = %+v", r)
	}
	// 响应里零值分支不应出现。
	out, _ := json.Marshal(StartResponse{Event: ProcessEvent{End: &EndEvent{ExitCode: 0, Exited: true, Status: "exited"}}})
	if strings.Contains(string(out), `"start"`) || strings.Contains(string(out), `"data"`) {
		t.Fatalf("unset oneof branches leaked: %s", out)
	}
}
