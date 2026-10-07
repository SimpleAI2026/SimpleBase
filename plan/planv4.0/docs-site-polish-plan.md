# 文档站 v4：样式优化 + 内容优化

> **仓库**：SimpleBase
> **状态**：P0/P1 已实施（2026-10-05）；P2 暂缓，真实环境人工验收待完成；实施记录见 §9。
> **范围**：渲染层 `ui/src/docs/`（`catalog.ts` / `render.ts`）、`ui/src/components/docs/`（`DocsArticle.vue` / `DocsSidebar.vue`）、`ui/src/pages/DocsWiki.vue`、`ui/src/layouts/DocsLayout.vue`；内容层 `docs/**`（7 个模块 28 篇）
> **方法**：逐篇阅读 `docs/`、对照控制台侧栏与路由、用 `marked` 实际渲染样例、查看 `ui/dist` 产物体积。下文「现状」均为本次核对结果；标注「未核对」的除外
> **约束**：沿用 `ui/AGENTS.md`——不新增依赖、不改 `package.json` / `vite.config.ts`、覆盖率门槛 95% 不调整

---

## 0. 一句话

文档站现在「能看」，但长文不可导航（无标题锚点、无页内目录、无搜索）、代码块不可复制、最大的一篇文档标题显示为 `Summary`；内容上入门页没有可执行的上手路径，四个控制台功能没有文档，另有若干失效链接。本版把这些补齐，不引入新依赖。

---

## 1. 现状与问题

### 1.1 渲染与样式

| # | 问题 | 证据 | 影响 |
|---|---|---|---|
| R1 | 标题没有 `id` | `marked.parse('## 小节 A')` 输出 `<h2>小节 A</h2>`；`render.ts` 未自定义 heading | 页内 `#锚点` 链接全部失效；`ducklake.md` 一篇就有 213 个 |
| R2 | `{#id}` 语法原样显示 | `# Specification {#specification}` 渲染为 `<h1>Specification {#specification}</h1>`；`ducklake.md` 中 257 处 | 标题带噪声 |
| R3 | 无页内目录（TOC） | `DocsArticle.vue` 只有面包屑 + 正文 + 上下篇 | `deployment.md`（250 行）、`kv.md`（223 行）、`ducklake.md`（4834 行）只能滚动找 |
| R4 | 无搜索 | — | 28 篇文档只能逐篇翻 |
| R5 | 代码块无复制、无语言标签、无高亮 | `render.ts` 未处理 `code`；全站代码块约 270 个（sql 189 / json 20 / go 12 / bash 12 …） | 以代码为主的文档可读性差 |
| R6 | 代码块浅色写死 | `DocsArticle.vue` 中 `pre { background:#f6f8fa; color:#1f2328; border:#e5e7eb }`，暗色靠 `:global(.dark)` 覆盖 | 与主题 token 脱节 |
| R7 | 标题三重重复 | `DocsWiki.vue` 顶部横幅显示模块名 + 面包屑显示模块名/页名 + 正文 H1 | 首屏约 200px 无信息量 |
| R8 | 全部 Markdown 同步打包 | `catalog.ts` 用 `import.meta.glob(..., { eager: true })`；`ui/dist/assets/DocsWiki-*.js` 为 430 KB，其中 `ducklake.md` 源文件 257 KB | 打开任何一篇都要下载全部文档 |
| R9 | 移动端判断用 JS | `DocsWiki.vue` 中 `window.innerWidth <= 768` + resize 监听 | 可用 CSS 断点替代，少一段状态 |
| R10 | 页面标题不随文档变化 | 三条 `/docs` 路由 `meta.title` 均为「使用文档」 | 浏览器标签页无法区分（`document.title` 的设置位置未核对） |
| R11 | `firstH1` 会匹配代码块内的 `# ` 行 | 正则 `/^#\s+(.+)$/m`；`gofunction/*.md` 代码块内有 `# → ...` 行（目前被 frontmatter 的 `title` 盖住，未触发） | 潜在错误标题 |
| R12 | 提示块无样式区分 | `> **Note** …` 渲染为普通 blockquote；`ducklake.md` 约 10 处 | 警告与引用看起来一样 |

