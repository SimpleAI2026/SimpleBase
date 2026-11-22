# planv6.7 · 对象存储路径访问模式 + API 预览参数选择 + 项目内置 MCP（OSP-* / APV-* / MCP-*）

> **状态：** 待实现（2026-11-20 起草）
> **关联 plan：** `planv6.5/object-storage-sites-plan.md`（子域名站点的事实来源，本文只写增量）、`planv6.2` MCP（`legacy-plan-consolidation-plan.md` §2 锚点）
> **关联代码（已核实）：**
> - 站点：`internal/config/sites.go`、`internal/config/{env,yaml,config}.go`、`internal/api/{site_dispatch,site_handler,s3_site_handler,s3_content_handler,site_store,middleware_http,router}.go`、`ui/src/pages/S3Manager.vue`、`ui/src/services/{types,http-api}.ts`
> - API 预览：`internal/api/route_meta.go`（`buildOpenAPI`）、`ui/src/pages/ApiPreview.vue`
> - MCP：`gocloudagent/mcp/*`（客户端，已直接依赖 `modelcontextprotocol/go-sdk v1.8.0`）、`internal/agentbridge/mcp.go`（连接器 + SSRF 防护 + 策略）、`internal/api/mcp_handler.go`、`internal/systemdb/mcp_servers.go`、`ui/src/components/settings/McpServersPanel.vue`、`ui/src/pages/ModelSettings.vue`

**目标：**

1. **OSP**：对象存储「外部访问」支持两种 URL 方案，由启动时环境变量决定：
   - `SIMPLEBASE_SITES_SUBDOMAIN=1` → 子域名方式 `https://<项目ID>.<base_domain>/<路径>`（即现有实现）；
   - 默认（未设置或 `=0`）→ 路径方式 `<base>/<项目ID>/<路径>`，不需要泛域名和泛证书。
   - 控制台展示的「访问 URL」「站点 URL」都由后端按当前模式生成，前端只负责展示。
2. **APV**：API 预览里的路径参数能自动填默认值，或者从下拉框里选：`projectID` 默认取当前项目，`databaseID` / `agentID` / `collection` 等列出当前项目下的资源。
3. **MCP**：把项目级 API 封装成一个 MCP Server，地址 `<base>/v1/projects/<项目ID>/mcp`（Streamable HTTP）。它作为**内置 MCP** 自动出现在「工作台 → 模型 → MCP」里，以后其他内置 MCP 也按同一套注册表加进来。

**不做：** 自定义域名；MCP 的 OAuth 授权流程（只支持 Bearer API Key / 委托令牌）；把 `/v1` 全部路由（约 150 条）原样做成工具。

---

## 0. 现状（代码事实）

| # | 事实 | 位置 | 影响 |
| --- | --- | --- | --- |
| 1 | 站点**只有子域名一种模式**：配置了 `sites.base_domain` 就注册 `e.Pre(siteHostMiddleware)`，按 Host 把请求改写到 `/__site/*`；没配置时站点完全关闭 | `router.go:261-268`、`site_dispatch.go` | 要增加一个「模式」维度，路径模式不再依赖 `base_domain` |
| 2 | URL 生成只有 `SitesConfig.SiteBaseURL(pid)`，输出 `scheme://<pid>.<domain>[:port]`；`S3Handler.siteURL` 在这个基础上逐段做 `PathEscape` | `config/sites.go:76`、`s3_site_handler.go:69` | 改成按模式生成；路径模式要有「base」的来源 |
| 3 | `siteReason` 规定：`base_domain` 未配置 → `site_domain_unconfigured`；项目 ID 不能作为 DNS 标签 → `project_id_not_dns_safe` | `s3_site_handler.go:58` | 路径模式下这两个原因**都不适用**（项目 ID 含大写也可以放进路径），只保留 `system_project` |
| 4 | `SiteHandler.Serve` 从 `siteProjectFrom(c)` 取 pid；补斜杠重定向写死成 `"/" + rel + "/"` | `site_handler.go:40,81` | 路径模式的重定向目标必须带 `/<prefix>/<pid>` 前缀 |
| 5 | 安全头按 `routeClassFor(c)` 分类；`site` 类使用 `defaultSiteCSP`（放开 inline/eval），`X-Frame-Options: SAMEORIGIN` | `middleware_http.go:41,105` | **路径模式下用户 HTML 和控制台同源**，直接沿用 site CSP 会形成控制台源上的存储型 XSS（控制台令牌在 `localStorage`）。必须加 `sandbox`，见 §1.4 |
| 6 | SPA fallback 接管所有未匹配的 GET；`router/index.ts` 有一批 legacy 根路径重定向（`/sql`、`/data`、`/settings`……），其中 `settings` 正好 8 个字符，长度和项目 ID 一样 | `web/web.go:69`、`ui/src/router/index.ts:161-168` | 路径模式**不能直接用 `/<pid>/...` 根路径**，必须加一个固定前缀 |
| 7 | 配置只能来自 `config.yaml` + `SIMPLEBASE_*`；`setEnvBool` 用 `strconv.ParseBool`，可以识别 `1`/`0`/`true`/`false` | `config/env.go:390`、根 `AGENTS.md` 硬性约束 2 | `SIMPLEBASE_SITES_SUBDOMAIN=1` 直接走 `setEnvBool` |
| 8 | OpenAPI 由 `buildOpenAPI(e.Routes())` 从路由表实时生成；路径参数只有 `{name, in:path, required, schema:string}`，没有任何来源信息 | `route_meta.go:124` | 参数来源要么写在后端（扩展字段），要么由前端按参数名推断。本文选后端扩展字段，见 §2.1 |
| 9 | `ApiPreview.vue` 切换端点时把所有参数清空成 `''`；`resolvedPath` 只替换每个 `{name}` 的第一次出现 | `ApiPreview.vue:122,144` | 默认值在 `watch(selectedKey)` 里注入 |
| 10 | MCP 只有**客户端**：`gocloudagent/mcp` 封装了 go-sdk 客户端；宿主连接器 `agentbridge.MCPConnector` 只支持 HTTP，并且 `guardDialContext` **拒绝回环和私网地址** | `agentbridge/mcp.go:123` | 云助手不能通过 HTTP 回连本机的内置 MCP，要走进程内会话，见 §3.6 |
| 11 | 根 `go.mod` 里 go-sdk 是 `// indirect`，只有 `gocloudagent/go.mod` 直接依赖它 | `go.mod:74` | 根模块直接 import SDK 会改动 `go.mod`（违反硬性约束 4）。**MCP Server 的 SDK 封装必须放在 `gocloudagent/mcp`**，宿主只用自有类型 |
| 12 | go-sdk v1.8.0 提供 `NewServer` / `Server.AddTool` / `NewStreamableHTTPHandler(…, &StreamableHTTPOptions{Stateless, JSONResponse})` / `NewInMemoryTransports` | `$GOMODCACHE/.../go-sdk@v1.8.0/mcp/{server,streamable,transport}.go` | 服务端和进程内传输都不需要新增依赖 |
| 13 | 客户端限额：单 Server 最多 `MaxToolsPerServer=64` 个工具，单次 run 最多 `MaxToolsPerRun=128` 个，单次结果最多 `MaxResultBytes=32KiB` | `gocloudagent/mcp/limits.go` | 内置 MCP 的工具集必须精选，控制在 64 个以内 |
| 14 | `sys_mcp_servers` 存用户配置的外部 Server；`validMCPName` 限制 `[A-Za-z0-9_-]{1,64}`；工具名拼成 `mcp__<server>__<tool>`；Agent 用 `mcp:<server>[/<tool>]` 引用 | `mcp_handler.go:289`、`cloudagent/mcpaccess.go` | 内置 Server 要占用保留名，避免和用户的配置重名 |
| 15 | `/v1/projects/:projectID/mcp/servers` 已被管理 API 占用 | `router.go:696-700` | 内置 MCP 主入口用 `/mcp`（精确路径），扩展入口用 `/mcp/builtin/:name`，不和 `/mcp/servers` 冲突 |
| 16 | 项目级 KV 设置表 `sys_settings_project(project_id, key, value_json)` 已经存在 | `systemdb/schema.go:278` | 内置 MCP 的启停状态放这里，**不新增系统表** |

