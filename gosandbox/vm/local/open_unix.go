//go:build unix

package local

import "syscall"

// nonBlock 让打开 FIFO 不阻塞；调用方随后用 Stat 确认是普通文件。
const nonBlock = syscall.O_NONBLOCK
