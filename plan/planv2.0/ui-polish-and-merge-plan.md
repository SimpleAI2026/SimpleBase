# UI 美化与小组件合并计划 / UI polish and component merge

> **状态**：plan-only（本文件只规划，不改 `ui/src` 产品代码）。
> **制定日期**：2026-09-22
> **Verified against**：`main` @ `010ac12`（`Merge pull request #20`，设置合并已落地）。行数均为该提交上 `wc -l` 的结果。
> **关联**：[`ui-principles.md`](./ui-principles.md)（原则；其中 antd / `tokens.css` 描述已过时，以本文 §2 与 [`ui/AGENTS.md`](../../ui/AGENTS.md) 为准）、[`ui-style-plan.md`](./ui-style-plan.md)（antd 时代样式债，勿再按它改代码）、[`ui-shadcn-migration-plan.md`](./ui-shadcn-migration-plan.md)、[`ui-settings-merge-plan.md`](./ui-settings-merge-plan.md)
> **需求原文**：对现在 UI 做美化；部分低于 300 行的前端组件可以合并。基于现状给 plan，不实现产品 UI。

## 0. 一句话目标 / One-sentence goal

在现有 Claude 暖色 + shadcn-vue token 上，把控制台的间距、字号、空态、表格、状态徽章和加载/错误收成同一套；并把只被一个父组件使用的薄包装文件折进父组件。不新开视觉语言，不重做信息架构，不动后端。

## 1. 现状摘要 / Current baseline

技术栈已是 Vue 3 + TypeScript + Tailwind CSS v4 + reka-ui（`ui/src/components/ui/` 本地 fork）+ Pinia。全局 token 只在 `ui/src/style.css`（220 行）：`:root` / `.dark` 语义色、`@theme inline` 的字体与 radius。控制台壳是 `layouts/DefaultLayout.vue`（146 行），页面标题按 `ui/AGENTS.md` **只保留 `PageContainer` 的一行 subtitle，不加 h2**。主内容区全宽，禁止加 `max-w-*`。

控制台路由（`router/index.ts`，侧栏由 `NavMenu.vue` 按 `meta` 渲染）：

| 页面 | 文件 | 行数 | 骨架 |
|---|---|---:|---|
| Dashboard | `ui/src/pages/Dashboard.vue` | 225 | 四张统计卡 + 手绘柱 + 库表 |
| Databases | `ui/src/pages/Databases.vue` | 428 | 单卡列表，行内展开集合 |
| S3 | `ui/src/pages/S3Manager.vue` | 240 | 单卡：工具栏 + 表 |
| GoFunctions | `ui/src/pages/GoFunctions.vue` | 229 | 单卡：工具栏 + 表 |
| CronJobs | `ui/src/pages/CronJobs.vue` | 312 | 同 GoFunctions，外加运行记录 Sheet |
| Agents | `ui/src/pages/AgentManager.vue` | 468 | 左列表 + 右 `AiChat` |
| Logs | `ui/src/pages/Logs.vue` | 231 | **三张卡**：筛选、保留策略、再一张「日志」表 |

已有、应继续用的业务组件：`SbModal`、`SbEmptyState`、`TablePager` + `usePagination`、`ConfirmAction`、`PageContainer`、`ProjectScope`。状态色映射在 `ui/src/lib/status.ts`（30 行）。

`ui-style-plan.md` 里的 antd、`!important`、`theme.css` 双份标题等问题 **已经不在当前树里**。本文只记 `010ac12` 上仍能指到行号的债。

## 2. 设计原则 / Design principles

执行时以这些为准。和更早的 `ui-principles.md` 冲突时，以 `ui/AGENTS.md` + `style.css` 为准。

1. **不换视觉语言。** 主色仍是 `--primary: #d97757`，背景 `--background: #f5f4ef`，暗色走 `html.dark`（`style.css` 53–136 行）。新样式只用已有语义类：`bg-primary`、`text-muted-foreground`、`bg-success/12`、`border-border`、`font-serif` / `font-sans` / `sb-mono`。禁止再引入 `bg-emerald-500` 这类未进 token 的色，禁止为美化新增 CSS 文件。
2. **字号走 Tailwind 标尺，不写任意像素。** 控制台正文已经是 `text-sm`（14px，`style.css` body）。收口时只用 `text-xs`（12）/ `text-sm`（14）/ `text-base`（16）/ `text-2xl`（24）。现存 `text-[11px]`、`text-[11.5px]`、`text-[12px]`、`text-[12.5px]`、`text-[13.5px]`、`text-[17px]`、`text-[26px]` 以及 `.sb-terminal` 的 `12.5px`（`style.css` 205 行）都是债。`components/ui/**` 里 shadcn 自带的任意值不动。
3. **间距沿用现在页面里已经反复出现的档。** 页内 `PageContainer` 是 `gap-5`；列表卡工具栏以 S3 / GoFunctions / CronJobs 的 `gap-2.5 py-4` 为基准；内容区 padding 保持 `DefaultLayout.vue` 77 行的 `p-4 sm:p-6 lg:p-8`。不要回到 `ui-principles.md` 的 `max-width: 1440px`，那条已被 AGENTS 否掉。
4. **重复两次的展示结构才抽组件；只被一个父级使用的薄包装则折回去。** 这是本计划合并方向，和「再拆更多文件」相反。`components/ui/**` 视为库代码，不合并、不改风格约定。
5. **已定稿的壳不动。** 侧栏项保持 `h-11`（`sidebarMenuButtonVariants` 的 default size，`components/ui/sidebar/index.ts` 47 行）。顶栏顺序保持：使用文档 → 项目切换器 `w-56` → 设置 → 刷新，间距 `gap-4 sm:gap-5`。设置仍是右上角 `SettingsModal`（`SbModal` + `maxWidth: 900` + `hideFooter`），不改回全页或 Sheet。确认仍用 `ConfirmAction`，提示仍用 `vue-sonner`。不重新引入已删除的 checkbox / drawer / dropdown-menu / pagination / radio-group。操作列溢出因此不能用 DropdownMenu 解决。
6. **中间态和终态一起改。** 空、加载中、失败要在表格里看得见，不能只在按钮上转圈或只 toast 一次。

