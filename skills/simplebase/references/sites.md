# 前端/JS 编译、对象存储上传与子域名站点发布

工作区的 `dev apply` 只重放数据库、迁移、集合、云函数与定时任务；前端/JS 需要先在本地完成编译，再把静态构建产物上传到项目对象存储，打开「发布网站」与路径「外部访问」两层开关后，即可通过 `<project_id>.<base_domain>` 子域名直接运行站点。

## 1. 编译 JS 与静态站点产物

### 仓库内 `examples` 的编译步骤

`examples` 下的 JS/Node 案例（`shop`、`jd`、`iot-telemetry`）引用 `packages/js-sdk/dist/index.js`，未编译直接运行会报找不到模块：

```bash
# 1) 编译 JS SDK（tsup 输出 dist/index.js、index.cjs、index.d.ts）
cd packages/js-sdk && npm run build

# 2) Go 案例（community、ops-assistant）在仓库根编译服务端
go build ./examples/community/... ./examples/ops-assistant/...
```

注意：`packages/js-sdk/yarn.lock` 是 Yarn Classic 1.22，如需安装依赖不要在该目录运行 Yarn 4（会改写 lockfile）。`./examples/init.sh` 在未加 `--skip-build` 时也会自动检查并构建 `packages/js-sdk`。

### 前端工程编译为静态目录

将前端应用（Vite / Rollup / esbuild，或基于 `examples/<case>/web/` 抽离的纯前端工程）构建为静态产物目录（如 `dist/` 或 `web/dist/`）：

```bash
npm run build   # 确认输出目录（如 dist/）根下生成 index.html 与静态资源
```

构建时的四条硬性约定：

1. **禁止打入项目密钥**：`SIMPLEBASE_API_KEY`（`sb_live_*`）绝不能写进前端源码、`VITE_*` 环境变量或构建产物（`sbdev.py site deploy` 会在上传前扫描并拦截）。浏览器端需要调接口时：
   - 指向独立的业务 BFF 地址（如 `examples/<case>/server` 部署地址，并在 BFF 上对 `https://<project_id>.<base_domain>` 放行 CORS）；或
   - 使用终端用户认证接口 `/v1/projects/<project_id>/end-users/{signup,login,me}`，不要暴露项目级 API Key。
2. **单页应用使用 Hash 路由**：站点面当前不开启未命中路径回退 `index.html`，History 路由直接访问或刷新 `/orders` 会返回 404；请用 `#/orders`（参见 `examples/jd/web/index.html`）或多页 HTML。
3. **资源路径与发布前缀对齐**：发布到子域名根 `/` 时可用 `/assets/...` 或 `./assets/...`；发布到子目录（如 `--prefix shop/`）时，构建工具须配置 `base: './'` 或 `base: '/shop/'`。
4. **SVG 去掉 `<?xml ...?>` 声明**：服务端上传会嗅探前 512 字节并与扩展名比对；以 `<?xml` 开头的 `.svg` 会被嗅探成 `text/xml` 触发 `415 mime_mismatch`，请直接以 `<svg xmlns="...">` 开头（与 `examples/*/seed/images/*.svg` 一致）。

## 2. 前置条件检查

1. **项目 ID 必须兼容 DNS 子域标签**：恰好 **8 位小写字母、数字或 `-`**（`^[a-z0-9-]{8}$`），**首尾不能是 `-`**，且不能是 `sb-admin`（如 `ex-shop1`、`myapp001` 均可；`MyApp001` 或 `-myapp01` 会返回 `project_id_not_dns_safe`，无法发布网站）。
2. **目标实例已启用 `sites` 配置**：
   - 本地开发启动方式（`*.localhost` 现代浏览器自动解析到 `127.0.0.1`，免改 `/etc/hosts`）：
     ```bash
     SIMPLEBASE_SITES_ENABLED=true \
     SIMPLEBASE_SITES_BASE_DOMAIN=localhost \
     SIMPLEBASE_SITES_URL_SCHEME=http \
     SIMPLEBASE_SITES_URL_PORT=8080 \
     ./build.sh dev
     ```
     若后端监听其它端口（如 `18080`），同步修改 `SIMPLEBASE_SITES_URL_PORT`。
   - 云端实例：`config.yaml` 配置 `sites.enabled: true` 与 `sites.base_domain: <根域名>`（如 `simplebase.top`），配好泛解析 `*.<base_domain>`、泛域名证书，并确保反向代理**透传原始 `Host` 头**（服务端只读 `Host`，忽略 `X-Forwarded-Host`）。
3. **Key 权限**：上传对象需要 `database:write`；开启/下线站点（`PUT /s3/site`）与设置公开路径（`PUT /s3/public`）需要 **`database:admin`**（发布 Key 已具备；云助手委托令牌不可调用）。

## 3. 上传到对象存储并发布站点

### 方式一：用 `sbdev.py site` 一键上传与发布（推荐）

`sbdev.py site deploy` 按顺序执行：校验目录并扫描疑似密钥 → 调用 `simplebase object upload` 逐文件上传 → 开启项目级站点开关 → 开启目标目录前缀外部访问 → 探活子域名 URL → 输出 `subdomain:`（如 `myapp001.localhost`）与 `url:`（可直接在浏览器打开的站点地址）。

