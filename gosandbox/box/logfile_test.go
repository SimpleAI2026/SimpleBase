// 文件说明：logfile_test.go — gosandbox/box 的单元测试（TestLogRoundTrip、TestLogSplitsLongLines 等）
//
// 用法
//
//	go test ./gosandbox/box -run 'TestLogRoundTrip|TestLogSplitsLongLines|TestSinkWritesOneRecordPerLine|TestSinkSplitsPartialLines'
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/logfile_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] backpressure_test.go, copytree.go, deadline.go, egress_e2e_test.go, files.go 等 26 个
//
// 主要符号
//
//	TestLogRoundTrip(t *testing.T)
//	TestLogSplitsLongLines(t *testing.T)
//	TestSinkWritesOneRecordPerLine(t *testing.T)
//	TestSinkSplitsPartialLines(t *testing.T)
//	TestLogRotation(t *testing.T)
//	TestTailFilters(t *testing.T)
//	TestTailIgnoresTornTail(t *testing.T)
//	TestTailDetectsCorruption(t *testing.T)
//	... 另有 2 个顶层符号

package box

// logfile_test.go：日志记录格式、轮转与回放。
//
//   1. 记录二进制安全（含 \n、NUL）；
//   2. 超过 MaxLine 的数据被切成多条；
//   3. 轮转保留 N 份，最老的被删；
//   4. Tail 支持按行数、时间、流过滤；
//   5. 尾部半条记录被忽略而不是让整份日志读不出来；
//   6. 文件被写坏时报 ErrLogCorrupt。

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogRoundTrip(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "p1", Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("hello\nworld\x00binary")
	if err := l.Write(StreamStdout, want); err != nil {
		t.Fatal(err)
	}
	if err := l.Write(StreamStderr, []byte("err\n")); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	recs, err := Tail(dir, "p1", TailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("records = %d", len(recs))
	}
	if !bytes.Equal(recs[0].Data, want) {
		t.Fatalf("data = %q", recs[0].Data)
	}
	if recs[0].Stream != StreamStdout || recs[1].Stream != StreamStderr {
		t.Fatalf("streams = %d / %d", recs[0].Stream, recs[1].Stream)
	}
}

// 超长数据被切成多条，且拼接后与原内容一致。
func TestLogSplitsLongLines(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(dir, "p1", Options{})
	long := bytes.Repeat([]byte("x"), MaxLine*2+100)
	if err := l.Write(StreamStdout, long); err != nil {
		t.Fatal(err)
	}
	_ = l.Close()

	recs, err := Tail(dir, "p1", TailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("records = %d, want 3", len(recs))
	}
	var joined []byte
	for _, r := range recs {
		joined = append(joined, r.Data...)
	}
	if !bytes.Equal(joined, long) {
		t.Fatal("split records must reassemble to the original")
	}
}

// 会话落盘按行切记录：Lines 是行数（docker logs --tail 的语义）。
//
// 一个 Output 帧里通常有好几行；整块写成一条记录会让「最后 2 行」
// 退回成整块，logs --tail 就失效了。
func TestSinkWritesOneRecordPerLine(t *testing.T) {
	dir := t.TempDir()
	sink := newLogSink(dir, Options{}, nil)
	l := sink.open("p1")
	if l == nil {
		t.Fatal("log must open")
	}
	sink.write(l, StreamStdout, []byte("1\n2\n3\n4\n"))
	sink.close("p1")

	recs, err := Tail(dir, "p1", TailOptions{Lines: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2 (one per line)", len(recs))
	}
	if string(recs[0].Data) != "3\n" || string(recs[1].Data) != "4\n" {
		t.Fatalf("tail = %q / %q", recs[0].Data, recs[1].Data)
	}
}

// 一次写里既有整行又有半行：半行单独一条，后续补上时另起一条。
func TestSinkSplitsPartialLines(t *testing.T) {
	dir := t.TempDir()
	sink := newLogSink(dir, Options{}, nil)
	l := sink.open("p1")
	sink.write(l, StreamStdout, []byte("hel"))
	sink.write(l, StreamStdout, []byte("lo\nworld"))
	sink.close("p1")

	recs, err := Tail(dir, "p1", TailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var all []byte
	for _, r := range recs {
		all = append(all, r.Data...)
	}
	if string(all) != "hello\nworld" {
		t.Fatalf("joined = %q", all)
	}
}

// 轮转：超过 MaxSize 后保留 N 份。
func TestLogRotation(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, "p1", Options{MaxSize: 1024, MaxFiles: 3})
	if err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte("a"), 256)
	for i := 0; i < 40; i++ {
		if err := l.Write(StreamStdout, chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	// 当前文件可能为空（刚轮转过），但 .1 应该存在。
	if _, err := os.Stat(filepath.Join(dir, "p1.log.1")); err != nil {
		t.Fatalf("rotation must create .1: %v", err)
	}
	// 不超过 MaxFiles 份。
	entries, _ := os.ReadDir(dir)
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "p1.log") {
			n++
		}
	}
	if n > 3 {
		t.Fatalf("kept %d files, want at most 3", n)
	}
}

