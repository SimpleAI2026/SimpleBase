# Tasks

## 1. P1 文件夹上传

- [x] 1.1 控制台目录选择与相对路径批量上传，逐文件复用 50MiB 上限与 MIME 校验
- [x] 1.2 失败文件逐项报告，成功项保留；上传过程可取消
- [x] 1.3 预览弹窗增加「在站点中打开」；`packages/js-sdk` 补 `s3.site.*` / `s3.setPublic` 与测试

## 2. P2 站点能力

- [x] 2.1 `spa_fallback` 字段与站点处理逻辑（仅无扩展名路径回退）
- [x] 2.2 按项目站点出口 / 请求配额与 `site_egress` 用量统计，超限错误明确
- [x] 2.3 管理员全局下线入口与审计；下线立即 404
- [x] 2.4 `sbcli` 与 `sbdev.py` 增加 `file site` / `file public` 语义（两端对账）

## 3. 部署清理（SITE-5）

- [x] 3.1 `site deploy --prune` 列出待删对象并要求确认；非交互退出码 3
- [x] 3.2 删除范围限定前缀内，跨前缀对象拒绝删除；补用例

## 4. 文档与验证

- [x] 4.1 `docs/ops` 补站点部署、缓存、配额与下线说明；示例 README 同步
- [x] 4.2 用例：目录上传、SPA 回退、静态资源 404、配额超限、下线、--prune 确认
- [x] 4.3 `go test ./...`、`sbdev.py` 单测、前端 `typecheck` + `test`、`yarn build` 通过

## 完成情况核对（2026-11-26）

已实现（4）：
- 1.1 控制台目录选择（`webkitdirectory`）、相对路径保留、跳过 `.DS_Store` / `__MACOSX/`，逐文件复用 50MiB 与 key 校验；`S3Manager.test.ts` 覆盖。
- 1.3 预览弹窗新增「在站点中打开」；JS SDK 已有 `S3SiteInfo.mode`、`getSite` / `listPublic` / `setPublic` 与 `storage.test.ts`。
- 2.1 `sites.spa_fallback` + `SIMPLEBASE_SITES_SPA_FALLBACK`；仅无扩展名路径回退根 index，静态资源仍 404；`TestSiteSpaFallback` 覆盖。
- 2.4 新增 `sbcli site enable|disable` 与 `object public`，Go SDK 增加 `SetSite` / `SetPublic`；`sbdev.py` 已有 `site deploy` / `site unpublish` 与公开前缀语义。

- 1.2 目录上传支持取消：`AbortController` 贯通到 `api.s3.upload`，取消后停止后续文件并提示已完成数量；`S3Manager.test.ts` 覆盖。
- 3.1 `sbdev.py site deploy --prune`：先列待删清单；非交互未带 `--yes` 退出 3 且不删除；执行时只删前缀内且不在上传清单的对象。
- 3.2 `TestSiteDeploy` 覆盖前缀边界：`web/old.txt` 被删，`other/old.txt` 保留。
- 4.3 全量验证：`go test ./internal/...` 仅既有 `TestNewLocalStorageAppUsesSystemDatabase` 失败；`yarn test` 通过（97.53 / 93.31 / 91.42）；sbdev 39 个用例通过。

- 2.3 新增 `POST /v1/system/projects/:projectID/site/takedown`：admin / superadmin 才可调用，关闭该项目全部公开路径并写审计；`takedown_test.go` 覆盖管理员与普通用户两态。

- 2.2 新增 `sys_site_quotas` / `sys_site_traffic` 两张系统表与 `ProjectQuota.MaxSiteRequests` / `MaxSiteEgressBytes`；站点 handler 经 `usage.Service` 内存计数，5 秒或 64 次请求批量落库，超限返回 429 + `Retry-After: 60`。
- 4.1 `docs/ops/deployment.md` 与 `skills/simplebase/references/sites.md` 已补站点配额、每日 UTC 聚合与异常退出丢最后一批的说明。
- 4.2 新增 `internal/usage/site_test.go`（请求 / 出口 / 跨日 / 无存储降级）与 `site_traffic_test.go`（429 与字节统计）。