---

## 1. OSP · 对象存储外部访问：子域名 / 路径两种方式

### 1.1 配置（`internal/config/sites.go`、`yaml.go`、`env.go`、`config.go`、`config.example.yaml`）

`SitesConfig` 增加三个字段，其余字段语义不变：

```go
// Subdomain=true：<pid>.<base_domain> 子域名方式（planv6.5 现有实现）；
// false（默认）：<base>/<path_prefix>/<pid>/<key> 路径方式，不依赖泛域名。
Subdomain bool `yaml:"subdomain"`
// PathPrefix 是路径方式的挂载前缀，默认 "/sites"。
PathPrefix string `yaml:"path_prefix"`
// PublicBaseURL 是生成访问 URL 用的外部基址（如 https://api.example.com）；
// 空时回退 http.public_url，再空时用请求的 scheme+Host（只用于展示，见 §1.3）。
PublicBaseURL string `yaml:"public_base_url"`
```

```yaml
sites:
  subdomain: false            # SIMPLEBASE_SITES_SUBDOMAIN=1 → 子域名方式；默认路径方式
  path_prefix: /sites         # 路径方式挂载点；SIMPLEBASE_SITES_PATH_PREFIX
  public_base_url: ""         # 例 https://api.example.com；SIMPLEBASE_SITES_PUBLIC_BASE_URL
  base_domain: ""             # 仅子域名方式使用
  # url_scheme / url_port 仅子域名方式使用；index_document / not_found_document / cache_ttl / csp 两种方式共用
```

- env：`setEnvBool(&cfg.Sites.Subdomain, "SIMPLEBASE_SITES_SUBDOMAIN")`（`ParseBool` 认 `1/0/true/false`）。另外两个是普通字符串。
- 新增方法（全部是纯函数，补表驱动测试）：
  - `Mode() string`：返回 `"subdomain"` 或 `"path"`
  - `EffectivePathPrefix() string`：规范成 `/xxx` 形式，去掉结尾 `/`；空串时取 `/sites`
  - `PathBaseURL(base, pid string) string`：返回 `base + prefix + "/" + url.PathEscape(pid)`
  - 原 `SiteBaseURL` 改名为 `SubdomainBaseURL`，行为不变
- `validateSites` 增加以下校验：
  - `path_prefix` 只允许单段 `[a-z0-9_-]`，**禁止**取 `v1`、`go`、`health`、`metrics`、`files`、`console`、`docs`、`assets`、`__site`、`e2b` 等已占用的前缀（写成一张保留表，测试里和 `router.go` 的顶层前缀做对账）
  - `public_base_url` 必须是 `http(s)://host[:port][/path]`，不能带 query 或 fragment
  - `subdomain=true` 但 `base_domain` 为空时**不报错**，沿用现状：可以启动，`publishable=false`，原因是 `site_domain_unconfigured`
- 启动日志（`internal/app`）：打印一条 `sites mode=path prefix=/sites` 或 `sites mode=subdomain domain=…`。**兼容性告警**：如果 `base_domain` 非空而 `subdomain=false`，打 warn：`sites.base_domain is set but SIMPLEBASE_SITES_SUBDOMAIN is not 1; subdomain routing is disabled, using path mode`。原来依赖子域名的部署升级后要显式加上 `SIMPLEBASE_SITES_SUBDOMAIN=1`，这一点写进 release note 和 `docs/ops`。
- 模式只在启动时读取一次，运行期不可切换（这是 `config.Load` 的现有语义）。前端不读取环境变量，一律以后端返回的 `site.mode` 和 URL 为准。

### 1.2 路由分流（`site_dispatch.go`、`router.go`）

两种模式都在 `e.Pre` 阶段把请求改写到同一个内部路由 `/__site/*`，`SiteHandler` 只保留一份实现。

```go
// siteRequest 是 Pre 阶段写入 context 的分流结果。
type siteRequest struct {
	ProjectID string
	Mode      string // "subdomain" | "path"
	Mount     string // 对外挂载前缀：子域名模式为 ""，路径模式为 "/sites/<pid>"
}
```

- **子域名模式**：`siteHostMiddleware` 保持现有逻辑不变，只把 `c.Set(siteProjectCtxKey, pid)` 改成写入 `siteRequest{pid, "subdomain", ""}`。
- **路径模式**：新增 `sitePathMiddleware(cfg)`：
  1. 只处理 `GET` 和 `HEAD`，并且 `URL.Path` 必须以 `prefix + "/"` 开头；不满足就直接放行（交给 SPA 或 404）。
  2. 取下一段作为 pid，必须通过 `objectstore.ValidateProjectID`（8 位 `[A-Za-z0-9-]`，**区分大小写**，含大写字母的项目 ID 在路径模式下也能发布）。校验失败时由站点面返回统一的 404，不放行给 SPA。
  3. 请求 `/sites/<pid>`（结尾没有 `/`）时返回 301 到 `/sites/<pid>/`。
  4. 写入 `siteRequest{pid, "path", prefix+"/"+pid}`，把 `URL.Path` 改写成 `/__site/<rest>`。如果存在 `RawPath`，同样只截掉前缀，避免二次解码。
- `router.go`：
  - `sitesOn` 的条件改为：`deps.Sites != nil && deps.S3FileStore != nil && (cfg.Sites.Mode()=="path" || cfg.Sites.Configured())`
  - 按模式只注册其中一个 Pre 中间件，**不同时注册**。子域名模式下不提供路径入口，避免出现同源 HTML（§1.4）
- `siteProjectFrom` 和 `isSiteRequest` 改为从 `siteRequest` 中读取；新增 `siteRequestFrom(c)`。
- `/__site/*` 仍然要求 context 里有分流标记，否则返回 404（现有测试保留）。

### 1.3 URL 生成（`s3_site_handler.go`、`s3_content_handler.go`）

把 `h.sitesCfg.SiteBaseURL(pid)` 全部替换成 `h.siteBase(c, pid)`：