### 1.2 目录与链接

| # | 问题 | 证据 |
|---|---|---|
| C1 | 4 篇缺 frontmatter | `database/ducklake.md`、`cronjob/overview.md`、`ops/deployment.md`、`ops/migration.md`；标题回退为首个 H1，`order` 回退为 100 |
| C2 | DuckLake 文档侧栏标题是 `Summary` | `ducklake.md` 第 1 行为 `# Summary`，全文 4 个 H1 |
| C3 | 链接指向 `docs/` 之外，改写后 404 | `cronjob/overview.md` → `../plan/planv2.0/proto-http.md`（被改写为 `/docs/plan/planv2.0/proto-http`）；`sdk/examples.md` 5 处 `../../examples/**`（被改写为 `/docs/../examples/...`） |
| C4 | 引用了不存在的示例目录 | `sdk/index.md` 写 `examples/node-sql`、`node-documents`、`node-storage`、`browser-quickstart`，仓库 `examples/` 下均不存在 |
| C5 | 模块名与内容不符 | `_meta.json` 中 `sdk` 模块标题为「JS SDK」，但含 6 篇 `go-*` 页面 |
| C6 | 已废弃文档仍在主导航 | `ops/migration.md` 首段自述 Deprecated（历史存档），入门页「建议阅读顺序」第 4 条仍指向它 |

### 1.3 内容缺口

控制台侧栏 10 个入口与文档模块对照：

| 控制台入口 | 文档 | 结论 |
|---|---|---|
| 监控大盘 | 无 | 缺 |
| 数据库管理 | `database/index.md`（20 行）+ `ducklake.md`（上游单文件镜像） | 缺 SimpleBase 自己的 SQL / 集合使用说明 |
| Key-Value | `database/kv.md` | 有 |
| 对象存储 | 仅 SDK 页 `storage.md`（17 行） | 缺 |
| 云函数 | `gofunction/` 2 篇 | 有 |
| 定时任务 | `cronjob/overview.md` | 有；API 一节只有一句话并外链到 plan |
| 云沙盒 | `sandbox/` 3 篇 | 有 |
| 云 Agent | 无 | 缺 |
| 日志管理 | 无 | 缺 |
| 用户管理 / API Key / 角色 | 仅 `sdk/auth.md`（30 行） | 缺 |

其它：

- **入门页没有上手路径**：`getting-started/index.md` 共 26 行，只有功能列表和阅读顺序，没有「启动 → 登录 → 拿 API Key → 跑第一条 SQL」。
- **SDK 页面过薄**：JS SDK 9 篇中 8 篇不足 800 字节；JS/Go SDK 均已实现沙盒接口（`packages/js-sdk/src/sandboxes.ts`、`packages/go-sdk/sandboxes.go`），`docs/sdk/` 没有对应页面。
- **没有统一的 HTTP API 参考与错误码页**：各模块各写一段。

---

## 2. 目标与非目标

### 2.1 目标

| # | 目标 | 落点 |
|---|---|---|
| G1 | 长文可导航 | 标题锚点、右侧页内目录、滚动高亮（§3.1） |
| G2 | 代码可用 | 复制按钮、语言标签、主题 token 配色（§3.2） |
| G3 | 能搜到 | 标题 + 正文的本地搜索（§3.3） |
| G4 | 首屏更干净、加载更轻 | 去掉重复标题；Markdown 正文按页懒加载（§3.4、§3.5） |
| G5 | 目录正确 | frontmatter 补齐、模块重排、失效链接清零并加自动检查（§4.1、§4.4） |
| G6 | 内容补齐 | 入门上手路径、4 个缺失模块、SDK 扩写（§4.2、§4.3） |

