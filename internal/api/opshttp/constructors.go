// 文件说明：constructors.go — 运维治理 handler 的构造函数（api 的路由注册使用；handler 类型本身不导出）
//
// 用法
//
//	NewMetricsHandler(store) / NewLogsHandler(store) / NewProjectSettingsHandler(store) / NewLLMSessionHandler(store)
//	NewQuotaHandler(usage) / NewAuditHandler(audit) / NewStorageSettingsHandler(cfg, path, logger) / NewMigrationHandler(store)
//
// 调用架构
//
//	[上游] internal/api（mountSystemRoutes / mountUsageAuditRoutes / mountProjectRoutes）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/opshttp/constructors.go
//	         │ 依赖
//	         ▼
//	[下游] 本包 handler
//
// 主要符号
//
//	NewMetricsHandler / NewLogsHandler / NewProjectSettingsHandler / NewLLMSessionHandler
//	NewQuotaHandler / NewAuditHandler / NewStorageSettingsHandler

package opshttp

import (
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func NewMetricsHandler(store *systemdb.Store) *metricsHandler { return &metricsHandler{store: store} }
func NewLogsHandler(store *systemdb.Store) *logsHTTPHandler   { return &logsHTTPHandler{store: store} }
func NewProjectSettingsHandler(store *systemdb.Store) *settingsHandler {
	return &settingsHandler{store: store}
}
func NewLLMSessionHandler(store *systemdb.Store) *llmSessionHandler {
	return &llmSessionHandler{store: store}
}
func NewQuotaHandler(svc httpkit.UsageService) *QuotaHandler { return &QuotaHandler{svc: svc} }
func NewAuditHandler(svc httpkit.AuditService) *AuditHandler { return &AuditHandler{svc: svc} }
func NewStorageSettingsHandler(cfg config.Config, path string, logger observability.Logger) *settingsStorageHandler {
	return &settingsStorageHandler{cfg: cfg, path: path, logger: logger}
}
