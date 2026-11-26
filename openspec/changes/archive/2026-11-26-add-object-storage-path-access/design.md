# Design

## 路由分流

```go
type siteRequest struct {
    ProjectID string
    Mode      string // subdomain | path
    Mount     string // 子域名 ""；路径 "/sites/<pid>"
}
```

- 子域名：现有 `siteHostMiddleware` 保持，只把结果改写入 `siteRequest`。
- 路径：新增 `sitePathMiddleware(cfg)`：只处理 GET/HEAD；`URL.Path` 以 `prefix + "/"` 开头才接管；取下一段为 pid 并校验（8 位 `[A-Za-z0-9-]`，区分大小写）；无结尾斜杠 301；改写 `URL.Path`（含 `RawPath`）到 `/__site/<rest>`。
- `router.go` 按模式只注册一个 Pre 中间件；`/__site/*` 仍要求 context 中有分流标记。

## 配置

```yaml
sites:
  subdomain: false
  path_prefix: /sites
  public_base_url: ""
  base_domain: ""
```

- `Mode()`、`EffectivePathPrefix()`、`PathBaseURL(base, pid)`、`SubdomainBaseURL(pid)` 均为纯函数并补表驱动测试。
- 保留前缀表与 `router.go` 顶层前缀对账；`public_base_url` 只允许 `http(s)://host[:port][/path]`，禁止 query/fragment。

## 同源安全

路径模式与控制台同源，必须同时做三件事：

1. 站点响应 CSP 增加 `sandbox allow-scripts allow-same-origin` 之外的收紧项（禁止表单与顶层导航；具体指令以浏览器实测为准）。
2. 控制台 Cookie 不设置 `Domain`，避免任何子域共享；路径模式本身不产生子域。
3. 站点分流只接管前缀内的 GET/HEAD，其他请求原样交给控制台；非法 pid 返回站点 404，不回 SPA。

## URL 与缓存

- `siteBase(c, pid)` 统一按模式生成；`S3Handler.siteURL` 在其后逐段 `PathEscape`。
- 路径模式的重定向目标带 `Mount` 前缀；`SiteHandler.Serve` 的补斜杠逻辑改为基于 `siteRequest.Mount`。
- 静态资源缓存沿用 `cache_ttl`；`index.html` 与 `config.js` 的缓存策略不变。

## 回滚

配置回退到 `subdomain=true` 并恢复泛域名即可；路径模式不写入任何持久化状态，回滚无需数据迁移。