### 2.2 非目标

- 不换文档框架（不引入 VitePress / Docusaurus），不做独立站点部署。
- 不做多语言、版本切换、评论、在线编辑。
- 不新增 npm 依赖。语法高亮因此不在默认范围内，见 §6 决策 D1。
- 不逐字校对上游 DuckLake 文档内容。

---

## 3. 样式与渲染方案

### 3.1 标题锚点 + 页内目录

- `render.ts`：自定义 `renderer.heading`。
  - 解析并去掉尾部 `{#id}`，有则用它作 `id`；没有则由标题文本生成 slug（保留中文、小写、空格转 `-`，同页重复追加 `-2`、`-3`）。
  - h2–h4 追加悬停可见的 `#` 链接，便于复制。
  - `renderMarkdown` 返回值由 `string` 改为 `{ html, toc }`，`toc` 为 `{ id, text, level }[]`（只收 h2、h3）。调用方只有 `DocsArticle.vue` 和测试。
- 新增 `components/docs/DocsToc.vue`：右侧 200px 粘性栏，`lg` 以上显示；用 `IntersectionObserver` 高亮当前小节；少于 3 个条目时不渲染。
- `DocsWiki.vue`：三栏布局（侧栏 240 / 正文 / 目录 200），容器由 1200 放宽到 1280；进入页面时若 URL 带 hash 则滚动到对应标题（标题已有 `scroll-margin-top`）。
- 需确认 DOMPurify 在现有配置下保留 `id` 属性，并用单测固定。

### 3.2 代码块

- `render.ts`：自定义 `renderer.code`，输出 `<div class="docs-code" data-lang="sql"><button data-copy>…</button><pre>…</pre></div>`。
- `DocsArticle.vue`：在正文容器上做事件委托处理 `[data-copy]` 点击，写剪贴板后 `toast`（不为每个代码块挂 Vue 组件）。
- 样式：`pre` 改用 `var(--muted)` / `var(--border)` / `var(--foreground)`，删除写死的十六进制色和 `:global(.dark)` 覆盖；右上角显示语言标签。
- 行内 `code`、表格、blockquote 维持现有样式，仅调整：表格去掉 `display:block`，改为外包一层可横向滚动的容器（由 `renderer.table` 输出），避免窄表格不撑满。

### 3.3 提示块

- `renderer.blockquote`：首段以 `**Note**` / `**Tip**` / `**Warning**` / `**注意**` / `**提示**` 开头时加 `data-callout` 属性，分别用主色 / 绿 / 琥珀色左边框。不引入新语法，现有文档无需改写。

### 3.4 搜索

- `catalog.ts` 新增 `searchDocs(query)`：对「页标题、h2/h3 标题、正文纯文本」做大小写不敏感的子串匹配，标题命中优先，返回 `{ page, heading?, snippet }[]`，最多 20 条。
- `DocsLayout.vue` 头部加搜索入口，`/` 或 `Ctrl/⌘+K` 聚焦；结果面板用现有 `Popover` + `Command` 组件。
- 索引来源与 §3.5 的懒加载相关：首次聚焦搜索框时才加载全部正文并建索引，之后缓存。

### 3.5 首屏与加载

- `DocsWiki.vue`：删除「SimpleBase · Documentation」横幅，「第 N / M 篇」并入面包屑行右侧；`isMobile` 改为 Tailwind 断点（`md:hidden` / `hidden md:block`）。
- `catalog.ts`：正文改为非 eager 的 `import.meta.glob`（按页动态加载）。目录需要的 `title` / `order` 仍要在首屏可得，两种做法见 §6 决策 D2。
- `DocsArticle.vue`：正文加载中显示 `Skeleton`；加载失败沿用现有 `Alert`。
- 文档页切换时把 `document.title` 设为「页标题 · 模块 · SimpleBase 文档」（先确认现有标题设置位置）。
- `firstH1` 改为先剔除围栏代码块再匹配。

