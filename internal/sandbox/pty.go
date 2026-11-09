// 文件说明：pty.go — internal/sandbox 中 PTYDimLimit、ClampPTYDim 等符号的实现
//
// 用法
//
//	ClampPTYDim(v uint32, def uint16) uint16  // ClampPTYDim 归一化一个 PTY 维度：0 视为未指定，回落到默认值
//
// 调用架构
//
//	[上游] internal/api, internal/app, internal/sandbox/sshd
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sandbox/pty.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] errors.go, manager.go, output_hub.go, paths.go, proc.go
//
// 主要符号
//
//	const PTYDimLimit
//	ClampPTYDim(v uint32, def uint16) uint16

package sandbox

// PTYDimLimit 是 PTY 行列数的上限（终端尺寸上限，防止异常值撑爆渲染）。
const PTYDimLimit = 1000

// ClampPTYDim 归一化一个 PTY 维度：0 视为未指定，回落到默认值；
// 超过 PTYDimLimit 的截到上限。
//
// 终端尺寸规则属于沙盒领域（e2b 协议与 sshd 会话共用），因此放在本包，
// 不进入 internal/utils（那里只放零业务依赖的纯函数）。
func ClampPTYDim(v uint32, def uint16) uint16 {
	if v == 0 {
		return def
	}
	if v > PTYDimLimit {
		return PTYDimLimit
	}
	return uint16(v)
}
