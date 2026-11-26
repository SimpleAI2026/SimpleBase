# Proposal

## Why

站点发布目前只支持 `<项目ID>.<base_domain>` 子域名模式，需要泛域名解析与泛证书；很多部署（本地、内网、单域名环境）无法提供。用户需要一条不依赖泛域名的路径访问方式，同时保持现有子域名部署不变。

## What Changes

- `sites` 配置新增 `subdomain`（默认 false）、`path_prefix`（默认 `/sites`）与 `public_base_url`；环境变量 `SIMPLEBASE_SITES_SUBDOMAIN` 等覆盖。
- 路径模式：外部地址为 `<base><path_prefix>/<项目ID>/<路径>`，在 `e.Pre` 阶段按前缀改写内部 `/__site/*`；`/sites/<pid>` 301 到带斜杠形式。
- 子域名模式保持现状；两种模式互斥，只注册其中一个分流中间件。`base_domain` 已配置但未显式开启子域名时，启动打 warn 并按路径模式运行。
- 路径模式与控制台同源，必须增加站点 CSP `sandbox`、禁止下发父域 Cookie，并保证控制台路径不被站点面代理。
- 站点 URL / 访问地址由后端按当前模式生成，前端只展示。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `file-sites`: 站点发布增加路径模式、模式选择、同源隔离与 URL 生成规则。

## Impact

- 受影响代码：`internal/config/sites.go`、`internal/config/{env,yaml}.go`、`internal/api/{site_dispatch,site_handler,s3_site_handler,s3_content_handler,middleware_http,router}.go`、`internal/app`（启动日志）、`config.example.yaml`、`ui/src/pages/S3Manager.vue`、`packages/js-sdk`、`docs/ops`。
- 不改变：对象键布局、上传校验、控制台源上的附件策略、子域名模式的安全头与缓存语义。
- 兼容性：默认路径模式；依赖子域名的部署升级后必须显式设置 `SIMPLEBASE_SITES_SUBDOMAIN=1`，写入发布说明。