## 3. UI 美化清单 / Polish inventory

每条都是当前源码里的具体位置。实现时应对着这些组件截「改前 / 改后」，本文不附截图。

### 3.1 字号与品牌标记

| 位置 | 现象 |
|---|---|
| `layouts/DefaultLayout.vue` 6、9 行；`layouts/DocsLayout.vue` 6–7 行 | 同一枚「S」方标 + `font-serif text-[17px]` 复制两份。17px 不在 `text-sm`/`text-base` 上。 |
| `pages/Dashboard.vue` 22–23 行 | 统计卡标签 `text-[12px]`、数字 `text-[26px]`。26px 介于 `text-2xl`(24) 与 `text-3xl`(30) 之间，四张卡并排时数字行高不齐（「正常 / 受限 / 数字」混在同一字号）。 |
| `pages/AgentManager.vue` 35、38 行；`GlobalProjectSwitcher.vue` 25 行；`modal/SqlWorkModal.vue` 55、92 行 | Badge 与辅助行被压到 `text-[11px]`，比 Badge 默认 `text-xs`（`components/ui/badge/index.ts` 7 行）更小。 |
| `components/docs/DocsSidebar.vue` 7、13 行；`DocsModuleTabs.vue` 5 行；`DocsArticle.vue` 23、50、59、200 行 | 文档站使用 `11.5px` / `12.5px` / `13.5px`。文档站不是本轮控制台主路径，但和同一套 token 抢视觉。 |
| `style.css` 196–207 行 `.sb-terminal` | `font-size: 12.5px`，和 `.sb-code` 的 12px、`.sb-mono` 的 13px 三者并存。 |

截图建议：Dashboard 四张统计卡并排；顶栏「S / SimpleBase」与文档站顶栏并排对比。

### 3.2 间距、密度、列表工具栏

列表页没有共用工具栏组件，class 各自写：

| 页面 | 工具栏 | 刷新按钮尺寸 |
|---|---|---|
| `S3Manager.vue` 9 行 | `gap-2.5 py-4`，贴在卡片 header 下 | 默认（`h-8.5`，`button/index.ts` 20 行） |
| `GoFunctions.vue` 12 行、`CronJobs.vue` 11 行 | 同上 | 默认 |
| `Logs.vue` 9 行 | `gap-3 py-5`，且筛选卡和表格卡是分开的 | 默认 |
| `Databases.vue` 10–28 行、`Dashboard.vue` 43 行、`AgentManager.vue` 9 行 | 动作在 `CardAction`，没有独立工具栏行 | `size="sm"`（`h-7.5`） |

同一屏里「刷新」有时高 34px、有时高 30px。Logs 的筛选行比 S3 更松，下面紧接着第二张「保留策略」卡，再第三张表，垂直节奏断掉。

分页脚也有两套：

- 五行列表页在表下再包一层 `flex ... border-t ... px-4 py-3`（`Logs.vue` 89、`S3Manager.vue` 73、`Databases.vue` 153、`GoFunctions.vue` 101、`CronJobs.vue` 137）。
- `TablePager.vue` 2 行自己还有 `pt-3`，和上面的 `py-3` 叠在一起，脚比表头更厚。
- `Dashboard.vue` 98 行、`DocumentListModal.vue` 54 行、`SqlWorkModal.vue` 74 行是裸 `TablePager`，没有这条 border。

`TableRow.vue` 13 行已经包含 `hover:bg-muted/40`。`Logs` / `S3` / `Databases` / `CronJobs` / `GoFunctions` / `CollectionPanel` / `DocumentListModal` / `SqlWorkModal` 又在每一行 class 上写了一遍，属于无效重复，改色时要改两处。

截图建议：S3 工具栏与 Logs 筛选卡上下对照；任意列表页表尾「共 N 条」区域。

### 3.3 空态

`SbEmptyState.vue`（40 行）把所有空态都画成同一个 `InboxIcon` + `Empty`（虚线框、`p-6`）。调用方再被 `TableEmpty.vue` 29 行包一层 `py-10`，表内空态因此又高又单一。

具体问题：

