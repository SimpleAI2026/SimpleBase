// Package gofunction 提供基于 SSA 的 Go 脚本解释执行引擎。
//
// 将 Go 源码编译为 SSA（golang.org/x/tools/go/ssa）后用反射逐指令
// 解释执行，可在不调用 go build 的情况下运行用户脚本，并支持通过
// importer 包将宿主函数注入脚本环境。
//
// 基本用法：
//
//	import _ "github.com/linkxzhou/SimpleBase/gofunction/packages" // 注册标准库
//	result, err := gofunction.Run("seq-1", "package main\nfunc add(a, b int) int { return a + b }", "add", 1, 2)
//
// 详细用法、限制与已注册标准库清单见 README.md；可运行示例见 examples/。
package gofunction

import (
	"crypto/rand"
	"encoding/hex"
)

// newSequenceID 生成随机序列 ID
func newSequenceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "seq-unknown"
	}
	return "seq-" + hex.EncodeToString(buf)
}
