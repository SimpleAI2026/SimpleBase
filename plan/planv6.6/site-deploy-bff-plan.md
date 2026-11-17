# 静态站点发布与 BFF 示例的衔接修正计划（v6.6）

> 起因：对 `skills/simplebase`（`site deploy` 流程）与 `examples/jd` 做了端到端逻辑分析，结论是「构建 → 上传对象存储 → 子域名访问」的**流程本身通顺，但 jd 并不是纯静态站点**，照做只会得到一个能打开、数据全部加载失败的空壳页面。
> 承接 `plan/planv6.5/object-storage-sites-plan.md`（站点面只读、`e.Pre` 按 Host 改写到 `/__site/*`）。本计划只登记**本轮发现**的问题，不改站点面的服务端语义。

## 0. 问题与证据

### P1 jd 前端依赖同源 BFF，站点面无法承载（高）

- `examples/jd/web/index.html:235` 为 `const API = ''`，40 多处请求是相对路径（`/api/home`、`/api/cart`、`/api/orders` 等）。
- 站点面 `internal/api/site_handler.go:45` 只放行 `GET` / `HEAD`，其余返回 405；只按对象 key 取文件，没有 `/api/*`。
- 发布到 `ex-jd001.<base_domain>` 后：页面可加载，所有 `/api/*` 为 404，POST 为 405。
- 业务逻辑（算价、下单事务、幂等、KV 抢锁）在 `examples/jd/server/main.mjs`，API Key 只在 BFF，**不能下放到前端**，所以这部分不可能放进对象存储。
- `examples/shop` 同样的结构（`web/index.html:61` 相对路径 `fetch`），问题相同。

### P2 现有「指向独立 BFF + CORS」建议在 jd / shop 上走不通（高）

`references/sites.md` §1 建议「指向独立 BFF 并放行 CORS」，但：

- `jd/server/main.mjs:710`、`shop/server/main.mjs:149`：有 `Origin` 且与 `Host` 不一致就直接 `Invalid origin`。站点域名与 BFF 必然跨源，请求全被拒。
- BFF 没有任何 `Access-Control-*` 头，也不处理 `OPTIONS` 预检（`idempotency-key` 请求头必触发预检）。
- 前端 `API` 写死，无注入基址的办法。
- 控制台侧已有 CORS 中间件（`internal/api/middleware_http.go` 的 `corsMiddleware`），但只作用于 SimpleBase 服务本身，与示例 BFF 无关。

### P3 jd 没有「构建」这一步，文档口径易误导（中）

- `jd/web/` 只有一个 `index.html`，没有 `package.json`，没有产物目录。
- `SKILL.md` 步骤 4 的「先编译 `packages/js-sdk`」只服务 `server/main.mjs`（它 import `js-sdk/dist/index.js`），与前端静态产物无关；`sites.md` §1 把 `shop` / `jd` / `iot-telemetry` 与「前端静态产物」并列，容易让人以为 jd 的前端依赖 SDK 或可直接发布。

### P4 `site deploy` 默认公开整个对象根（中，安全面）

- 不带 `--prefix` 时 `sbdev.py:1385` 调用 `PUT /s3/public {key:"", kind:"folder"}`，匿名公开**整个项目对象存储根**。
- jd 的对象还包括 `jd/` 商品图、`jd/reviews/*` 用户晒图，以及 `/api/platform/upload` 可写入的任意 key，全部会被一并公开。
- 加 `--prefix web/` 可收窄，但访问路径变成 `/web/`，不是站点根。

### P5 其他（低）

- `site deploy` 只上传不清理：产物带 hash 文件名时，旧文件会留在公开前缀下继续可访问。
- 文档未说明「站点面只读、没有后端接口，页面只能调用外部 BFF 或 `end-users` 接口」。

---

## 1. 修复项

### SITE-1 jd / shop 前端支持可配置的 BFF 基址（P0）

- `web/index.html` 的 `const API = ''` 改为读 `window.__SB_CONFIG__?.apiBase ?? ''`；`index.html` 在主脚本前加载同目录 `./config.js`（相对路径，兼容站点根与子目录前缀）。
- 新增 `web/config.js`，默认内容 `window.__SB_CONFIG__ = { apiBase: '' }`（同源模式行为不变）；**不含任何密钥**，只放 BFF 公开地址。
- BFF 为 `GET /config.js` 返回同样的空配置（`jd` 的 `handle` 现在只服务 `/` 与 `/index.html`），保证原有「本地 BFF 同时托管前端」的演示路径不受影响。
- 发布时由使用者把 `config.js` 改成 BFF 的公网地址；不引入构建工具、不新增依赖。