// 回放：按行数、时间、流过滤。
func TestTailFilters(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(dir, "p1", Options{})
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 10; i++ {
		_ = l.Write(StreamStdout, []byte{'a' + byte(i)})
	}
	_ = l.Write(StreamStderr, []byte("e"))
	_ = l.Close()

	// 只留最后 3 条。
	recs, err := Tail(dir, "p1", TailOptions{Lines: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("lines = %d", len(recs))
	}

	// 只看 stderr。
	recs, err = Tail(dir, "p1", TailOptions{Streams: []Stream{StreamStderr}})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || string(recs[0].Data) != "e" {
		t.Fatalf("stderr filter = %+v", recs)
	}

	// since 在未来：一条都没有。
	recs, err = Tail(dir, "p1", TailOptions{Since: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Fatalf("since filter = %d records", len(recs))
	}
	// since 在过去：全都在。
	recs, _ = Tail(dir, "p1", TailOptions{Since: base})
	if len(recs) != 11 {
		t.Fatalf("since in the past = %d records", len(recs))
	}
}

// 尾部半条记录被忽略。
func TestTailIgnoresTornTail(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(dir, "p1", Options{})
	_ = l.Write(StreamStdout, []byte("good"))
	_ = l.Close()

	path := filepath.Join(dir, "p1.log")
	st, _ := os.Stat(path)
	// 追加半条记录：只有 4 字节的长度头。
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte{0, 0, 0, 13})
	_ = f.Close()
	if st.Size() == 0 {
		t.Fatal("precondition: file must have content")
	}

	recs, err := Tail(dir, "p1", TailOptions{})
	if err != nil {
		t.Fatalf("torn tail must not fail: %v", err)
	}
	if len(recs) != 1 || string(recs[0].Data) != "good" {
		t.Fatalf("records = %+v", recs)
	}
}

// 记录长度自相矛盾：报 ErrLogCorrupt。
func TestTailDetectsCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p1.log")
	// 长度头声明 8 字节，小于最小记录长度（17），说明文件被写坏。
	_ = os.WriteFile(path, []byte{0, 0, 0, 8, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 0o600)
	if _, err := Tail(dir, "p1", TailOptions{}); !errors.Is(err, ErrLogCorrupt) {
		t.Fatalf("err = %v", err)
	}
}

// Reader 把记录拼成一条流。
func TestReader(t *testing.T) {
	recs := []Record{{Data: []byte("a")}, {Data: []byte("b")}}
	body, err := ioReadAll(Reader(recs))
	if err != nil {
		t.Fatal(err)
	}
	if body != "ab" {
		t.Fatalf("body = %q", body)
	}
}

// ioReadAll 是 io.ReadAll 的薄包装，避免测试里多一个 import。
func ioReadAll(r interface{ Read([]byte) (int, error) }) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 64)
	for {
		n, err := r.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			if err.Error() == "EOF" {
				return sb.String(), nil
			}
			return sb.String(), err
		}
	}
}
