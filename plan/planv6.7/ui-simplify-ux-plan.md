# planv6.7 · 控制台 UI 样式统一、代码清理与体验优化计划（UX-*）

> 依据：`skills/simplify-code`（保持行为与公共 API；行为变更单列「需确认」）+ `ui/AGENTS.md` 样式定稿规范。
> 范围：`ui/src/pages/*`、`ui/src/layouts/DefaultLayout.vue`、顶层业务组件（`NavMenu` / `GlobalProjectSwitcher` / `UserMenu` / `SbAsyncRegion` / `TablePager`）、`components/{bi,databases,kv,modal,models}` 中被页面直接使用的部分。
> 不在范围：`src/components/ui/**`（本地 fork 库代码，只增删不改风格）、后端、`services/http-api.ts` 的类型清理。
> 状态：**第一轮已落地（2026-11-18）**，进度见文末 §9。每一项都标了 **apply**（等价重构，可直接做）或 **propose**（会改变行为或交互，需要确认）。

---

## 0. 结论速览

| 类别 | 主要问题 | 项数 |
| --- | --- | --- |
| 样式统一 | 顶栏控件高度有 4 种（h-7.5 / h-8 / h-8.5 / h-9）；列表页右上角的「刷新 + 新建」写了 11 份；行内操作按钮有的用 outline、有的用 ghost，还有的加了 `px-2`、`h-6`、`h-7` 手动覆盖；数据看板工具栏用默认尺寸，跟其他页面的 `sm` 不一致 | S1–S8 |
| 逻辑 bug | 切项目时可能重复请求；保存后分页跳回第 1 页；日志级别选了就清不掉；日志保留天数清空后提交 0；S3 在子目录里上传的文件落到根目录；下载时立刻 revoke 可能拿到空文件；同一项目只能有一个库，「新建数据库」却一直显示 | B1–B12 |
| 死代码 | 未用的 import 有 4 个文件 27 处；`CronJobs.trigger()` 和 `triggering` 从没被调用；`AgentManager` 里的 `threadShowEmpty` / `nextCursor` 没人读；`DataTabs` 只有一个页签；6 处 `<div class="p-0">` 包装没有作用 | C1–C9 |
| 冗余按钮 | Users 页「查看」和「编辑」打开的是同一个弹窗；GoFunctions 行内 5 个按钮，其中「查看」和「编辑」重叠；S3 行内 4 个按钮挤在 `w-32` 列里；顶栏「刷新页面」和每页的「刷新」重复；Databases 的「新建数据库」在单库约束下无意义 | R1–R6 |
| 体验 | Dashboard 4 张卡片里 3 张讲的是同一个库；S3 没有返回上级/面包屑；Users 筛选后没有结果时，文案还是「暂无用户」；Logs 的「刷新」按钮实际作用是「查询」 | X1–X8 |

---

## 1. 样式统一（S）

### S1 按钮尺寸规范（apply；写进 `ui/AGENTS.md`「样式与 UI 约定」）

按场景只允许以下组合，禁止在 `<Button>` 上再写 `h-* / px-* / size-*` 覆盖：

| 场景 | variant | size | 说明 |
| --- | --- | --- | --- |
| 卡片头 `CardAction`（刷新 / 新建 / 上传） | 次要用 `outline`，主动作用 `default` | `sm` | 主动作每张卡片最多 1 个，放在最右 |
| 筛选栏的「查询 / 导出 / 保存」 | 同上 | `sm` | 与卡片头同高（h-7.5） |
| 表格行内操作 | `ghost`，删除用 `destructiveGhost` | `sm` | 不用 `outline`；按钮文字最多 4 个汉字 |
| 行内图标操作（复制等） | `ghost` | `icon-sm` | 必须带 `aria-label` |
| 标签 / chip 类小按钮 | `ghost` / `outline` | `xs` | 替代 `class="h-6 px-2 text-xs"` |
| 顶栏工具按钮 | `ghost` | `icon`，统一用 `size-8` | 见 S3 |
| 弹窗页脚 | 由 `SbModal` 统一 | `default` | 业务侧不再自己写页脚按钮 |

需要修改的位置（已用 grep 核实）：

- `pages/GoFunctions.vue:104-115`：去掉 5 处 `class="px-2"`；「测试」从 `outline` 改为 `ghost`。
- `components/models/ModelTraceList.vue:112`：`size="sm" class="h-7 px-2"` 改为 `size="xs"`。
- `components/bi/BiFilterBar.vue:49,56`：`size="sm" class="h-6 px-2 text-xs"` 改为 `size="xs"`。
- `components/modal/CronJobRunsModal.vue:75,85`：`class="h-6 px-2"` 改为 `size="xs"`（第 85 行保留 `text-destructive`，或改为 `destructiveGhost`）。
- `components/modal/GoFuncTestModal.vue:78`：`h-7 px-2 text-xs` 改为 `size="xs"`。
- `components/bi/BiToolbar.vue:48,57`：「刷新」「数据操作」用的是默认尺寸（h-8.5），补 `size="sm"`；它旁边的 `SelectTrigger` 同步加 `size="sm"`。
- `pages/Users.vue:98-103`：行内按钮从 `outline` 改为 `ghost`（与其他列表页一致）。
- `pages/ApiPreview.vue:41`：「运行」补 `size="sm"`。

