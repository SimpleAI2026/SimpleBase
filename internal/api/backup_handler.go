// 文件说明：backup_handler.go — 备份 handler 的同包转发面（实现已迁到 internal/api/backuphttp，plan §6.2 P5a）
//
// 用法
//
//	type BackupHandler = backuphttp.Handler   // Dependencies.BackupHandler 与路由注册使用
//	type BackupService = backuphttp.Service
//	var NewBackupHandler = backuphttp.New
//
// 调用架构
//
//	[上游] internal/app（构造）、routes_v1.go（注册）
//	         │ 同包别名
//	         ▼
//	[本文件] internal/api/backup_handler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/backuphttp
//
// 主要符号
//
//	type BackupHandler = backuphttp.Handler
//	type BackupService = backuphttp.Service
//	NewBackupHandler（函数变量）

package api

import "github.com/linkxzhou/SimpleBase/internal/api/backuphttp"

type (
	BackupHandler = backuphttp.Handler
)

var NewBackupHandler = backuphttp.New
