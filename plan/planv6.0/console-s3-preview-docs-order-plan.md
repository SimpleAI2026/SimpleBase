# planv6.0 控制台优化计划：趋势图默认折线 / S3 文件预览 / 文档站重排与扩写

> 状态：**P0~P3 已实施完成**（2026-10-10）。范围限于 `ui/src` 与 `docs/`；未改后端。
> 约束沿用 `AGENTS.md` / `ui/AGENTS.md`：不新增依赖、不改 `package.json` / `vite.config.ts`、对象形状用 `interface`、文案同时补 `zh-CN` + `en-US`、样式钩子用 `data-[state=…]`、`SbModal` / `SbAsyncRegion` / `SbEmptyState` 复用。
>
> 落地时对 §6 的四个问题采用了默认方案：① 错误率方案 A（比值 ×100，4 位小数，同时修正口径）；② S3 预览纯前端 + CORS 失败降级；③ 文档拆成独立模块并加重定向；④ 不做 pdf / 音频 / 视频。
>
> **后续变更**：§3 的文档模块拆分已被 [`planv6.1/docs-six-modules-consolidation-plan.md`](../planv6.1/docs-six-modules-consolidation-plan.md) 取代——13 个 Tab 收敛为 6 个大模块，`users` 文档删除。

## 实施结果速览

| 阶段 | 状态 | 主要产出 |
|---|---|---|
| P0 趋势图 | 已完成 | `TrendChart` 默认 `line`；`Dashboard` 默认 `line`；新增 `formatErrorRate()` |
| P1 S3 预览 | 已完成 | `utils/filePreview.ts`、`components/modal/S3PreviewModal.vue`、表头改为「文件」、写入下载/复制能力 |
| P2 文档结构 | 已完成 | 4 个模块迁移（`dashboard` / `kv` / `logs` / `users`）、`_meta.json` 重排、旧路径重定向、`docs-order` 测试 |
| P3 文档扩写 | 已完成 | 入门 / 云助手 / 大盘 / 数据库 / KV / 对象存储 / 云函数 / 日志 / 用户 / SDK 共 10 个模块补「使用场景 + 常见问题」 |
| P4（可选） | 未做 | tooltip 错误率、后端下载代理、pdf/audio/video 预览 |

验证结果：`cd ui && yarn test` → 105 文件 / 662 用例通过，statements 99.49%、branches 95.03%、functions 95.35%（均达标）；`yarn build` 通过；`go build ./internal/... ./cmd/...` 通过。

---

## 0. 需求清单

| # | 需求 | 涉及文件 |
|---|---|---|
| 1 | 请求趋势默认折线图；错误率只保留 4 位小数 | `components/TrendChart.vue`、`pages/Dashboard.vue`、`utils/format.ts`、`locales/*` |
| 2 | 对象存储表头 `Key` 改为「文件」；点击「打开」弹出多模态预览 Modal（图片 / 文本），Modal 内可直接下载 | `pages/S3Manager.vue`、新增 `components/modal/S3PreviewModal.vue`、新增 `utils/filePreview.ts`、`locales/*` |
| 3 | 文档站 Tab 顺序与控制台侧边栏一致；文档内容扩写，补充更多使用场景示例 | `docs/_meta.json`、`docs/**`、`ui/src/docs/catalog.ts`、`pages/DocsWiki.vue` |

---

## 1. 趋势图默认折线 + 错误率 4 位小数

### 1.1 现状

- `TrendChart.vue:63-69`：`withDefaults(..., { mode: 'bar' })`。
- `Dashboard.vue:190`：`const chartMode = ref<ChartMode>('bar')`。
- `Dashboard.vue:36-40` 摘要行 `t('pages.dashboard.summaryLine', { rate: summary.errorRate })`，文案 `请求 {requests} · 错误率 {rate}% · 延迟 {latency}ms`，**直接插值原始浮点数**，会出现 `0.2857142857%` 之类长小数。
- 趋势图本身（`TrendPoint`）只有 `requests` / `errors` 计数，没有错误率字段；「错误率」只出现在摘要行。

### 1.2 待确认的口径问题（实现前必须定）