- `GoFunctions.vue` 37 行、`CronJobs.vue` 38 行在 admin 时把 `description` 设成 `''`。`SbEmptyState` 仍会渲染 `EmptyDescription`，空字符串留下一块空白说明。
- `Dashboard.vue` 71 行趋势空态只有「暂无趋势数据」，没有说明是「后端没数据」还是「请求失败」（见 §3.7）。
- `Logs.vue` 76 行空态没有「清除筛选」一类的出口；筛选条和空表还分属两张都叫「日志」的卡（5 行与 61 行两个 `CardTitle`）。
- `AgentManager.vue` 23 行与 80 行、`AiChat.vue` 34 行三处空态文案不同，图标相同，左右栏看起来像同一句提示复制了两次。
- `ProjectScope.vue` 7–11 行的「请先选择项目」是做得对的（有 CTA）。`SettingsPanel.vue` 23–27、91–95 行把同一句又写了一遍，因为设置弹窗里的模型和供应商区不走 `ProjectScope`。

截图建议：GoFunctions 在系统项目下的空表；Logs 无匹配时的三张卡。

### 3.4 表格

`components/ui/table/Table.vue` 只做横向滚动，表头不粘性。`TableHead` 是 `h-11`、`uppercase tracking-wider`，数据行是 `text-sm`，但 ID / 时间格经常再降到 `text-xs`（`Databases.vue` 76、84 行，`Dashboard.vue` 89、93 行），一列里两级字号。

操作列密度（窄屏会换行，且不能用已删除的 DropdownMenu）：

- `Databases.vue` 86–121 行：SQL / 打开 / 关闭 / 新建集合 / 删除，外加展开按钮 `size-6`（55 行），和别的 `size="sm"` 图标按钮不齐。
- `GoFunctions.vue` 73–96 行：查看 / 编辑 / 复制路径 / 删除。
- `CronJobs.vue` 109–131 行：立即执行 / 记录 / 编辑 / 删除，状态列还叠了 Badge + 时间（91–98 行），行高明显高于 S3。

状态文案不统一，这是最值得截的一处：

- `Databases.vue` 82 行：`statusText()` → 「就绪 / 创建中 / …」（`lib/status.ts`）。
- `Dashboard.vue` 91 行：同一枚 Badge，文案是原始英文 `record.status`（`ready`、`degraded`）。
- `CronJobs.vue` 238–249 行另写了一套 `statusVariant` / `statusText`（成功 / 失败 / 执行中），没有放进 `lib/status.ts`。
- `Logs.vue` 16–18、81 行：级别筛选和 Badge 都是英文 `info` / `warn` / `error`。映射函数 `logLevelVariant` 已在 `lib/status.ts` 26 行，缺的是中文标签，不是颜色。

加载时：`v-if="!paged.length && !loading"` 让 tbody 在请求期间是空的，只有按钮上的 `Spinner`。`CollectionPanel.vue` 3–6 行和 `Dashboard.vue` 7–12 行已经有 Skeleton，列表页没有。

截图建议：Dashboard 与 Databases 的状态列并排；Databases 一行操作按钮在 1280px 与 768px。

### 3.5 弹窗、表单、设置

- `modal/SbModal.vue` 6 行把 `description` 放进 `class="sr-only"`。全仓只有 `SettingsModal.vue` 5 行传入 description（「主题、连接、默认模型与厂商 API Key」），界面上看不到这句，弹窗标题下直接是 Tab。
- 设置外观 Tab（`SettingsPanel.vue` 2–19 行）在 `maxWidth: 900` 的弹窗里只有一组 Toggle，大片空白。模型 Tab 的 Slider（65–72 行）没有 0 / 2 的端点文字。供应商编辑是网格下面再插一张 Card（138 行），滚动后和卡片网格贴在一起，没有与「配置」按钮的视觉从属。
- `ConnectionPanel.vue` 30–32 行把 DevMode 种子 Key 以等宽字铺在表单下，和正式字段同一权重。
- 破坏性按钮没有用已存在、且 **零引用** 的 `destructiveGhost`（`button/index.ts` 16 行）。各页手写 `variant="ghost" class="text-destructive hover:bg-destructive/10"`（S3 63、Databases 117、GoFunctions 91、CronJobs 127、AgentManager 48、DocumentListModal 46）。暗色下 hover 深浅会漂。
- `Logs.vue` 10–30 行的级别 Select、关键字、`datetime-local`（`w-52`）没有 `FieldLabel`，只靠 placeholder。窄屏换行后「至」会单独掉到一行（29 行）。
- 新建数据库的表单内联在 `Databases.vue` 165 行以后的 `SbModal` 里；新建集合 / 新建项目 / 新增文档已经是独立 modal 文件。这是结构不一致，但字段和校验不同，**不要合成一个通用表单**（见 §4.3）。美化时只统一 `Field` + 错误文案的间距。

截图建议：打开设置 → 外观 Tab；再打开设置 → 供应商，点「配置」后的长弹窗。

### 3.6 侧栏与顶栏

- 面包屑只有当前页一个 `BreadcrumbPage`（`DefaultLayout.vue` 28–34 行）。页面又没有 h1/h2（这是定稿，不要加回大标题）。结果是：页身份只靠顶栏一个普通字重的名字 + 内容区一行 `text-sm text-muted-foreground` subtitle。顶栏名字可以提到 `font-medium`，subtitle 保持现状。
- 顶栏右侧在窄屏同时放「使用文档」文字按钮、`w-56` 切换器、两个 icon 按钮（`DefaultLayout.vue` 36–74 行）。Mock Badge 已在 `sm` 以下隐藏（49 行），文档按钮没有。顺序与 `w-56` 不能改；若溢出，只允许在 `< sm` 把「使用文档」收成图标，按钮仍在原位。
- `SidebarFooter` 存在于 `components/ui/sidebar/` 但 `DefaultLayout` 没有页脚。不要为了「看起来完整」硬加版本号或第二套导航。
- 全局 `:focus-visible { outline: none }`（`style.css` 154–156 行）配合组件上的 `focus-visible:ring-*`。改动时不要再加第二套 outline，避免双环。