### 3.6 语法高亮（默认不做）

不新增依赖的前提下没有现成高亮器。备选见 §6 决策 D1；默认交付「语言标签 + 复制」，不着色。

---

## 4. 内容方案

### 4.1 目录重排

`docs/_meta.json` 调整为（顺序与控制台侧栏分组一致）：

| order | id | 标题 | 页面 |
|---|---|---|---|
| 1 | `getting-started` | 入门 | 概览、**5 分钟上手**（新）、**核心概念**（新：项目 / API Key / 权限 / 系统项目） |
| 2 | `database` | 数据库 | 概览、**SQL 与集合**（新）、Key-Value、**DuckLake 使用须知**（新，见 D3）、DuckLake 上游参考 |
| 3 | `storage` | 对象存储 | **概览**（新）、**HTTP API**（新） |
| 4 | `gofunction` | 云函数 | 概览、调用指南 |
| 5 | `cronjob` | 定时任务 | 概览、**HTTP API**（新，替换指向 plan 的外链） |
| 6 | `sandbox` | 云沙盒 | 概览、HTTP API、e2e 与 CI |
| 7 | `agent` | 云 Agent | **概览**（新）、**工具与模块**（新） |
| 8 | `ops` | 运维 | 概览、部署、**日志与审计**（新）、**用户与角色**（新）、迁移（历史存档，排最后并在标题标注） |
| 9 | `sdk` | SDK | 概览（JS / Go 选择）、JS 9 篇、Go 6 篇、**沙盒**（新，JS + Go 各一节） |
| 10 | `reference` | 参考 | **错误码**（新，汇总各模块 `code`）、**配置项**（新，对应 `config.example.yaml`） |

侧栏支持模块内分组小标题（`sdk` 下分「JavaScript」「Go」）：frontmatter 增加可选 `group` 字段，`DocsSidebar.vue` 按 `group` 分段渲染；无 `group` 的模块表现不变。

### 4.2 新增与重写清单

| 优先级 | 文件 | 内容要点 | 事实来源 |
|---|---|---|---|
| P0 | `getting-started/quickstart.md` | `./build.sh dev` 启动 → 登录 → 取 API Key → curl 建库并跑一条 SQL → 写一个 KV → 指向 SDK | `build.sh`、`internal/systemdb/seed.go`、`internal/api/router.go` |
| P0 | `getting-started/index.md`（重写） | 一段定位 + 功能卡片式列表（每项链到对应模块）+ 修正阅读顺序（去掉迁移） | 控制台侧栏 |
| P0 | `database/ducklake-notes.md` | SimpleBase 下的约束：无主键/唯一/索引、单写实例、`sqlguard` 拦截的语句、最终一致读 | `internal/database/sqlguard`、`plan/planv4.0/ducklake-rw-latency-eventual-consistency-plan.md` |
| P1 | `database/sql.md` | SQL 工作台三种模式、query/execute/batch 端点、集合与文档接口 | `internal/api`、`docs/sdk/database-sql.md` |
| P1 | `storage/index.md`、`storage/api.md` | 项目级隔离、上传/列举/预签名/删除、大小限制 | `internal/api` S3 handler、`internal/objectstore` |
| P1 | `agent/index.md`、`agent/tools.md` | 模块、工具列表、`@` 调用、沙盒工具、定时调度 | `internal/cloudagent` |
| P1 | `ops/logs.md`、`ops/users.md` | 日志级别与保留期、审计事件；三种角色、API Key 权限点 | `internal/log`、`internal/audit`、`internal/auth` |
| P1 | `cronjob/api.md` | 7 个端点的请求/响应示例 | `internal/api` cron handler |
| P2 | `sdk/sandboxes.md` | JS `client.sandboxes.*`、Go `*Sandbox*` 方法、分页 | `packages/js-sdk/src/sandboxes.ts`、`packages/go-sdk/sandboxes.go` |
| P2 | `reference/errors.md`、`reference/config.md` | 错误码表；配置项表 | `internal/api/error.go`、`config.example.yaml` |
| P2 | JS SDK 薄页扩写 | 每页补「完整可运行示例 + 常见错误」 | `packages/js-sdk` |