后端 `internal/systemdb/metrics.go:119` 为 `ErrorRate = errors / requests`，**是 0~1 的比值**（`systemdb_test.go:753` 断言 `0.2`）。而 UI 文案在数值后面拼了 `%`，即当前 20% 的错误会显示成 `0.2%`——这是既有的口径 bug。

- 方案 A（推荐）：展示 `ratio * 100` 并保留 4 位，如 `28.5714%`；顺带修正口径。
- 方案 B：只截断 4 位，不换算（`0.2857%`），保持现有（错误）口径。

默认按 A 实施，除非你否决。

### 1.3 改动点

1. `TrendChart.vue`：`withDefaults` 的 `mode` 默认值改为 `'line'`。
2. `Dashboard.vue`：`chartMode` 初始值改为 `'line'`。
3. `utils/format.ts` 新增 `formatErrorRate(ratio: number): string`：非有限数返回 `0.0000`；否则 `(ratio * 100).toFixed(4)`；单一入口，后续其它页面复用。
4. `Dashboard.vue` 摘要行改为 `rate: formatErrorRate(summary.errorRate)`；`summaryLine` 文案保持 `{rate}%`，en-US 同构。
5. （可选，P2）`TrendChart` tooltip 增加「错误率」行：`errors / requests`，同样走 `formatErrorRate`，`requests = 0` 时显示 `0.0000%`。需要自定义 `tooltip.formatter`，仅在确认需要时做。

### 1.4 测试

- `tests/TrendChart.test.ts`：新增「不传 mode 时 series 类型为 `line`」；现有显式 `mode: 'bar'` 用例不动。
- `tests/Dashboard.test.ts`：断言默认折线；摘要行错误率为 4 位小数（mock `errorRate` 当前为 `1.5`，按口径 A 期望 `150.0000%`，建议把 mock 改为更真实的 `0.015`）。
- `tests/format.test.ts`（或现有 utils 测试）：覆盖 `formatErrorRate` 的 0、整数、长小数、`NaN`。

---

## 2. S3 文件预览 Modal

### 2.1 现状

- `S3Manager.vue:43`：表头硬编码 `Key`（未走 i18n）。
- `S3Manager.vue:74-76` 与 `221-228`：「打开」调用 `api.s3.presign` 后 `window.open(url, '_blank')`。
- `GET /s3/presign` 返回 `{ url }`：S3 实现是 15 分钟有效的预签名 GET；**本地 `localFileStore` 返回 `data:<mime>;base64,…`**（`localfilestore.go:136-153`）。
- `S3Object` 仅有 `key / size / lastModified`，**没有 content-type**，类型只能按扩展名判断。
- `SbModal` 已支持 `maxWidth`、`hideFooter`、`#footer` 插槽，body 内部滚动，可直接复用。

### 2.2 交互设计

- 表头：`Key` → `t('pages.s3.colFile')`（zh「文件」/ en「File」）。
- 「打开」按钮文案不变，行为改为：打开 `S3PreviewModal`，传入 `projectId`、`record`（key/size/lastModified）。
- Modal 结构（`SbModal`，`maxWidth` 约 `880`，自定义 footer）：
  - 标题：文件名（key 的 basename），描述：完整 key · 大小 · 修改时间。
  - 内容区按类型渲染，加载/错误态走 `SbAsyncRegion`（`block="spinner"`）。
  - Footer：`下载`（主按钮，`DownloadIcon`）、`关闭`；文本类额外一个「复制内容」。
- 不支持预览的类型：用 `SbEmptyState` 展示「该类型暂不支持预览」+ 保留下载按钮。

### 2.3 类型识别（纯函数，便于 100% 覆盖）

新增 `ui/src/utils/filePreview.ts`：

```ts
export type PreviewKind = 'image' | 'markdown' | 'text' | 'unsupported'

export interface PreviewInfo {
  kind: PreviewKind
  ext: string
  basename: string
}

export function getPreviewInfo(key: string): PreviewInfo
```