截图建议：1280px 与 390px 的顶栏右侧。

### 3.7 加载、错误、各控制台页

| 页 | 具体问题 |
|---|---|
| Dashboard | `load()`（192–211 行）用 `Promise.allSettled` 且 **不 toast、不占位错误**。metrics 失败时趋势区落到「暂无趋势数据」，和真的没数据无法区分。配额卡在「正常」时仍用 `TriangleAlertIcon`（186 行）。柱状图（52–69 行）没有图例，请求条 `bg-primary/85`、错误条 `bg-destructive/70` 靠 title 属性才能区分；`barHeight` 最低 4px，0 值也有一条。 |
| Databases | 信息架构已经清楚。债是操作列密度、与 Dashboard 不一致的状态文案、以及展开区 `CollectionPanel` 有 Skeleton 而主表没有。admin 正确走 `storeToRefs(projectStore).isAdmin`（261 行）。 |
| S3 | 前缀框的图标是 `CloudUploadIcon`（12 行），和「上传对象」同一语义。上传进度是 `class="w-24"` 的 `Progress`（31 行），贴在按钮旁边，10MB 上传时几乎看不清。失败只 toast（211、223、232 行），表保持旧数据，没有行内错误。 |
| GoFunctions | admin 判断把 UUID 再写死一遍（160–164 行），没有用 `stores/project.ts` 的 `isAdmin`（CronJobs 203–207 行同样）。空 description 见 §3.3。导出函数 Badge 可以无限换行（55 行），行高随导出数量跳。 |
| CronJobs | 与 GoFunctions 同一套工具栏，这是对的。额外债：本地 status 映射、目标缺失用 `AlertTriangleIcon` 但颜色只在父 span 上（71–80 行），图标本身不一定是 destructive 色。运行记录在 `CronJobRunsDrawer.vue`（211 行）里，错误块用 `bg-destructive/10`（80 行），和列表 Badge 的 destructive 变体不是同一组件。 |
| Agents | 列表项是 `<button>` 套 Card 风格（25–51 行），选中态 `bg-primary/8`。定时启用点是 `bg-emerald-500`（41 行），暗色下和 `--success` 不是一个绿。模块 Badge `text-[11px]`。对话空态与列表空态抢同一图标。新建/编辑表单内联在页面 87–129 行，而定时是 `AgentScheduleModal`（321 行）。不要把两者合并（后者已超过 300 行，且含运行记录）。 |
| Logs | 三卡结构、重复标题、英文级别、`events.length`（40 行）和 `TablePager` 的「共 N 条」双重计数。轮询文案只有「轮询 / 手动」（38 行），间隔 10s 写在 218 行，界面不说。保留天数 Input 与筛选 Input 不在同一套 `Field` 里。 |
| 设置 | 见 §3.5。连接 Tab 的 401 `Alert`（`ConnectionPanel.vue` 3–6 行）是正确的行内错误，其它 Tab 没有同等模式。 |
| 编辑器（云函数弹窗会带到） | `GoMonacoEditor.vue` 48–77 行只注册 `sb-light`，背景 `#faf9f5`。`html.dark` 时编辑器仍是米白，和弹窗暗色卡片割裂。这不是新功能，是主题 token 没接到 Monaco。 |
| 文档代码块（非控制台，但同一 `ui/`） | `DocsArticle.vue` 178–180 行 `pre` 写死 `#1e1e1e` / `#e5e5e5`，不跟 `.dark`。 |

截图建议：Dashboard 在 metrics 失败时的空柱区；暗色下打开云函数编辑弹窗；Agents 左栏选中卡上的绿点。

## 4. 组件合并 / Merge candidates

范围：`ui/src` 下自有 `.vue`（不含 `components/ui/**`，不含 `*.test.ts`）。「低于 300 行」按 `wc -l`。合并的含义是：**薄包装折进唯一父组件，或把复制的表尾 chrome 收进已有的 `TablePager`**。不借机再拆文件。

### 4.1 建议合并

#### M1. 聊天布局薄包装 → `AiChat.vue`

| 文件 | 行数 | 产品引用 |
|---|---:|---|
| `components/chat/Message.vue` | 10 | 仅 `AiChat.vue` |
| `components/chat/MessageContent.vue` | 5 | 仅 `AiChat.vue` |
| `components/chat/MessageAvatar.vue` | 15 | 仅 `AiChat.vue` |
| `components/chat/Bubble.vue` | 19 | 仅 `AiChat.vue` |
| `components/chat/Marker.vue` | 13 | **无产品引用**，只有 `chat-components.test.ts` |

`MessageContent.vue` 2 行的 `max-w-[75%]` 与 `md:max-w-[75%]` 相同，折进去时删掉重复的 md 规则。

