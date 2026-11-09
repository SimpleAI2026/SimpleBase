# planv6.1 文档站六大模块收敛

> 状态：**已实施完成**（2026-10-10）。承接 `planv6.0`（其中 §3 的文档模块拆分被本文取代）。
> 起因：planv6.0 把文档拆成 13 个 Tab，`DocsWiki.vue` 顶部导航过长、与控制台侧栏分组对不上。

## 1. 目标结构

Tab 从 13 个收敛为 6 个，与控制台侧栏分组一一对应：

| 顺序 | 模块 id | Tab | 子主题（`group`） | 对应控制台入口 |
|---|---|---|---|---|
| 1 | `getting-started` | 入门 | 上手、核心概念 | — |
| 2 | `workbench` | 工作台 | 云助手、监控大盘 | 工作台 |
| 3 | `data` | 数据 | 数据库管理、Key-Value、对象存储 | 数据 |
| 4 | `automation` | 自动化 | 云函数、定时任务、云沙盒 | 自动化 |
| 5 | `ops` | 运维 | 日志与审计、部署与故障处理 | 运维 |
| 6 | `sdk` | SDK | JavaScript、Go | — |

`用户管理` 不再进入文档站（页面仍在控制台，仅超管可见），相关文档已删除。

## 2. 目录迁移

原来「一个子主题一个目录」改为「一个 Tab 一个目录」，子主题用文件名前缀区分，不再嵌套子目录。

| 旧路径 | 新路径 |
|---|---|
| `agent/index.md` | `workbench/agent.md` |
| `agent/tools.md` | `workbench/agent-tools.md` |
| `agent/e2e.md` | `workbench/agent-e2e.md` |
| `dashboard/index.md` | `workbench/dashboard.md` |
| `database/index.md` | `data/index.md` |
| `database/sql.md` | `data/sql.md` |
| `database/ducklake-notes.md` | `data/ducklake-notes.md` |
| `database/ducklake.md` | `data/ducklake.md` |
| `kv/index.md` | `data/kv.md` |
| `storage/index.md` | `data/storage.md` |
| `storage/api.md` | `data/storage-api.md` |
| `gofunction/overview.md` | `automation/gofunction.md` |
| `gofunction/invoke.md` | `automation/gofunction-invoke.md` |
| `cronjob/overview.md` | `automation/cronjob.md` |
| `cronjob/api.md` | `automation/cronjob-api.md` |
| `sandbox/index.md` | `automation/sandbox.md` |
| `sandbox/api.md` | `automation/sandbox-api.md` |
| `sandbox/e2e.md` | `automation/sandbox-e2e.md` |
| `logs/index.md` | `ops/logs.md` |
| `users/index.md` | **删除** |

新增 3 个模块首页：`workbench/index.md`、`data/index.md`（由原数据库概览改写）、`automation/index.md`。

迁移全部用 `git mv`，保留文件历史。

## 3. 侧栏分组规则

`DocsSidebar.vue` 在 `group` 变化时插入分组标题，因此**同一 `group` 的页面 `order` 必须连续**，否则会出现两个同名标题。

| 模块 | order 分配 |
|---|---|
| `workbench` | index 0；云助手 1/2/3；监控大盘 10 |
| `data` | index 0；数据库管理 1/2/3；Key-Value 10；对象存储 20/21 |
| `automation` | index 0；云函数 10/11；定时任务 20/21；云沙盒 30/31/32 |
| `ops` | index 0；日志 1；部署 2 |

`data/index.md` 由「数据库概览 + 使用场景」精简为落地页（怎么选 + 约束），数据库的 5 个使用场景移到 `data/sql.md`，避免首页过长。

## 4. 代码改动

| 文件 | 改动 |
|---|---|
| `docs/_meta.json` | 重写为 6 个模块 |
| `docs/README.md` | 重写写作约定：六大模块表、`group` 连续规则、文件名规则 |
| `ui/src/docs/catalog.ts` | 懒加载路径 `database/ducklake.md` → `data/ducklake.md`；`largeMeta` 补 `group`；`legacyDocRedirects` 扩展为「模块级 + 子页级」两张映射，`resolveLegacyDocPath` 同时支持 `模块` 与 `模块/slug` |
| `ui/tests/docs-order.test.ts` | 重写：断言只有 6 个 Tab、顺序与标题固定、各模块含约定子主题页、user 文档已移除、`group` 连续不交错、旧路径重定向目标存在 |
| `ui/tests/docs-content.test.ts` | 把硬编码的 `database/ducklake.md` 改为 `isLargeDoc()` 判断 |
| `ui/tests/docs-catalog.test.ts` | 懒加载路径与搜索断言改为新模块 id |
| `ui/tests/DocsSearch.test.ts`、`docs-render.test.ts`、`DocsWiki.test.ts` | 路径断言同步 |
| `docs/**`、`README.md`、`packages/*/README.md` | 全仓路径引用批量迁移 |

### 重定向策略

`legacyDocRedirects` 是**扁平表**，key 支持两种形式：

- `模块/slug`：`ops/dashboard`、`ops/logs`、`ops/users`、`database/kv`
- `模块`：`agent`、`dashboard`、`kv`、`logs`、`users`、`database`、`storage`、`gofunction`、`cronjob`、`sandbox`

`resolveLegacyDocPath(moduleId, slug)` 在 `slug` 非空且不为 `index` 时先查 `模块/slug`，否则查 `模块`。`ops/logs` 现在也是合法路径（日志已并入运维），表中把它指向自身以保持「重定向目标必须存在」的断言通过。

## 5. 验证

```bash
cd ui && npx vitest run tests/docs-order.test.ts tests/docs-content.test.ts \
  tests/docs-catalog.test.ts tests/DocsWiki.test.ts tests/docs-components.test.ts \
  tests/DocsSearch.test.ts tests/docs-render.test.ts      # 43 passed
cd ui && yarn test        # 105 files / 664 tests，statements 99.49% / branches 95.01% / functions 95.35%
cd ui && yarn build       # 通过
```

`_meta.json` 输出 6 个模块，Tab 顺序为 入门 → 工作台 → 数据 → 自动化 → 运维 → SDK。

## 6. 遗留

- 用户管理页面仍在控制台（`/console/users`，`requiresRole: superadminl1`），只是不再有对应文档；`ui/src/locales/*.ts` 的 `nav.users` 保留。
- `/docs/users` 旧链接重定向到 `/docs/data`（数据模块首页），不是用户管理文档。