```go
func (h *S3Handler) siteBase(c echo.Context, pid string) string {
	if h.sitesCfg.Mode() == "subdomain" {
		return h.sitesCfg.SubdomainBaseURL(pid)
	}
	return h.sitesCfg.PathBaseURL(publicBaseURL(c, h.cfgHTTPPublicURL, h.sitesCfg.PublicBaseURL), pid)
}
```

- 新增 `publicBaseURL(c, httpPublicURL, override)`，放在 `internal/api/public_base.go`，§3 的 MCP 端点也复用它。优先级是：`sites.public_base_url` → `http.public_url` → `c.Scheme()+"://"+c.Request().Host`。最后一级只用于**展示**，不参与任何鉴权或路由判定；`c.Scheme()` 在 `trusted_proxies` 之外不会采信 `X-Forwarded-Proto`，所以反向代理后面的生产环境**必须**配置前两项之一（`docs/ops` 写明）。
- 涉及的调用点有四处：`siteURL`（逐段 `PathEscape`，保持不变）、`loadSite`、`listResponse`、`PutPublic`。这些函数都要多传一个 `echo.Context`，签名改动只在包内。
- `siteReason(pid)` 按模式区分：
  - 两种模式都会返回：`h.sites == nil` → `site_unavailable`（新增，替代路径模式下的 `site_domain_unconfigured`）；`sb-admin` → `system_project`
  - 只在子域名模式返回：`site_domain_unconfigured`、`project_id_not_dns_safe`
- `siteInfoDTO` 增加 `Mode string \`json:"mode"\``。这是只增不改的字段，`gocloudagent_access.go` 的解析不受影响（加一条断言）。
- `PutPublic` 中委托令牌的 `web/` 限制、入口文件大小校验与模式无关，不做改动。

### 1.4 安全（`middleware_http.go`）——路径模式的同源风险

路径模式下，用户上传的 HTML 和控制台**同源**，而控制台的 Bearer 令牌存放在 `localStorage`。如果沿用 `defaultSiteCSP`（允许 inline/eval，并且没有 sandbox），任何能发布网站的人都能写一个页面，读走访客的控制台令牌。所以：

- `routeClassFor(c)`：路径模式的请求同样返回 `"site"`，但 CSP 改用 `sitePathCSP`：

  ```text
  sandbox allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads allow-modals;
  default-src 'self' https: data: blob: 'unsafe-inline' 'unsafe-eval'; frame-ancestors 'none'; base-uri 'self'; form-action 'self' https:
  ```

  - **不加 `allow-same-origin`**：文档会得到 opaque origin，读不到控制台的 `localStorage`、IndexedDB 和 Cookie，也不能注册 Service Worker。它对 `/v1` 发出的请求是跨源请求，并且不带凭据。
  - `frame-ancestors 'none'`，同时发送 `X-Frame-Options: DENY`，防止控制台页面被用户站点嵌套，或者用户站点被嵌套后用于点击劫持。
  - `sites.csp` 的自定义值在路径模式下**只能追加，不能去掉 sandbox**：如果配置值里没有 `sandbox`，就自动在前面补上，并打一条 warn 日志。`off` 在路径模式下视为非法，启动校验直接报错。
- 代价（写进文档）：路径模式下，站点脚本不能使用自己的 `localStorage` 和 Cookie，`fetch` 同源资源时 `Origin` 为 `null`。需要完整同源能力的站点改用子域名模式。
- 残余风险：控制台 CSP 是 `script-src 'self'`，所以 `/sites/<pid>/x.js` 在理论上能被控制台加载。前提是控制台本身先存在 HTML 注入漏洞，发布网站又需要 `database:admin` 权限并会写审计。缓解措施：对路径模式下的 `.js` / `.mjs` 响应加 `Cross-Origin-Resource-Policy: same-origin` 没有意义，因为两者本来就同源。因此在 §6 风险表中登记，并在 production profile 下给出提示：**多租户或对外开放注册的实例建议使用子域名模式**。
- `TestSecurityHeadersByRouteClass` 增加路径模式用例，断言 `sandbox` 存在且不包含 `allow-same-origin`。

### 1.5 站点 handler（`site_handler.go`）

- 第 81 行的补斜杠重定向改为 `target := req.Mount + "/" + rel + "/"`；路径模式下的 `not_found_document` 逻辑不变。
- 其余逻辑（公开判定、`ValidateFileKey`、Content-Type 推导、Range/ETag/HEAD、405）完全复用，不做改动。
- 路径模式的 pid 已在 Pre 阶段校验过，handler 内的 `IsAdminProject` 判断继续保留，作为双保险。

### 1.6 前端（`ui/`）

- `services/types.ts`：`S3SiteInfo` 增加 `mode?: 'subdomain' | 'path'`；`reason` 的注释补上 `site_unavailable`。`http-api.ts` 的 `getSite` 和 `listDetailed` 映射这个字段，不新增 `any`。
- `pages/S3Manager.vue`：
  - 「访问 URL」列与站点 URL 区块的展示逻辑不变，URL 都由后端给出。
  - 站点说明文案按 `site.mode` 二选一：子域名模式显示「站点地址：`<pid>.<domain>`」；路径模式显示「站点地址：`…/sites/<pid>/`。脚本运行在隔离沙箱中，无法访问控制台登录态」。
  - 不可发布原因的 tooltip 增加 `site_unavailable` 的文案。
- `locales/{zh-CN,en-US}.ts` 的 `pages.s3.site` 下新增 `modePath`、`modeSubdomain`、`modePathHint`、`notPublishable.site_unavailable` 四个 key，两棵树同构。
- 开发态：Vite 代理只转发 `/v1`、`/health`、`/go`，`/sites` 会落到 Vite 自己的 SPA。**不改 `vite.config.ts`**（硬性约束 4），改为由 `build.sh dev` 在启动后端时导出 `SIMPLEBASE_SITES_PUBLIC_BASE_URL=http://localhost:<api-port>`，让访问 URL 直接指向后端端口。

### 1.7 其它联动

- `gocloudagent/skills/simplebase/references/site.md`、`skills/simplebase/references/{sites,cli}.md`：把「子域名站点」改成「站点（子域名或路径方式，以 `baseUrl` 为准）」，并补上 `site_unavailable`。助手不能自己拼 URL，必须使用 API 返回的 `baseUrl` 或 `publicUrl`。
- `internal/sbcli` 的 `site` 命令原样输出 API 字段，只需在 `site_test.go` 补一条 `mode=path` 的用例。
- `docs/data/` 对象存储文档的「发布网站」章节，`docs/ops/` 的部署说明（两种模式的对比表、反向代理配置、`public_base_url`）。
- 根 `AGENTS.md`「设计计划文档」段补上本文件，并注明：「站点有两种方式，默认路径方式（`/sites/<pid>/`，强制 `sandbox` CSP）；`SIMPLEBASE_SITES_SUBDOMAIN=1` 切换为子域名」。

### 1.8 测试