- 为什么：四个文件各是一层 class，改气泡对齐要打开五处。`Marker` 是死代码。
- 目标：`components/ai/AiChat.vue`（现 172 行，折入后大约 190–210，仍低于 300）。删除上述五个文件。
- 风险：低。行为都在 class 上。`MessageScroller.vue`（55 行）有滚动吸底，**本项不折入**。
- 测试：改 `components/chat/chat-components.test.ts`、`components/components-coverage.test.ts`。`AiChat.test.ts` 仍覆盖对话渲染。

#### M2. `GithubMark.vue` → `DocsLayout.vue`

| 文件 | 行数 | 产品引用 |
|---|---:|---|
| `components/icons/GithubMark.vue` | 5 | 仅 `layouts/DocsLayout.vue` 18 行 |

- 为什么：单路径 SVG，唯一调用方就是文档顶栏的 GitHub 链接。
- 目标：把 `<svg>` 内联进 `DocsLayout.vue`（现 33 行）。不要为此新建 `BrandMark`。控制台与文档站的「S」字标虽然也重复（§3.1），但那是两套 layout 的品牌，**保持两处 class 一致即可，不抽组件**。
- 风险：低。
- 测试：删除或改写 `components/icons/GithubMark.test.ts`；`layouts/DocsLayout.test.ts` 仍应看得到 GitHub 链接。

#### M3. `DocsModuleTabs.vue` → `DocsWiki.vue`

| 文件 | 行数 | 产品引用 |
|---|---:|---|
| `components/docs/DocsModuleTabs.vue` | 25 | 仅 `pages/DocsWiki.vue` |

- 为什么：它只是 `TabsList variant="line"` 加一层 `max-w-[1200px]`，没有独立状态。
- 目标：`pages/DocsWiki.vue`（现 158 行，合并后仍低于 200）。
- 不要带着一起合并：`DocsSidebar.vue`（39 行，sticky 高度计算是独立布局）和 `DocsArticle.vue`（236 行，含整段 markdown CSS）。合进 DocsWiki 会超过 400 行，且文章样式不该进页面脚本。
- 风险：低。文档路由断言在 `DocsWiki.test.ts` / `docs-components.test.ts`。
- 测试：把 `DocsModuleTabs` 的挂载用例并进 `DocsWiki.test.ts` 或删掉只测 wrapper 的用例。

#### M4. 五行列表页的表尾壳 → `TablePager.vue`

不是新组件。`TablePager.vue`（29 行）增加一个可选外观，例如默认保持今天的裸分页（Dashboard、两个 modal 依赖这个间距），另给 `variant="footer"`：自己渲染现在复制的那层 `border-t px-4 py-3`，并去掉组件内部多出来的 `pt-3`，避免双 padding。

调用方改为 footer 变体：

- `pages/Logs.vue` 89–97
- `pages/S3Manager.vue` 73–81
- `pages/Databases.vue` 153–161
- `pages/GoFunctions.vue` 101–109
- `pages/CronJobs.vue` 137–145

保持裸用法：`Dashboard.vue` 98、`DocumentListModal.vue` 54、`SqlWorkModal.vue` 74。

- 为什么：同一段 class 复制五次，和 `TablePager` 自己的 `pt-3` 还互相加厚。
- 风险：中低。分页按钮和「共 N 条」的文案会被页面测试断言到；改 class 不应改文案。`Databases` 页超过 300 行，本项只删包装 div，不动业务。
- 测试：`pages/Databases.test.ts`、`GoFunctions.test.ts`、`CronJobs.test.ts`、`S3Manager.test.ts`、`Logs.test.ts`、`Dashboard.test.ts` 里凡是找「共 N 条 / 上一页」的用例都要跑一遍。

### 4.2 明确不合并

