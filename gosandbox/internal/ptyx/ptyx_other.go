//go:build !linux && !darwin

// 文件说明：ptyx_other.go — 其它平台上伪终端不可用：Open / SetWinsize 返回 ErrUnsupported
//
// 用法：host / agent 经 ptyx.Open / ptyx.SetWinsize 调用；其它平台见 ptyx_linux.go、ptyx_darwin.go。
// 调用架构：Open / SetWinsize → ErrUnsupported。
// 主要符号：Open、SetWinsize。

package ptyx

import "os"

// Open 在当前平台不可用。
func Open() (master, slave *os.File, err error) {
	return nil, nil, ErrUnsupported
}

// SetWinsize 在当前平台不可用。
func SetWinsize(*os.File, uint16, uint16) error {
	return ErrUnsupported
}