| 文件 | 用例 |
| --- | --- |
| `config/sites_test.go` | `Mode`；env `SIMPLEBASE_SITES_SUBDOMAIN=1/0/true/垃圾值`；`EffectivePathPrefix` 规范化；保留前缀拒绝；`public_base_url` 校验；`PathBaseURL` 转义 |
| `api/site_dispatch_test.go` | 路径模式：`/sites/abcd1234/a.html` 命中；`/sites/ABcd1234/` 命中（大小写敏感）；`/sites/abc/` 返回 404；`/sites/abcd1234` 返回 301；POST 不分流；子域名模式下 `/sites/...` 落到 SPA；路径模式下站点 Host 不分流 |
| `api/site_handler_test.go` | 路径模式下目录 301 的目标带 mount；`%2e%2e` 穿越返回 404；`sb-admin` 返回 404 |
| `api/production_entry_test.go` | 路径模式 CSP 含 `sandbox`、不含 `allow-same-origin`，`X-Frame-Options: DENY`；自定义 CSP 自动补 sandbox |
| `api/s3_site_handler_test.go` / `s3_handler_test.go` | `mode` 字段；路径模式下 `publicUrl` = `public_base_url + /sites/<pid>/<escaped>`；三级回退；含大写的 pid 可发布；`site_unavailable` |
| `ui/tests/S3Manager.test.ts` | 两种模式的说明文案；URL 原样展示和复制 |

---

## 2. APV · API 预览：参数默认值与资源下拉

### 2.1 后端：给 OpenAPI 参数标注来源（`internal/api/route_meta.go`）

参数来源在后端声明，原因有两个：一是和路由表放在一起维护，新增路由时 `route_meta_test` 能检查遗漏；二是前端不用猜 `id` 这类有歧义的参数名。

- `operationParameter` 增加扩展字段 `Source *paramSource \`json:"x-sb-source,omitempty"\``：

  ```go
  // paramSource 描述路径参数的取值来源（API 预览用于默认值与下拉）。
  type paramSource struct {
  	Kind      string   `json:"kind"`                 // project | databases | collections | tables | agents | agent-threads | cron-jobs | gofunctions | function-triggers | sandboxes | mcp-servers | api-keys | backups
  	DependsOn []string `json:"depends_on,omitempty"` // 依赖的其它参数，如 collections 依赖 databaseID
  }
  ```

- 新增解析函数 `paramSourceFor(paramName, path string) *paramSource`，用一张**有序**表匹配，前缀越具体越靠前：

  | 参数 | 路径条件 | kind | depends_on |
  | --- | --- | --- | --- |
  | `projectID` | 任意 | `project` | — |
  | `databaseID` | 任意 | `databases` | — |
  | `collection` | 含 `/data/collections/` | `collections` | `databaseID` |
  | `table` | 含 `/schema/tables/` | `tables` | `databaseID` |
  | `agentID` | 任意 | `agents` | — |
  | `threadID` | 任意 | `agent-threads` | — |
  | `jobID` | 任意 | `cron-jobs` | — |
  | `name` | `/gofunctions/` 或 `/go/{projectID}/` | `gofunctions` | — |
  | `id` | 含 `/function-triggers/` | `function-triggers` | — |
  | `id` | 含 `/backups/` | `backups` | — |
  | `serverID` | 任意 | `mcp-servers` | — |
  | `sandboxID` | 任意 | `sandboxes` | — |
  | `keyID` | 任意 | `api-keys` | — |
  | 其余（`runID`、`callID`、`planID`、`ver`、`port`、文档 `id`、`functionName` 等） | — | 不标注，前端保持自由输入 | — |

- `route_meta_test.go` 增加两类用例：一是表驱动地覆盖上表每一行；二是**对账测试**，遍历 `buildOpenAPI` 的输出，确认所有 `projectID` 参数都带有 `x-sb-source.kind=project`，防止新路由漏标。
- 这个扩展字段只给控制台使用，不改变 OpenAPI 的其它结构。`x-` 前缀符合规范，外部工具会忽略它。

### 2.2 前端：参数控件（`ui/src/pages/ApiPreview.vue` + 新 composable）

- `services/types.ts`：`OpenAPIParameter` 增加 `'x-sb-source'?: OpenAPIParamSource`，新增 `interface OpenAPIParamSource { kind: string; depends_on?: string[] }`。
- 新增 `ui/src/composables/useApiParamOptions.ts`：
  - 函数签名：`useApiParamOptions(projectId: Ref<string>)`，返回 `{ options(kind, deps): Promise<ParamOption[]> }`，其中 `interface ParamOption { value: string; label: string; hint?: string }`
  - 数据源全部复用现有的 `api.*` 方法，**不新增后端接口**：`databases.list`、`db.collections`（以实际函数名为准）、`db.schema` 的表、`agents.list`、`agentThreads.list`、`cronjobs.list`、`gofunctions.list`、`triggers.list`、`sandboxes.list`、`mcpServers.list`、`apiKeys.list`、`backups.list`
  - 按 `projectId + kind + deps` 缓存，切换项目时清空。单个数据源失败只影响自己的下拉，下拉退化为自由输入，并显示一行 `text-muted-foreground` 提示，不弹 toast
  - `project` 不发请求，直接从 `useProjectStore()` 的项目列表构造，`value` 取 id，`label` 取 name
- 控件：
  - 有 `x-sb-source` 的参数用 `components/ui/combobox`，既可以从列表选，也可以手动输入，这样能保留调试不存在 ID 时的 404 场景；没有来源的参数仍然用 `Input`
  - `option.label` 显示名称，`hint` 显示 id（等宽字体），选中后写入 `parameters[name] = value`
  - 依赖参数为空时，该下拉禁用，并提示「请先选择 databaseID」
- 默认值（在 `watch(selectedKey)` 重置参数后注入）：
  1. `project` 取当前项目 `projectStore.projectId`，没有当前项目时留空
  2. 其它 kind：**只有在选项恰好一个时**才自动填入。单项目单库的场景下 `databaseID` 总会自动填上，多选项时不替用户做选择
  3. 同一会话内记住每个 kind 上次选择的值（`sessionStorage` 键 `sb.apiPreview.params.<projectId>`），切换端点后优先复用
  4. 依赖参数变化时，清空所有依赖它的参数，并重新拉取选项
- 切换项目时（`watch(projectId)`）重新注入默认值，避免请求打到旧项目。
- 代码片段（cURL / JS / Go）与 `resolvedPath` 继续使用最终值，逻辑不变。顺带修复 §0 #9：`path.replace` 改成 `replaceAll`。
- 文案：`pages.apiPreview` 增加 `paramPick`、`paramDependsOn`、`paramOptionsFailed`、`paramCurrentProject` 四个 key，中英两棵树同构。

### 2.3 测试

- `ui/tests/ApiPreview.test.ts`：
  - `projectID` 默认为当前项目
  - 只有一个库时 `databaseID` 自动填入；有两个时不自动填
  - 选择 `databaseID` 后 `collection` 下拉启用并拉取选项，换库后 `collection` 被清空
  - 数据源失败时退化为自由输入
  - 切换项目后重新注入
  - 手动输入任意值能正常发出请求
  - `sessionStorage` 复用
  - **不对 Combobox 打桩**，断言打在真实 DOM 上