| kind | 扩展名（大小写不敏感） | 渲染方式 |
|---|---|---|
| `image` | `jpg` `jpeg` `png` `webp` `svg`（可顺带 `gif` `bmp` `ico` `avif`） | `<img :src="url">`，`object-contain`，限制最大高度 |
| `markdown` | `md` `markdown` | 默认渲染视图（复用 `docs/render.ts` 的 `renderMarkdown`，已含 DOMPurify），可切换「源码」 |
| `text` | `txt` `log` `logs`（可顺带 `json` `csv` `yaml` `yml` `xml` `ini` `conf` `sql`） | `<pre class="sb-mono">`，保留换行，自动换行可滚动 |
| `unsupported` | 其它 | 空态 + 下载 |

> 「多模态」首期落地图片 + 文本 + Markdown，满足需求中列出的类型。`pdf` / `audio` / `video` 若需要，只是在表中加行（`<iframe>` / `<audio>` / `<video>`），列为 P2，不影响本次结构。

### 2.4 数据获取与下载

1. 每次打开 Modal 时重新调用 `api.s3.presign`（URL 15 分钟过期，不缓存跨会话）。
2. 图片：直接把 URL 当 `<img src>`，**不需要 CORS**。SVG 只能走 `<img>`，**禁止 `v-html` 内联**，避免脚本执行。
3. 文本 / Markdown：`fetch(url)` 读取文本。
   - 先看 `record.size`：超过阈值（建议 1 MB，常量 `MAX_TEXT_PREVIEW_BYTES`）不拉取，直接提示「文件过大，请下载查看」。
   - 用 `AbortController`，Modal 关闭或切换文件时中止请求，避免竞态回写。
   - `data:` URL（本地 FileStore）`fetch` 同样可用。
4. 下载：
   - 优先 `fetch(url) → blob → URL.createObjectURL → <a download=basename>`，结束后 `revokeObjectURL`。
   - `fetch` 失败（CORS 等）时降级为 `window.open(url, '_blank')`，并 toast 提示。
   - `data:` URL 可直接 `<a href download>`。
5. 错误提示统一 `errorMessage(e, t('pages.s3.previewFailed'))` + `vue-sonner`。

### 2.5 风险与可选后端方案

- **CORS 风险**：S3 预签名 URL 跨域 `fetch` 取文本，要求 bucket 配置了对控制台域名的 CORS（`GET`）。若生产 bucket 未配置，文本预览会失败（图片不受影响）。首期按「前端读取 + 失败降级」实现，并在文档（部署指南）补一段 CORS 配置示例。
- **可选方案 B（默认不做，需你确认再启动）**：后端扩展 `GET /s3/presign`：
  - 新增 `download=1` 查询参数，S3 实现通过 `GetObjectInput.ResponseContentDisposition = attachment; filename=…` 让浏览器直接下载（解决跨域 `<a download>` 被忽略问题）；
  - 或新增同源代理 `GET /s3/objects/content?key=`（带大小上限、`Content-Type` 白名单、`X-Content-Type-Options: nosniff`），彻底避开 CORS。
  - 影响面：`objectstore.FileStore.PresignGet` 签名/所有 fake 实现、`s3_handler.go`、`packages/go-sdk` / `js-sdk`，需补「系统库拒绝 / writable」之外的新用例，改动量显著大于前端方案。

### 2.6 文件拆分与 i18n

- 新增 `components/modal/S3PreviewModal.vue`（props：`open`、`projectId`、`record: S3Object | null`；emit：`update:open`）。`S3Manager.vue` 只保留 `previewOpen` / `previewTarget` 两个状态，删除 `open()` 里的 `window.open`。
- 新增 `utils/filePreview.ts`（上表）。
- `locales/zh-CN.ts` + `en-US.ts` 的 `pages.s3`（或 `components.s3Preview`）补充，key 集合必须一致：`colFile`、`preview`（若要把按钮改名则用；本次保持 `open`）、`download`、`copyContent`、`copied`、`previewFailed`、`previewTooLarge`、`previewUnsupported`、`viewRendered`、`viewSource`、`downloadFailed`。注意文案中避免裸 `@`。

### 2.7 测试

