# 控制台表格居中、默认进入云助手、新建数据库间距

> **仓库**：SimpleBase（https://github.com/linkxzhou/SimpleBase）
> **状态**：**已实施**（2026-10-09，按本文落地，没有被迫改设计）
> **日期**：2026-10-09
> **Verified against**：`main` @ `707b7ae`。路径、class 与路由按该提交核对。#31 实施结束时仍未合入 `main`，因此没有做冲突合并。
> **并行**：[#31](https://github.com/linkxzhou/SimpleBase/pull/31)（`cursor/console-loading-states-plan-a0aa`）只新增 `plan/planv5.0/console-loading-states-plan.md`，实施时会改多张表的骨架与空态。本计划与它对齐方式不同：默认对齐落在共享 `TableHead` / `TableCell`，页面只改会盖住默认值的 class。预期冲突文件见 §6。

## 0. 目标

1. 控制台数据表的表头和单元格默认水平居中。操作列（「操作」）的标题和按钮组一起居中。长文本、代码、动态 SQL 结果、以及「单元格里嵌了一整块面板」的容器保留左对齐。
2. 进入控制台的默认落点改为云助手 `/console/agents`。`/console`、首页「控制台 / 进入控制台」、文档站「返回控制台」、设置 deep link、无权限时从用户页弹回，都落到云助手。已有深链（`/console/databases`、`/llm`、`/agents` 等）继续打开原来的页面。监控大盘保留为 `/console/dashboard`，侧栏仍可进入。
3. 「新建数据库」弹窗里，名称、数据类型、初始化 SQL 这几组字段之间的垂直间距，与「新建项目」等弹窗使用同一套 `FieldGroup` 的 `gap-5`。

不改加载骨架、空态文案、侧栏顺序（云助手已经是工作台第一项）、登录成功后的强制跳转、文档站 Markdown 表格。

## 1. 表格对齐

### 1.1 共享组件（默认值的来源）

| 组件 | 文件 | 现状 |
| --- | --- | --- |
| `Table` | `ui/src/components/ui/table/Table.vue` | `<table>` 只有宽度、字号、边框。不设 `text-align`。外层是 `overflow-x-auto` |
| `TableHead` | `ui/src/components/ui/table/TableHead.vue:13` | `text-left align-middle`，另有 `h-11 px-4 text-xs font-medium uppercase tracking-wider whitespace-nowrap` |
| `TableCell` | `ui/src/components/ui/table/TableCell.vue:13` | `px-4 py-3 align-middle whitespace-nowrap`。**没有**水平对齐，浏览器默认左对齐 |
| `TableEmpty` | `ui/src/components/ui/table/TableEmpty.vue:29` | 空态内容在 `flex items-center justify-center` 里，已经居中 |
| `TableHeader` / `TableRow` | 同目录 | 只负责边框和 hover，不设对齐 |

`cn()` 走 tailwind-merge。页面传的 `text-left` / `text-right` 写在 `props.class` 里，会盖过组件上的 `text-center`。所以只改组件、不删页面上的反向 class，操作列和数字列仍然偏右或偏左。

按钮行是另一个缺口。`text-center` 能居中行内内容（普通文字、`inline-flex` 的按钮）。`class="flex …"` 是块级 flex，占满单元格宽度，子项默认 `justify-start`，表头居中了，按钮仍贴在左边。当前操作列几乎都是这种写法。

`ui/AGENTS.md` 把 `src/components/ui/` 视为本地 fork，「只增删组件不改风格约定」。本次是明确要求的对齐修正，范围只限 `TableHead` 的 `text-left` 改为 `text-center`，以及 `TableCell` 增加 `text-center` 和直接子级 flex 的 `justify-center`。不改字号、字距、大写、padding、空态结构。

### 1.2 逐表盘点

「表头」列写的是相对组件默认 `text-left` 的额外 class。「单元格」列写的是相对 `TableCell`（无水平对齐）的额外 class。操作列的按钮容器单独标出。没有标对齐的列 = 表头左、单元格左。

| # | 表面 | 文件 | 表头 | 单元格 | 操作列 |
| --- | --- | --- | --- | --- | --- |
| 1 | 数据库 | `pages/Databases.vue` 桌面表 | 展开钮 `text-center`；数据量 `text-right`；其余左，含「操作」 | 展开钮 `text-center`；数据量 `text-right tabular-nums`；名称是 `inline-flex`；其余左 | `div.flex.flex-wrap`，无 justify，按钮靠左 |
| 2 | 数据库展开行 | 同上 `TableCell colspan="7"` | — | 容器 `p-0`，里面是 `SchemaPanel` / `DataTabs` | 不是数据列 |
| 3 | 数据库移动端 | 同上 `md:hidden` 卡片 | 不是表。定义列表 `grid-cols-[auto_minmax(0,1fr)]`，标签左、值左 | — | 按钮行靠左 |
| 4 | 监控大盘资源表 | `pages/Dashboard.vue` | 数量 `text-right`；资源类型、说明左 | 数量 `text-right tabular-nums`；其余左 | 无操作列 |
| 5 | 云函数 | `pages/GoFunctions.vue` | 全部左，含「操作」 | 全部左。导出函数按钮自带 `text-left`（多行函数名） | `div.flex.flex-nowrap`，靠左 |
| 6 | 定时任务 | `pages/CronJobs.vue` | 全部左，含「操作」 | 全部左。启用列是 `Switch` | `div.flex.flex-wrap`，靠左 |
| 7 | 云沙盒 | `pages/Sandboxes.vue` | 全部左，含「操作」 | 全部左 | `div.flex.flex-wrap`，靠左 |
| 8 | 对象存储 | `pages/S3Manager.vue` | 全部左，含「操作」 | 全部左 | `div.flex`，靠左 |
| 9 | 日志 | `pages/Logs.vue` | 全部左 | 全部左。消息列 `truncate` | 无操作列 |
| 10 | 用户 | `pages/Users.vue` | 项目数 `text-right`；「操作」左 | 项目数**没有** `text-right`（表头右、数字左，已经不一致）；操作左 | `div.flex`，靠左 |
| 11 | 集合 | `components/databases/CollectionPanel.vue` | 操作 `text-right`；名称左 | 操作 `text-right` | `flex justify-end` |
| 12 | KV 列表 | `components/databases/kv/KvPanel.vue` | 操作 `text-right`；其余左 | 操作 `text-right`；按钮是 `inline-flex` | 整组靠右 |
| 13 | KV API | `components/databases/kv/KvApiPanel.vue` | 操作 `text-right`；命令 / 参数 / 语义左 | 操作 `text-right`；参数 `truncate`；语义是句子 | `flex justify-end` |
| 14 | Hash | `components/databases/kv/editors/KvHashEditor.vue` | 操作 `text-right`；字段、值左 | 操作 `text-right`；值可换行 | `flex justify-end` |
| 15 | List | `components/databases/kv/editors/KvListEditor.vue` | `#` 与操作 `text-right`；元素左 | `#` 为 `text-right tabular-nums`；操作 `text-right`；元素可换行 | 单个「改」按钮靠右 |
| 16 | ZSet | `components/databases/kv/editors/KvZSetEditor.vue` | 分数、操作 `text-right`；成员左 | 分数 `text-right`（编辑时 `justify-end`）；操作 `text-right` | `flex justify-end` |
| 17 | Set | `KvSetEditor.vue` | 不是表。徽章流式排列 | — | — |
| 18 | 文档行 | `components/modal/DocumentListModal.vue` | 全部左，含「操作」 | ID 左；数据是 `SbCodeBlock`；操作是单个按钮，无 flex | 按钮靠左 |
| 19 | SQL 查询结果 | `components/modal/SqlWorkModal.vue` | 动态列名，全部左 | 动态值 `truncate`，全部左 | 无 |
| 20 | SQL 批量结果 | 同上 | 全部左（`#`、状态、受影响行数、耗时、错误详情） | 全部左 | 无 |
| 21 | 云助手 SQL 工具卡 | `components/agent/AgentToolCard.vue:19` | 原生 `<table class="w-full border-collapse text-left">`，`th`/`td` 只有 `border-b px-1 py-1` | 动态 SQL 预览，整表左对齐 | 无 |
| 22 | 表结构 | `components/databases/SchemaPanel.vue` | 不是表。名称 + 列清单 | — | — |
| 23 | 文档站 Markdown | `ui/src/docs/render.ts` 把 marked 的 `<table>` 包进 `docs-table-scroll` | 阅读用表格，浏览器默认左对齐 | — | — |

骨架行（`Skeleton` 横跨整行）和对齐无关。`TableEmpty` 已居中。

### 1.3 决策

| # | 决策 | 含义 |
| --- | --- | --- |
| 1 | 默认居中写在 `TableHead` 和 `TableCell` | `TableHead` 的 `text-left` 换成 `text-center`。`TableCell` 增加 `text-center`，并加 `[&>.flex]:justify-center`，让直接子级 `flex` 操作条居中。`text-center` 同时盖住普通文字和 `inline-flex`（名称旁的图标、KV 列表的按钮组、单个删除按钮） |
| 2 | 页面上的反向 class 必须删掉 | `text-right`、操作列的 `justify-end`、分数编辑行的 `justify-end` / `text-right`。留下它们会在 tailwind-merge 里赢过组件默认值。`tabular-nums` 保留 |
| 3 | 长文本、代码、动态结果保持左对齐 | 见下表。表头和单元格一起加 `text-left`，避免标题居中、内容靠左 |
| 4 | 展开行容器保持左对齐 | 数据库展开单元格加 `text-left`。否则 `text-align` 会继承进 `SchemaPanel` / `DataTabs` |
| 5 | 云助手工具卡不改 | 原生表是工具输出预览，列是动态的，保持 `text-left` |
| 6 | 下列表面不改对齐 | 数据库移动端卡片、KV Set 徽章、Schema 清单、文档站 Markdown 表、空态（已经居中） |
| 7 | 不改骨架标记 | 加载行、空态条件、`SbTableSkeleton` 属于 #31。居中默认值会自动作用到它新建的 `TableCell` |

保留左对齐的列：

| 位置 | 列 | 理由 |
| --- | --- | --- |
| `Logs.vue` | 消息 | 日志正文，截断必须从开头读 |
| `DocumentListModal.vue` | 数据 | JSON 代码块。单元格也要 `text-left`，避免 `pre` 继承居中 |
| `SqlWorkModal.vue` 查询结果 | 全部动态列 | 任意 SQL 值，经常是长字符串。在该 `<Table>` 上加 `[&_th]:text-left [&_td]:text-left` |
| `SqlWorkModal.vue` 批量结果 | 错误详情 | 错误字符串。`#`、状态、行数、耗时走默认居中 |
| `KvApiPanel.vue` | 参数、语义 | 命令签名和说明句。命令、操作居中 |
| `KvHashEditor` / `KvListEditor` / `KvZSetEditor` | 值、元素、成员（含对应表头） | 无上界的字符串。`#`、分数、字段名、操作居中。字段名按短标识处理；若目视后前缀被截断裁切，再只给该列加 `text-left` |
| `Databases.vue` | 展开行那个 `colspan` 单元格 | 嵌套面板，不是数据列 |
| `AgentToolCard.vue` | 整张原生表 | 工具结果，不进共享组件 |
| `GoFunctions.vue` | 导出函数按钮上的 `text-left` | 留在按钮上，只影响按钮内部换行，不把整列拉回左边 |

名称、Key、ID、文件名、状态、时间、数量、角色、操作全部走默认居中，包括现在右对齐的数据量、项目数、大盘数量、分数和 `#`。用户表「项目数」表头右、单元格左的不一致会一起消失。

截断列（数据库 ID、S3 Key、KV Key）居中后，省略号可能不在字符串末尾。实施时在这些列上目视一次。若前缀丢失影响辨认，只给该列补 `text-left`，不动共享组件。这是验收时的检查点，不预先把这些列排除。

## 2. 默认进入云助手

### 2.1 现状

登录成功**不会**改路由。`stores/auth.ts` 的 `login()` / `applyTokens()` 只写 token 并关掉弹窗。`LoginModal` 挂在 `DefaultLayout` 里，用户留在进入控制台时的那个页面。项目切换器不调用 router。侧栏 Logo 是 `div`，不是链接。首页 Logo、文档站 Logo 指向 `/`（营销首页）。

真正把人送进控制台、并且今天落到监控大盘的入口：

| 入口 | 文件 | 行为 |
| --- | --- | --- |
| `/console` 空子路径 | `router/index.ts:34-38` | `name: 'dashboard'`，组件是 `Dashboard.vue`，URL 就是 `/console` |
| 首页「控制台」「进入控制台」 | `pages/Home.vue:14`、`:34` | `<router-link :to="{ name: 'dashboard' }">`，解析成 `/console` |
| `/` 带 `settings` | `router/index.ts:24-28` | `beforeEnter` 到 `{ name: 'dashboard', query }` |
| `/settings`、`/console/settings` | `router/index.ts:86-92` 以及 legacy `/settings` | 重定向到 `{ name: 'dashboard', query: { settings } }`，再由 `DefaultLayout` 打开设置弹窗并清掉 query |
| 文档站「返回控制台」 | `layouts/DocsLayout.vue:16` | `{ name: 'dashboard' }` |
| 非超管打开用户页 | `layouts/DefaultLayout.vue:188-197` | `replace({ name: 'dashboard' })` |
| 侧栏无路由名时的选中回退 | `NavMenu.vue:144` | `selectedKey` 回退 `'dashboard'` |

深链已经独立于空路径：`/console/databases`、`/console/agents`、`/console/logs` 等是具名子路由；`/databases` 等八个旧路径进 `/console/*`；`/sql`、`/data` → `/console/databases`；`/llm` → `/console/agents`。这些都不要改目标。

`ui/tests/router.test.ts` 把 `{ name: 'dashboard' }` 断言为 path `/console`。`Home.test.ts` 断言首页链接的 href 是 `/console`、路由名是 `dashboard`。`layouts.test.ts` 断言「返回控制台」的 href 是 `/console`。`ui/README.md` 写「`/console` = 监控大盘」。`ui/AGENTS.md` 写「路由 redirect 保持现状」。

### 2.2 决策

| # | 决策 | 含义 |
| --- | --- | --- |
| 1 | `/console` 空路径改为重定向 | `redirect: (to) => ({ name: 'agents', query: to.query, hash: to.hash })`。不给这段 redirect 起 `dashboard` 这个名字 |
| 2 | 监控大盘改到稳定路径 | `path: 'dashboard'`、`name: 'dashboard'`、`meta.title` 仍是「监控大盘」。侧栏 `GROUP_DEFS` 里已有 `dashboard`，不用改顺序。旧书签 `/console` 的含义从大盘变为云助手，这是本需求本身 |
| 3 | 「进入控制台」使用具名路由 `agents` | 首页两处、文档站「返回控制台」、`/` 的 settings `beforeEnter`、`/console/settings` 的 redirect、用户页无权限回退，全部改为 `{ name: 'agents' }`，并继续带上 query / hash。设置弹窗仍由 `DefaultLayout` 看 `route.query.settings`，与落在哪一页无关 |
| 4 | 登录成功不新增跳转 | 深链页上弹出登录框，登录后必须留在该页。默认落点只来自「进入控制台」的入口，不来自 `auth.login` |
| 5 | 项目切换、侧栏 Logo、营销首页 Logo 不动 | 切换项目留在当前页。Logo 回 `/` 或保持不可点 |
| 6 | `NavMenu` 的空 name 回退改为 `'agents'` | 避免 redirect 瞬间把侧栏亮在监控大盘上 |
| 7 | 旧路径 redirect 的目标不变 | `/databases` → `/console/databases`，`/llm` → `/console/agents`，`/sql` 与 `/data` → `/console/databases`。只更新 `ui/AGENTS.md` 里描述默认入口的那一句，以及 `ui/README.md` 的路由表 |

设置 deep link 的 URL 会从 `/console?settings=…` 变成 `/console/agents?settings=…`。弹窗行为不变。`router.test.ts` 里对应断言要改。

## 3. 新建数据库弹窗间距

### 3.1 现状

`pages/Databases.vue:228-263` 的 `SbModal`（标题「新建数据库」）里，三个 `Field` 是插槽的直接子节点：名称、数据类型、以及 `dataModel === 'sql'` 时的初始化 SQL。

间距来源：

| 层 | class | 作用 |
| --- | --- | --- |
| `DialogContent` | `grid gap-5 p-6`（`ui/dialog/DialogContent.vue:37`） | 页眉、插槽、页脚之间 1.25rem。不是字段与字段之间 |
| `SbModal` 插槽包裹 | `div.min-w-0`（`SbModal.vue:8`） | 没有 flex、没有 gap、没有额外 py |
| `Field` | `fieldVariants` 的 `gap-2`（`ui/field/index.ts:4`） | 标签、控件、说明之间 0.5rem |
| 字段组与字段组 | 无 | 相邻 `Field` 是普通块级元素，外边距为 0 |

对照：

| 弹窗 | 字段组间距 |
| --- | --- |
| 新建项目 `CreateProjectModal.vue` | `FieldGroup`：`gap-5`（1.25rem） |
| 用户表单 `UserFormModal.vue` | 同上 |
| 文档 KV `DocumentKvModal.vue` | 同上 |
| 登录 `LoginModal.vue` | 表单 `gap-4`，内部仍是 `FieldGroup` 的 `gap-5` |
| 定时任务 `CronJobModal.vue` | 外层滚动区 `gap-4 px-2 py-3`，内部 `FieldGroup` 仍是 `gap-5` |
| 新建沙盒 `SandboxCreateModal.vue` | `flex flex-col gap-4 py-3`（手写 label，不是 Field） |

`FieldGroup`（`ui/field/FieldGroup.vue:14`）的 class 是 `gap-5 … flex w-full flex-col`。`gap-5` 就是这套表单的组间距 token。

### 3.2 决策

用 `FieldGroup` 包住这三个 `Field`。组与组之间变为 `gap-5`（1.25rem / 20px），与新建项目、用户表单、文档 KV 相同。SQL 模式多出来的「初始化 SQL」用同一间距。

不改 `Field` 内部的 `gap-2`，不改 `DialogContent` 的 `gap-5` / `p-6`，不加 `py-*`。对话框本身已经有 `p-6`，再加 CronJob 那种 `py-3` 会把正文从页眉和页脚上推开一截，和标准表单弹窗不一致。

## 4. 分文件改动

实施时只改这些文件。

| 文件 | 改动 |
| --- | --- |
| `ui/src/components/ui/table/TableHead.vue` | `text-left` → `text-center`。其余 class 不动 |
| `ui/src/components/ui/table/TableCell.vue` | 增加 `text-center` 与 `[&>.flex]:justify-center` |
| `ui/src/pages/Databases.vue` | 去掉数据量的 `text-right`。操作条加上 `justify-center`（与单元格规则重复无害，写上便于读模板）。展开单元格加 `text-left`。新建数据库的三个 `Field` 包进 `FieldGroup`。不改移动端卡片，不改骨架行 |
| `ui/src/pages/Dashboard.vue` | 去掉数量列的 `text-right` |
| `ui/src/pages/GoFunctions.vue` | 操作条 `justify-center`。保留导出函数按钮自己的 `text-left` |
| `ui/src/pages/CronJobs.vue` | 操作条 `justify-center` |
| `ui/src/pages/Sandboxes.vue` | 操作条 `justify-center` |
| `ui/src/pages/S3Manager.vue` | 操作条 `justify-center` |
| `ui/src/pages/Logs.vue` | 消息的 `TableHead` 与 `TableCell` 加 `text-left` |
| `ui/src/pages/Users.vue` | 去掉项目数表头的 `text-right`。操作条 `justify-center` |
| `ui/src/components/databases/CollectionPanel.vue` | 去掉操作列 `text-right`，`justify-end` → `justify-center` |
| `ui/src/components/databases/kv/KvPanel.vue` | 去掉操作列 `text-right` |
| `ui/src/components/databases/kv/KvApiPanel.vue` | 去掉操作列 `text-right`，`justify-end` → `justify-center`。参数、语义加 `text-left` |
| `ui/src/components/databases/kv/editors/KvHashEditor.vue` | 值列 `text-left`。去掉操作列 `text-right` / `justify-end` |
| `ui/src/components/databases/kv/editors/KvListEditor.vue` | 元素列 `text-left`。去掉 `#` 与操作列的 `text-right` |
| `ui/src/components/databases/kv/editors/KvZSetEditor.vue` | 成员列 `text-left`。去掉分数、操作列的 `text-right` / `justify-end`，分数数字改为普通居中（保留 `tabular-nums`） |
| `ui/src/components/modal/DocumentListModal.vue` | 数据列 `text-left` |
| `ui/src/components/modal/SqlWorkModal.vue` | 查询结果表加左对齐覆盖。批量结果只给错误详情加 `text-left` |
| `ui/src/router/index.ts` | 空路径 redirect 到 `agents`；`dashboard` 改为 `path: 'dashboard'`；settings 与首页 settings 入口改为 `agents`。保留 query / hash |
| `ui/src/pages/Home.vue` | 两处 `{ name: 'dashboard' }` 改为 `{ name: 'agents' }` |
| `ui/src/layouts/DocsLayout.vue` | 「返回控制台」改为 `{ name: 'agents' }` |
| `ui/src/layouts/DefaultLayout.vue` | 用户页无权限回退改为 `{ name: 'agents' }` |
| `ui/src/components/NavMenu.vue` | `selectedKey` 回退改为 `'agents'` |
| `ui/AGENTS.md` | 路由一句改为：`/console` 与控制台入口落到 `agents`；监控大盘是 `/console/dashboard`；旧路径 redirect 仍是 `/sql`→`/console/databases`、`/llm`→`/console/agents`、`/settings`→`/console/agents?settings=1` |
| `ui/README.md` | 路由表：`/console` 重定向到云助手；补上 `/console/dashboard` |
| `ui/tests/router.test.ts` | 见 §5 |
| `ui/tests/Home.test.ts` | 假路由注册 `agents`；href 与路由名改为云助手 |
| `ui/tests/layouts.test.ts` | 「返回控制台」href 改为 `/console/agents`（假路由要有 `agents`） |
| `ui/tests/table-align.test.ts` | 新增。挂载真实 `TableHead` / `TableCell`，断言 `text-center`，以及 `text-left` 传入后能盖过默认值 |

`AgentToolCard.vue`、`SchemaPanel.vue`、`KvSetEditor.vue`、`KvStringEditor.vue`、`auth.ts`、`GlobalProjectSwitcher.vue` 不改。

## 5. 测试

命令：`cd ui && yarn test`，以及 `cd ui && yarn build`。覆盖率阈值仍是语句 / 行 / 分支 / 函数各 95%（`ui/vite.config.ts`）。`src/components/ui/**` 不计入覆盖，因此 `TableHead.vue` / `TableCell.vue` 的 class 改动不进分母。

当前基线：`plan/planv5.0/code-and-style-optimization-plan.md` §2.4 记录 functions **94.24%**，`yarn test` 已因阈值非零退出。本改动不负责补上这个历史缺口。验收是相对实施前基线，statements / branches / functions **不下降**。不新增未测试的函数；路由和弹窗都是模板与路由表改动。

断言：

1. `table-align.test.ts` 不使用 `src/test/helpers.ts` 的 Table 桩（桩是裸 `<th>` / `<td>`，没有 class，会把对齐测成假通过）。直接 import `@/components/ui/table`。`TableHead`、`TableCell` 的 class 含 `text-center`。再传 `class="text-left"`，结果含 `text-left` 且不再含 `text-center`。
2. 抽一张操作列页面（建议 `Users.vue` 或 `CollectionPanel`，沿用现有 mount）断言操作容器 class 含 `justify-center`，且模板里不再有 `text-right`。页面测试继续走 helpers 桩，不断言 `th`/`td` 自身的 class。
3. `Databases` 打开「新建数据库」后，名称、数据类型处在 `[data-slot=field-group]` 内。切到 SQL 后「初始化 SQL」仍在同一组里。现有创建用例继续通过。
4. `router.test.ts`：`push('/console')` 与 `push('/console?settings=1')` 落到 `name === 'agents'`，path 为 `/console/agents`，query 保留。`push({ name: 'dashboard' })` 的 path 为 `/console/dashboard`。`/settings?settings=appearance&from=bookmark` 的 fullPath 为 `/console/agents?settings=appearance&from=bookmark`。`/?settings=models&from=bookmark` 同理落到 agents。`/databases`、`/llm`、`/sql`、`/data`、`/console/databases` 的断言保持。
5. `Home.test.ts`：头部与主按钮 href 为 `/console/agents`，点击后路由名是 `agents`。
6. `layouts.test.ts`：「返回控制台」href 为 `/console/agents`。若已有用户页守卫用例，非超管应落到 `agents`；没有则补一条，用真实 `DefaultLayout` 或抽出守卫不强制，优先在现有 layout 测试上加。

`src/test/helpers.ts` 里给页面测试用的假路由 `{ name: 'dashboard' }` 不是应用默认路由，不要为了「看起来一致」去改它，除非某条断言真的解析到了应用路由。

目视（实施阶段，有浏览器时）：数据库、用户、KV、日志各打开一次。确认操作列表头和按钮居中，日志「消息」和文档 JSON 仍从左读，展开数据库后集合页签没有被居中继承打乱，`/console` 打开云助手，直接打开 `/console/databases` 仍是数据库页，`/console/dashboard` 仍是监控大盘。

## 6. 与 #31 的冲突

#31 当前 PR 只有一份计划，和本文件文件名不同，文档阶段无冲突。

#31 **实施**时会改表格骨架和空态，和本计划的 class 编辑叠在同一模板上。它明确不改 `ui/src/components/ui/` 里除 `Spinner` 文案以外的组件，因此 `TableHead.vue` / `TableCell.vue` 不应和它对撞。`SbTableSkeleton` 是新文件，会用到 `TableCell`，自动继承居中。

预期会冲突的文件（两边都会改同一段表体）：

- `ui/src/pages/Databases.vue`
- `ui/src/pages/Dashboard.vue`
- `ui/src/pages/GoFunctions.vue`
- `ui/src/pages/CronJobs.vue`
- `ui/src/pages/Sandboxes.vue`
- `ui/src/pages/S3Manager.vue`
- `ui/src/pages/Logs.vue`
- `ui/src/pages/Users.vue`
- `ui/src/components/databases/CollectionPanel.vue`
- `ui/src/components/databases/kv/KvPanel.vue`
- `ui/src/components/databases/kv/editors/KvHashEditor.vue`
- `ui/src/components/databases/kv/editors/KvListEditor.vue`
- `ui/src/components/databases/kv/editors/KvZSetEditor.vue`
- `ui/src/components/modal/DocumentListModal.vue`
- `ui/src/components/modal/SqlWorkModal.vue`

冲突时保留双方：骨架 / 空态 / 错误闸门用 #31 的结构，对齐 class 用本计划。不要在解决冲突时把 `text-right` 或 `justify-end` 合并回来。

本计划独有、#31 不碰的文件：`TableHead.vue`、`TableCell.vue`、`router/index.ts`、`Home.vue`、`DocsLayout.vue`、`DefaultLayout.vue`、`NavMenu.vue`、`KvApiPanel.vue`、`ui/AGENTS.md`、`ui/README.md`，以及对应测试。

## 7. 范围外

- #31 的加载骨架、空态误报、`useLoadState`、`SbTableSkeleton`、`Spinner` 文案。
- 数据库移动端卡片、Schema 清单、KV Set 徽章、文档站 Markdown 表、云助手工具卡原生表的对齐。
- 侧栏分组顺序、云助手改名、监控大盘改名。
- 登录成功或切换项目后强制跳到云助手。
- `DialogContent` / `Field` / `SbModal` 的全局间距。其他弹窗已经用 `FieldGroup`。
- 后端、路由权限、`go.mod`、前端依赖与 `vite.config.ts`。
- 把 functions 覆盖率从 94.24% 补到 95%。

## 8. 实施记录

2026-10-09 按 §1.3–§4 落地，没有被迫改设计。共享组件默认居中、左对齐例外、`/console` → `/console/agents`、监控大盘 `/console/dashboard`、新建数据库 `FieldGroup` 的 `gap-5` 均与本文一致。

实施时 `main` 仍是 `707b7ae`，#31 未合入，未做冲突合并。

验收命令：

| 命令 | 结果 |
| --- | --- |
| `cd ui && yarn test` | 86 个文件、489 个用例通过。statements 99.04%、branches 95.22%、functions **95.02%**、lines 99.04%，四项都过 95% 阈值。§5 所写 94.24% 是更早基线，当前仓库已高于阈值 |
| `cd ui && yarn typecheck` | 通过 |
| `cd ui && yarn build` | 通过 |
| 浏览器 | `/console` 落到 `/console/agents`；`/console/dashboard`、`/console/databases`、`/databases?from=bookmark` 仍是原页面。用户表表头（含「操作」「项目数」）计算样式为 `center`；日志「消息」为 `left`。新建数据库弹窗的 `[data-slot=field-group]` 行间距 20px，SQL 模式下「初始化 SQL」在同一组 |

后端未改，未跑 `go test`。
