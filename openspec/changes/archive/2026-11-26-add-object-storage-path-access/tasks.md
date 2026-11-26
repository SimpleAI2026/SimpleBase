# Tasks

## 1. 配置（OSP-1）

- [x] 1.1 `SitesConfig` 增加 `subdomain` / `path_prefix` / `public_base_url`，接入 YAML 与 `SIMPLEBASE_*` 环境变量
- [x] 1.2 实现 `Mode` / `EffectivePathPrefix` / `PathBaseURL` / `SubdomainBaseURL`，补表驱动测试
- [x] 1.3 校验保留前缀、`public_base_url` 格式；`base_domain` 与 `subdomain` 组合的告警
- [x] 1.4 启动日志输出 `sites mode=path|subdomain`，更新 `config.example.yaml`

## 2. 路由与 handler（OSP-2/3）

- [x] 2.1 新增 `sitePathMiddleware`，按模式只注册一个 Pre 中间件
- [x] 2.2 `siteRequest` 取代裸 pid context；`siteProjectFrom` / `isSiteRequest` 同步
- [x] 2.3 无结尾斜杠 301 带前缀；非法 pid 返回站点 404；`RawPath` 处理不二次解码
- [x] 2.4 URL 生成统一走 `siteBase(c, pid)`，发布 / 列表 / 预览字段全部按模式输出

## 3. 同源安全（OSP-4）

- [x] 3.1 路径模式站点响应增加 CSP `sandbox` 收紧项，编写浏览器验证用例
- [x] 3.2 确认控制台 Cookie 不设置 `Domain`，站点脚本无法读取控制台存储
- [x] 3.3 断言 `/v1/*`、`/console`、`/docs` 不经站点面代理

## 4. 界面与文档

- [x] 4.1 `S3Manager.vue` 只展示后端返回的站点 URL 与模式提示，不自行拼接
- [x] 4.2 `packages/js-sdk` 类型与文档补充 `site.mode` 字段
- [x] 4.3 `docs/ops` 写明两种模式的部署差异、升级注意与回滚方式

## 5. 验证

- [x] 5.1 路由边界用例：子域名 / 路径互斥、301、非法 pid、保留前缀
- [x] 5.2 安全用例：路径模式站点不能读写控制台存储或 `/v1`
- [x] 5.3 `go test ./internal/...`、`TestArchitectureGate`、前端 `typecheck` + `test` 通过

## 完成情况核对（2026-11-26）

已实现（14）：
- 配置：`internal/config/sites.go` 的 `Subdomain` / `PathPrefix` / `PublicBaseURL` / `Mode` / `EffectivePathPrefix` / `PathBaseURL` / `SubdomainBaseURL`，保留前缀与 `public_base_url` 校验；`config.example.yaml` 与环境变量已接入。
- 路由：`internal/api/filehttp/site_dispatch.go` 的 `SitePathMiddleware`、`SiteRequest`、301、非法 pid、RawPath；`internal/api/router.go` 按模式只注册一个 Pre。
- URL 与安全：`s3_site_handler.go#siteBase`；`middleware_http.go#pathSiteCSPFor`（sandbox，去 allow-same-origin）；全仓无 `SetCookie` / `Domain` 下发。
- 测试：`site_path_test.go`（ServeRules、DispatchBoundaries、CustomPrefix、SubdomainModeIgnoresPath、CSPFor）。
- 启动：`internal/app/startup.go` 的模式日志与兼容性告警。
- 前端：`S3Manager.vue` 展示 `site.mode` 与后端 URL。
- 验证：`go test ./internal/api ./internal/api/filehttp ./internal/config` 通过；前端 `typecheck` + 全量 `test` 通过。
- 验证：`go test ./internal/...` 仅 `TestNewLocalStorageAppUsesSystemDatabase` 既有失败（DDD 计划已登记），其余包全绿。

- 3.3：`TestSitePathDispatchBoundaries` 已补 `/console`、`/docs` 不被站点面代理的断言。
- 4.2：`packages/js-sdk` 新增 `S3SiteInfo`（含 `mode`）、`getSite` / `listPublic` / `setPublic`，并补 `storage.test.ts`。
- 4.3：`docs/ops/deployment.md` 新增「站点发布（对象存储）」两种模式、升级与回滚说明。
