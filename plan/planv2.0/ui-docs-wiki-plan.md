# UI 使用文档（Wiki 风格）计划

> **状态**：已实现（本地 MVP）  
> **日期**：2026-09-17  
> **目标**：顶栏右上角「使用文档」进入应用内 Wiki：顶部模块（文件夹）、左侧 MD 目录、右侧正文；样式与信息架构参考 GitHub Wiki。  
> **关联**：[`ui-plan-v2.md`](./ui-plan-v2.md)、[`ui-principles.md`](./ui-principles.md)、[`ui-style-plan.md`](./ui-style-plan.md)

---

## 0. 一句话目标

在产品内提供**只读**使用文档站：源文件为仓库 `docs/<module>/*.md`，前端按 GitHub Wiki 布局浏览（模块 Tab + 侧栏页面列表 + Markdown 渲染），不依赖后端、不进入侧栏主导航。

---

## 1. 需求还原

| # | 需求 | 落点 |
|---|---|---|
| 1 | 右上角增加「使用文档」，点击进入文档页 | `DefaultLayout` header-right，位于 GitHub 图标左侧 |
| 2 | UI 左右结构：左读 MD 结构，右为内容；顶部为文件夹模块，每模块多篇 MD | 路由 `/docs`；顶栏模块、左 TOC、右正文 |
| — | 参考 GitHub Wiki 样式与结构 | 侧栏页名列表、当前页高亮、面包屑、正文排版、锚点标题 |
| 3 | （原需求第 3 点为空） | **本计划默认假设**见 §2；若后续要可编辑 / 远端拉取，另开修订 |

---

## 2. 默认假设（第 3 点空缺时的锁定）

1. **只读**：产品内不可编辑 MD；改文档走 Git PR。  
2. **源目录**：`docs/<module>/*.md`（一期把现有扁平文件迁入子目录，并留兼容 redirect）。  
3. **交付形态**：前端静态加载（Vite `import.meta.glob`）；**不新增**后端文档 API。  
4. **不进侧栏** `NavMenu`；仅顶栏入口 + 可选页脚链接。  
5. **不要求登录**：与现有布局一致即可（仍在 `DefaultLayout` 内）。

若产品后续要「可编辑 Wiki / 多租户文档」，在本计划 Phase 4 占位，本期不做。

---

## 3. 信息架构

### 3.1 路由

| 路径 | 说明 |
|---|---|
| `/docs` | 默认跳到第一个模块的首页（或 `getting-started/README`） |
| `/docs/:module` | 模块首页（该模块 `index.md` / `README.md`，否则第一篇） |
| `/docs/:module/:slug` | 指定文档（`slug` = 文件名去 `.md`） |

旧书签：无。

### 3.2 页面骨架（GitHub Wiki 感）

```
┌─────────────────────────────────────────────────────────────┐
│ DefaultLayout header … [使用文档] [GitHub] [刷新]            │
├─────────────────────────────────────────────────────────────┤
│ 模块 Tab / 分段：入门 | 数据库 | 对象存储 | 运维 | …          │  ← 对应 docs 子目录
├──────────────┬──────────────────────────────────────────────┤
│ 本模块页面    │  面包屑：使用文档 / 模块 / 当前页               │
│ · 简介        │  # 标题                                       │
│ · 快速开始 ●  │  正文 Markdown（含代码块、表格、内链）          │
│ · FAQ         │  （可选）页内 TOC / 上一篇下一篇               │
└──────────────┴──────────────────────────────────────────────┘
```

窄屏：模块改为 Select；左侧 TOC 收入 Drawer。

### 3.3 源目录约定（一期重组）

当前扁平：

```
docs/deployment.md
docs/ducklake-docs.md
docs/migration-guide.md
```

目标：

```
docs/
  getting-started/
    index.md          # 模块首页（必填其一：index.md 或 README.md）
    overview.md       # 可选扩充
  database/
    index.md          # 从 ducklake-docs 拆摘要或迁入全文
    ducklake.md       # 原 ducklake-docs.md（可保留长文）
  ops/
    index.md
    deployment.md     # 原 deployment.md
    migration.md      # 原 migration-guide.md
  _meta.json          # 可选：模块顺序、显示名、图标（见 §4）
```