### S2 卡片头「刷新 + 主动作」统一封装（apply）

`Databases` / `GoFunctions` / `CronJobs` / `FunctionTriggers` 都写了两份 `CardAction`：`v-if="!isAdmin"` 那份带新建按钮，`v-else` 那份只有刷新，刷新按钮因此重复。`S3Manager` / `Users` / `Sandboxes` / `KeyValue` / `Dashboard` 以及 `ApprovalsPanel` / `McpServersPanel` / `ModelTraceList` / `DocumentListModal` / `CronJobRunsModal` / `BiToolbar` 又各写了一遍「`Spinner v-if` + `RefreshCwIcon v-else`」，共 16 处。

- 新增 `components/SbRefreshButton.vue`（props：`pending`、`label?`，默认值 `common.refresh`；emit：`click`），固定为 `variant="outline" size="sm"`，内部处理 Spinner 与图标切换。
- 每个页面只保留**一个** `CardAction`：`<div class="flex items-center gap-2"><SbRefreshButton …/><Button v-if="canCreate" size="sm">…</Button></div>`。
- 「能不能新建」统一由页面里的一个 computed 算出：`canCreate = !isAdmin && auth.canWrite`。目前只有 `FunctionTriggers` 和 `Sandboxes` 检查了 `auth.canWrite`，其余页面对只读管理员（admin 角色）也显示了写按钮（见 B11）。
- 测试：在 `tests/` 里补 `SbRefreshButton` 用例（pending 时禁用并显示 Spinner），避免 functions 覆盖率下降。

### S3 顶栏控件同高（apply，对应 `DefaultLayout.vue:62-119`）

现在的高度：文档按钮 `size="sm" + h-8`；图标按钮 `size="icon" + size-8`；`LocaleSelect` 为 `h-8`；`GlobalProjectSwitcher` 为 `h-9`；`UserMenu` 为 `size="sm"`（h-7.5）。

- 全部统一到 **h-8**：`GlobalProjectSwitcher` 的 `h-9` 改为 `h-8`（`w-56` 不动，这是定稿规范）；`UserMenu` 的触发按钮加 `h-8`。
- 3 个图标按钮的 class 完全相同（`size-8 rounded-lg text-muted-foreground hover:text-foreground`），抽成模板内常量 `const toolBtn = '…'`，或新增 `components/SbToolbarIconButton.vue`（内置 Tooltip + aria-label）。三处 Tooltip 包装可以一起去掉。
- 移动端侧栏底部 4 个入口的 class 也完全相同（`DefaultLayout.vue:18-31`），抽成 `const sideEntry`。

### S4 Home 主按钮复用 `buttonVariants`（apply）

`Home.vue:203` 手写了一个 `primaryBtn` class 串，与 `ui/button` 的样式重复，焦点环也不一样（outline 对比 ring）。改为 `:class="cn(buttonVariants({ size: 'lg' }), 'h-11 px-5 self-start sm:self-auto')"`，与控制台的主色、焦点表现保持一致。

### S5 删除无用包装、统一卡片结构（apply）

- 6 处 `<div class="p-0">`（Logs:57、CronJobs:29、S3Manager:81、FunctionTriggers:29、Users:54、GoFunctions:32）不产生任何样式，直接去掉。
- 卡片头统一写 `CardHeader class="flex-wrap gap-3 border-b max-sm:[&_[data-slot=card-action]]:w-full max-sm:[&_[data-slot=card-action]]:justify-start"`。现在只有 3 个页面写了这一串，窄屏下其余页面的按钮会挤到标题右侧。建议在 `PageContainer` 同目录新增 `SbListCard.vue`（插槽：title / description / actions / toolbar / default / footer），把这串 class 和 `TablePager` 的 footer 一起收进去。

### S6 表格列宽与行内操作列（apply）

- 操作列统一为 `TableHead class="sb-col-act"`，内容统一为 `<div class="flex flex-wrap items-center justify-center gap-1">`。现在有的写 `justify-center`，有的写 `inline-flex`（KvKeyListPanel），列宽也有 `w-32`、`w-48`、`min-w-72`、`min-w-56`、`min-w-40` 几种。
- `Sandboxes.vue:65-72` 的 8 个列头都没有宽度类，补上 `sb-col-name / sb-col-sm / sb-col-md / sb-col-act`。
- `Logs` 的 Request ID 列头用了 `sb-col-id`，单元格却写 `max-w-48`；单元格也改为 `sb-col-id`。

