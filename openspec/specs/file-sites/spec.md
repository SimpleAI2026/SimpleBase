# File Sites Specification

## Purpose

定义对象存储的键布局、上传类型校验、内容下载的附件策略、子域名站点发布与站点安全边界，确保匿名可访问的静态站点面与需要登录的控制台面在来源、Cookie 与内容渲染上完全隔离。

## Requirements

### Requirement: 对象键与上传校验

对象键 MUST 使用内部生成的 UUID 与固定前缀，禁止把用户可控字符串直接拼进 S3 键。上传声明的 Content-Type 与内容嗅探不一致时 MUST 返回 415；CSV、JSON、YAML、Markdown 等纯文本子类型允许声明原类型（嗅探为 `text/plain` 视为一致），`text/html` 等仍拒绝。

#### Scenario: 伪装上传
- **WHEN** 客户端声明 `text/html` 但内容实际为其他类型，或声明与嗅探冲突
- **THEN** 返回 415，不写入对象

#### Scenario: 文本子类型
- **WHEN** 客户端以 `text/csv` 上传 CSV 内容
- **THEN** 上传成功，对象元数据保留声明的类型

### Requirement: 下载内容强制附件

控制台源上的 `/s3/objects/content` 与 `/files/:token` SHALL 对 HTML 及其他可执行内容强制 `Content-Disposition: attachment`，不得以内联方式渲染用户上传的 HTML。文件令牌 MUST 经过签名与有效期校验。

#### Scenario: 内联打开用户 HTML
- **WHEN** 客户端访问对象内容接口获取用户上传的 HTML
- **THEN** 响应以附件下载，浏览器不将其作为页面执行

#### Scenario: 令牌失效
- **WHEN** 文件令牌过期或被篡改
- **THEN** 返回 401/404，不返回文件内容

### Requirement: 子域名站点发布

启用了站点发布的项目 SHALL 通过 `<项目ID>.<base_domain>` 提供静态站点；请求在 `e.Pre` 阶段按 Host 改写到内部 `/__site/*` 路由，站点内容 MUST 来自该项目的已发布对象集合。未发布或不存在时 MUST 返回站点级 404，不得回落到控制台 SPA。

#### Scenario: 访问已发布站点
- **WHEN** 浏览器请求 `<项目ID>.<base_domain>/index.html`
- **THEN** 返回该项目已发布对象的内容，并携带站点缓存与安全头

#### Scenario: 访问未发布项目
- **WHEN** 请求的主机指向一个未发布或已下线的项目
- **THEN** 返回 404，不泄露控制台页面或其他项目内容

### Requirement: 发布前置条件

站点发布 SHALL 要求项目管理员权限并写入审计；`base_domain` 未配置时项目 MUST 不可发布（原因 `site_domain_unconfigured`），项目 ID 不能作为 DNS 标签时 MUST 返回 `project_id_not_dns_safe`。系统项目 MUST 不可发布。

#### Scenario: 缺少泛域名配置
- **WHEN** 管理员在未配置 `sites.base_domain` 的实例上发布站点
- **THEN** 返回不可发布原因，站点保持关闭

#### Scenario: 非 DNS 安全的项目 ID
- **WHEN** 项目 ID 含大写字母或非法字符时尝试发布
- **THEN** 返回 `project_id_not_dns_safe`，不上线

### Requirement: 站点与控制台安全隔离

站点响应 MUST 使用站点专用 CSP 与安全头，不得继承控制台页面的脚本权限；控制台下发的 Cookie MUST NOT 设置 `Domain=<父域>`，避免被站点子域共享。控制台路由（`/v1/*`、`/console`、`/docs` 等）MUST NOT 通过站点面暴露。

#### Scenario: 站点页面尝试读取控制台会话
- **WHEN** 站点子域中的脚本尝试访问控制台令牌或控制台接口
- **THEN** 因同源策略与 CSP 限制失败，控制台会话不受影响

#### Scenario: 控制台路径经站点访问
- **WHEN** 在站点子域请求 `/v1/projects` 或其他控制台路径
- **THEN** 按站点 404 处理，不代理到控制台 API

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

### Requirement: 站点文件夹上传

控制台 SHALL 支持一次选择整个目录上传，按相对路径保留目录结构创建对象。每个文件 MUST 沿用单文件大小上限与 Content-Type 嗅探规则；任一文件失败 MUST 报告文件名与原因，已成功文件不回滚但需在结果中列出。

#### Scenario: 上传站点目录
- **WHEN** 用户选择包含 `index.html` 与 `assets/app.js` 的目录
- **THEN** 两个对象按相对路径写入，站点可直接访问

#### Scenario: 单个文件超限
- **WHEN** 目录中某个文件超过上限
- **THEN** 该文件被拒绝，其余文件继续上传，结果列出失败文件

### Requirement: SPA 回退

站点 SHALL 支持 `spa_fallback` 开关：开启后，未命中的无扩展名路径 MUST 返回 `index.html` 内容（200）；带扩展名的静态资源未命中 MUST 返回 404，不得回退为 HTML。

#### Scenario: 刷新前端子路由
- **WHEN** 用户直接访问 `/orders/42` 且站点开启 `spa_fallback`
- **THEN** 返回 `index.html`，由前端路由渲染

#### Scenario: 缺失静态资源
- **WHEN** 请求 `/assets/missing.js` 且文件不存在
- **THEN** 返回 404，不返回 `index.html`

### Requirement: 站点配额与下线

站点 SHALL 支持按项目配置出口用量上限与请求数上限，超限时返回明确错误并记录 `site_egress` 用量；管理员 MUST 能全局下线站点，下线后立即返回 404 并写审计。

#### Scenario: 出口超限
- **WHEN** 项目站点出口用量达到配置上限
- **THEN** 后续请求返回配额错误，控制台展示用量与重置时间

#### Scenario: 管理员下线
- **WHEN** 管理员下线一个站点
- **THEN** 站点请求立即 404，审计记录操作者、项目与时间