迁移策略：

- Git 用 `git mv` 保留历史。  
- 在旧路径留 **短 stub**（可选）：`docs/deployment.md` → 一行说明已迁至 `ops/deployment.md`（前端可不加载 stub）。  
- `ducklake-docs.md` 体积大：一期整文件迁到 `database/ducklake.md`，`database/index.md` 写短导读 + 链接。

### 3.4 模块与文案（默认中文名）

| 目录 `module` | 顶栏显示名 | 说明 |
|---|---|---|
| `getting-started` | 入门 | 产品是什么、如何打开控制台 |
| `database` | 数据库 | DuckLake / SQL / 集合 |
| `ops` | 运维 | 部署、迁移 |

后续可加 `s3` / `agents` / `faq`，只需加目录与 `_meta.json` 项，无需改路由代码（约定优于配置）。

---

## 4. 元数据与目录发现

### 4.1 `_meta.json`（推荐，可选）

```json
{
  "modules": [
    { "id": "getting-started", "title": "入门", "order": 1 },
    { "id": "database", "title": "数据库", "order": 2 },
    { "id": "ops", "title": "运维", "order": 3 }
  ]
}
```

无 `_meta.json` 时：按目录名排序；显示名 = 目录名。

### 4.2 单页 frontmatter（可选，一期可用 H1）

```md
---
title: 部署指南
order: 10
---
# 部署指南
```

无 frontmatter 时：`title` = 首个 `#` 标题，否则 = `slug`。

### 4.3 前端加载

```ts
// ui/src/docs/loadDocs.ts
const raw = import.meta.glob('/docs/**/*.md', { query: '?raw', import: 'default', eager: true })
```

注意：Vite 默认只扫 `ui/` 内；需二选一：

**方案 A（推荐）**：把文档副本或软链放到 `ui/public/docs` / `ui/src/docs-content`，构建时 `import.meta.glob`。  
**方案 B**：Vite `server.fs.allow` + glob 指向仓库根 `docs/`（dev OK；生产需把 md 打进 bundle 或 copy 插件）。

**本计划锁定方案 A 变体**：构建脚本或 Vite 插件将仓库根 `docs/**` **复制/别名**到 `ui/src/docs-content/**`，单一真相仍是仓库根 `docs/`；避免文档写两份。

实现建议：`vite.config.ts` 里 `resolve.alias: { '@docs': path.resolve(__dirname, '../docs') }` + `import.meta.glob('@docs/**/*.md', { as: 'raw', eager: true })`，并配置 `server.fs.allow: ['..']`。

---

## 5. UI 组件与文件

| 文件 | 职责 |
|---|---|
| `ui/src/pages/DocsWiki.vue` | 页面壳：模块条 + 左右分栏 |
| `ui/src/components/docs/DocsModuleTabs.vue` | 顶部模块切换 |
| `ui/src/components/docs/DocsSidebar.vue` | 左栏页面列表 |
| `ui/src/components/docs/DocsArticle.vue` | 右栏：面包屑 + Markdown 正文 |
| `ui/src/docs/catalog.ts` | 解析 glob → modules/pages 树 |
| `ui/src/docs/render.ts` | MD → 安全 HTML |

### 5.1 Header 入口

在 `DefaultLayout.vue` 的 `sb-header-right` 中，**GitHub 图标左侧**增加：

```vue
<a-tooltip title="使用文档">
  <router-link to="/docs" class="sb-icon-link sb-docs-link">使用文档</router-link>
</a-tooltip>
```

可用 `ReadOutlined` / `BookOutlined` + 文案「使用文档」（桌面显示文字，窄屏可只图标）。  
当前页为 `/docs*` 时链接高亮。

### 5.2 Markdown 渲染

