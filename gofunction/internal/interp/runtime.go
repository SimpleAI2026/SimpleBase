// 文件说明：runtime.go — gofunction/internal/interp 中 Runtime、NewRuntime 等符号的实现
//
// 用法
//
//	NewRuntime() *Runtime
//	(r *Runtime) Initialize(sequenceID string)  // Initialize 初始化运行时
//	同包调用：Runtime → executor.go；Initialize() → executor.go；Reset() → executor.go
//
// 调用架构
//
//	[上游] gofunction
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/internal/interp/runtime.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] call.go, compile.go, eval.go, executor.go, frame.go 等 13 个
//	[关键引用] Runtime ← executor.go
//
// 主要符号
//
//	type Runtime struct {…}
//	NewRuntime() *Runtime
//	(r *Runtime) Initialize(sequenceID string)
//	(r *Runtime) Reset()
//	... 另有 1 个顶层符号

package interp

// Runtime Go代码运行时环境
type Runtime struct {
	Program *Program

	// 系统成员
	SequenceID string
}

// NewRuntime 创建新的运行时实例
func NewRuntime() *Runtime {
	return &Runtime{}
}

// Initialize 初始化运行时。sequenceID 由调用方提供（如请求 ID）；
// 传入空字符串时自动生成。
func (r *Runtime) Initialize(sequenceID string) {
	r.SequenceID = sequenceID
	if r.SequenceID == "" {
		r.SequenceID = newSequenceID()
	}
}

// Reset 重置运行时状态
func (r *Runtime) Reset() {
	r.Program = nil
}

// Cleanup 清理运行时资源
func (r *Runtime) Cleanup() {
	// 预留：后续接入主工程可观测性时扩展
}