- `ui/tests/useApiParamOptions.test.ts`：缓存、按项目失效、依赖键。
- 覆盖率：新增函数必须全部覆盖，`functions` 指标不能低于当前基线 94.1%。

---

## 3. MCP · 项目 API 封装为内置 MCP Server

### 3.1 形态与地址

| 项 | 取值 |
| --- | --- |
| 端点 | `POST <base>/v1/projects/<项目ID>/mcp`（MCP Streamable HTTP，**stateless + JSON 响应**）。`<base>` 使用 §1.3 的 `publicBaseURL` |
| 扩展端点 | `POST <base>/v1/projects/<项目ID>/mcp/builtin/<name>`，留给以后其他内置 MCP；`/mcp` 等价于 `/mcp/builtin/simplebase` |
| 鉴权 | 与 `/v1` 完全一致：`Authorization: Bearer <API Key / 登录 JWT / 委托令牌>`，挂 `projectContextMiddlewareEcho`。**不接受** query 参数传令牌 |
| 方法 | 只接受 POST。GET 和 DELETE 返回 405 JSON（stateless 模式不支持 SSE 推送，也不需要会话删除） |
| 内置名 | `simplebase`。同时保留 `sb-*` 前缀给以后的内置 Server |
| 开关 | 实例级：`agent.mcp.builtin: true`（默认开启，对应 `SIMPLEBASE_AGENT_MCP_BUILTIN`）；项目级：`sys_settings_project` 中的 `mcp.builtin.<name>.enabled`（默认 true） |

选择 `/v1/projects/<pid>/mcp` 而不是根路径 `/<pid>/mcp`，原因如下：一是它和 SPA fallback、legacy 重定向互不冲突（§0 #6）；二是自动继承 `/v1` 的鉴权、限流、`api` 类安全头和 CORS；三是和需求中的 `xxx/{项目ID}/mcp` 形状一致，这里 `xxx` 就是 `<base>/v1/projects`。

### 3.2 SDK 封装放在 `gocloudagent/mcp`（新增 `server.go`）

根模块不能直接 import go-sdk，否则 `go.mod` 里的 `// indirect` 会变成直接依赖（§0 #11）。服务端封装放进 `gocloudagent/mcp`，保持这个包「不对外暴露 SDK 类型」的约定：

```go
// ServerTool 是一个服务端工具（不含 SDK 类型）。
type ServerTool struct {
	Name        string
	Description string
	InputSchema json.RawMessage // 必须是 type=object
	ReadOnly    bool            // 写入 annotations.readOnlyHint
	Destructive bool            // 写入 annotations.destructiveHint
	Handler     func(ctx context.Context, arguments []byte) (CallResult, error)
}

// ServerSpec 是一个 MCP Server 的完整定义。
type ServerSpec struct {
	Name, Version, Instructions string
	Tools                       []ServerTool
}

// NewHTTPHandler 返回 stateless + JSONResponse 的 Streamable HTTP handler。
// resolve 按请求返回 spec（宿主在这里绑定项目与主体）；返回 nil 时回 404。
func NewHTTPHandler(resolve func(*http.Request) *ServerSpec) http.Handler

// ConnectInMemory 用 InMemoryTransport 直连 spec，返回与 ConnectHTTP 同接口的 Session。
// 供宿主在进程内把内置 Server 交给云助手，不经网络、不受 SSRF 守卫影响。
func ConnectInMemory(ctx context.Context, serverID string, spec *ServerSpec) (Session, error)
```

- 实现方式：`sdkmcp.NewServer(&Implementation{Name, Version}, &ServerOptions{Instructions})`，再逐个调用 `Server.AddTool(&sdkmcp.Tool{Name, Description, InputSchema, Annotations}, handler)`。
- 限额：`NewHTTPHandler` 和 `ConnectInMemory` 在构造时检查 `len(Tools) <= MaxToolsPerServer`、单个 schema 不超过 `MaxSchemaBytes`，超出直接返回错误。这在测试期就能发现，不会等到运行时才暴露。
- 请求体上限：handler 外层套 `http.MaxBytesReader(1 MiB)`。
- 测试 `server_test.go` 用 `ConnectInMemory` 和 `httptest` 两条路径验证 `tools/list`、`tools/call`、annotations 透传、非 object schema 拒绝、超限拒绝。依赖关系不变，`GOWORK=off go test ./...` 能通过。

### 3.3 工具集（`internal/api/mcp_builtin_simplebase.go`）

每个工具都是「一条 `/v1` 路由的薄封装」。工具执行时**通过 `e.ServeHTTP` 在进程内重新发起一次真实的 HTTP 子请求**，路径固定拼成当前项目：

```go
type builtinAPITool struct {
	Name, Description string
	Method            string            // GET / POST / PUT / PATCH / DELETE
	Path              string            // 例 "/v1/projects/{projectID}/databases/{databaseID}/query"
	PathArgs          []string          // 从 arguments 取出并 PathEscape 的路径参数（不含 projectID）
	QueryArgs         []string          // 拼进 query 的参数
	BodyArg           string            // 作为 JSON body 的参数名；"" = 整个剩余 arguments；"-" = 无 body
	Schema            json.RawMessage
	ReadOnly          bool
	Destructive       bool
}
```

这样做的好处是**鉴权、`delegable` 和委托降权、`writeGate`、系统库保护、`sqlguard`、配额、审计、限流全部复用路由层的现有实现**，MCP 层不重复做任何权限判定，口径天然一致（硬性约束 5、6、7）。

首批工具共 22 个，远低于 64 个的上限。body 字段以 handler 实际定义为准，实现时逐条对照对应 `*_handler.go`：

| 工具 | 路由 | RO | Destr. |
| --- | --- | --- | --- |
| `project_info` | GET `/v1/projects/{pid}/settings` + `/quota`（两次子请求合并） | ✓ | |
| `list_databases` | GET `/databases` | ✓ | |
| `get_schema` | GET `/databases/{databaseID}/schema` | ✓ | |
| `sql_query` | POST `/databases/{databaseID}/query` | ✓ | |
| `sql_execute` | POST `/databases/{databaseID}/execute` | | ✓ |
| `list_collections` | GET `/databases/{databaseID}/data/collections` | ✓ | |
| `query_documents` | GET `/data/collections/{collection}/documents`（filter/limit/offset/sort → query） | ✓ | |
| `get_document` | GET `/documents/{id}` | ✓ | |
| `create_document` | POST `/documents` | | |
| `update_document` | PATCH `/documents/{id}` | | |
| `delete_document` | DELETE `/documents/{id}` | | ✓ |
| `aggregate_collection` | POST `/data/collections/{collection}/aggregate` | ✓ | |
| `kv_command` | POST `/kv`（只读命令由 `kv_commands.go` 判定，见下） | | |
| `list_objects` | GET `/s3/objects`（prefix、delimiter） | ✓ | |
| `get_object_link` | POST `/s3/link`（返回限时私链，不回文件正文） | ✓ | |
| `list_gofunctions` | GET `/gofunctions` | ✓ | |
| `invoke_gofunction` | POST `/go/{pid}/{name}/{functionName}` | | |
| `list_cron_jobs` | GET `/cron-jobs` | ✓ | |
| `trigger_cron_job` | POST `/cron-jobs/{jobID}/trigger` | | |
| `list_agents` | GET `/agents` | ✓ | |
| `query_logs` | GET `/logs`（level/since/limit） | ✓ | |
| `site_status` | GET `/s3/site` | ✓ | |