- 依赖：`marked`（或 `markdown-it`）+ `DOMPurify`（防 XSS）。  
- 代码高亮：一期可用简单 `<pre><code>`；二期再加 `shiki` / highlight.js。  
- **站内链接**：`[部署](./deployment.md)` / `[部署](/docs/ops/deployment)` 解析为 `router-link` 或点击拦截。  
- 外链：`target=_blank` + `rel=noopener`。  
- 图片：相对路径相对当前 md 文件解析（一期可限制为绝对 URL）。

### 5.3 GitHub Wiki 样式要点

- 左栏宽约 240–280px，底边/右边分割线用 `--sb-border`。  
- 当前页：左侧项主色背景浅底或左边框高亮。  
- 正文：最大宽 ~900px，标题层级间距参考 GitHub；表格、引用块有基本样式。  
- 可选：右上「在 GitHub 上编辑」链到 `https://github.com/<org>/SimpleBase/blob/main/docs/...`（只读增强，非编辑器）。

---

## 6. 与侧栏 / 权限

- **不**加入 `NavMenu` `routeOrder`。  
- 文档页仍包在 `DefaultLayout`（有项目切换器）；文档内容**不绑定**当前项目（全局产品文档）。  
- 无需新 API Key scope。

---

## 7. 实施阶段

### Phase 0 — 文档重组（0.5d）

- `git mv` 现有 3 个 md 到 `docs/ops/`、`docs/database/`。  
- 补 `getting-started/index.md`、各模块 `index.md` 短文。  
- 可选 `_meta.json`。

### Phase 1 — 路由 + 壳 + 入口（0.5–1d）

- `/docs/:module?/:slug?` + redirect。  
- Header「使用文档」。  
- 空壳：模块条 + 左列表 + 右占位。

### Phase 2 — Catalog + Markdown 渲染（1d）

- alias/glob 加载 `docs/**/*.md`。  
- `marked` + `DOMPurify`；侧栏与路由联动。  
- 基础正文 CSS；站内 `./xxx.md` 链接。

### Phase 3 — 体验打磨

- 面包屑、上一篇/下一篇、窄屏 Drawer。  
- 「在 GitHub 上查看」外链。  
- `yarn build` 验证 md 打进产物。

### Phase 4 — 非目标占位

- 可编辑 Wiki、搜索全文、多语言、后端托管、版本切换。

---

## 8. 验收标准（MVP = Phase 0–2）

1. 顶栏右上角可见「使用文档」，点击进入 `/docs`。  
2. 顶部可切换 ≥2 个模块；切换后左侧页面列表变化。  
3. 点击左侧项，右侧渲染对应 Markdown（含标题、段落、代码块）。  
4. 刷新深链 `/docs/ops/deployment` 仍正确。  
5. 不进侧栏菜单；`yarn build` 通过。  
6. 源文件在仓库根 `docs/<module>/`，非只存在于 `dist`。

---

## 9. 非目标

- 文档在线编辑 / 评论 / 点赞。  
- 按项目或租户隔离文档。  
- 后端 `/v1/docs` API。  
- 将 `plan/planv2.0/*.md` 自动暴露为用户文档（计划 ≠ 使用文档）。

---

## 10. 风险与对策

| 风险 | 对策 |
|---|---|
| Vite 无法直接 glob 仓库根外文件 | alias + `fs.allow`；或 copy 插件到 `ui/src/docs-content` |
| `ducklake-docs.md` 过大拖慢首包 | 路由级动态 `import()` 非 eager；或拆章 |
| MD 含 HTML 导致 XSS | 一律 DOMPurify |
| 相对链接失效 | 统一链接改写工具 + 迁移时检查 |

---

## 11. 文档与导航更新（实现时）

- [x] 本文件状态改为「实现中 / 已实现」  
- [x] [`ui-plan-v2.md`](./ui-plan-v2.md) 配套表增加一行  
- [ ] README 可链到 `/docs` 或 `docs/getting-started`  

---

## 12. 决议摘要

1. 入口：顶栏「使用文档」→ `/docs`。  
2. 布局：顶模块 + 左 TOC + 右 MD（GitHub Wiki 风）。  
3. 源：`docs/<module>/*.md`，只读；前端加载，无新后端。  
4. 原需求第 3 点空缺：按 §2 假设执行；要可编辑再开新 plan。
