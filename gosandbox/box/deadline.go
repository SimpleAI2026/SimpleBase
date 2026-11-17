// 文件说明：deadline.go — 给 net.Conn 设读超时，兼容不支持超时的连接
//
// 用法
//
//	setReadDeadline(c net.Conn, d time.Duration) error  // d <= 0 表示清除超时
//
// 调用架构
//
//	[上游] gosandbox/box/session.go
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/deadline.go
//	         │
//	         ▼
//	[下游] net
//	[同包] session.go
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