- **`projectID` 不是工具参数**，固定取 MCP 端点 URL 中的项目，从结构上杜绝跨项目调用。`databaseID` 可以省略，省略时自动取项目唯一的用户库（单项目单库，见硬性约束 8），所以 schema 里它不是必填项。
- 不纳入首批的内容：流式和下载类端点（`/realtime`、`/export`、`/objects/content`、agent run 的 SSE），与 `routeTryable` 的口径一致；用户、API Key、备份恢复、删库、沙盒等管理面操作；把 `/v1` 全部路由做成一个通用 `api_request` 工具（模型很难判断它的风险，策略也没法精确到工具级）。以上都列入 P2 待议。
- `kv_command` 的读写判定：工具本身标为「写」，但只读命令在 Agent 侧可以免确认。做法是在 `MCPPolicy` 中复用 `kv_commands.go` 的只读命令表做参数级判定（§3.6）。外部客户端看到的仍然是「非只读」。
- 结果映射：
  - 2xx：`CallResult.Text` = 响应 JSON 原文，服务端截断到 64 KiB，并附上 `"truncated": true` 提示。
  - 非 2xx：`IsError=true`，`Text` = `{"status":..,"code":..,"message":..,"request_id":..}`，取自 `APIErrorBody`。**不回传**请求头和令牌。
- 子请求的构造（`internal/api/mcp_dispatch.go`）：
  - 只复制外层请求的 `Authorization` 头（进程内会话不带这个头，见 §3.6），设置 `Content-Type: application/json`，并额外设置 `X-Request-ID = <外层 request_id>.<n>`，方便把日志串起来。
  - `RemoteAddr` 继承外层，保证 IP 限流仍按真实客户端计算。
  - context 带上 `mcpCallCtxKey{server, tool}`，`accessLogMiddleware` 据此补充 `via=mcp tool=<name>` 字段。审计日志照常写入，不记录参数，遵守硬性约束 3。
  - **防递归**：子请求的路径如果以 `/mcp` 结尾或包含 `/mcp/builtin/`，直接拒绝；工具表的路径白名单也有单测锁定。
  - 用 `bytes.Buffer` 和一个极简 `http.ResponseWriter` 收集响应，上限 4 MiB，超出时中断并返回 `IsError`。不要在非测试代码里引入 `httptest`。

### 3.4 HTTP 端点与注册表（`internal/api/mcp_builtin.go`、`router.go`）

```go
// builtinMCP 是一个内置 MCP Server 的注册项；新增内置 MCP 只需往 builtinMCPs 追加。
type builtinMCP struct {
	Name        string                                   // simplebase / sb-xxx
	TitleKey    string                                   // 前端文案 key
	Available   func(deps Dependencies) bool             // 依赖缺失时不出现（如沙盒未配置）
	Build       func(b *builtinMCPBuilder, pc ProjectContext) *mcp.ServerSpec
}

var builtinMCPs = []builtinMCP{simplebaseAPIMCP}
```

- `builtinMCPBuilder` 持有 `*echo.Echo`、`Dependencies` 和 `config`，负责把 `builtinAPITool` 表转换成 `[]mcp.ServerTool`。每个工具的 `Handler` 闭包绑定 dispatcher。
- 路由注册在 `NewRouter` 的**末尾**，也就是所有 `/v1` 路由已挂载、`web.Register` 之前，因为 dispatcher 需要拿到完整的 `e`：

  ```go
  p.POST("/mcp", bh.Serve, require(auth.DatabaseRead))            // = builtin/simplebase
  p.POST("/mcp/builtin/:name", bh.Serve, require(auth.DatabaseRead))
  p.Match([]string{GET, DELETE}, "/mcp", bh.MethodNotAllowed, require(auth.DatabaseRead))
  ```

  入口只要求 `database:read`，**不挂 `writeGate`**。具体的写操作由子请求各自对应的路由把关：只读实例上，只读工具照常可用，写工具返回 503。挂载方式是 `delegable(auth.DatabaseRead)`，允许云助手的委托令牌从外部访问；进程内会话本来就不走这条路由。
- `bh.Serve` 的处理流程：
  1. 实例开关关闭，或者 `name` 不在注册表里，或者 `Available` 返回 false，都返回 404。
  2. 项目级开关为 false 时返回 404。
  3. 系统项目 `sb-admin` 返回 404。系统库禁止写入，只读查询走控制台，不经过 MCP 暴露，避免误把系统表交给外部模型。
  4. 调用 `mcp.NewHTTPHandler(resolve).ServeHTTP(c.Response(), c.Request())`。spec 按 `(pid, name)` 缓存 60 秒；工具闭包不保存主体信息，主体只从子请求的 `Authorization` 头里来。
- `route_meta.go`：让 `publicAPIRoute` 排除 `/mcp` 和 `/mcp/builtin/*`。原因是 API 预览没法用普通的 JSON 请求调试 MCP。这些端点单独在 MCP 面板里展示，并提供「测试」按钮。

### 3.5 「工作台 → 模型 → MCP」展示与管理

**后端 API**（扩展 `mcp_handler.go`，只增字段）：

- `GET /mcp/servers` 的响应在 `servers` 之外**追加** `builtin` 数组，`servers` 本身不变：

  ```jsonc
  {
    "servers": [ /* 用户配置，不变 */ ],
    "builtin": [{
      "name": "simplebase", "builtin": true, "enabled": true,
      "transport": "http", "url": "https://api.example.com/v1/projects/abcd1234/mcp",
      "tools": [{ "name": "sql_query", "read_only": true, "destructive": false }, ...]
    }]
  }
  ```

- `PUT /mcp/builtin/:name/state`，请求体 `{enabled}`，需要 `database:write` + `writeGate`，写入 `sys_settings_project`，并写审计事件 `mcp.builtin.enable` / `mcp.builtin.disable`。
- `POST /mcp/builtin/:name/test` 直接在进程内调用 `ConnectInMemory`，再执行 `ListTools`，返回 `{ok, tools}`。这样不依赖 `agent.mcp.enabled`。
- **保留名**：`validMCPName` 之外再加 `reservedMCPName(name)`，拒绝 `simplebase` 和 `sb-*`（不区分大小写）。`Create` 和 `Update` 都要校验，返回 `400 mcp_name_reserved`，这个哨兵要加进 `sentinelErrorMappings`。如果已有用户配置占用了这些名字，`List` 中仍然返回它，但加上 `"name_conflict": true`。Agent 引用时内置 Server 优先，并在启动时打 warn。

**前端**（`McpServersPanel.vue` + 新组件 `McpBuiltinServers.vue`）：