### S7 筛选栏统一（apply）

`Logs` / `Users` / `Sandboxes` / `S3Manager` / `KvKeyListPanel` 的筛选栏有三种写法：标签在上（Logs / Users）、标签在左（Sandboxes）、没有标签（S3 / KV）。统一为「标签在上 + `SelectTrigger size="sm"` + `InputGroup` 搜索框」：Sandboxes 改为标签在上，并去掉 `class="h-8"`；KV 的 `SelectTrigger` 补 `size="sm"`。

### S8 ModelSettings 改用 `SbTabsPanel`（apply）

`ModelSettings.vue:4-23` 手写了 `Tabs + TabsList + TabsTrigger`，违反 `ui/AGENTS.md`「多页签面板用 SbTabsPanel」。改用 `SbTabsPanel :items`，`tabFromQuery` 可以简化为 `TABS.includes(raw) ? raw : 'models'`。

---

## 2. 逻辑 bug（B）

> 按 simplify-code 的约定，bug 修复与重构分开提交。每项都给出触发条件，用来写回归测试。

| # | 位置 | 现象 / 触发条件 | 修复 | 类型 |
| --- | --- | --- | --- | --- |
| B1 | Dashboard / Databases / GoFunctions / CronJobs / S3Manager / Logs / Sandboxes / FunctionTriggers / AgentManager 里的 `watch(projectId → load)` | 这些页面都包在 `ProjectScope` 里，而 `ProjectScope` 用 `:key="store.id"` 让整页重新挂载。切项目时，旧实例的 pre-flush watcher 会在卸载前先发一次 `load()`，新实例挂载后再发一次，结果同一组请求打两遍，旧请求失败时还会多弹一条 toast | 删掉页面里的 project watch，只保留 `void load()`；`FunctionTriggers` 的 `watch(..., { immediate: true })` 改为直接调用 | apply（加测试：切项目后每个列表接口只被调用 1 次） |
| B2 | GoFunctions:206 / CronJobs:248 / FunctionTriggers:163,169 | `load()` 每次都执行 `page.value = 1`。用户在第 3 页编辑、启停或删除一条记录后，列表跳回第 1 页 | 去掉 `page = 1`，越界由 `usePagination` 的 `watch(total)` 兜底；只在切换筛选条件时重置页码 | propose（交互变化） |
| B3 | Logs:12 | 级别 `Select` 没有「全部」选项，选了某个级别后清不回去，只有空态里的「清除筛选」能重置 | 加 `SelectItem value="__all__"`，映射为 `undefined`（与 Sandboxes 的 `FILTER_ALL` 同一种写法） | apply |
| B4 | Logs:187-193 | 保留天数输入框清空后 `Number('') === 0`，`Number.isFinite` 为真，会提交 `keepDays=0`；输入框上的 `min=1/max=365` 只在 UI 上生效 | setter 夹到 `[1, 365]`，非法值保持原值；`saveRetention` 前再校验一次 | apply |
| B5 | Logs:252,265 | 导出和保存失败用的是 `(e as Error)?.message`，绕过了 `errorMessage()`，后端错误码不会被翻译 | 统一改为 `errorMessage(e, t(...))` | apply |
| B6 | Logs:278-285 | 自动刷新在页面隐藏时照样每 10 秒轮询（DataBrowser 已经做了隐藏暂停，planv6.7 F12） | 定时回调里加 `document.visibilityState !== 'hidden'` 判断 | apply |
| B7 | S3Manager:289 | 在子目录（`prefix=docs/`）里上传文件时用的是 `file.name`，文件落到根目录，上传后当前视图里看不到 | key 改为 `normalizedPrefix + file.name`（`prefix` 以 `/` 结尾时直接拼接），再走 `validateKey` | propose（改变落盘路径） |
| B8 | S3Manager:393-398 / Logs:245-250 / DataBrowser:494-499 / BackupPanel / SandboxDrawer / S3PreviewModal | `a.click()` 之后同步 `revokeObjectURL`，Safari / Firefox 偶尔会下载到空文件；同样的逻辑复制了 6 份 | 新增 `utils/download.ts` 的 `downloadBlob(blob, filename)`，内部 `setTimeout(() => revoke, 0)`；6 处改为调用它 | apply |
| B9 | Sandboxes:27 | `<pre>` 里写的 `sandbox:\n  enabled: true` 在模板里是字面量，页面上会直接显示 `\n` | 改为 `<pre>{{ SANDBOX_CONFIG_SAMPLE }}</pre>`，常量放在 script 里 | apply |
| B10 | KvPanel:284-291 | 删掉最后一个 key 后 `rows=[]`，但 `hasData` 仍为 true，表格空着，不显示空态和「新建」引导 | 删除后如果 `rows` 为空，调用 `settle(false)`，或者直接 `reload()` | apply |
| B11 | Databases / GoFunctions / CronJobs / KeyValue | 新建、编辑、删除按钮只检查了 `!isAdmin`（系统项目），没检查 `auth.canWrite`。只读管理员（role=admin）也能看到写按钮，点击后才收到 403 | 统一用 `canWrite = !isAdmin && auth.canWrite` 控制（与 FunctionTriggers / Sandboxes 一致）。前端只读只是体验层，后端仍然校验 | apply |
| B12 | Databases:10-22 | 后端 `HasDefaultDatabase` 限定每个项目只能有一个库，但「新建数据库」按钮和空态引导一直显示。已有库时点击必然失败（`ErrAlreadyExists`） | 已有库（`databases.length > 0`）时隐藏新建入口；卡片描述改为「每个项目一个数据库」 | propose（见 R5） |
| B13 | GlobalProjectSwitcher:62-73 | `canCreateProject=false` 时，`Separator` 和一个空的 `div.p-1.5` 仍会渲染，下拉底部多出一条空白 | `v-if` 移到外层，连同 Separator 一起不渲染 | apply |
| B14 | GoFunctions:263 | 复制的是 `POST /go/...` 这段文字，不能直接粘贴到 curl 或浏览器里用 | 改为复制完整 URL（`new URL('/go/…', location.origin)`），toast 里再提示用 POST 方法 | propose |
| B15 | ApiPreview:22 | 页面在 `SidebarInset`（它本身就是 `<main>`）里又嵌了一层 `<main>`，地标重复，不符合 a11y 要求；`const document = ref()` 遮蔽了全局 `document` | 改为 `<section>`；变量改名为 `openapiDoc` | apply |
| B16 | UserMenu:57-61 | 「修改密码」通过 `auth.mustChangePassword = true` 打开弹窗，借用了「强制改密」状态，靠 LoginModal 关闭时再还原（LoginModal:150） | auth store 新增 `openPasswordChange()`，单独维护一个 `passwordDialogOpen` 状态 | propose（store API 变更） |

