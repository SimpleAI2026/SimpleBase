// 文件说明：arch.go — 架构名映射
//
// 用法
//
//	runtimeArch() string  // GOARCH（便于测试替换）
//
// 调用架构
//
//	[上游] gosandbox/image（pull、import）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/arch.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] pull.go, import.go
//
// 主要符号
//
//	runtimeArch() string

package image

import "runtime"

// runtimeArch 是 runtime.GOARCH 的可替换包装（测试里换掉）。
var runtimeArch = func() string { return runtime.GOARCH }