- 面板顶部新增「内置」区块，放在现有用户表格之上，使用 `SbListCard`。每一行包含：
  - 名称，加 `Badge` 显示「内置」
  - 端点 URL（等宽字体，带复制按钮）
  - 工具数，点击后展开只读工具清单，带 RO/破坏性标记
  - 启用开关（真实的 `Switch`，用 `modelValue`；`!auth.canWrite` 时禁用）
  - 「测试」按钮
  - 「接入配置」按钮：打开 `SbModal`，给出 Claude Desktop / Cursor / 通用 `mcp.json` 三种片段：

    ```json
    { "mcpServers": { "simplebase": { "url": "<url>", "headers": { "Authorization": "Bearer <SIMPLEBASE_API_KEY>" } } } }
    ```

    片段里**只放占位符，不嵌入真实令牌**。旁边链接到「设置 → 连接」签发 API Key，并建议签发只读 Key。
- 内置行**不提供**编辑和删除。
- `services/types.ts` 增加 `interface McpBuiltinServer`、`interface McpBuiltinTool`；`http-api.ts` 的 `mcpServers.list` 改成返回 `{ servers, builtin }`，调用方同步修改。同时新增 `setBuiltinEnabled` 和 `testBuiltin`，映射函数沿用 `toMcpServer` 的风格。
- 文案新增 `components.mcpServers.builtin.*`：title、desc、badge、endpoint、copy、tools、configure、configureDesc、testOk、disabled。注意 JSON 片段里没有 `@`，但如果出现邮箱示例，需要转义。
- `AgentFormModal` 的 MCP 选择器中，内置 Server 显示在最前面，引用写法是 `mcp:simplebase` 或 `mcp:simplebase/sql_query`。

### 3.6 云助手使用内置 MCP（进程内，P1）

`agentbridge.MCPConnector` 的 SSRF 守卫会拒绝回环地址（§0 #10），而且内置 Server 本来就不应该绕网络一圈，所以走进程内会话：

- `agentbridge` 新增接口（**不 import `api`**）：

  ```go
  // BuiltinMCP 由宿主 api 层实现：按主体与项目构造进程内内置 Server。
  type BuiltinMCP interface {
  	Spec(ctx context.Context, p auth.Principal, projectID, name string) (*mcp.ServerSpec, bool)
  	Policy(name, tool string, arguments []byte) (cloudagent.CommandDecision, bool)
  }
  ```

- `MCPConnector.Connect` 的处理：`name` 命中内置名时调用 `mcp.ConnectInMemory(ctx, name, spec)`，否则沿用现有的 HTTP 分支。
- 主体传递：进程内会话没有 `Authorization` 头。dispatcher 把 `auth.Principal` 放进子请求 context（`internalPrincipalCtxKey`，未导出的 key 类型），鉴权中间件**优先读取这个 context 值**。外部请求没法伪造 context，因此不存在头部注入。这个主体是 `ToHostPrincipal` 转换后的委托身份，`Delegated=true`，所以 `delegable` 和只读降权在子请求上照样生效，与 CLI 工具的口径一致（硬性约束 7）。
- `MCPPolicy.Classify`：内置名优先查 `BuiltinMCP.Policy`，事实来源是工具表的 `ReadOnly` / `Destructive`，再加上 `kv_command` 的参数级判定，**不读** `tools_policy_json`。未知工具按「写」处理（fail-closed）。
- `internal/app/assemble_agent.go`：`a.cfg.Agent.MCP.Enabled || a.cfg.Agent.MCP.Builtin` 任一为真时就装配连接器，并通过 `SetBuiltin(impl)` 注入实现。因为 api 在 app 中晚于 runtime 创建，这里采用 setter 注入，`Close` 时无需释放。
- 内置 MCP 和现有 `simplebase` CLI 工具有能力重叠。默认**不**给 Agent 自动挂载，用户需要在 Agent 表单里显式勾选。系统提示词里也不额外宣传。

### 3.7 配置（`internal/config`）

```yaml
agent:
  mcp:
    enabled: false   # 现有：外部 MCP 客户端
    builtin: true    # 新增：项目 API 封装的内置 MCP Server；SIMPLEBASE_AGENT_MCP_BUILTIN
```

- `yaml.go`、`env.go`、`config.go`（默认值为 true）、`config.example.yaml` 同步修改；`profile.go` 不做特殊处理。
- 关闭时：`/mcp` 返回 404，`GET /mcp/servers` 的 `builtin` 为 `[]`，前端不渲染内置区块。

### 3.8 测试

| 文件 | 用例 |
| --- | --- |
| `gocloudagent/mcp/server_test.go` | 见 §3.2 |
| `api/mcp_builtin_test.go` | 未鉴权返回 401；无项目权限返回 403/404；`sb-admin` 返回 404；实例开关和项目开关关闭时返回 404；未知 name 返回 404；GET 返回 405；`tools/list` 恰好 22 个工具且 annotations 正确；用真实 go-sdk 客户端（`ConnectHTTP` + `httptest.Server`）跑通 initialize → list → call |
| `api/mcp_dispatch_test.go` | 子请求鉴权口径：read Key 调 `sql_execute` 返回 `IsError` 403；**`writable=false` 时写工具返回 503、读工具正常**；**系统库拒绝**；委托令牌 + 只读降权；`projectID` 不能通过参数覆盖（传 `{"projectID":"other"}` 会被忽略）；防递归；结果截断；错误体不含令牌；`X-Request-ID` 串联 |
| `api/mcp_handler_test.go` | `builtin` 字段；保留名拒绝；`name_conflict`；`PUT state` 的 writeGate、只读实例返回 503 和审计；`test` |
| `api/route_meta_test.go` | `/mcp` 不出现在 OpenAPI 中；工具表路径都能在 `e.Routes()` 里找到（对账测试，防止路由改名后工具失效） |
| `agentbridge/mcp_test.go` | 内置名走进程内会话，不经过 `guardDialContext`；`Classify` 对内置工具的判定，以及 `kv_command` 只读参数 |
| `ui/tests/McpServersPanel.test.ts` | 内置区块渲染、复制 URL、开关成功与失败回滚、测试、配置弹窗不含真实令牌、无写权限时只读、`builtin=[]` 时隐藏 |

---

## 4. 风险与决策