---

## 3. 代码清理（C）

| # | 位置 | 内容 | 类型 |
| --- | --- | --- | --- |
| C1 | `pages/AgentManager.vue:227-243` | 未使用的 import：`BotIcon` `PlusIcon` `RefreshCwIcon` `CardAction` `Select*`（6 个）`Spinner` `Textarea` | apply |
| C2 | `pages/GoFunctions.vue:154-174` | 未使用的 import：`CopyIcon` `EyeIcon` `PencilIcon` `Trash2Icon` `Tooltip*`（4 个） | apply |
| C3 | `components/databases/kv/KvPanel.vue:74,77` | 未使用的 import：`Button`、`Select*`（5 个）；`onCreated(key)` 里的 `void key` 改为不带参数 | apply |
| C4 | `components/SbAsyncRegion.vue:6` | 未使用的 import：`TableCell` `TableRow` | apply |
| C5 | `pages/CronJobs.vue:223,296-307` | `trigger()` 和 `triggering` 没有任何调用方（「立即执行」走的是 `openRuns(…, { trigger: true })`） | apply |
| C6 | `pages/AgentManager.vue:288,306` | 解构出来的 `threadShowEmpty` 没有用到；`nextCursor` 只写不读，会话列表也没有「加载更多」。删掉；或者在 ThreadSwitcher 里补上分页，二选一 | apply（删除）/ propose（补分页） |
| C7 | `components/databases/DataTabs.vue` | 只有一个页签（KV 已迁出），`Tabs` 外壳没有意义。直接渲染 `CollectionPanel`，事件原样透传 | apply（同步改 tests 里的 DataTabs 用例） |
| C8 | `layouts/DefaultLayout.vue:211,223`、`components/NavMenu.vue:87,95` | `router.getRoutes().find(r => r.name === name)` 写了 4 遍。在 `router/index.ts` 导出 `routeMeta(name)` | apply |
| C9 | `pages/Users.vue:99`、`components/modal/UserFormModal.vue:106` | 字面量 `'simplebase2026'` 硬编码了两处，改为 `stores/auth.ts` 导出的常量 `BOOTSTRAP_USERNAME` | apply |
| C10 | 13 个文件 | `navigator.clipboard.writeText().then(toast…)` 重复。新增 `utils/clipboard.ts` 的 `copyText(text, okMsg, failMsg)`，先替换页面层（S3Manager / GoFunctions），modal 后续顺手改 | apply |
| C11 | Sandboxes:22-32 | 用 `SbAsyncRegion :show-empty="true"` 包了一层静态空态，直接渲染 `SbEmptyState` 即可 | apply |
| C12 | `NavMenu.vue:106-114` | `menuItems` 只被测试读取（`tests/simple-components.test.ts:138`），**保留**；注释改为「测试观察点」 | skip |

