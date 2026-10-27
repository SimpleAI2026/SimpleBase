// 文件说明：id.go — internal/auth 的内部实现（newID 等）
//
// 用法
//
//	同包调用：newID() → session.go, user.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/auth/id.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] api_key_repository.go, cache.go, delegation.go, jwt.go, middleware.go 等 15 个
//	[关键引用] newID() ← session.go, user.go
//
// 主要符号
//
//	newID() string

package auth

import "github.com/google/uuid"

func newID() string { return uuid.NewString() }