| 风险 / 决策点 | 说明 | 处理 |
| --- | --- | --- |
| 默认模式从子域名改为路径（**行为变更**） | 已经配置了 `base_domain` 的部署，升级后访问 URL 会变成 `/sites/<pid>/`，旧的子域名链接返回 SPA 页面 | 启动 warn（§1.1）+ release note + `docs/ops` 升级说明；需要保留子域名的部署设置 `SIMPLEBASE_SITES_SUBDOMAIN=1` |
| 路径模式同源 XSS | 用户 HTML 和控制台同源 | 强制 `sandbox`（不含 `allow-same-origin`）+ `frame-ancestors 'none'`；自定义 CSP 时自动补 sandbox；`csp=off` 非法（§1.4）；多租户场景建议使用子域名模式 |
| 路径模式脚本能力受限 | opaque origin 下没有 `localStorage` 和 Cookie | 在文档和控制台提示里说明；需要完整能力的站点改用子域名模式 |
| `public_base_url` 未配置 | 回退到请求的 Host，在反向代理后面可能拿到内网地址 | 只影响展示，不影响路由和鉴权；production profile 下 `public_base_url` 和 `http.public_url` 都为空时给出 warn |
| 内置 MCP 绕过权限 | MCP 层如果自己做判定，可能和路由层口径不一致 | 全部走 `e.ServeHTTP` 子请求，复用路由层中间件；MCP 层只拼路径，`projectID` 从 URL 固定取得 |
| 进程内主体注入 | context 主体可能被伪造 | key 类型未导出，只有 dispatcher 能写；外部请求没有入口；单测锁定 |
| 子请求递归 / 放大 | 工具调用 `/mcp` 自身 | 路径白名单 + 前缀拒绝；子请求继承外层 IP 限流；响应 4 MiB 上限 |
| 外部模型拿到写能力 | 用户把可写 Key 填进第三方客户端 | 接入配置弹窗建议使用只读 Key；写工具带 `destructiveHint`；只读实例上写工具统一返回 503 |
| 工具集膨胀 | 超过客户端 64 个工具的上限 | 首批 22 个；`NewHTTPHandler` 构造期检查；新增工具需要更新本文 §3.3 的表 |
| 用户 Server 与内置重名 | 历史数据中可能已有 `simplebase` | 保留名校验 + `name_conflict` 标记 + 内置优先 |
| API 预览下拉的请求量 | 每切换一次端点就拉一次资源 | 按项目和 kind 缓存，只在第一次用到时拉取；失败时退化为自由输入 |

---

## 5. 分期

| 阶段 | 内容 | 验收 |
| --- | --- | --- |
| **P0** | OSP §1.1–1.6（配置、双模式分流、URL 生成、sandbox CSP、前端文案）；APV §2.1–2.2；MCP §3.1–3.5、§3.7（HTTP 端点、22 个工具、模型页内置区块、保留名、开关） | §6 全部命令通过；手工 E2E：①路径模式下上传 `demo/index.html` 并公开，访问 `/sites/<pid>/demo/` 返回 200，响应头带 sandbox CSP；`SIMPLEBASE_SITES_SUBDOMAIN=1` 重启后 URL 变成子域名并且能访问。②API 预览中 `projectID` 和 `databaseID` 自动填入，`collection` 下拉可选。③用 Claude Desktop 或 `npx @modelcontextprotocol/inspector` 连接 `/v1/projects/<pid>/mcp`，`sql_query` 能正常执行 |
| **P1** | MCP §3.6 云助手进程内会话与策略；OSP §1.7 技能文档、sbcli 用例、docs；`build.sh dev` 导出 `public_base_url` | Agent 勾选 `mcp:simplebase` 后能调用 `sql_query`；写工具需要确认；只读 Agent 被拒绝 |
| **P2** | 更多内置 MCP（`sb-sandbox`：沙盒 exec/文件；`sb-docs`：文档检索）；按 Key 细化的工具白名单；MCP 调用用量计入 `usage`（`mcp_call`）；OAuth 2.1 授权（单独立项） | — |

---

## 6. 验证命令

```bash
go build ./internal/... ./cmd/...
go test ./internal/config/ ./internal/api/ ./internal/agentbridge/ ./internal/sbcli/ ./internal/app/
go test ./...
(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...)
gofmt -l internal gocloudagent | grep -v thirdparty   # 必须为空
cd ui && yarn typecheck && yarn test && yarn build
./scripts/smoke.sh
```

`git diff go.mod gocloudagent/go.mod` 必须为空：不新增依赖，也不把 go-sdk 提升为根模块的直接依赖。

---

## 7. 改动文件清单

| 文件 | 改动 | 阶段 |
| --- | --- | --- |
| `internal/config/{sites,yaml,env,config}.go`、`sites_test.go`、`config.example.yaml` | `subdomain` / `path_prefix` / `public_base_url`；`agent.mcp.builtin` | P0 |
| `internal/app/*` | 启动日志与兼容告警；P1 注入 `BuiltinMCP` | P0/P1 |
| `internal/api/site_dispatch.go` | `siteRequest`、`sitePathMiddleware` | P0 |
| `internal/api/site_handler.go` | 补斜杠重定向带 mount | P0 |
| `internal/api/s3_site_handler.go`、`s3_content_handler.go` | `siteBase`、`siteReason` 按模式区分、`mode` 字段 | P0 |
| `internal/api/public_base.go`（新） | `publicBaseURL` | P0 |
| `internal/api/middleware_http.go` | `sitePathCSP`、强制 sandbox | P0 |
| `internal/api/route_meta.go` | `x-sb-source`；排除 `/mcp` | P0 |
| `internal/api/mcp_builtin.go`（新）、`mcp_builtin_simplebase.go`（新）、`mcp_dispatch.go`（新） | 注册表、工具表、子请求分发 | P0 |
| `internal/api/mcp_handler.go` | `builtin` 字段、state/test 接口、保留名 | P0 |
| `internal/api/error.go` | `ErrMCPNameReserved → 400 mcp_name_reserved` | P0 |
| `internal/api/router.go` | 按模式挂载站点中间件；`/mcp` 路由（放在 `/v1` 全部挂载之后） | P0 |
| `internal/api/accesslog`（`accessLogMiddleware` 所在文件） | `via=mcp tool=` 字段 | P0 |
| 鉴权中间件（`authMW` 所在文件） | 优先读取 `internalPrincipalCtxKey` | P1 |
| `gocloudagent/mcp/server.go`（新）+ `server_test.go` | `ServerSpec` / `NewHTTPHandler` / `ConnectInMemory` | P0 |
| `internal/agentbridge/mcp.go` | 内置名走进程内会话；`Classify` 内置优先 | P1 |
| `ui/src/services/{types,http-api}.ts` | `S3SiteInfo.mode`、`OpenAPIParamSource`、`McpBuiltin*` | P0 |
| `ui/src/pages/S3Manager.vue` | 模式文案 | P0 |
| `ui/src/pages/ApiPreview.vue`、`composables/useApiParamOptions.ts`（新） | 参数下拉与默认值 | P0 |
| `ui/src/components/settings/McpServersPanel.vue`、`McpBuiltinServers.vue`（新） | 内置区块、接入配置弹窗 | P0 |
| `ui/src/components/modal/AgentFormModal.vue` | 内置 Server 置顶 | P1 |
| `ui/src/locales/{zh-CN,en-US}.ts` | 新增文案（两棵树同构） | P0 |
| `ui/tests/*` | §1.8 / §2.3 / §3.8 | P0 |
| `gocloudagent/skills/simplebase/references/site.md`、`skills/simplebase/references/{sites,cli}.md` | 两种模式与 `site_unavailable` | P1 |
| `docs/data/`、`docs/ops/` | 发布网站两种模式、MCP 接入 | P1 |
| 根 `AGENTS.md` | 「设计计划文档」段补上本文件；站点默认路径模式且强制 sandbox；内置 MCP 只经子请求复用路由鉴权 | P0 |