| 对象 | 行数 | 原因 |
|---|---:|---|
| `components/ui/**` | 单个文件多在 15–60 行 | AGENTS：本地 fork 的库代码。薄是 shadcn 的组合方式，不是碎片。 |
| `pages/Databases.vue` | 428 | 大页。只允许 M4 那种删除重复包装。 |
| `pages/AgentManager.vue` | 468 | 大页。新建表单不要抽成第六个 modal，除非后续单独立项；本计划不动它的文件边界。 |
| `pages/CronJobs.vue` | 312 | 已超过 300。不与 GoFunctions 合成「通用列表页」。 |
| `components/settings/SettingsPanel.vue` | 364 | 三个 Tab 的表单与厂商编辑。折进 `SettingsModal`（55 行）会得到约 420 行的弹窗。 |
| `components/ai/AgentScheduleModal.vue` | 321 | 定时表单 + 运行记录，超过 300。 |
| `components/modal/SqlWorkModal.vue` | 360 | SQL 编辑与结果表，超过 300。 |
| `components/modal/CronJobModal.vue` | 374 | 超过 300。 |
| `components/ai/AiChatComposer.vue` | 199 | 与 `AiChat`（172）相加约 370，且 composer 含 `@` mention。保持拆分。 |
| `components/chat/MessageScroller.vue` | 55 | 有吸底状态，不是 class 包装。M1 之后若 AiChat 仍薄，可以再评估，不进第一批。 |
| `components/PageContainer.vue` | 9 | 7 个页面 + `ProjectScope` 都在用。折进各页会复制 subtitle 间距。 |
| `components/ProjectScope.vue` | 25 | 无项目门闩，所有控制台页共用。 |
| `components/SbModal.vue` | 110 | 所有弹窗的壳。设置计划已禁止再写一套 Dialog。 |
| `components/SbEmptyState.vue` | 40 | 空态唯一入口。美化用 props（例如可选 icon），不按页面复制。 |
| `components/ConfirmAction.vue` | 42 | 破坏性确认的唯一入口。 |
| `components/SbCodeBlock.vue` | 18 | `DocumentListModal` 使用；保留，避免 JSON 样式再散回各页。 |
| `components/NavMenu.vue` | 69 | 只被 DefaultLayout 使用，但是路由 meta 的菜单边界，且 `simple-components.test.ts`、`interactions.test.ts` 单独挂载。折进布局会把路由顺序和顶栏 query 缠在一起。 |
| `components/settings/ConnectionPanel.vue` | 83 | 只被 SettingsModal 使用，但是完整表单 + 401 Alert，有自己的测试。不是薄包装。 |
| `components/databases/CollectionPanel.vue` | 81 | 行展开子表，有自己的请求。Databases 已经 428 行。 |
| `modal/CreateCollectionModal.vue` 88、`CreateProjectModal.vue` 111、`DocumentKvModal.vue` 98、`DocumentListModal.vue` 148、`GoFunctionModal.vue` 187 | — | 都走 `SbModal`，壳不重复。字段、校验、API 不同，不要做成一个 `NameFormModal`。 |
| `components/editor/GoMonacoEditor.vue` 113 + `goMonarch.ts` 161 | — | 编辑器生命周期和语法高亮分开是对的。暗色主题在 Phase C 里加 `sb-dark`，不把 monarch 并进 SFC。 |
| `components/GlobalProjectSwitcher.vue` | 119 | 顶栏定稿组件，内含 `CreateProjectModal`。不折进 DefaultLayout。 |
| `composables/usePagination.ts` 18、`useAsyncAction.ts` 47、`useAiChat.ts` 79 | — | 逻辑复用，不是 UI 碎片。页面继续调用，不内联。 |

GoFunctions 与 CronJobs 的「刷新 + 新建」两颗按钮看起来像同一工具栏。它们只有两个按钮、文案不同，**不要新建 `ListToolbar.vue`**。Phase A 把 class 和按钮 `size` 对齐即可。

### 4.3 行数附录（业务 `.vue`，不含 `components/ui`，不含测试）

便于复查，按行数升序。`components/ui/**` 约 150 个生成式薄文件，全部排除在合并范围外。

```
5   components/chat/MessageContent.vue
5   components/icons/GithubMark.vue
9   components/PageContainer.vue
10  components/chat/Message.vue
13  components/chat/Marker.vue
15  components/chat/MessageAvatar.vue
18  components/SbCodeBlock.vue
19  components/chat/Bubble.vue
25  components/ProjectScope.vue
25  components/docs/DocsModuleTabs.vue
29  components/TablePager.vue
33  layouts/DocsLayout.vue
39  components/docs/DocsSidebar.vue
40  components/SbEmptyState.vue
42  components/ConfirmAction.vue
55  components/SettingsModal.vue
55  components/chat/MessageScroller.vue
69  components/NavMenu.vue
81  components/databases/CollectionPanel.vue
83  components/settings/ConnectionPanel.vue
88  components/modal/CreateCollectionModal.vue
98  components/modal/DocumentKvModal.vue
110 components/modal/SbModal.vue
111 components/modal/CreateProjectModal.vue
113 components/editor/GoMonacoEditor.vue
119 components/GlobalProjectSwitcher.vue
146 layouts/DefaultLayout.vue
148 components/modal/DocumentListModal.vue
158 pages/DocsWiki.vue
172 components/ai/AiChat.vue
187 components/modal/GoFunctionModal.vue
199 components/ai/AiChatComposer.vue
211 components/CronJobRunsDrawer.vue
225 pages/Dashboard.vue
229 pages/GoFunctions.vue
231 pages/Logs.vue
236 components/docs/DocsArticle.vue
240 pages/S3Manager.vue
312 pages/CronJobs.vue          ← 超过 300，不作为合并宿主
321 components/ai/AgentScheduleModal.vue
360 components/modal/SqlWorkModal.vue
364 components/settings/SettingsPanel.vue
374 components/modal/CronJobModal.vue
428 pages/Databases.vue
468 pages/AgentManager.vue
```

## 5. 分阶段执行 / Phased execution

每阶段单独 PR。先 A 后 B 后 C：A 不改文件边界，B 的 diff 才容易审；C 依赖 A 的字号和徽章已经收口。三阶段都不要改 `package.json`、不要加依赖、不要改 `components/ui/**` 的变体定义（使用已有 `destructiveGhost` 可以，改 Button 的尺寸标尺不行）。

验收命令都是：`cd ui && npm test`（`vitest run --coverage`）。另加 `npm run build`。下面「测试影响」只列预期会改断言的文件，不是允许跳过其余测试。

### Phase A — 观感收口（不移动文件）

做这些，不做合并：