---

## 4. 冗余按钮与入口（R）

| # | 位置 | 现状 | 建议 | 类型 |
| --- | --- | --- | --- | --- |
| R1 | Users:98-100 | 「查看」和「编辑」都调用 `openEdit(u)`，打开同一个弹窗 | 有管理权限时只显示「编辑」，否则只显示「查看」 | apply（行为等价，只去掉重复入口） |
| R2 | GoFunctions:104-116 | 行内 5 个按钮：测试 / 查看 / 版本 / 编辑 / 删除，`min-w-72` 撑宽了表格 | 「查看」并入「编辑」：可写时打开编辑，只读时打开查看，变成 4 个按钮。文件名单元格也可以点击打开查看 | propose |
| R3 | S3Manager:165-172 | 行内 4 个按钮（打开 / 下载 / 私链 / 删除）挤在 `w-32` 列里，会换行 | 「打开」（预览弹窗里已经有下载按钮）和「删除」留在行内，「下载」「私链」收进预览弹窗；或者把文件名改成预览链接，行内只留图标按钮 | propose |
| R4 | DefaultLayout:109-116 + 侧栏底部 | 顶栏「刷新页面」（`location.reload()`）和每个页面卡片头的「刷新」重复，整页刷新还会丢掉页面状态 | 删除顶栏刷新按钮。注意 `ui/AGENTS.md` 的定稿规范写了「右上角顺序：…→ 设置 → 刷新」，要先改规范再改代码 | propose（规范变更） |
| R5 | Databases | 单库约束下，「新建数据库」只在没有库时有意义（见 B12） | 有库时隐藏；空态保留「新建」引导 | propose |
| R6 | KeyValue | 卡片标题「Key-Value」与面包屑重复；`PageContainer` 没有 subtitle，副标题塞在 CardDescription 里，与其他页面写法不一致 | 改为 `PageContainer :subtitle`，卡片标题用「键值列表」，与其他页面保持同一套结构 | apply |

---

## 5. 体验优化（X，按页面）

### 首页 Home
- X1：首屏按钮按 S4 统一。已登录用户的「进入控制台」文案改为「继续使用」，并显示当前项目名（读 `useProjectStore().displayName`）。propose。
- 页脚和顶栏都有 GitHub 链接，保留，不处理。

### 监控大盘 Dashboard
- X2：4 张统计卡里有 3 张（库总数 / 就绪 / 异常）讲的是同一个库（单库约束），信息量很低。建议改为「数据库状态（就绪 / 异常 一张卡）· 24h 请求数 · 错误率 · 配额」，数据都来自已有的 `summary`，不需要新接口。propose。
- X3：「请求趋势」卡里塞了分位数、趋势图、资源表三块内容，拆成「请求与延迟」「资源汇总」两张卡，资源表放到下方。apply（纯布局）。
- 资源表里的「对象存储」数量来自 `api.s3.list`，只统计根目录对象。在 hint 里注明，或者改用 `listDetailed` 的总数。propose。

### 数据库 Databases
- 桌面端展开按钮用的是 `rounded-full` 的 ±（`icon-sm`），移动端用的是文字按钮，两端不一致。统一为 `ChevronRight` 旋转图标 + `ghost icon-sm`。apply。
- i18n key `pages.databases.expand` 的文案其实是「收起数据」，key 名和语义相反。改为 `collapseData`，两棵语言树同步。apply。

### 数据看板 DataBrowser
- 工具栏尺寸按 S1 统一为 `sm`。「图表类型」选择器放在 Agent 面板上方，与 Agent 关系紧密，用 `FieldLabel` 组合起来。apply。
- 「添加图表 / 恢复默认」这一行放在明细表上方，与卡片网格贴得很紧。移到 BiToolbar 右侧的同一行，减少一行视觉层级。propose。

### 对象存储 S3Manager
- X4：进入子目录后没有返回上级的入口，只能手动清空输入框。在搜索框左侧加面包屑（根 / a / b），每一段都可以点；搜索框改成输入后自动查询（300ms debounce），不再需要按 Enter。propose。
- 上传进度条只在 `uploadPercent > 0` 时显示，0 到首个进度事件之间没有反馈。按钮内的 Spinner 已经能提示，保留，不处理。

### 云函数 / 触发器 / 定时任务
- 三个页面的表格结构几乎一样（名称 / 目标 / 状态 / 启用 / 操作），套用 S5 的 `SbListCard` 后，每页大约少 40 行模板。apply。
- FunctionTriggers：`events.join(',')` 改为 `join(', ')`，或者每个事件渲染成一个 Badge。apply。

