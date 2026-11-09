// 文件说明：repeat_detector.go — gocloudagent/cli 的内部实现（repeatDetector、callSig 等）
//
// 用法
//
//	type repeatDetector（包内使用）
//	type callSig（包内使用）
//	同包调用：newRepeatDetector() → runner.go；record() → runner.go
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/repeat_detector.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] client.go, command.go, compaction.go, config.go, prompt.go 等 9 个
//	[关键引用] newRepeatDetector() ← runner.go
//
// 主要符号
//
//	type repeatDetector struct {…}
//	type callSig struct {…}
//	newRepeatDetector(windowSize, maxSame int) *repeatDetector
//	(d *repeatDetector) record(toolName, args string) string

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// repeatDetector 检测重复工具调用（planv6.1 O3-7），避免 layout-config-recreation 式黑洞。
// 维护固定窗口最近 N 次调用，当同一 (tool, args_hash) 重复超阈值时返回警告信息。
type repeatDetector struct {
	window   []callSig       // 固定大小滑动窗口
	pos      int             // 下一次写入位置
	counts   map[callSig]int // 当前窗口内每个签名的计数
	maxCalls int             // 窗口大小
	maxSame  int             // 同一签名允许的最大重复次数
}

type callSig struct {
	tool string
	hash string // SHA-256(args)[:12]
}

func newRepeatDetector(windowSize, maxSame int) *repeatDetector {
	return &repeatDetector{
		window:   make([]callSig, windowSize),
		counts:   make(map[callSig]int),
		maxCalls: windowSize,
		maxSame:  maxSame,
	}
}

// record 记录一次工具调用，返回警告信息（空表示未超限）。
func (d *repeatDetector) record(toolName, args string) string {
	h := sha256.Sum256([]byte(args))
	sig := callSig{tool: toolName, hash: hex.EncodeToString(h[:])[:12]}

	// 如果窗口未满，当前位置为零值（空字符串）不会在 counts 里，直接写。
	// 如果窗口满了，当前位置有旧签名，先减计数。
	if d.pos < len(d.window) && d.window[d.pos].tool != "" {
		old := d.window[d.pos]
		d.counts[old]--
		if d.counts[old] == 0 {
			delete(d.counts, old)
		}
	}
	d.window[d.pos] = sig
	d.pos = (d.pos + 1) % d.maxCalls
	d.counts[sig]++

	if d.counts[sig] > d.maxSame {
		return fmt.Sprintf("⚠️ 检测到工具 %s（参数签名 %s）重复调用 %d 次，可能陷入循环。建议检查任务是否可行或换方向尝试。",
			toolName, sig.hash, d.counts[sig])
	}
	return ""
}