- `tests/S3Manager.test.ts`：原用例「点击打开 → `window.open` 被调用」改为「点击打开 → Modal 出现，且调用 `presign`」；表头断言为「文件」。
- 新增 `tests/S3PreviewModal.test.ts`：图片渲染 `<img>`；文本 `fetch` 成功/失败；超过阈值不请求；Markdown 渲染/源码切换；不支持类型出现空态；下载成功走 blob、失败降级 `window.open`；关闭时 `abort` 与 `revokeObjectURL`。
- 新增 `tests/filePreview.test.ts`：各扩展名、大小写、无扩展名、多级目录、`.tar.gz`、点开头文件。
- 不桩掉 `SbModal` 之外的 `ui/` 交互组件；按真实 DOM 断言。保持 coverage 阈值 95%。

---

## 3. 文档站 Tab 顺序与内容扩写

### 3.1 现状

- Tab 来自 `docs/_meta.json`，当前顺序：入门 → 数据库 → 对象存储 → 云函数 → 定时任务 → 云沙盒 → 云助手 → 运维 → SDK。
- 控制台侧边栏顺序（`NavMenu.vue` GROUP_DEFS + 路由 meta）：**云助手 → 监控大盘 → 数据库管理 → Key-Value → 对象存储 → 云函数 → 定时任务 → 云沙盒 → 日志管理 → 用户管理**。
- 差异：文档里「监控大盘 / 日志 / 用户」藏在 `ops/`，「Key-Value」藏在 `database/kv.md`，Tab 层级无法与侧栏一一对应。

### 3.2 目标 Tab 顺序

`入门` → `云助手` → `监控大盘` → `数据库管理` → `Key-Value` → `对象存储` → `云函数` → `定时任务` → `云沙盒` → `日志管理` → `用户管理` → `部署运维` → `SDK`

（入门放首位作为导览；`部署运维` 与 `SDK` 不在侧栏，放末尾。）

### 3.3 目录调整

| 现路径 | 新路径 | Tab |
|---|---|---|
| `getting-started/*` | 不变 | 入门 |
| `agent/*` | 不变 | 云助手 |
| `ops/dashboard.md` | `dashboard/index.md` | 监控大盘 |
| `database/{index,sql,ducklake-notes,ducklake}.md` | 不变 | 数据库管理（title 改为「数据库管理」） |
| `database/kv.md` | `kv/index.md` | Key-Value |
| `storage/*` | 不变 | 对象存储 |
| `gofunction/*`、`cronjob/*`、`sandbox/*` | 不变 | 云函数 / 定时任务 / 云沙盒 |
| `ops/logs.md` | `logs/index.md` | 日志管理 |
| `ops/users.md` | `users/index.md` | 用户管理 |
| `ops/{index,deployment,migration}.md` | 不变 | 部署运维 |
| `sdk/*` | 不变 | SDK |

`docs/_meta.json` 按上表重写 `order`（1~13）与 `title`，title 与 `locales/zh-CN.ts` 的 `nav.*` 保持字面一致（数据库管理、日志管理、用户管理、监控大盘）。

### 3.4 联动改动（必须一起做，否则 `docs-content` 测试会失败）

1. 全仓搜索并更新指向旧路径的链接：`docs/**/*.md`、根 `README.md`、`docs/README.md`、`ui/src/pages/Home.vue` 等站内入口。相对链接（`../ops/logs.md`）会按 `rewriteDocHref` 重写，需逐条核对锚点。
2. 旧链接兼容：`ui/src/docs/catalog.ts` 增加 `legacyDocRedirects`（`ops/dashboard → dashboard`、`ops/logs → logs`、`ops/users → users`、`database/kv → kv`），在 `DocsWiki.vue` 的 `syncFromRoute` 里命中时 `router.replace`，避免外部书签 404。
3. `ops/index.md` 中对已迁出页面的引用改成跳转链接。
4. `plan/` 下历史文档不改（仅历史记录）。

### 3.5 顺序一致性保护

新增 `tests/docs-order.test.ts`：把预期顺序写成常量（对应侧栏路由名 `agents → dashboard → databases → key-value → s3 → gofunctions → cron-jobs → sandboxes → logs → users` 映射到文档模块 id），断言 `docCatalog` 中这些模块相对顺序一致。后续调整侧栏时测试会提示同步文档。

### 3.6 内容扩写规范

每个模块的概览页补充「使用场景」章节，场景示例遵循：