### SITE-2 jd / shop BFF 增加 CORS：Origin 使用 `*`，放行全部方法与请求头（P0）

决策（2026-11-17）：示例 BFF 的 CORS 直接放开，Origin 用通配符 `*`，不做白名单；方法与请求头也全部允许。

- **移除 Origin 拦截**：删除 `jd/server/main.mjs:710`、`shop/server/main.mjs:149` 的 `Invalid origin` 判断（含 `sec-fetch-site: cross-site` 拒绝），不再引入 `*_ALLOWED_ORIGINS` 环境变量。
- **所有响应**（含错误响应与 404）统一回写：
  - `Access-Control-Allow-Origin: *`
  - `Access-Control-Allow-Methods: *`（兼容旧浏览器时同时写 `GET,POST,PUT,PATCH,DELETE,OPTIONS`）
  - `Access-Control-Allow-Headers: *`；预检请求带 `Access-Control-Request-Headers` 时**原样回显**，覆盖 `idempotency-key`、`content-type` 以及自定义头（`*` 在规范里不覆盖 `Authorization`，回显可兜住）
  - `Access-Control-Expose-Headers: *`
  - `Access-Control-Max-Age: 600`
- **不设 `Access-Control-Allow-Credentials`**：与 `*` 互斥，示例本来就不用 Cookie，前端 `fetch` 也不带 `credentials`。
- 处理 `OPTIONS` 预检，直接 204，不进入业务路由、不要求 `Idempotency-Key` 等业务头。
- 把 CORS 头与预检封成 BFF 内的一个小函数，`jd` 与 `shop` 各自内联一份（不新增跨示例共享模块、不新增依赖）。
- 补离线测试：普通 GET / POST 带 CORS 头、预检 204 且头齐全、预检回显自定义头、错误响应与 404 也带 CORS 头、无 `Origin` 的同源请求行为不变。
- **安全影响（必须在 README 写明）**：原 Origin 校验同时承担了对本地 BFF 的 CSRF 防护；放开后，任意网页都能让用户浏览器调用该 BFF。示例是单用户、无登录（`user-demo-0001`），因此**只适合本地与演示环境**，不得绑定公网地址上承载真实数据（见 SITE-3）。

### SITE-3 BFF 公网暴露的风险收口（P1）

CORS 本来就不是鉴权；SITE-2 又放开为 `*` 且去掉了 Origin 校验，BFF 一旦公网可达，任何网页和 curl 都能直接调用。jd 是单用户演示（`user-demo-0001`）。

- `jd` 的写入类平台接口（`/api/platform/emit-event|upload|invoke-trigger|run-cron`）加开关 `JD_PLATFORM_WRITE=1` 才放行，默认关闭，关闭时返回 403（CORS 头照常带上，便于前端展示错误）。
- BFF 默认仍只监听 `127.0.0.1`（现状）；README 说明改成 `0.0.0.0` 或反代到公网的后果。
- README「限制」补充：`Access-Control-Allow-Origin: *` 仅用于本地与演示；生产应加终端用户登录（`end-users`）并由 BFF 校验，同时收紧为白名单。

### SITE-4 `site deploy` 对象根公开的防护（P1）

- `sbdev.py` 在不带 `--prefix` 且非 `--no-publish` 时，先 `object list --prefix ""`，若存在**不在本次上传清单内**的对象，输出警告并列出前缀汇总（如 `jd/`：38 个、`jd/reviews/`：N 个）。
- 远端（`--env-file`）预览阶段（退出码 3）的计划输出里同样显示「将公开的非产物对象」，让用户在 `--yes` 之前能看到；本地不阻断，只警告。
- 文档推荐：存在业务对象的项目用 `--prefix web/`；站点必须落在根时，把业务对象放到另一个前缀并确认其未命中公开规则。
- 不新增公开语义：不改 `PUT /s3/public`，只做提示。需要时补 `--public-only-uploaded` 的方案（逐文件公开）留作后续评估。

### SITE-5 `site deploy` 清理旧产物（P2，可选）