### 日志 Logs
- X5：「刷新」按钮实际作用是「按筛选条件查询」，文案改为「查询」（`SearchIcon`），并且是本页唯一的主按钮；「导出」保持 outline。apply（文案变化，补 i18n）。
- X6：趋势图放在表格下方，用户得先滚过 10 行日志才能看到。移到筛选栏和表格之间，并改用 `TrendChart` 的紧凑高度。propose。
- 「保留策略」是低频设置，从页面底部挪进卡片头的「⋯」或设置弹窗的「存储」页签。propose。

### 用户 Users
- X7：筛选后没有结果时显示的还是「暂无用户」（和真正没有用户时同一句），应改为「没有匹配的用户」，并提供「清除筛选」按钮。apply。
- `api.users.list(100)` 只取 100 条，也没有分页，超过 100 个用户时多出的部分会被悄悄截掉。接入 `usePagination` + `TablePager`，取数改为后端游标分页。propose。
- 状态列用彩色文字，其他页面用 Badge，改为 `Badge variant="success|outline"`。apply。

### 沙盒 Sandboxes
- 状态 Badge 的颜色按 `item.status` 判断，文字显示的却是 `item.state`，两个字段混用。统一按 `state` 判断颜色和文字，`status==='error'` 单独显示一个 destructive 点。apply。
- 筛选后没有结果时文案同 X7，改为「没有匹配的沙盒」。apply。

### 模型 ModelSettings / 云助手 AgentManager
- ModelSettings 按 S8 处理。
- X8：AgentManager 左栏固定 `200px`，助手名称经常被截断。改为 `md:grid-cols-[220px_minmax(0,1fr)] xl:grid-cols-[240px_…]`。propose。
- 头部「⋯」菜单里有 5 个手写的 `<button class=...>`，样式完全相同，抽成 `const menuItemClass`，或者改用 `Button variant="ghost" size="sm" class="w-full justify-start"`（与 BiToolbar 的数据操作菜单一致）。apply。

### API 预览 ApiPreview
- 搜索没有结果时左栏是空的，补一个 `SbEmptyState`（小号）。原生 `<textarea>` 改为 `ui/textarea`。apply。

---

## 6. 执行顺序与提交拆分

1. **PR-1 清理（只做 apply、零行为变化）**：C1–C5、C7–C11、S5 中删除 `p-0` 包装、B15 改名部分。做完后跑 `yarn typecheck && yarn build && yarn test`，与基线对比。
2. **PR-2 样式统一**：S1–S8（含新增 `SbRefreshButton` / `SbListCard` 及测试），R1、R6，以及各页面里标 apply 的 X 项。顺手更新 `ui/AGENTS.md` 的「按钮尺寸规范」小节。
3. **PR-3 bug 修复（apply 部分）**：B1、B3–B6、B8–B11、B13，每项配一个回归用例。
4. **PR-4 交互调整（全部 propose，需逐条确认）**：B2、B7、B12/R5、B14、B16、R2–R4、X1、X2、X4、X6、X8、Users 分页。

## 7. 验证

- 基线（修改前先记录，按 AGENTS.md）：`cd ui && yarn typecheck`（应为 0 错误）、`yarn build`、`yarn test`（statements 98.43% / branches 94.92% / functions 94.1%）。
- 每个 PR 结束后重跑这三条，覆盖率不得低于基线；新组件 `SbRefreshButton` / `SbListCard` / `utils/download.ts` / `utils/clipboard.ts` 都要补测，争取把 functions 拉回 95% 以上。
- 必须同步修改的测试：`tests/simple-components.test.ts`（NavMenu 读取 `menuItems`，保持可用）、DataTabs 相关用例（C7）、`tests/modal-input-layout.test.ts`（S7 改筛选栏时要保持断言通过）、`tests/i18n.test.ts`（新增或重命名 key 时两棵语言树同步）。
- 手工走查：窄屏（<640px）下各列表页卡片头换行、顶栏控件同高、切换项目时在 Network 面板确认每个接口只请求 1 次（B1）。

## 8. 本计划的产出范围

- 本次只做了只读分析，没有修改任何源码。
- 未使用 import 的清单由脚本扫描 `<script>` 块得出，人工核对后列为 C1–C4；`services/http-api.ts` 的扫描结果是误报（类型通过 `export type` 再导出），已排除。
- B1 的重复请求是根据 Vue 调度顺序推理得出的（pre-flush watcher 在父组件重新渲染之前执行），还没有写测试复现，PR-3 里要先补「失败用例 → 修复」。

---

## 9. 执行进度（2026-11-18 第一轮）

### 已完成

