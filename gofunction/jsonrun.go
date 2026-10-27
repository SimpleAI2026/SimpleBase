// 文件说明：jsonrun.go — HTTP+JSON 调用云函数所需的公共 API 门面：
//
// 用法
//
//	ValidateHTTPFuncs(source string) ([]FuncInfo, error)  // ValidateHTTPFuncs 编译源码并校验全部包级导出函数符合 HTTP 约定：
//	RunJSON(ctx context.Context, seqid, fname, source, funcName string, body []byte) ...  // RunJSON 编译并执行 funcName，body 解到唯一形参，返回值 JSON 编码
//	同包调用：RunJSON() → gofunction.go
//
// 调用架构
//
//	[上游] gofunction/bench, gofunction/cmd/gofunction, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/jsonrun.go
//	         │ 依赖
//	         ▼
//	[下游] gofunction/internal/interp
//	[同包] gofunction.go
//	[关键引用] RunJSON() ← gofunction.go
//
// 主要符号
//
//	var ErrFunctionNotFound
//	var ErrTimeout
//	var ErrBind
//	var ErrCompile
//	ValidateHTTPFuncs(source string) ([]FuncInfo, error)
//	RunJSON(ctx context.Context, seqid, fname, source, funcName string, body []byte) ...

package gofunction

import (
	"context"

	"github.com/linkxzhou/SimpleBase/gofunction/internal/interp"
)

// 错误哨兵：转发自实现包，供 handler 按 errors.Is 映射 HTTP 状态码。
var (
	// ErrFunctionNotFound 表示目标函数不存在或未导出（对外统一语义）
	ErrFunctionNotFound = interp.ErrFunctionNotFound
	// ErrTimeout 哨兵：解释器超时被 recover 包装后 errors.Is 会失配，
	// RunJSON 识别后用本错误包裹返回，供 handler 映射 504
	ErrTimeout = interp.ErrTimeout
	// ErrBind 表示请求 body 无法解到函数形参，供 handler 映射 400
	ErrBind = interp.ErrBind
	// ErrCompile 表示 invoke 时 BuildProgram 失败（保存后环境变化），
	// 供 handler 映射 500 gofunction_compile_error
	ErrCompile = interp.ErrCompile
)

// ValidateHTTPFuncs 编译源码并校验全部包级导出函数符合 HTTP 约定：
// Params==1 且 Results==1，且参数/返回类型可通过 typeChange 预演
// （拦截未导出 struct 字段、解释器不支持的类型），把问题提前到保存时。
// 错误信息列出每个不合规函数的名字与原因，可原样透传给前端。
func ValidateHTTPFuncs(source string) ([]FuncInfo, error) {
	return interp.ValidateHTTPFuncs(source)
}

// RunJSON 编译并执行 funcName，body 解到唯一形参，返回值 JSON 编码。
// body 为空视为 {}。函数不存在或未导出返回 ErrFunctionNotFound。
// 解释器超时返回 ErrTimeout（%w 包裹 context.DeadlineExceeded 语义）。
func RunJSON(ctx context.Context, seqid, fname, source, funcName string, body []byte) ([]byte, error) {
	return interp.RunJSON(ctx, seqid, fname, source, funcName, body)
}