- 示例必须与真实接口一致（写前逐条对照 `internal/api/*_handler.go` / `router.go` 与 `packages/*-sdk`），统一变量 `BASE` / `PROJECT` / `SB_KEY`，本地开发 Key 用 `sb_live_dev_key_12345`、项目 `dev-shop`。
- 每个场景结构：**背景 → 前置条件/权限 → 步骤（控制台 + curl/SDK 二选一或并列）→ 预期结果 → 常见错误**。
- 代码围栏必须标注语言；每篇仅一个 H1；站内链接与锚点可解析（`docs-content.test.ts` 守护）。
- 不写未实现能力（遵守 `gofunction/README.md` 能力边界，预留空实现不宣传）。

| 模块 | 计划新增场景 |
|---|---|
| 入门 | 「电商订单后端」端到端串联：建库建表 → 写订单 → KV 缓存库存 → 上传商品图 → 云函数算金额 → 定时任务日报 |
| 云助手 | 自然语言查库；用日志排查报错；启用云沙盒跑脚本；Agent 定时调度做周报；`@` 选择多 Agent 的用法 |
| 监控大盘 | 如何读懂请求/错误率/延迟分位数（说明错误率为百分比、保留 4 位）；用 `metrics/summary` 写简易告警脚本；趋势图折线/柱状切换 |
| 数据库管理 | 建表与批量写入；分页与聚合查询；只读 Key 出报表；系统库只读规则；常见 SQL 拒绝原因 |
| Key-Value | 会话缓存 + TTL；`INCR` 限流计数；`ZSET` 排行榜；`SET NX EX` 简易分布式锁；`LIST` 轻量队列；只读 Key 能做什么 |
| 对象存储 | 上传头像并分享预签名链接；按前缀组织目录；**控制台预览/下载（本次新功能）**；大文件上限与 key 命名规则；CORS 配置示例 |
| 云函数 | Webhook 接收并写库；读写 KV 的函数；函数版本回滚；调用示例（curl + JS SDK） |
| 定时任务 | 每日清理过期数据；定时汇总写报表；固定入参健康检查；查看运行记录与失败排查 |
| 云沙盒 | 数据清洗脚本；安装依赖并运行；文件上传下载；生命周期与资源回收 |
| 日志管理 | 按 level/关键字/时间排错；保留策略设置；审计与业务日志的区别 |
| 用户管理 | 角色与权限矩阵；给运维创建只读账号；首次登录强制改密流程 |
| SDK | JS 与 Go 各一个「小型完整项目」示例（建库 → 写入 → 查询 → 上传 → 错误处理） |
| 部署运维 | 补 S3 CORS 与私有 bucket 示例、监控指标采集建议 |

### 3.7 DocsWiki 页面本身

- `DocsWiki.vue` 的 Tab 渲染直接用 `docCatalog` 顺序，调整 `_meta.json` 后无需改模板。
- Tab 增多（13 个）后，保留 `overflow-x-auto`；检查选中 Tab 是否滚入可视区（必要时在 `onModuleTab` / 路由同步后对激活项 `scrollIntoView({ inline: 'center', block: 'nearest' })`）。
- 现有 `tests/DocsWiki*.test.ts`、`docs-catalog.test.ts` 若依赖旧模块 id，按新结构同步。

---

## 4. 实施顺序与工作量

| 阶段 | 内容 | 预估 |
|---|---|---|
| P0 | §1 默认折线 + 错误率格式化（含口径确认） | 0.5h |
| P1 | §2 S3 表头改名 + `filePreview.ts` + `S3PreviewModal` + i18n + 测试 | 3~4h |
| P2 | §3.2~3.5 文档目录迁移、`_meta.json`、旧链接重定向、顺序测试 | 1.5h |
| P3 | §3.6 文档内容扩写（最耗时，可按模块分批提交，每批过 `docs-content` 测试） | 4~6h |
| P4（可选） | §1.3-5 tooltip 错误率；§2.5 方案 B 后端下载/代理；pdf/audio/video 预览 | 另议 |

## 5. 验收清单