每篇新文档的示例命令和端点必须对照源码写，并在 fake / dev 模式下实际跑通一遍再提交；跑不了的在文档里注明前提条件。

### 4.3 写作规范（写入 `docs/README.md`；`catalog.ts` 忽略 `docs/` 根目录下的文件，不会出现在文档站）

- 每篇必须有 frontmatter：`title`、`order`，可选 `group`、`description`（用于搜索摘要）。
- 每篇只有一个 H1，且与 `title` 一致；小节从 H2 起，不跳级。
- 链接只用两种：站内 `/docs/<module>/<slug>` 或相对 `./x.md`、`../module/x.md`；指向仓库其它文件一律用 GitHub 完整 URL。
- 代码块必须标语言；HTTP 示例统一 `http`，命令行统一 `bash`。
- 概览页结构统一：是什么 → 何时用 → 最小示例 → 限制 → 相关链接。

### 4.4 存量修复

| 项 | 处理 |
|---|---|
| C1 | 4 篇补 frontmatter |
| C2 | 见 D3；至少把侧栏标题改为「DuckLake 上游参考」 |
| C3 | `cronjob/overview.md` 外链改指新的 `cronjob/api.md`；`sdk/examples.md` 5 处改为 GitHub URL |
| C4 | `sdk/index.md` 示例目录改为实际存在的目录。注意：工作区里 `examples/` 有大量已暂存的删除（`booking-service`、`community-service` 等），实施前先确认示例目录的最终形态 |
| C5 | 模块标题「JS SDK」→「SDK」，配合 `group` 分段 |
| C6 | 迁移文档标题加「（历史存档）」并排到模块末尾，入门页不再引用 |
| 链接改写 | `rewriteDocHref` 对解析后落在 `docs/` 之外的相对链接改写为 GitHub blob URL，而不是生成 404 路由 |

### 4.5 链接与目录自动检查

新增 `ui/tests/docs-content.test.ts`（纯 vitest，读真实 `docs/`）：

1. 每篇有 frontmatter `title` 与 `order`，且恰好一个 H1（`ducklake.md` 按 D3 结论豁免或拆分）。
2. 所有站内链接能解析到 `docCatalog` 中的页面；所有 `#锚点` 能在目标页的标题 id 集合里找到。
3. `_meta.json` 的每个模块至少有一篇文档；每个文档目录都在 `_meta.json` 中。
4. 所有围栏代码块带语言标注。

这样 C3 / C4 一类问题以后在 `yarn test` 就会失败。

---

## 5. 实施顺序

每步结束保持 `cd ui && yarn build && yarn test` 通过（覆盖率四项 ≥ 95%）。

| 步骤 | 内容 | 产出 |
|---|---|---|
| S1 存量修复 | §4.4 全部；`firstH1` 修正；`rewriteDocHref` 外链改写 | 失效链接清零；`docs-content.test.ts` 的 1–3 条 |
| S2 锚点与目录 | §3.1：heading renderer、`DocsToc.vue`、三栏布局、hash 滚动 | `docs-render` / `DocsToc` 测试 |
| S3 代码块与提示块 | §3.2、§3.3；样式改 token | 渲染与复制测试；`docs-content` 第 4 条 |
| S4 首屏与懒加载 | §3.5；按 D2 结论实现 | 产物体积对比记录 |
| S5 搜索 | §3.4 | `searchDocs` 单测 + 交互测试 |
| S6 P0 内容 | 入门两篇、DuckLake 使用须知、目录重排 + `group` | — |
| S7 P1 / P2 内容 | §4.2 其余 | — |
| S8 收尾 | `ui/README.md`、`ui/AGENTS.md` 的文档站说明同步；`IMPLEMENTATION_SUMMARY.md` 追加一节 | — |