| 编号 | 落地内容 |
| --- | --- |
| C1–C5 | 删除未使用的 import（AgentManager / GoFunctions / KvPanel / SbAsyncRegion）；删除 `CronJobs.trigger()` 和 `triggering` |
| C6 | 删除 AgentManager 中的 `threadShowEmpty` 和 `nextCursor`（会话分页仍按 propose 处理，暂不做） |
| C7 | `DataTabs` 去掉单页签的 Tabs 外壳，保留「集合文档」分组标题，事件透传不变 |
| C8 | `router/index.ts` 新增 `routeMeta` / `routeTitle`，NavMenu 和 DefaultLayout 改用它们 |
| C9 | `stores/auth.ts` 新增 `BOOTSTRAP_USERNAME`，Users 和 UserFormModal 改为引用该常量 |
| C10（部分） | 新增 `utils/clipboard.ts` 的 `copyText`，GoFunctions 已改用。S3 的 `copyUrl` 保留原实现，因为它的失败文案取自异常 message，有测试依赖这一行为 |
| C11 | Sandboxes 的静态空态不再包一层 `SbAsyncRegion` |
| S1 | 去掉 9 处 `<Button>` 上的 `h-*`/`px-*` 手写覆盖，改用 `xs` / `sm`；GoFunctions 的「测试」按钮和 Users 的行内按钮改为 `ghost`；BiToolbar 的按钮和下拉、ApiPreview 的「运行」统一为 `sm` |
| S2 | 新增 `components/SbRefreshButton.vue`，替换 9 个页面中的刷新按钮；Databases / GoFunctions / CronJobs / FunctionTriggers 的双 `CardAction` 合并为一个 |
| S3 | 顶栏 GlobalProjectSwitcher 和 UserMenu 的高度统一为 h-8；DefaultLayout 的工具按钮和侧栏入口样式抽成常量 `TOOL_BTN` / `SIDE_ENTRY` |
| S4 | Home 页主按钮改用 `buttonVariants` |
| S5 | 删除 6 处 `<div class="p-0">`；CronJobs / S3 / Users / Sandboxes / KeyValue 的卡片头补上窄屏换行 class（`SbListCard` 未实现，见下文） |
| S6 | 操作列统一用 `sb-col-act`，内容统一为 `flex flex-wrap items-center justify-center gap-1`；Sandboxes 的列头补上宽度；Logs 的 Request ID 单元格改为 `sb-col-id` |
| S7 | Sandboxes 筛选栏改为标签在上的 `sm` 下拉；KV 的类型下拉改为 `sm` |
| S8 | ModelSettings 改用 `SbTabsPanel` |
| B3–B6 | Logs：级别下拉加「全部」；保留天数夹在 [1, 365] 区间，清空或输入非数字时保持原值；错误统一走 `errorMessage`；页面隐藏时暂停轮询 |
| B7 | S3 当前前缀以 `/` 结尾（即在某个目录下）时，上传落到该目录；其他情况与原来一致，仍落到根目录。已补回归用例 |
| B8 | 新增 `utils/download.ts` 的 `downloadBlob`（URL 延迟一个宏任务回收），替换 Logs / S3 / DataBrowser / BackupPanel / SandboxDrawer 中的 5 处实现。S3PreviewModal 的下载带 CORS 回退逻辑，暂不替换 |
| B9 | Sandboxes 配置示例中的 `\n` 现在能正常换行 |
| B10 | KV 删除最后一个 key 后重新扫描，页面会显示空态和「新建」引导 |
| B11 | Databases / GoFunctions / CronJobs / KeyValue 的写入口按 `!isAdmin && auth.canWrite` 显示；KeyValue 面板对只读角色进入只读模式 |
| B13 | 项目切换器：无建项目权限时，分隔线和空容器都不再渲染 |
| B15 | ApiPreview 内层的 `<main>` 改为 `<section>`；`document` 变量改名为 `openapiDoc`；原生 textarea 改为 `ui/textarea`；搜索无结果时显示提示 |
| R1 | Users 页：有管理权限时显示「编辑」，否则显示「查看」，不再两个按钮并列 |
| R6 | KeyValue 页的副标题改走 `PageContainer`，与其他页面结构一致 |
| X5 | Logs 页主按钮文案改为「查询」（`SearchIcon`），新增 `common.query` |
| X7 | Users / Sandboxes 筛选无结果时显示「没有符合当前筛选条件的结果」和「清除筛选」按钮，新增 `common.noMatchFilter` / `common.clearFilter` |
| 其他 | AgentManager「⋯」菜单项样式抽成常量 `MENU_ITEM`；Databases 行展开按钮改为 `ChevronRight` 旋转图标（测试改用 `data-row-expand` 定位）；i18n key `pages.databases.expand` 改名为 `collapseData` |
| 规范 | `ui/AGENTS.md` 新增「按钮尺寸规范」，以及 `downloadBlob` / `copyText` 的使用约定 |

### 验证中修正的结论

- **B1 不成立，已撤销**：写了一个最小探针复现 `:key` 重挂载场景，结果只有 `mount:a → mount:b`，旧实例上的 watch 没有再触发，不存在重复请求。各页面的 project watch 保持原样，不改。

### 第二轮已落地（2026-11-18）

