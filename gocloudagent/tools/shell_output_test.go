// 文件说明：shell_output_test.go — gocloudagent/tools 的单元测试（TestShellOutputKeepsStdoutStderrInterleaving、TestShellExitCodeStillReported 等）
//
// 用法
//
//	go test ./gocloudagent/tools -run 'TestShellOutputKeepsStdoutStderrInterleaving|TestShellExitCodeStillReported|TestShellOutputWithinLimitIsUntouched|TestShellHugeOutputIsBoundedAndMarked'
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/tools/shell_output_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] base.go, fsutil.go, http.go, isolation_test.go, mcp.go 等 15 个
//
// 主要符号
//
//	type shellOut struct{…}
//	runBareShell(t *testing.T, cmd string) shellOut
//	TestShellOutputKeepsStdoutStderrInterleaving(t *testing.T)
//	TestShellExitCodeStillReported(t *testing.T)
//	TestShellOutputWithinLimitIsUntouched(t *testing.T)
//	TestShellHugeOutputIsBoundedAndMarked(t *testing.T)
//	TestShellHugeOutputDoesNotBufferEverything(t *testing.T)
//	TestBoundedBufferReportsFullLength(t *testing.T)

package tools

import (
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

type shellOut struct {
	OK       bool   `json:"ok"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

func runBareShell(t *testing.T, cmd string) shellOut {
	t.Helper()
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	ws := &Workspace{Root: t.TempDir(), Yolo: true}
	raw := ws.runShell(t.Context(), cmd, ws.Root)
	var out shellOut
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("not json: %s (%v)", raw, err)
	}
	return out
}

// 合并输出：stdout 与 stderr 按真实写入顺序交错出现。
func TestShellOutputKeepsStdoutStderrInterleaving(t *testing.T) {
	out := runBareShell(t, `echo one; echo two 1>&2; echo three`)
	if !out.OK || out.Output != "one\ntwo\nthree\n" {
		t.Fatalf("out=%+v", out)
	}
}

func TestShellExitCodeStillReported(t *testing.T) {
	out := runBareShell(t, `echo partial; exit 7`)
	if out.OK || out.ExitCode != 7 || !strings.Contains(out.Output, "partial") {
		t.Fatalf("out=%+v", out)
	}
}

// 上限内的输出原样保留，无截断标记；恰好等于上限也不截断。
func TestShellOutputWithinLimitIsUntouched(t *testing.T) {
	out := runBareShell(t, `head -c 1000 /dev/zero | tr '\0' 'a'`)
	if len(out.Output) != 1000 || strings.Contains(out.Output, "truncated") {
		t.Fatalf("len=%d", len(out.Output))
	}
}

// 超限输出：结果被截断并带标记，长度有界；命令即使吐出远超上限的内容也能正常结束。
func TestShellHugeOutputIsBoundedAndMarked(t *testing.T) {
	out := runBareShell(t, `head -c 5000000 /dev/zero | tr '\0' 'x'`)
	if !out.OK {
		t.Fatalf("a chatty command that exits 0 must stay ok: %+v", out)
	}
	if !strings.HasSuffix(out.Output, "\n…(truncated)") {
		t.Fatalf("missing truncation marker, tail=%q", out.Output[max(0, len(out.Output)-40):])
	}
	if len(out.Output) > ShellOutputLimit+len("\n…(truncated)") {
		t.Fatalf("len=%d exceeds limit %d", len(out.Output), ShellOutputLimit)
	}
}

// 内存上界：产生 200MB 输出时，堆增长不应与输出量成正比。
// 旧实现 CombinedOutput 先全部缓冲再截断，会把 200MB 全留在内存里。
func TestShellHugeOutputDoesNotBufferEverything(t *testing.T) {
	if testing.Short() {
		t.Skip("produces ~200MB of output")
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	peak := uint64(0)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var ms runtime.MemStats
		for {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
				runtime.ReadMemStats(&ms)
				if ms.HeapAlloc > peak {
					peak = ms.HeapAlloc
				}
			}
		}
	}()
	out := runBareShell(t, `head -c 200000000 /dev/zero | tr '\0' 'y'`)
	close(stop)
	<-done
	runtime.ReadMemStats(&after)
	if !out.OK || len(out.Output) > ShellOutputLimit+64 {
		t.Fatalf("ok=%v len=%d", out.OK, len(out.Output))
	}
	const budget = 64 << 20 // 远小于 200MB，给 GC 抖动留足余量
	if grown := int64(peak) - int64(before.HeapAlloc); grown > budget {
		t.Fatalf("heap grew by %d MiB while the command streamed 200MB; output must not be buffered unbounded", grown>>20)
	}
}

// boundedBuffer 必须报告完整输入长度（io.Writer 契约），同时只保留 limit 字节。
func TestBoundedBufferReportsFullLength(t *testing.T) {
	b := &boundedBuffer{limit: 5}
	for _, chunk := range []string{"abc", "defgh", "ijk"} {
		if n, err := b.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("Write(%q)=%d,%v", chunk, n, err)
		}
	}
	if b.String() != "abcde" {
		t.Fatalf("kept=%q", b.String())
	}
}
