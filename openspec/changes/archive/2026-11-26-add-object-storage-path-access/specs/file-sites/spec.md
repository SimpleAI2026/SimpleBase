# Spec Delta

## ADDED Requirements

### Requirement: 路径模式站点访问

当 `sites.subdomain=false` 时，站点 SHALL 通过 `<base><path_prefix>/<项目ID>/<路径>` 提供，MUST 不依赖泛域名或泛证书。`<path_prefix>` 默认 `/sites`，MUST 为单段保留前缀，且 MUST NOT 与 `/v1`、`/go`、`/health`、`/metrics`、`/files`、`/console`、`/docs`、`/assets`、`/__site`、`/e2b` 冲突。请求 `/sites/<pid>`（无结尾斜杠）MUST 301 到 `/sites/<pid>/`。

#### Scenario: 通过路径访问站点
- **WHEN** 浏览器请求 `<base>/sites/<项目ID>/index.html`
- **THEN** 返回该项目已发布对象的内容，缓存与安全头按站点规则处理

#### Scenario: 缺少结尾斜杠
- **WHEN** 请求 `<base>/sites/<项目ID>`
- **THEN** 返回 301 到 `<base>/sites/<项目ID>/`，不直接返回内容

#### Scenario: 保留前缀冲突
- **WHEN** 配置的 `path_prefix` 与保留前缀相同
- **THEN** 启动校验失败并列出冲突前缀

### Requirement: 站点模式选择与升级

站点模式 MUST 在启动时确定且运行期不可切换。`subdomain=true` 时 MUST 要求 `base_domain` 才可发布；`base_domain` 已配置但 `subdomain=false` 时 SHALL 按路径模式运行并输出兼容性告警。两种模式 MUST NOT 同时启用。

#### Scenario: 升级后未显式开启子域名
- **WHEN** 旧部署配置了 `base_domain` 但未设置 `SIMPLEBASE_SITES_SUBDOMAIN=1`
- **THEN** 服务以路径模式启动，日志给出告警与开启方式，原站点 URL 不再生效

#### Scenario: 显式路径模式
- **WHEN** `subdomain=false` 且 `base_domain` 为空
- **THEN** 站点按路径模式可用，不报 `site_domain_unconfigured`

### Requirement: 路径模式的同源安全隔离

路径模式与控制台同源，站点响应 MUST 增加 CSP `sandbox`（至少限制脚本、表单与顶层导航），MUST NOT 允许站点页面读取控制台存储；服务端 MUST NOT 下发 `Domain=<父域>` 的 Cookie，控制台路径 MUST NOT 被站点面代理。项目 ID 非法时 MUST 返回站点 404，不得回落到控制台 SPA。

#### Scenario: 站点脚本读取控制台令牌
- **WHEN** 路径模式站点中的脚本尝试访问控制台 `localStorage` 或调用 `/v1`
- **THEN** 因同源策略、CSP 与路径分流限制失败，控制台会话不受影响

#### Scenario: 非法项目 ID
- **WHEN** 请求 `/sites/<非法ID>/index.html`
- **THEN** 返回站点 404，不返回控制台页面

### Requirement: 站点 URL 按模式生成

站点访问地址、列表字段与发布结果中的 URL MUST 由后端按当前模式生成：子域名模式为 `<scheme>://<pid>.<base_domain>[:port]`，路径模式为 `<public_base_url 或请求基址><prefix>/<pid>`。前端 MUST 只展示后端返回值，不得自行拼接站点地址。

#### Scenario: 路径模式发布结果
- **WHEN** 管理员在路径模式下发布站点
- **THEN** 返回的访问地址带 `/sites/<pid>/` 前缀，可直接打开

#### Scenario: 切换模式后的旧链接
- **WHEN** 实例从子域名模式切到路径模式
- **THEN** 新生成的地址使用路径形式，旧子域名地址不再由服务端生成