- [x] 打开监控大盘，趋势图默认为折线；摘要行错误率为 4 位小数（采用方案 A）。
- [x] 对象存储表头显示「文件」；点击「打开」弹出 Modal；jpg/jpeg/png/webp/svg 显示图片，md/txt/log/logs 显示文本，其它类型显示空态；Modal 内「下载」可用；关闭 Modal 会中止请求并释放 Blob URL。
- [x] 本地 FileStore（`data:` URL）与真实 S3 两种模式均可预览；CORS 失败时给出提示并降级为新窗口打开。
- [x] 文档站 Tab 顺序与侧边栏一致；`/docs/ops/logs`、`/docs/ops/dashboard`、`/docs/ops/users`、`/docs/database/kv` 会重定向到新模块。
- [x] 各模块文档含「使用场景」示例；云函数示例已在解释器中实跑验证（`Calc` → 487.5 / 465.075，`Fetch` 能带上 `Authorization` 头）。
- [x] `cd ui && yarn test`、`yarn build` 通过，覆盖率不低于基线；`en-US` 与 `zh-CN` key 集合一致。
- [x] 未新增依赖，未修改 `package.json` / `vite.config.ts` / `tsconfig.json`；新增对象形状均使用 `interface`。

## 5.1 实施记录与偏差

| 计划条目 | 实际做法 | 说明 |
|---|---|---|
| §1.3-5 tooltip 错误率 | 未做 | 归入 P4；趋势图数据本身无错误率字段，需要自定义 formatter，按「可选」处理 |
| §2.5 后端下载/代理 | 未做 | 采用纯前端方案；下载统一走 `fetch → blob → <a download>`，失败降级 `window.open` |
| §3.3 目录调整 | 已完成 | `ops/dashboard.md`→`dashboard/index.md`、`ops/logs.md`→`logs/index.md`、`ops/users.md`→`users/index.md`、`database/kv.md`→`kv/index.md`（用 `git mv` 保留历史） |
| §3.5 顺序一致性 | 用 `tests/docs-order.test.ts` 固化 | 断言「入门在前 + 侧栏顺序 + 旧路径重定向目标存在」 |
| §3.7 激活 Tab 滚入可视区 | 已实现 | `DocsWiki.vue` 的 `revealActiveTab()`，在 `moduleId` 变化与 `onMounted` 后调用 |
| 对象存储 CORS 说明 | 已补进 `docs/ops/deployment.md` | 新增「对象存储 CORS」小节与 Prometheus 采集告警建议 |

补充发现（实施中修正的事实性问题）：

1. **错误率口径**：`internal/systemdb/metrics.go` 的 `ErrorRate = errors / requests` 是 0~1 比值，旧 UI 直接拼 `%` 会出现「0.2%」这类错误显示。已改为 ×100 并固定 4 位，`api-mock.ts` 的 `errorRate` 由 `1.5` 改为更真实的 `0.015`。
2. **只读角色是 `admin` 不是 `user`**：`internal/auth/role.go` 中 `admin` 只授予 `database:read` + `llm:invoke`，`user` 反而是可写角色。用户文档初稿写反，已修正。
3. **用户管理入口**：控制台 `Users.vue` 依赖 `meta.requiresRole = 'superadminl1'`，`admin` 可调只读用户接口但看不到页面入口。
4. **指标窗口是近 24 小时**：`MetricsSummary` 的请求数/错误率/平均延迟都以 `since = now-24h` 计算，初稿写成「近期」，已改为「近 24 小时」。
5. **日志是内存缓冲批量刷写**：默认约 2 秒一批，`RecordLog` 队列满会丢弃最早的记录；文档已如实说明。
6. **KV 写命令需要 `database:write`**：读命令只需 `database:read`，写命令缺少权限返回 403（`auth.ErrForbidden`），不是 503。
7. **云函数不能直连 DB/KV/S3**：没有内置 SDK，只能通过 `net/http` 回调 HTTP API；示例据此编写并实跑验证。

## 6. 需要你确认的点（落地时按默认方案执行）

1. 错误率口径：**采用方案 A**（比值 ×100，显示 `xx.xxxx%`，同时修正原口径 bug）。
2. S3 预览：**采用纯前端 + CORS 失败降级**；若后续需要彻底避开 CORS，再走方案 B（下载参数 / 同源内容代理）。
3. 文档：**已将 `dashboard / kv / logs / users` 拆为独立模块**，并加旧路径重定向。
4. pdf / 音频 / 视频：**未纳入首期**；`filePreview.ts` 的扩展名表可低成本扩展。