```bash
# 查看站点可发布状态、baseUrl 与当前已公开路径
python3 scripts/sbdev.py site status -C <dir>

# 本地发布到子域名根路径（如 http://myapp001.localhost:8080/）
python3 scripts/sbdev.py site deploy -C <dir> --dist dist

# 发布到指定子目录前缀（如 http://myapp001.localhost:8080/web/）
python3 scripts/sbdev.py site deploy -C <dir> --dist web --prefix web/

# 只上传构建产物到对象存储，暂不改动站点与公开开关
python3 scripts/sbdev.py site deploy -C <dir> --dist dist --no-publish

# 机器可读结果：最后一行输出 {"subdomain","base_url","url","http_status",...}
python3 scripts/sbdev.py site deploy -C <dir> --dist dist --json

# 发布到云端：先不带 --yes 预览计划（退出码 3），经用户明确同意后再加 --yes
python3 scripts/sbdev.py site deploy -C <dir> --dist dist --env-file .env.cloud
python3 scripts/sbdev.py site deploy -C <dir> --dist dist --env-file .env.cloud --yes

# 下线站点（立即使子域名全部路径返回 404，保留路径级开关记录）
python3 scripts/sbdev.py site unpublish -C <dir> [--env-file .env.cloud --yes]
```

### 方式二：用 `simplebase` CLI + 管理 API 分步执行

1. **上传文件**：`simplebase object upload` 默认以 `application/octet-stream` 上传，避免 `.js` / `.css` 因嗅探为 `text/plain` 报 415；子域名站点响应时会按文件扩展名自动补齐正确的 `Content-Type`（如 `text/javascript; charset=utf-8`、`text/css; charset=utf-8`）：
   ```bash
   set -a; . ./.env; set +a
   simplebase object upload --key index.html --file dist/index.html
   simplebase object upload --key assets/app.js --file dist/assets/app.js
   ```
2. **开启两层开关**（必须**同时打开**项目级站点开关和路径级外部访问开关，缺任一层统一返回 404；注意用 `@-` 从 stdin 传认证头，避免 Key 出现在 `ps` 与历史记录中）：
   ```bash
   # 1) 开启项目站点开关（返回 {enabled, baseUrl, publishable}）
   printf 'Authorization: Bearer %s\n' "$SIMPLEBASE_API_KEY" | \
     curl -sS -X PUT "$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID/s3/site" \
     -H @- -H 'Content-Type: application/json' -d '{"enabled":true}'

   # 2) 开启路径级外部访问（key="" 且 kind="folder" 表示整站根公开；子目录用 "web/"，必须以 / 结尾）
   printf 'Authorization: Bearer %s\n' "$SIMPLEBASE_API_KEY" | \
     curl -sS -X PUT "$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID/s3/public" \
     -H @- -H 'Content-Type: application/json' -d '{"key":"","kind":"folder","public":true}'
   ```
   也可在控制台「对象存储」页点击顶部「发布网站」，再打开对应文件或文件夹行的「外部访问」开关。

## 4. 通过子域名访问与运行验证

- **访问地址**：
  - 本地开发：`http://<project_id>.localhost:8080/`（或带前缀 `http://<project_id>.localhost:8080/<prefix>`）
  - 云端环境：`https://<project_id>.<base_domain>/`
- **命令行验证**（即使本机系统未解析 `*.localhost`，也可显式带 `Host` 头验证服务端分流与渲染）：
  ```bash
  curl -i -H "Host: ${SIMPLEBASE_PROJECT_ID}.localhost" "http://127.0.0.1:8080/"
  ```
  预期返回 `HTTP/1.1 200 OK`、`Content-Type: text/html; charset=utf-8`（HTML 为 `Cache-Control: no-cache`，JS/CSS/图片等为 `public, max-age=300`）。

## 5. 行为细节与常见问题

| 现象 / 规则 | 说明与处理 |
| --- | --- |
| 控制台下载链接打不开网页（直接变成下载附件） | 预期行为：控制台源的 `/s3/objects/content` 与 `/files/:token` 强制 HTML 走 `attachment` 防 XSS；**必须在 `<project_id>.<base_domain>` 子域名上访问**才会内联渲染并执行脚本 |
| 子域名访问返回 `404` | 逐项核对：① 请求 `Host` 是否精确为 `<8位小写project_id>.<base_domain>`（控制台域名直接请求 `/__site/*` 必返 404）；② `GET /s3/site` 中 `enabled` 是否为 `true`；③ `GET /s3/public` 是否覆盖了实际文件路径（访问 `/` 时实际查找 `index.html`）；④ 只读副本最多有 `sites.cache_ttl`（默认 30s）同步延迟 |
| `site_not_publishable`（400） | 调 `GET /v1/projects/<id>/s3/site` 看 `reason`：`sites_disabled`（服务端未开 `sites.enabled` / `base_domain`）、`project_id_not_dns_safe`（项目 ID 含大写或首尾 `-`）、`system_project`（`sb-admin` 禁止发布） |
| `mime_mismatch`（415） | 手工 `curl -F` 上传 `.js`/`.css`/`.html` 时声明了与嗅探结果冲突的 MIME（改走 `simplebase object upload` 或声明 `application/octet-stream`）；或 `.svg` 含 `<?xml` 头（去掉该头） |
| `forbidden`（403） | 开启站点或公开路径用了仅有 `database:write` 的运行时 Key；改用具备 `database:admin` 的发布 Key |
| 目录无斜杠跳转 | 访问 `/app`（无扩展名）且 `app/index.html` 已公开存在时，服务端返回 `301` 重定向到 `/app/` |