1. 控制台任意字号改为 `text-xs` / `text-sm` / `text-2xl`：`DefaultLayout.vue`、`DocsLayout.vue` 的 17px 标；`Dashboard.vue` 22–23 行；`AgentManager.vue` 35、38 行；`GlobalProjectSwitcher.vue` 25 行。`.sb-terminal` 的 12.5px 改为 12px 或 13px，与 `.sb-code` / `.sb-mono` 二选一对齐。
2. `Dashboard.vue` 91 行改用 `statusText`，与 Databases 一致。配额「正常」改用 `CircleCheckIcon`（该文件已经 import），「受限」才用 `TriangleAlertIcon`。
3. `AgentManager.vue` 41 行 `bg-emerald-500` 改为 `bg-success`。
4. 删除各页 `TableRow` 上重复的 `hover:bg-muted/40`。
5. 列表工具栏对齐到 `gap-2.5 py-4`，刷新/主按钮用 `size="sm"`：至少改 `Logs.vue` 9 行及 S3 / GoFunctions / CronJobs 的默认尺寸按钮。
6. 破坏性文字按钮改为 `variant="destructiveGhost"`，删掉手写的 destructive class。
7. 空描述不要传 `''`：GoFunctions / CronJobs 的 admin 空态给一句可见说明（例如「系统项目不提供此功能」），title 保留。
8. `SbModal` 的 description：设置弹窗这句改为可见的 `DialogDescription`（去掉 `sr-only`），或删掉 SettingsModal 上永远看不见的 prop。二选一，不要留死文案。全仓只有这一处传入 description，视觉影响面就是设置弹窗。
9. Dashboard `load()`：任一 `rejected` 时 `toast.error`，趋势区在「全部失败且无缓存」时不要复用「暂无趋势数据」——用同一 `SbEmptyState` 但 title/description 写成加载失败。不要新增错误协议。
10. 列表在 `loading && !rows.length` 时显示 3 行 `Skeleton`，照 `CollectionPanel.vue` 3–6 行。已有数据时保持旧行，按钮继续 disabled。
11. GoFunctions / CronJobs 的 admin 判断改为 `useProjectStore().isAdmin`，删掉页面内再写的 UUID。行为应与现在相同（常量就是 `ADMIN_PROJECT_ID`）。

**验收**

- 控制台页面模板里不再出现 `text-[11px]`、`text-[12px]`、`text-[17px]`、`text-[26px]`、`bg-emerald-500`。
- Dashboard 与 Databases 对同一 `ready` 显示「就绪」。
- 设置弹窗标题下能看见说明，或该 prop 已删除且界面没有空白占位。
- 空列表在请求未完成时不是一块白表。
- 侧栏高度、顶栏顺序、`w-56`、无 h2、内容区无 `max-w-*`，与 AGENTS 一致。

**测试影响**

- `pages/Dashboard.test.ts`、`pages/pages.test.ts`、`pages/interactions.test.ts`：状态文案、配额图标、失败 toast。
- `pages/GoFunctions.test.ts`、`pages/CronJobs.test.ts`：admin 仍隐藏新建；空态文案若被断言则更新。
- `pages/AgentManager.test.ts`：若快照或 class 断言选中卡。
- `components/SettingsModal.test.ts`：description 可见性。
- 不改 API mock 形状。

### Phase B — 文件合并（M1–M4）

按 §4.1 执行，一个 PR 即可（四项互不依赖）。合并时顺手做的样式只限于被搬动的那几行，不夹带 Phase C。

**验收**

- 删除的文件：`Message.vue`、`MessageContent.vue`、`MessageAvatar.vue`、`Bubble.vue`、`Marker.vue`、`GithubMark.vue`、`DocsModuleTabs.vue`。全仓无残留 import。
- `AiChat` 对话气泡、头像、左右对齐与合并前一致；`MessageScroller` 仍独立。
- 五个列表页表尾只有一个 `TablePager`，Dashboard 与两个 modal 的分页间距不变。
- `npm test` 与 `npm run build` 通过。

**测试影响**

- 删除或改写：`chat-components.test.ts`（Marker / Bubble / Message* 的直接 mount）、`GithubMark.test.ts`、`docs-components.test.ts` 里只覆盖 `DocsModuleTabs` 的用例。
- 保留并跑绿：`AiChat.test.ts`、`DocsLayout.test.ts`、`DocsWiki.test.ts`、五个列表页测试、`components-coverage.test.ts`。

### Phase C — 按页打磨

在 A 的标尺上处理 §3 里剩下的、需要看整页的问题。仍不改路由、不改接口。

