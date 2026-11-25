// 文件说明：pty.go — gosandbox/vm 中 ClampPTYDim 等符号的实现
//
// 用法
//
//	ClampPTYDim(v uint32, def uint16) uint16  // ClampPTYDim 归一化一个 PTY 维度：0 视为未指定，回落到默认值；超过 PTYDimLimit 的截到上限
//
// 调用架构
//
//	[上游] gosandbox/vm/box, gosandbox/vm/fake, gosandbox/vm/local, internal/api 等 7 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/vm/pty.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] driver.go, env.go, env_test.go, errors.go, output.go 等 9 个
//	[关键引用] ClampPTYDim ← pty_test.go
//
// 主要符号
//
//	const PTYDimLimit
//	ClampPTYDim(v uint32, def uint16) uint16

package vm

// PTYDimLimit 是 PTY 行列数的上限。终端尺寸来自调用方（E2B process API、SSH 的
// window-change），异常值会撑爆渲染，所以在进入驱动前统一收敛。
const PTYDimLimit = 1000

// ClampPTYDim 归一化一个 PTY 维度：0 视为未指定，回落到默认值；超过
// PTYDimLimit 的截到上限。
//
// 规则只在这里实现一次：E2B 的 process 端点与 sshd 会话共用同一套终端尺寸语义。
func ClampPTYDim(v uint32, def uint16) uint16 {
	if v == 0 {
		return def
	}
	if v > PTYDimLimit {
		return PTYDimLimit
	}
	return uint16(v)
}