- 新增 `--prune`：上传完成后，列出 `prefix` 下不在本次清单内的对象并展示；仅在本地、或远端加 `--yes` 后才删除（删对象属于破坏性操作，沿用现有确认口径）。
- 默认关闭；不带 `--prune` 时只在输出末尾提示「存在 N 个旧产物未清理」。

### SITE-6 文档与口径对齐（P0）

`skills/simplebase/SKILL.md`、`references/sites.md`、`references/cli.md`：

- 明确站点面**只读**（仅 GET/HEAD）、**没有后端接口**，页面只能调用外部 BFF 或 `end-users` 接口。
- 把 `sites.md` §1 的「指向独立 BFF」补成可操作步骤：配置 `config.js` → 启动已放开 CORS（`*`）的 BFF → 校验 CORS（给一条 `curl -i -X OPTIONS -H 'Origin: http://ex-jd001.localhost:8080' -H 'Access-Control-Request-Method: POST' -H 'Access-Control-Request-Headers: content-type,idempotency-key'` 示例，期望 204 与 `Access-Control-Allow-Origin: *`）。
- 文档中把 `*` 标注为示例级配置，并给出生产应改为白名单的提醒。
- 区分三类案例：
  - 纯静态（可直接 `site deploy`）；
  - 前端 + 同源 BFF（jd / shop，需 SITE-1/2 后才能跨源发布）；
  - 仅 Node/Go 服务（`iot-telemetry`、`community`、`ops-assistant`，不适用）。
- `SKILL.md` 步骤 4 把「编译 js-sdk」与「前端静态产物」拆成两条，说明前者只针对 `server/main.mjs`。
- 补充 §2「对象根公开」风险说明与 `--prefix` 推荐。
- `examples/jd/README.md`、`examples/shop/README.md` 增加「发布到子域名站点」小节，写明需要独立运行的 BFF。

---

## 2. 不做的事

- 不改站点面服务端：不加 History 回退、不加 `/api` 代理、不放开 POST（会破坏 `planv6.5` 的只读与同站安全模型）。
- 不新增第三方依赖、不改 `go.mod` / `package.json` / `tsconfig.json` / `vite.config.ts`。
- 不下发 `Domain=<父域>` 的 Cookie（planv6.5 硬约束）；示例保持无 Cookie 的 CORS。
- 不把 API Key 放进前端、`config.js` 或构建产物。

## 3. 实施批次

| 批次 | 内容 | 风险 | 是否需审批 |
| --- | --- | --- | --- |
| **S1** | SITE-6 文档；SITE-1 前端基址；SITE-2 BFF CORS（jd + shop） | 低中：去掉 Origin 校验、CORS 全放开，BFF 须保持只监听 `127.0.0.1`（由 SITE-3 的开关与 README 兜底） | 否 |
| **S2** | SITE-3 写入开关；SITE-4 对象根公开提示（`sbdev.py` + 单测） | 低 | 否 |
| **S3** | SITE-5 `--prune` | 中：涉及删对象 | 是（破坏性操作语义） |

## 4. 验证

```bash
(cd skills/simplebase && python3 -m unittest discover -s scripts -p 'test_sbdev.py')
node --test examples/jd/server examples/shop/server      # 新增的 CORS 离线用例（不连真实服务）
./scripts/test.sh                                         # examples 清单离线校验
scripts/examples-verify/verify.sh --ports 18090,18091 --only jd,shop   # 0 FAIL
```

端到端手工验收（本地，复制到临时目录，避免写入仓库 `.simplebase/`）：

1. 起 SimpleBase（`SIMPLEBASE_SITES_ENABLED=true SIMPLEBASE_SITES_BASE_DOMAIN=localhost SIMPLEBASE_SITES_URL_SCHEME=http SIMPLEBASE_SITES_URL_PORT=8080`），`init.sh --only jd`。
2. 以 `JD_PORT=8093` 启动 BFF（无需配置 Origin）。
3. 把 `config.js` 改为 `apiBase: 'http://127.0.0.1:8093'`，`site deploy --dist web --prefix web/`。
4. 浏览器打开 `http://ex-jd001.localhost:8080/web/`：首页商品加载、加购、下单成功（带 `Idempotency-Key`）；任意 `Origin`（含 `http://evil.example`）的 `curl` 请求都返回 `Access-Control-Allow-Origin: *`；`OPTIONS` 预检返回 204，且回显 `Access-Control-Request-Headers`。
5. 对照确认：不加 SITE-1/2 时同样步骤会复现 P1/P2（首页 `/api/home` 404）。

