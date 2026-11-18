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
