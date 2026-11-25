// 文件说明：helpers_test.go — internal/sandbox/sshd 的测试辅助代码
//
// 用法
//
//	同包调用：writeGarbage() → server_test.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sandbox/sshd/helpers_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] membuf.go, server.go, server_test.go, session.go
//	[关键引用] writeGarbage ← server_test.go
//
// 主要符号
//
//	writeGarbage(path string) error

package sshd

import "os"

// writeGarbage 把一个无法解析的内容写到密钥文件上，用来验证损坏文件不会被静默覆盖。
func writeGarbage(path string) error { return os.WriteFile(path, []byte("not a key"), 0o600) }