## 5. 风险与待确认

- `jd` / `shop` 的 BFF 公网部署由使用者自行负责，本计划不提供部署脚本；是否需要在示例里加最小登录（`end-users`）另议。
- CORS 取 `*` 是本次明确决策，代价是失去 Origin 层面的 CSRF 防护与 `Allow-Credentials`（Cookie / `Authorization` 凭据模式不可用）。若后续示例加入登录态，需改回白名单回显 Origin。
- `shop`、`jd` 的 `AGENTS.md` / `examples-verify` 现有用例若断言了 `Invalid origin`，需同步改为断言「放行并带 CORS 头」，实施 S1 时先全仓搜索该文案。
- SITE-4 的 `object list` 在对象很多的项目上可能较慢，需要限制扫描上限（如前 1000 个）并在超限时说明「仅抽样」。
- `index.html` 与 `config.js` 缓存策略不同（HTML `no-cache`，其余 `max-age=300`）：改 `config.js` 后最多 5 分钟生效，需写进 README。

## 6. 执行状态（2026-11-17）

| 批次 | 状态 | 说明 |
| --- | --- | --- |
| S1 | 已完成 | SITE-1：`jd` / `shop` 的 `web/` 新增 `config.js`，`index.html` 主脚本前加载 `./config.js`，`API` 读 `window.__SB_CONFIG__.apiBase`（默认空串 = 同源，行为不变；`shop` 原先 `fetch(path)` 同步改为 `fetch(API + path)`）；BFF 新增 `GET /config.js`。SITE-2：两个 BFF 删除 `Invalid origin` 判断，新增 `applyCors`（`Allow-Origin: *`、方法列表、`Allow-Headers` 回显请求头或 `*`、`Expose-Headers: *`、`Max-Age: 600`，不设 `Allow-Credentials`）与 `OPTIONS` 204；`jd` 的 `handle` 同步改为 `export` 以便离线测试。SITE-6：`SKILL.md`、`sites.md`（新增 §0 案例分类与「前端调后端」小节）、`cli.md`、两个示例 README 已更新 |
| S2 | 已完成 | SITE-3：`jd` 的 `/api/platform/*` 默认 403，`JD_PLATFORM_WRITE=1` 才放行（`shop` 不在计划范围，仅 README 提示）。SITE-4：`sbdev.py` 新增 `list_foreign_objects` / `warn_foreign_objects`，`site deploy` 在公开前提示前缀下不属于本次产物的已有对象（按一级目录汇总，扫描上限 1000，列表失败或结构未知时静默跳过）；远端预览（退出码 3）在有可执行 CLI 时同样显示，无二进制时跳过提示、不新增失败 |
| S3 | 未开始（待审批） | SITE-5 `--prune` 涉及删除对象，按计划需审批 |

### 验证结果

- `node --test examples/jd/server/cors.test.mjs examples/shop/server/cors.test.mjs`：11 个用例全部通过（任意 Origin、预检回显、无请求头回退 `*`、404 带 CORS 头、jd 平台写入默认 403、`/` 与 `/config.js` 同源可访问）。
- `python3 -m unittest discover -s scripts -p 'test_sbdev.py'`（`skills/simplebase`）：38 个全部通过，含新增 4 个（根公开提示、仅产物无提示、`--prefix` 只统计前缀内对象、远端预览带提示）。
- `sbdev.collect_site_files` 对复制到临时目录的 `jd/web` 检查通过，上传清单为 `web/config.js`、`web/index.html`。
- 未做：`scripts/examples-verify/verify.sh` 与浏览器端到端手工验收（需要真实服务与站点配置，见 §4）。
- `./scripts/test.sh` 当前失败，原因是工作区已有的无关改动：`gocloudagent/skills/embed.go` 的 `simplebase` embed 目录缺失，以及 `internal/agentjob` 目录不存在；与本计划改动无关，未处理。

### 与原计划的差异

- 已确认没有任何文件断言 `Invalid origin`（全仓搜索，`plan/` 除外），无需改既有用例。
- SITE-4 的「`--prefix` 提示」只在没有前缀时输出，带前缀时只列出该前缀内的非产物对象。
- 预览阶段没有二进制时不再强制 `t.require()`，保持原先「预览不需要 CLI」的行为。