S1–S3 一个 PR；S4–S5 一个 PR；S6–S8 按模块拆 PR。S1 不依赖任何决策，可立即开始。

---

## 6. 待决策

| # | 问题 | 选项 | 建议 |
|---|---|---|---|
| D1 | 是否做语法高亮 | A 不做，只给语言标签 + 复制；B 新增 `highlight.js` 或 `shiki`（需要明确同意新增依赖，且会增大文档 chunk）；C 复用已有的 `monaco-editor` 的 `colorize`（零新依赖，但文档页要加载 Monaco，体积大） | A。全站 70% 的代码块是 SQL，B 的收益最明显，如同意加依赖再做 |
| D2 | 懒加载后目录元数据从哪来 | A 把每篇的 `title` / `order` / `group` 挪进 `_meta.json`，Markdown 全部懒加载；B 只对大文件懒加载（阈值如 50 KB，目前只有 `ducklake.md`），其余保持 eager | B。改动小，frontmatter 仍是单一事实来源，已能去掉约 60% 的体积 |
| D3 | `ducklake.md`（上游单文件镜像，257 KB，4 个 H1）怎么放 | A 保留原文件，仅补 frontmatter、懒加载、标题改为「DuckLake 上游参考」，另写一篇 SimpleBase 使用须知；B 按 H1 / H2 拆成多页；C 删除镜像，只链到 ducklake.select | A。B 会让后续同步上游变难；C 让离线部署失去参考。镜像对应的上游版本未核对，实施时在页首注明版本与同步日期 |
| D4 | 新增模块的范围 | 全做 §4.2；或只做 P0 + P1 | 先 P0 + P1，P2 视反馈 |

---

## 7. 验收

1. 任意文档页：h2–h4 有稳定 `id`；带 hash 的 URL 直接打开能定位到对应小节；`{#id}` 不再出现在页面文字中。
2. 含 3 个以上小节的页面在 ≥ `lg` 宽度显示右侧目录，滚动时高亮当前小节。
3. 每个代码块有语言标签和复制按钮，复制内容与源文本一致；明暗主题下配色都来自主题 token。
4. 搜索「幂等」「cursor」「ErrBusy」等词能命中对应页面与小节，回车跳转并定位。
5. 打开入门页时不下载 `ducklake.md` 内容（Network 面板确认）；记录 `DocsWiki` chunk 前后体积。
6. `docs-content.test.ts` 通过：无缺 frontmatter、无失效站内链接与锚点、无未标语言的代码块。
7. 侧栏标题不再出现 `Summary`；`sdk` 模块按 JavaScript / Go 分段。
8. 按「5 分钟上手」从零操作一遍，每条命令的输出与文档一致。
9. 控制台侧栏 10 个入口都能在文档里找到对应页面（P0 + P1 完成后）。
10. `yarn build`、`yarn test`（覆盖率四项 ≥ 95%）通过；未新增依赖，未改 `vite.config.ts`。

---

## 8. 风险

| 风险 | 应对 |
|---|---|
| 覆盖率余量很小（当前 functions 95.17%、branches 95.31%） | 每个新组件 / renderer 分支随 PR 带测试；`DocsToc` 的 `IntersectionObserver`、剪贴板、懒加载失败路径都要覆盖 |
| `renderMarkdown` 返回值变更 | 源码调用方只有 `DocsArticle.vue`，另有若干 `ui/tests/docs-*.test.ts` / `DocsArticle*.test.ts` 用例，同一 PR 内改完 |
| 新文档写错接口 | §4.2 要求对照源码并实际跑通；`docs-content.test.ts` 只能查链接，查不了语义，评审时逐篇核对 |
| `examples/` 目录正在变动 | C4 的修复等示例目录定稿后再做，或先全部改为指向 `examples/README.md` 的 GitHub URL |
| 上游 DuckLake 文档里的 `<details markdown='1'>` 等 HTML | 现有渲染可显示（已用样例确认）；S2 改 heading renderer 后对该文件做一次整页渲染冒烟 |

