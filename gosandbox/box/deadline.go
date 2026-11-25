// 文件说明：deadline.go — gosandbox/box 的内部实现（setReadDeadline 等）
//
// 用法
//
//	同包调用：setReadDeadline() → session.go
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/deadline.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] backpressure_test.go, copytree.go, egress_e2e_test.go, files.go, handle.go 等 26 个
//	[关键引用] setReadDeadline ← session.go
//
// 主要符号
//
//	setReadDeadline(c net.Conn, d time.Duration) error

package box

import (
	"errors"
	"net"
	"time"
)

// setReadDeadline 设读超时。不支持超时的连接（测试用的内存管道）不算错误：
// 它们本来就不会挂住。
func setReadDeadline(c net.Conn, d time.Duration) error {
	var err error
	if d <= 0 {
		err = c.SetReadDeadline(time.Time{})
	} else {
		err = c.SetReadDeadline(time.Now().Add(d))
	}
	var nerr net.Error
	if errors.As(err, &nerr) {
		return nil
	}
	return err
}