1. **Logs**：筛选与表格收成一张卡（对齐 S3 的 header + 工具栏 + 表）。「保留策略」留在第二张卡。去掉与 `TablePager` 重复的「N 条」，或只在工具栏保留「已加载 N 条 / 最多 200」。级别 Select 显示中文，Badge 仍用 `logLevelVariant`。轮询标签写明间隔（现在是 10 秒）。`datetime-local` 在窄屏占满行。
2. **Dashboard**：柱状图加一行图例（请求 / 错误，用现有 `bg-primary` 与 `bg-destructive`）。0 值不再画 4px 柱。失败空态与无数据空态文案分开（A 已做行为，这里补图例）。统计卡数字统一 `text-2xl`。
3. **Databases**：不改列的业务含义。操作列在 `lg` 以下收成图标按钮 + 已有 `Tooltip`（SQL / 打开 / 关闭 / 新建集合 / 删除都已有文案可放进 tooltip）。展开按钮尺寸向 `icon-sm` 靠，不要新组件。
4. **S3**：前缀图标改为 `SearchIcon` 或 `FilterIcon`（`@lucide/vue` 已在用）。`Progress` 放到工具栏下一整行，而不是 `w-24`。
5. **GoFunctions / CronJobs**：导出函数 Badge 超过一行时用 `line-clamp` 或限制展示个数并在 tooltip 里给全量，避免行高跳动。Cron 状态映射搬进 `lib/status.ts`，页面删掉本地副本。缺失目标的图标使用 `text-destructive`。
6. **Agents**：列表卡辅助字号回到 `text-xs`。定时点用 `bg-success`（A 已改色的话，这里只检查暗色对比）。左栏空态与对话空态保留不同文案；可为 `SbEmptyState` 增加可选 `icon` prop（默认仍是 Inbox），Agents 左栏用 `BotIcon`、对话用现有 Bot 即可。不要拆 468 行的页面。
7. **设置**：外观 Tab 的 `FieldGroup` 保持 `max-w-md`，在 Toggle 下加一行 `text-sm text-muted-foreground` 说明「跟随系统 / 浅色 / 深色」存在本机。供应商「配置」Card 与网格之间加 `border-t` 或增加 `gap`，让它明确是第二步而不是第六张厂商卡。Slider 两端标 0 和 2。不改存储、不改 Tab id。
8. **顶栏窄屏**：仅当 390px 下右侧溢出时，把「使用文档」的文字用 `hidden sm:inline` 藏起来，图标按钮留在原顺序。不改 `w-56`。
9. **Monaco**：`html.dark` 时使用一套 `sb-dark`，颜色取自 `style.css` `.dark` 的 `--card` / `--foreground` / `--muted-foreground`，不要新色板。亮色主题保持现在的米白。
10. **文档 `pre`（可选，同一 PR 的最后一项）**：`DocsArticle.vue` 178 行改为 `var(--card)` 在暗色、或单独用现有 `--foreground` 搭配 `bg-card`。不改文档信息架构，不改 `max-w-[1200px]`。

**验收**

- Logs 只有一个标题为「日志」的卡，保留策略单独一张。
- 柱状图在不靠 hover 时能区分请求与错误。
- 暗色下云函数编辑器背景不是 `#faf9f5`。
- 390px 顶栏不出现横向滚动条（文档按钮可只剩图标）。
- 七个控制台路由的空、加载、失败都能在页面上看见，失败仍 toast。
- 仍然满足 AGENTS 的四条定稿（侧栏高度、全宽、顶栏顺序、无 h2）。

**测试影响**

- `Logs.test.ts`：卡结构、级别文案、计数。
- `Dashboard.test.ts`：图例文本、零值柱。
- `S3Manager.test.ts`：上传进度仍在 DOM 中（选择器若绑在 `w-24` 上要改）。
- `GoFunctions.test.ts`、`CronJobs.test.ts`、`pages-coverage.test.ts`（245 行附近直接调页面内 `statusText`）：映射搬迁后从 `lib/status` 测，页面用例改为看 Badge 文本。
- `GoMonacoEditor.test.ts`：主题名或 dark class。
- `AgentManager.test.ts`、`SettingsPanel.test.ts`：文案若增加说明行。

## 6. 不做的事 / Out of scope

- 后端、proto、mock 数据形状、新的产品能力（新的筛选维度、批量操作、拖拽上传、图表库）。
- 从零重做布局或换一套配色。不引入新的字体文件（Inter 已经在 `style.css` 1 行引入；等宽继续走 `ui-monospace` 栈，不补 JetBrains Mono webfont）。
- 修改 `components/ui/**` 的结构或默认尺寸。例外：不改。Phase A 只**使用**已有 `destructiveGhost`。
- 把 `ui-principles.md` 里的 antd token、`tokens.css` 四文件拆分、内容区 `max-width: 1440px`、页面 h2 标题做回来。
- 重新引入 dropdown-menu 来收纳操作列。
- 把超过 300 行的页面或 modal 拆开或捏合。那是另一次重构，不是这次美化。
- 文档站信息架构、设置项的存储位置、admin 只读规则的语义。Phase A.11 只是改成调用已有 `isAdmin`，不改变谁能看见删除按钮。

## 7. 风险

| 风险 | 处理 |
|---|---|
| 测试按 class 或中文空态文案断言 | 每个阶段先跑 `cd ui && npm test`，只更新真正被文案/结构改变的断言，不放宽覆盖率门槛。 |
| M4 的 footer 变体误用到 Dashboard，分页跳位 | footer 必须由调用方显式打开；Dashboard 测试锁住「表在上、分页在卡内、无额外 border 容器」的结构。 |
| 去掉 `sr-only` description 后，其它弹窗突然多出一行 | 先 `rg description=` 确认只有 SettingsModal 传入。若实现时已有第二个调用方，改为 Settings 自己渲染说明，SbModal 保持 sr-only。 |
| 暗色 Monaco 与 shadcn token 色值不一致 | 只从 `style.css` 的 `.dark` 变量取色，不在编辑器里新写一套橙。 |
| 美化 PR 顺手改 API 或加依赖 | 评审时拒绝。本计划的三个阶段都不需要新依赖。 |
