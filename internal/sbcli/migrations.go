// 文件说明：migrations.go — internal/sbcli 的内部实现（cmdMigrateStatus、cmdMigratePlan 等）
//
// 用法
//
//	同包调用：cmdMigrateStatus() → commands.go；cmdMigratePlan() → commands.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sbcli/migrations.go
//	         │ 依赖
//	         ▼
//	[下游] packages/go-sdk
//	[同包] collection_transfer.go, commands.go, config.go, output.go, redact.go 等 6 个
//	[关键引用] cmdMigrateStatus() ← commands.go
//
// 主要符号
//
//	cmdMigrateStatus(ctx context.Context, c *gosdk.Client, _ loadedConfig, args ...
//	cmdMigratePlan(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string ...

package sbcli

import (
	"context"
	"io"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func cmdMigrateStatus(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	if len(args) != 0 {
		return nil, usagef("migrate status takes no arguments")
	}
	return c.MigrationStatus(ctx)
}

func cmdMigratePlan(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	if len(args) != 0 {
		return nil, usagef("migrate plan takes no arguments")
	}
	return c.MigrationPlan(ctx)
}