---

## 9. 实施记录（2026-10-05）

**决策**：D1=A（不加高亮依赖，只给代码语言标签与复制）；D2=B（仅 `ducklake.md` 异步加载，其余小文档 eager）；D3=A（保留上游镜像、标注版本未知、另写 SimpleBase 使用须知）；D4=先 P0/P1，P2 留待下轮。

| 阶段 | 状态 | 结果 |
|---|---|---|
| S1 | 已完成 | 4 篇补 frontmatter；修复 SDK 失效示例链接、定时任务计划外链与 `dev-shop` 种子项目 ID；`docs-content.test.ts` 校验所有正式模块的元数据、站内链接、锚点与代码围栏。DuckLake 镜像中的 `<details>` 导致 `marked.lexer` 误判 3 个无语言代码 token，围栏语言检查对该**上游镜像**豁免，其他文档严格执行 |
| S2 | 已完成 | 标题 `id` / `{#id}` 清洗与重复标题序号、`DocsToc.vue` 的桌面目录和滚动高亮、hash 深链跳转；上游 213 个锚点通过静态检查，额外修复原镜像 3 处相对锚点 |
| S3 | 已完成 | 代码语言标签 / 复制按钮、主题 token 配色、提示块变体与滚动表格；没有增加高亮依赖 |
| S4 | 已完成 | `ducklake.md` 独立动态 chunk，文档页小文档保持 eager；`DocsWiki` JS chunk 从基线约 **430 KB** 降至本轮 **30.94 KB**，另有按需加载的 DuckLake chunk **265.77 KB**。搜索首次使用时会加载该 chunk，属于设计预期；去重复标题、移动端 CSS 显隐、动态浏览器标题 |
| S5 | 已完成 | `DocsSearch.vue` 本地检索标题 / h2-h3 / 正文，快捷键 `⌘/Ctrl+K` 与 `/`，回车跳转；首次搜索构建并缓存索引 |
| S6 | 已完成 | 入门 `quickstart`、`concepts`，DuckLake 使用须知与上游参考元数据，模块顺序及 SDK 分组；修正 SDK 过期项目 ID |
| S7 | P0/P1 已完成，P2 延后 | 补数据库 SQL 与集合、对象存储、云 Agent、日志、用户角色、监控大盘、定时任务 HTTP API；`docs/README.md` 写作规范。`sdk/sandboxes.md`、`reference/*` 与 JS SDK 薄页扩写属于 P2，按 D4 暂不实施 |
| S8 | 已完成 | 同步 `ui/README.md`、`ui/AGENTS.md` 和 `plan/planv4.0/IMPLEMENTATION_SUMMARY.md` |

**验证**：`cd ui && yarn build` 通过；`cd ui && yarn test` 通过（statements **98.70%**、branches **95.13%**、functions **95.08%**、lines **98.70%**；阈值未调整）。`go test ./internal/api -run 'Test(CreateDatabase_Success|SQLQuery_Success|KVCmdStringRoundTrip|S3_UploadAndList|CronJob_CRUDLifecycle|MetricsAndLogsHandlers)' -count=1` 通过。`git diff --check` 通过。未新增依赖，也未改前端工程配置。

**尚未人工验证**：未启动需要外部 S3 / Cloud 等环境的完整服务，也未以真实浏览器逐一检查移动端、暗色模式、长文滚动、快捷键与「5 分钟上手」的完整端到端输出。以上运行结果只确认服务端 handler 契约和静态/组件测试，不能替代真实环境验收。后续在可用的开发环境按照 §7 逐项确认。