| 编号 | 落地内容 |
| --- | --- |
| B2 | GoFunctions / CronJobs / FunctionTriggers / ApprovalsPanel 的 `load()` 不再重置页码；项目（或筛选项）切换时才回到第 1 页。保存、删除后刷新保留当前页 |
| B14 | 云函数导出 Badge 点击复制完整调用 URL（`location.origin + /go/<pid>/<库>/<函数>`，含 encodeURIComponent），toast 文案改为「已复制调用 URL：{path}」 |
| B16 | `stores/auth.ts` 新增 `passwordDialogOpen`（主动改密独立状态），UserMenu 改走 `openPasswordChange()`；LoginModal 在主动改密时显示「取消」而不是「退出登录」，取消/改密成功均不动 `mustChangePassword`；`markUnauthorized` / `logout` / `changePassword` 同步清理该状态 |
| R2 | GoFunctions 行内按钮精简为 测试 / 编辑 / 版本 / 删除（去掉「查看」，列宽 `min-w-72 → min-w-40`） |
| R3 | S3 行内按钮精简为 打开 / 删除；「复制链接」收进 S3PreviewModal 底部（含完整 URL 拼接），下载本来就在预览弹窗内 |
| R4 | 删除顶栏与窄屏侧栏的「刷新页面」按钮及 `layout.refreshPage` 文案；先更新 `ui/AGENTS.md` 定稿规范（刷新只在各卡片头 `SbRefreshButton`） |
| Users 分页 | `/v1/users` 游标分页（20/页，后端上限 100）：`cursors` 数组缓存每页游标，`TablePager` 内嵌；筛选仍作用于当前页数据 |
| S5（完成） | 新增 `components/SbListCard.vue`（title / description / actions / toolbar / 默认插槽 + `pager` 内嵌 TablePager），替换 GoFunctions / CronJobs / FunctionTriggers / S3Manager / Databases / Users / Sandboxes / KeyValue / McpServersPanel / ApprovalsPanel 共 10 处样板卡片头；`ui/AGENTS.md` 补充使用约定 |

新增测试：LoginModal 主动改密弹窗（B16）、Users 游标分页、SbListCard 渲染与事件转发、S3 行内精简回归。

### 第三轮已落地（2026-11-18）

| 编号 | 落地内容 |
| --- | --- |
| X1 | Home 首屏主按钮：已登录显示「继续使用 · 项目名」（无当前项目时「继续使用」），未登录仍为「进入工作台」 |
| X2 | Dashboard 顶部 4 张卡改为「数据库状态（就绪 / 总数 + 异常提示）· 24 小时请求 · 错误率 · 配额」，不新增接口；`summaryLine` 摘要并入卡片 |
| X3 | Dashboard「资源汇总」拆为独立卡片（趋势卡只保留延迟分位数与趋势图） |
| X4 | S3 面包屑（根目录 / a / b，可逐级返回）；前缀输入 300ms 防抖自动查询，Enter / 点面包屑 / 打开目录立即查询 |
| X6 | Logs：趋势图上移到筛选栏与表格之间（`TrendChart` 新增 `dense` 紧凑高度）；保留策略收进卡片头「保留策略」Popover |
| X8 | AgentManager 左栏改为 `md:220px` / `xl:240px` |
| 胶囊页签 | `SbTabsPanel`（default 变体）与 `KvPanel` / `KvApiPanel` 的胶囊页签去掉 `overflow-x-auto`，改为 `flex-wrap` + `h-auto`，页签 `px-4 flex-none` 按文字撑宽，窄屏折行而不出现滚动条；line 变体不变 |

### 暂缓（仍需确认）

B12 / R5（单库约束下隐藏「新建数据库」）。

### 验证结果

- `yarn typecheck`：0 错误（基线也是 0）。
- `yarn build`：通过。
- `yarn test`：150 个文件、1498 条用例，唯一失败的是 `docs-content.test.ts`，原因是 `docs/getting-started/openai-compatible.md` 链接到了 `plan/` 目录。这条在修改前的基线里就已经失败，与本次改动无关。
- 覆盖率（排除上述用例后运行）：statements 98.58% / branches 94.88% / functions 94.35%。基线是 98.43% / 94.92% / 94.1%；branches 比基线低 0.04pt，functions 和 branches 仍低于 95% 阈值，与基线状态一致。
- **第二轮验证（2026-11-18）**：typecheck 0 错误、build 通过；150 个文件 1502 条用例，唯一失败仍是预先存在的 `docs-content.test.ts`（与 UI 改动无关）；覆盖率 statements 98.54% / branches 94.89% / functions 94.35%，全部高于 90% 阈值，与第一轮持平。
- 新增测试：`tests/ui-simplify-helpers.test.ts`（覆盖 SbRefreshButton / downloadBlob / copyText）、S3 子目录上传用例、Users 和 Sandboxes 的无匹配用例、Logs 保留天数夹值用例。
