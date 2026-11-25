// 文件说明：arch.go — gosandbox/image 的内部实现（runtimeArch 等）
//
// 用法
//
//	同包调用：runtimeArch → import.go
//
// 调用架构
//
//	[上游] gosandbox/vm/box, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/arch.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] import.go, manifest.go, manifest_test.go, pull.go, pull_test.go 等 16 个
//
// 主要符号
//
//	var runtimeArch

package image

import "runtime"

// runtimeArch 是 runtime.GOARCH 的可替换包装（测试里换掉）。
var runtimeArch = func() string { return runtime.GOARCH }
