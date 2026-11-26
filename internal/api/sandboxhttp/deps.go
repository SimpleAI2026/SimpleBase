// 文件说明：deps.go — 沙盒 / E2B 入站的窄依赖（取代整包 Dependencies）
//
// 用法
//
//	sandboxhttp.NewSandboxHandler(svc, writable, audit, store, usage, sandboxhttp.Deps{…})   // api / app 装配
//
// 调用架构
//
//	[上游] internal/api（routes_e2b.go）、internal/app（SSH 认证器）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sandboxhttp/deps.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/httpkit, internal/auth, internal/config, internal/observability, internal/systemdb, gosandbox/image
//
// 主要符号
//
//	type CatalogResolver interface {…}
//	type Deps struct {…}

package sandboxhttp

import (
	"context"

	"github.com/linkxzhou/SimpleBase/gosandbox/image"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// CatalogResolver 是 SSH 认证时解析项目租户的最小端口（catalog.Service 满足）。
type CatalogResolver interface {
	ResolveProjectTenant(ctx context.Context, projectID string) (string, error)
}

// Deps 是沙盒 / E2B 入站需要的其余依赖：只收自己用到的字段，不接收整包 Dependencies。
type Deps struct {
	Config config.Config
	// Sandbox 是项目级云沙盒服务；nil 时仅 capabilities 路由可用。
	Sandbox SandboxService
	// Audit / System / SandboxUsage 供 MountRoutes 构造 handler（与 api.Dependencies 同名字段同义）。
	Audit        httpkit.AuditService
	System       *systemdb.Store
	SandboxUsage interface {
		RecordSandbox(context.Context, string, string, int64) error
	}
	Logger   observability.Logger
	Auth     *auth.Service
	Catalog  CatalogResolver
	Images   *image.Store
	Terminal *TerminalBridge
}
