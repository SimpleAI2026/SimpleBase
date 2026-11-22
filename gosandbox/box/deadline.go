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
