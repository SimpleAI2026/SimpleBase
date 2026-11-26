// 文件说明：projects_handler.go — 项目 handler 的同包转发面（实现已迁到 internal/api/cataloghttp，plan §6.2）
//
// 用法
//
//	type ProjectsHandler = cataloghttp.ProjectsHandler   // Dependencies 与路由注册使用
//	var NewProjectsHandler = cataloghttp.NewProjectsHandler
//	type KVInitializer = cataloghttp.KVInitializer       // 项目 KV 初始化端口（实现仍在 kv_init.go）
//	type ProjectResponse / ProjectListResponse / CreateProjectRequest（同名别名，供同包与测试使用）
//	type ProjectResponse / ProjectListResponse / CreateProjectRequest（同名别名，供同包与测试使用）
//
// 调用架构
//
//	[上游] routes_v1.go、kv_init.go
//	         │ 同包别名
//	         ▼
//	[本文件] internal/api/projects_handler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/cataloghttp
//
// 主要符号
//
//	type ProjectsHandler = cataloghttp.ProjectsHandler
//	type KVInitializer = cataloghttp.KVInitializer
//	NewProjectsHandler（函数变量）

package api

import "github.com/linkxzhou/SimpleBase/internal/api/cataloghttp"

type (
	ProjectsHandler = cataloghttp.ProjectsHandler
	KVInitializer   = cataloghttp.KVInitializer
)

var NewProjectsHandler = cataloghttp.NewProjectsHandler
