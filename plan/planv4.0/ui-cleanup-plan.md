# SimpleBase v4.0 前端清理计划

> 日期：2026-09-29（v1.1 修订于同日；**v1.2 执行完毕标记**，见文末执行记录）
> 范围：`ui/src/`、`ui/tests/`、`ui/` 根目录（含 README/AGENTS 文档一致性、package.json 依赖、类型健康）
> 方法：与后端 cleanup-plan 同一套证据标准——「全仓 import 解析（静态 + 动态 `import()`）+ 桶导出消费分析 + `Api` 接口方法消费扫描 + 契约↔实现三方比对 + stores/lib/constants 逐成员消费核对 + 依赖消费核对 + `tsc --noEmit` 体检 + git 跟踪状态核对」；结论均经脚本验证
> 基线：`yarn build` 通过；`yarn test` 393/395 通过（2 个既有失败，见 F0，均为本计划待修项而非阻塞项）；`tsc --noEmit` 有 4 处 src 真实错误（见 G5）
> 对照：后端清理见同目录 `cleanup-plan.md`；本计划独立执行，不依赖后端清理进度

---

## ✅ 执行状态（2026-09-29 v1.2）

**本计划已全部执行完毕。** F/G/H/I 组全部落地，J4 按建议提级修复，J1-J3 按计划延后决策。最终状态：

- `yarn build` 通过；`yarn test` **85 文件 / 395 测试全绿**（两轮验证稳定）；`yarn typecheck`（新增 script）src 内零真实错误
- 覆盖率 98.03% stmts / 93.67% funcs / 91.68% branches——**低于 95% 阈值系清理前基线即如此**（已验证与本次清理无关），阈值未调整
- `src/components/ui`：171 → 146 个 .vue；桶导出 197 → 150
- 共 14 个 commit（每项独立提交，可按 commit 单独 revert），见文末执行记录

| 组 | 状态 | 说明 |
| --- | --- | --- |
| F1-F3 | ✅ 已完成 | .DS_Store 删除；KeyValue 断言改为现文案；router 用例加 15s 超时 |
| G1 | ✅ 已完成 | `DatabaseListResult` 删除，全仓零引用 |
| G2 | ✅ 已完成 | `llm.providers` 契约+双端实现+5 处测试桩删除（选删除方案；后端路由未动） |
| G3 | ✅ 已完成 | `auth.refresh` api 层副本删除（http.ts tryRefresh 为真实路径，已有测试覆盖） |
| G4 | ✅ 已完成 | `agentThreads.run` 非流式版删除，4 处测试改写；`AgentRun` 类型保留（cancel 仍返回） |
| G5 | ✅ 已完成 | `gofunctions.update` 双端实现删除，5 处测试引用迁移到 saveVersion；TS2353 消失 |
| G6 | ✅ 已完成 | http.ts content-type 改 `String()` 强转（TS2339 消失） |
| G7 | ✅ 已完成 | vite-env.d.ts 非法声明删除；catalog.ts 改走 `@docs` alias（TS2436 消失） |
| G8 | ✅ 已完成 | `@types/dompurify`、`@types/marked` 从 devDependencies 删除 |
| G9 | ✅ 记录在案 | 无动作（全部依赖活跃） |
| H1 | ✅ 已完成 | 16 个整文件删除。**修正两处计划偏差**：`ScrollBar.vue` 是 ScrollArea 内部依赖→文件保留（仅删桶导出）；`DialogOverlay.vue` 被 DialogContent import→文件保留（仅删桶导出） |
| H2 | ✅ 已完成 | ~30 个零消费桶导出裁剪；toggle 桶保留 `toggleVariants` 仅删组件导出；**Select 的 Scroll{Up,Down}Button 导出恢复**（SelectContent.vue 经 `from '.'` 内部消费，属计划漏判的内部依赖） |
| H3 | ✅ 已完成 | sidebar 10 文件删除（计划列 11，其中 `SidebarFooter` 等全数命中；`useSidebar` 桶导出删除但 utils.ts 保留——内部 4 处消费） |
| H4 | ✅ 遵守 | 先删导出行再删文件；分批提交；coverage exclude 不变 |
| I1/I2 | ✅ 已完成 | README 目录树补 `tests/` 平级节点与说明；测试小节更新 |
| I3 | ✅ 已完成 | `/go` 代理注释补充「UI 自身不请求此前缀」 |
| I4 | ✅ 已完成（保守方案） | 加 `"typecheck": "tsc --noEmit"` script（零新依赖）；src 当前零真实错误；tests 内约 15 处 TS7006/TS2339 历史噪声留待后续；vue-tsc 引入单独立项 |
| I5 | ✅ 已完成（方案 a） | LoginModal 补 `loginReason` 展示（Alert 组件）；`booting` 死状态删除 |
| J1 | ⏸ 延后 | mock 退役决策，按计划不动 |
| J2 | ⏸ 无动作 | 已删除组件回归检查，记录在案 |
| J3 | ⏸ 延后 | 测试拼盘重排，按计划不动 |
| J4 | ✅ 已提级修复 | `persist()` 补 `projectDefaults` 落盘 + reload 存活测试；独立 commit |
| K | — | 防误删清单在执行中全部复核有效，无误删 |

**执行中发现并处理的计划外事项**：

1. **测试迁移路径改写恢复**：用户工作区原有的 tests 迁移配套改动（`import '@/lib/utils'` 别名化路径改写、tsconfig include tests/、KV 功能页面与组件）在会话中曾被 stash 往返意外覆盖，已从 dangling stash 对象完整找回并提交（28a031f、be245d5），85 文件 394→395 测试全绿验证无损。
2. **计划 H 组两处内部依赖漏判**（ScrollBar、Select ScrollButtons）在「删导出行→build 即报错」的验证闭环中被当场捕获并修正，印证了 H4 约束的有效性。

---

## 0. 结论总览（原始计划，保留存档）

| 级别 | 含义 | 数量 |
| --- | --- | --- |
| F | 立即处理（垃圾文件 / 既有测试失败修复） | 3 项 |
| G | 死代码与契约漂移（零消费删除、类型错误修复，需回归测试） | 9 项 |
| H | ui 基础组件库瘦身（本地 fork 库代码，零消费导出裁剪） | 4 项 |
| I | 文档与实现漂移修正 | 4 项 |
| J | 退役候选（需决策，默认本轮不动） | 4 项 |
| K | 防误删清单（已核实为活跃代码） | — |

预计收益：修复 2 个既有失败测试；修复 4 处类型错误（其中 1 处是**契约漂移实锤**：`gofunctions.update` 不在 `Api` 接口却在实现里）；删除 1 个死类型 + 2 个 stub 类型包 + 约 50 个零消费 ui 库导出；`ui/` 文档与 tests 目录迁移后的实际结构对齐；清理 1 个本地垃圾文件。

**重要前置说明**：工作区当前有大量未提交变更（tests 从 `src/**` 迁至顶层 `ui/tests/`、KV 功能新增、`Home.vue`/`KeyValue.vue` 等 untracked 文件）。**建议先将现有工作成果提交，再开始本计划的清理项**，避免清理 commit 与功能 commit 混杂。

---

## F. 立即处理（零风险）

### F1. 本地垃圾文件 `src/pages/.DS_Store`
- **证据**：存在于工作区；`.gitignore` 第 2 行已含 `.DS_Store`，`git ls-files` 确认未被跟踪。
- **动作**：`rm src/pages/.DS_Store`（仅本地磁盘，不涉及 git）。

### F2. 既有失败测试①：`tests/KeyValue.test.ts:60` 文案断言过期
- **证据**：`yarn test` 全量跑 `tests/KeyValue.test.ts > 渲染 KvPanel 并透传当前项目上下文` 失败——期望 `项目级存储`，实际 `KeyValue.vue:8` 文案已改为 `Redis 语义的 Key-Value 数据服务：string / hash / list / set / zset。`（key-value-ducklake-plan 落地时改的，测试未跟上）。
- **动作**：将断言改为与现文案匹配（`expect(w.text()).toContain('Redis 语义')` 或直接删掉该行——上下两行已分别断言标题与面板存在）。
- **验证**：`npx vitest run tests/KeyValue.test.ts`。

### F3. 既有失败测试②：`tests/router.test.ts` 旧链接用例偶发超时
- **证据**：全量跑 5000ms 超时（该用例逐条 push 8+4 条旧路径并等待解析）；单跑 3 次均 ~2.2s 通过。属临界超时，非逻辑错误。
- **动作**：给该用例加显式超时参数 `it(..., 15000)`（或在该文件顶部 `describe` 级配置）。改动一行。
- **验证**：`yarn test` 全量两轮确认稳定。

---

## G. 死代码与契约漂移清理（低风险，删除/修复后必须回归测试）

### G1. 死类型 `DatabaseListResult`（`src/services/types.ts:194`）
- **证据**：
  - `Api.databases.list` 的契约是 `Promise<DatabaseItem[]>`（types.ts:583），http-api.ts:759-764 实现直接返回数组（`r.data?.databases` 已在实现内拍平，cursor 被丢弃），mock.js:708 同样返回数组；
  - 全仓 grep `DatabaseListResult` 仅 types.ts 定义处 1 次命中，零引用；
  - 与之对应的 `UserListResult` 是真实消费的（users.list 返回它），勿混淆。
- **动作**：删除 types.ts:194-197 的 `DatabaseListResult` 接口（4 行）。
- **验证**：`yarn build && yarn test`。

### G2. `Api.llm.providers` 契约 + 双端实现（前端零消费）
- **证据**：
  - 业务代码（pages/components/composables/stores）零处调用 `api.llm.providers`；仅 `src/test/api-mock.ts:259`、`tests/pages-coverage.test.ts:177` 两个测试桩在 mock 它；
  - SettingsPanel 的「providers」分区实际渲染的是**前端静态 catalog** `constants/llmProviders.ts` 的 presets，不请求后端；
  - 消费面只剩 `api.llm.chat/stream`（useAiChat.ts:40/58）。
- **动作**（二选一）：
  1. **删除**（默认）：types.ts `llm.providers` 字段 + http-api.ts 对应实现 + mock.js 对应方法 + 2 处测试桩引用；
  2. **保留**：若计划让 providers 分区改为动态拉取后端已配置厂商（后端 `GET /llm/providers` 路由仍活跃，router.go:295），则留待该需求落地再评估。
- **验证**：`yarn build && yarn test`。
- **注意**：后端路由本轮不动（后端 cleanup-plan D2 已单列 llm 面收紧决策）。

### G3. `Api.auth.refresh` 契约 + 双端实现（前端零消费）
- **证据**：
  - `auth.refresh` 在业务代码零消费；实际刷新走 `services/http.ts:70` 的 `tryRefresh()`——直接 `axios.post('/v1/auth/refresh')`，**不经 api 层**；
  - 即「同一能力存在两套实现」：http.ts 私有版（在用）+ api 层版（死代码）。
- **动作**：删除 types.ts `auth.refresh` 字段 + http-api.ts 对应实现 + mock.js 对应方法（先 grep 确认无测试依赖；`auth.test.ts` 若有用例需同步迁移到 http.ts 的 tryRefresh 已覆盖范围——http.test.ts 已覆盖 tryRefresh）。
- **验证**：`yarn build && yarn test`。

### G4. `Api.agentThreads.run`（非流式版，前端零消费）
- **证据**：
  - 业务代码只用 `agentThreads.streamRun`（AgentManager.vue:462）；`run` 仅 `tests/http-api.test.ts:271/575`、`tests/mock.test.ts:234/329` 在测实现本身；
  - 与 G2/G3 同理：实现完整、契约在册、无真实调用方。后端对应 handler 属后端 D2 决策范围，本轮只清前端。
- **动作**：删除 types.ts `agentThreads.run` 字段 + http-api.ts 实现 + mock.js 方法 + 4 处测试改写（mock.test.ts 的 thread-run 语义用例改为走 streamRun 或删除）。
- **验证**：`yarn build && yarn test`。
- **保守选项**：此项涉及改写 4 处测试，若不想动测试可降级到 J 组决策。

### G5. `gofunctions.update` 契约漂移（v1.1 新增，实锤类型错误）
- **证据**：
  - `Api.gofunctions` 接口（types.ts:542-562）**不含** `update`，但 http-api.ts:667-672 实现了它（注释自称「兼容旧签名：保存为新版本并生效」，实现等价 `saveVersion` + `activate: true`），mock.js 同样实现；
  - `tsc --noEmit` 直接报错：`src/services/http-api.ts(673,5): error TS2353: 'update' does not exist in type ...`——**当前代码库类型检查根本过不去**，只因工具链没有 typecheck 环节才没被发现（见 G6）；
  - 业务代码零消费：GoFunctionModal.vue:176 编辑路径走的是 `saveVersion`；仅 `src/test/api-mock.ts:341`、`tests/http-api.test.ts:46`、`tests/mock.test.ts:127-129` 在引用。
- **动作**：
  1. 删除 http-api.ts 的 `update` 实现（667-672 行）+ mock.js 对应方法；
  2. `tests/http-api.test.ts:46` 用例改为测 `saveVersion`；`tests/mock.test.ts:127-129` 的三行改为 saveVersion 语义或删除；`src/test/api-mock.ts:341` 删 stub 行。
- **验证**：`npx tsc --noEmit` 该错误消失 + `yarn test`。

### G6. `src/services/http.ts:51` axios 类型错误（v1.1 新增）
- **证据**：`tsc --noEmit` 报 `TS2339: Property 'includes' does not exist on type 'string | number | true | string[] | AxiosHeaders'`——`r.headers?.['content-type']` 在 axios 1.20 类型下是联合类型，`ct.includes(...)` 直接编译错（运行时碰巧正常因为值总是 string）。
- **动作**：改为 `const ct = String(r.headers?.['content-type'] || '')`。
- **验证**：`npx tsc --noEmit` src 下零错误（ui/ 库与 .vue 模块声明噪声除外）。

### G7. `src/vite-env.d.ts:29` 非法 ambient 声明（v1.1 新增）
- **证据**：`tsc --noEmit` 报 `TS2436: Ambient module declaration cannot specify relative module name`——`declare module '../../../docs/_meta.json'` 是相对路径，TS 不允许；它对应 `src/docs/catalog.ts:1` 的 `import metaJson from '../../../docs/_meta.json'`。
- **动作**：删掉 vite-env.d.ts:29-34 这段声明；把 catalog.ts 的 import 改为别名路径 `@docs/_meta.json`（vite.config.ts 已配 `@docs` alias，且 vite-env.d.ts:22 已有对应声明）。
- **验证**：`npx tsc --noEmit` + `yarn build`（catalog 是 DocsWiki 数据源，必须确认 glob 不受影响）。

---

## Gbis. 依赖与类型包清理（v1.1 新增）

### G8. stub 类型包 `@types/dompurify`、`@types/marked`（devDependencies 死项）
- **证据**：
  - 两个包的 package.json 均自标 `"deprecated": "This is a stub types definition. XXX provides its own type definitions, so you do not need this installed."`——dompurify 3.4.16 与 marked 18.0.13 均自带完整 `.d.ts`；
  - 安装后没有任何 `.d.ts` 文件落地（`ls node_modules/@types/dompurify/` 仅 README/package.json），纯粹是 npm 装了个空壳占位。
- **动作**：从 `package.json` devDependencies 删除两行 + `yarn install` 刷新 lock 文件。
- **例外说明**：本项目 AGENTS.md 约定「不改 package.json 除非用户明确要求」——本计划本身即为清理授权，但仍建议该项单独 commit 并在提交说明标注。
- **验证**：`yarn install && yarn build && yarn test`（类型来自包本体，删除无感）。

### G9. 依赖消费体检结论（全部活跃，无死依赖）
- **证据**（grep 逐包计数）：`axios`(1)、`echarts`(2)、`marked`(1)、`dompurify`(1)、`monaco-editor`(1，GoMonacoEditor.vue 经 `monaco-editor/editor.js` 深路径 import)、`@tailwindcss/vite`(vite.config.ts 插件)、`tw-animate-css`(style.css import)、`class-variance-authority`(12)、`clsx`/`tailwind-merge`(lib/utils.ts)、`vue-sonner`(38)、`@lucide/vue`(40)、`@vueuse/core`(62，其中 `reactiveOmit` 59 处集中在 ui 库)、`reka-ui`(90)——**dependencies 全部在用，无一可删**。
- **结论**：无动作；记录在案防后人误判（尤其 monaco 深路径 import 与 tailwind 插件这类非显式消费）。

---

## H. `src/components/ui/` 基础组件库瘦身

> 依据 AGENTS.md：ui/ 是「本地 fork 的库代码，只增删组件不改风格约定；删除组件前必须全局确认零引用」。
> 以下均经桶导出消费分析验证（含 tests/ 消费）。**默认策略：删导出 + 删对应 .vue 文件**；若嫌激进，可只做「删桶导出」保留文件（编译不再进包，体积收益相同，目录保留备用）。

### H1. 整文件零消费清单（12 个 .vue，纯死文件）
| 文件 | 证据 |
| --- | --- |
| `ui/avatar/AvatarImage.vue` | 桶导出未被消费（SettingsPanel 只用 Avatar/AvatarFallback） |
| `ui/breadcrumb/BreadcrumbEllipsis.vue` | 同上 |
| `ui/combobox/ComboboxCancel`（reka-ui re-export） | index.ts:13 直接 re-export，零消费 |
| `ui/command/CommandDialog.vue` | 零消费 |
| `ui/dialog/DialogClose.vue`、`DialogOverlay.vue`、`DialogScrollContent.vue`、`DialogTrigger.vue` | 4 件全零消费（SbModal 只用 Dialog/DialogContent/DialogTitle/DialogDescription；CommandDialog 依赖链随 CommandDialog 删除） |
| `ui/field/FieldSeparator.vue` | 零消费 |
| `ui/popover/PopoverAnchor.vue`、`PopoverDescription.vue`、`PopoverHeader.vue`、`PopoverTitle.vue` | 4 件全零消费 |
| `ui/scroll-area/ScrollBar.vue` | 零消费（DocsSidebar 只用 ScrollArea） |
| `ui/table/TableCaption.vue`、`TableFooter.vue` | 零消费 |
| `ui/sheet/SheetClose.vue`、`SheetTrigger.vue` | 零消费 |

**注意**：`DialogScrollContent` 若被 `CommandDialog` import，删除 CommandDialog 后它才变零消费——两者同批删。**删文件前必须同步删对应 index.ts 导出行**。

### H2. 零消费导出（保留文件、仅删导出，或整文件删）
| 桶 | 零消费导出 |
| --- | --- |
| `ui/alert` | AlertAction |
| `ui/alert-dialog` | AlertDialogMedia |
| `ui/avatar` | AvatarBadge、AvatarGroup、AvatarGroupCount、AvatarImage |
| `ui/combobox` | ComboboxCancel、ComboboxItemIndicator、ComboboxSeparator、ComboboxTrigger |
| `ui/command` | CommandDialog、CommandSeparator、CommandShortcut |
| `ui/dialog` | DialogClose、DialogOverlay、DialogScrollContent、DialogTrigger |
| `ui/field` | FieldSeparator |
| `ui/input-group` | InputGroupButton、InputGroupText、InputGroupTextarea |
| `ui/popover` | PopoverAnchor、PopoverDescription、PopoverHeader、PopoverTitle |
| `ui/scroll-area` | ScrollBar |
| `ui/select` | SelectItemText、SelectLabel、SelectScrollDownButton、SelectScrollUpButton、SelectSeparator |
| `ui/sheet` | SheetClose、SheetTrigger |
| `ui/sidebar` | SidebarFooter、SidebarGroupAction、SidebarInput、SidebarMenuAction、SidebarMenuBadge、SidebarMenuSkeleton、SidebarMenuSub、SidebarMenuSubButton、SidebarMenuSubItem、SidebarSeparator、useSidebar |
| `ui/table` | TableCaption、TableFooter |
| `ui/toggle` | Toggle（组件本身零消费，但 `toggleVariants` 被 toggle-group 两文件 import——**保留 index.ts 的 toggleVariants 导出，仅删 Toggle 组件导出**，或整个目录并入 toggle-group 后删） |

合计约 50 个零消费导出；对应删除后 `src/components/ui` 文件数可从 233 降至约 218。

### H3. sidebar 目录专项（26 文件中 11 件零消费，最大单项）
- **证据**：仅 `NavMenu.vue` 与 `DefaultLayout.vue` 两个消费者；消费明细经逐名核对（见 H2 表 sidebar 行）。
- **动作**：删除 11 个零消费文件 + index.ts 对应导出行。`useSidebar`（index.ts re-export 的 composable）与 `sidebar/utils.ts` 依赖关系删除前复核一次。
- **验证**：`yarn build && yarn test`。

### H4. 执行约束（对所有 H 组）
- 每删一个文件必须同步删 index.ts 导出行，否则 vite build 报 missing module；
- `yarn test` 中 `tests/simple-components.test.ts`、`tests/components-interactions.test.ts` 等若直接 import 这些 ui 文件则需同步清理断言；
- **每 10 个文件一批提交**，出问题按批 revert；
- coverage 配置 exclude 了 `src/components/ui/**`，删除不影响 95% 阈值。

---

## I. 文档与实现漂移修正

### I1. `ui/README.md` 目录树缺 `tests/`（迁移后漂移）
- **证据**：tests 已从 `src/**` 全量迁至顶层 `ui/tests/`（85 个文件，git 状态显示 76 个 RM 重命名 + 9 个新增），但 README 目录树仍写 `└─ test/  Vitest setup`（指 `src/test/`），未提 `ui/tests/`；「页面（每页同名 .test.ts）」的说法已失效。
- **动作**：目录树补 `tests/`（与 `src/` 平级），修正页面描述；顺带确认 AGENTS.md 是否需要同步（AGENTS.md 未描述 tests 位置，无硬伤，可选）。

### I2. `ui/README.md` 测试小节补失败用例修复说明
- **证据**：F2/F3 修复后，README「测试」小节可补一句「测试位于 `ui/tests/`，与源码分离」。
- **动作**：随 I1 一并改，合并为一个 commit。

### I3. `vite.config.ts` 的 `/go` 代理注释与 UI 实际调用面核对
- **证据**：`/go` 代理（vite.config.ts:60，源自 ui-gofunction-plan §7.2）对应后端 `e.Group("/go/:projectID")`（router.go:407）；UI 侧唯一消费是 `GoFuncTestModal.vue:159` 的展示用 `invokeUrl`（拼给用户 curl 的示例地址），**浏览器不发 `/go` 请求**。
- **结论**：代理配置**保留**——dev 模式下用户手动 curl `invokeUrl` 时仍需要它转发；但注释「云函数调用面前缀」可补一句「UI 自身不请求此前缀，仅供 invokeUrl 手动调用」防止后人误判为死配置。**可选，低优先级**。

### I4. 工具链缺类型检查环节（v1.1 新增，流程性漂移）
- **证据**：
  - `package.json` scripts 只有 dev/build/preview/test/test:watch，**没有 typecheck**；
  - `yarn build`（vite/esbuild）不做类型检查，`yarn test`（vitest）也不做——导致 G5 的契约漂移（`update` 不在 Api 接口却进了实现）、G6 的 axios 类型错误、G7 的非法 ambient 声明**长期潜伏**；
  - 后端有 `go vet` 等价物，前端没有；`tsc --noEmit` 当前 386 个错误中剔除 ui 库与 `.vue` 模块声明噪声后 src 内真实错误 4 处、tests 内约 15 处（TS7006 隐式 any 等）。
- **动作**（需要用户确认，因涉及 package.json）：
  1. 修完 G5-G7 后，加 script `"typecheck": "vue-tsc --noEmit"`（vue-tsc 未安装，需加 devDependency）或 `"typecheck": "tsc --noEmit"`（零新依赖，但 .vue 文件不覆盖——当前 .vue 的 TS 错误本来就没被任何环节检查，先求有）；
  2. 噪声治理：`.vue` 模块声明缺失可通过在 tsconfig include `"src/**/*.vue"` + 一个全局 `env.d.ts` 的 `declare module '*.vue'` 解决（当前 tsconfig include 已含 `.vue` 但 tsc 不解析 SFC，需 vue-tsc 才彻底）。
- **保守建议**：本轮先落地「修 4 处 src 错误 + 加 `tsc --noEmit` script」，vue-tsc 引入单独立项。
- **验证**：`yarn typecheck` 退出码 0（src 范围）。

### I5. `stores/auth.ts` 死状态字段（v1.1 新增）
- **证据**：`loginReason`（state + openLogin 写入 + closeLogin 清空）与 `booting`（bootstrap 写入）在全仓**零读取**——LoginModal 从不展示 reason，无组件监听 booting；属「写了没人读」的死状态。
- **动作**：`loginReason` 二选一——(a) LoginModal 补一行 reason 展示（markUnauthorized 传的「登录态已失效，请重新登录」提示本来有意义，推荐补齐闭环）；(b) 删字段与写入。`booting` 直接删（或留作将来骨架屏，建议删）。
- **验证**：`yarn build && yarn test`（tests/auth.test.ts 若断言这两个字段需同步）。

---

## J. 退役候选（需决策，默认本轮不动）

### J1. `services/mock.js`（1182 行）+ `mock-kv.js`（943 行）双实现维护成本
- **现状**：`VITE_USE_MOCK` 三套 env 全默认 `false`（dev/prod 均走真实后端，仅 `.env.example` 演示 true）；后端 cleanup-plan E 组已将其标记为「防误删」，且 AGENTS.md 明文约定「新增接口 mock 与实现两边都要实现」——**每个新 API 都要维护三份**（types 契约 + http 实现 + mock）。
- **决策点**：若确认 mock 已无使用场景（本地开发都有真实后端，`./build.sh dev` 一键起全套），可整体退役 mock.js/mock-kv.js/mock.d.ts/api.ts 的切换逻辑（api.ts 固定导出 httpApi），新接口只维护两份。**收益**：2125 行 + 消除三份维护；**代价**：失去无后端演示模式。
- **建议**：本轮不动；在下一次新增大批 API 前决策。

### J2. 已删除组件约定（checkbox / drawer / dropdown-menu / pagination / radio-group）的回归检查机制
- **现状**：AGENTS.md 明令「已删除的零引用组件不得重新引入」；本次扫描确认 32 个 ui 子目录中确实无此 5 目录（grep 命中均为 tailwind class 字符串如 `[role=checkbox]`，属误报）。
- **建议**：无需动作；记录在案，防误删清单互引。

### J3. `tests/pages.test.ts` 等迁移后遗留的低价值测试文件
- **现状**：85 个测试文件中有若干「迁移拼盘」（如 `simple-components.test.ts`、`coverage-gaps.test.ts`、`low-branch-coverage.test.ts`、`pages.test.ts` 与 `pages-coverage.test.ts` 职责重叠），是历史上为凑 95% 覆盖率逐次追加的。
- **建议**：本轮不动（重排测试属于重构非清理）；若未来做，建议按「一个被测对象一个测试文件」合并，预计可减 10+ 个文件。

### J4. `stores/settings.ts` persist() 不写 `projectDefaults`（v1.1 新增，疑似 bug 而非清理）
- **现状**：`persist()`（settings.ts:76-85）只落盘 `theme` + `providerConfigs`，**不写 `projectDefaults`**；但 `setProjectDefaults`/`setDefaultProvider` 更新 state 后都调 `persist()`——用户设置的「默认供应商/模型」刷新即丢。
- **证据链**：load() 读 `parsed.projectDefaults`（39 行）说明存储结构预期含它；写入侧遗漏是唯一的解释。
- **决策点**：这是**功能 bug 修复**，超出清理范畴但同批发现；建议直接修（persist 对象补 `projectDefaults: this.projectDefaults`），并补一个「设置默认模型 → reload → 仍在」的测试。若暂不修，至少在 J 组记录。
- **风险**：修复后存量用户 localStorage 无此键，load 回退 `{}`，无兼容问题。

---

## K. 防误删清单（已核实为活跃代码）

| 项 | 证据 |
| --- | --- |
| `src/pages/Users.vue` | 静态 import 分析显示「零引用」是**误报**——router/index.ts:97 用 `() => import('../pages/Users.vue')` 动态注册（`meta.requiresRole: 'superadminl1'`） |
| `src/services/mock.js` + `mock-kv.js` + `mock.d.ts` | `VITE_USE_MOCK` 开关实现，AGENTS.md 约定；tests/mock.test.ts、mock-kv.test.ts 全量覆盖（退役决策见 J1，非本轮） |
| `src/components/ui/toggle/` | `Toggle` 组件零消费但 `toggleVariants` 被 `toggle-group/ToggleGroup.vue`、`ToggleGroupItem.vue` import——**index.ts 的 toggleVariants 导出必须保留** |
| `src/components/ui/dialog/Dialog.vue` 等 Dialog 基座 | SbModal（全部业务弹窗底座）在用；仅 H1 列出的 4 个衍生件零消费 |
| `src/components/ui/command/Command.vue` 基座 | GlobalProjectSwitcher、AiChatComposer 在用；仅 CommandDialog 等衍生件零消费 |
| `src/components/databases/kv/` 全目录（14 文件） | KvApiPanel 消费 kv-endpoints.ts；KvPanel 被 KeyValue.vue 消费；全部 KV 编辑器被 KvDetailSheet 动态消费 |
| `src/components/editor/GoMonacoEditor.vue` + `goMonarch.ts` | GoFunctionModal/GoFuncTestModal/CronJobModal 在用；goMonarch 是 Monaco Go 高亮注册 |
| `src/docs/catalog.ts` + `render.ts` | DocsWiki.vue 渲染仓库根 `docs/` 的数据源；`import.meta.glob('../../../docs/**/*.md')` 跨目录读取是设计使然（catalog.ts:20-27 有注释解释 alias 不可用） |
| `src/composables/useAiChat.ts` | AiChat.vue + AgentManager.vue 双消费 |
| `src/constants/llmProviders.ts` | SettingsPanel providers 分区静态 catalog（G2 删除动态拉取契约后此文件成为唯一厂商清单，地位反而提升） |
| `src/lib/status.ts`、`src/lib/utils.ts`、`src/utils/format.ts` | status 映射 6 页消费；`cn()` 被 ui 库全体 + AiChat 消费；format 7 页消费（4 个函数全活跃） |
| `src/lib/status.ts` 的 `logLevelTextMap` | 导出但外部零消费——**内部 `logLevelText()` 在用**，属「导出面过宽」非死代码；可降级为非导出（顺手项，非必须） |
| `src/stores/auth.ts` 全部 getters/actions | 逐成员核对：`canCreateProject`(4)、`updateKey`(4)、`applyTokens`(11)、`canViewUsers`(3)、`displayName`(52)、`mustChangePassword`(44) 等全活跃；唯二例外（loginReason/booting）见 I5 |
| `src/stores/project.ts` 全部成员 | `current`(9)、`remember`(4)、`history`、`loading`、`createModalOpen` 等逐项核对全活跃 |
| `src/stores/settings.ts` 全部 actions | `setTheme`(4)、`resolvedTheme`(4)、`setProjectDefaults`(3)、`upsertProviderConfig`(5)、`clearProviderCredentials`(3)、`setDefaultProvider`(4)、`isProviderConfigured`(7)、`defaultsFor`(9)、`configsFor`(5) 全活跃；`effectiveTheme` 外部零消费但 `applyThemeToDom` 内部在用；persist 缺陷见 J4 |
| `src/services/http.ts` 全部导出 | `baseURL`(6)、`getAccessToken`(13)、`getRefreshToken`(7)、`setTokens`(14)、`clearTokens`(15)、`getApiKey`(17)、`setApiKey`(6) 全活跃；`setUnauthorizedHandler` 仅 tests/http.test.ts 消费但它是 http→auth 解耦回调机制核心，保留 |
| `kv` API 面（`exec`/`execBatch`） | 业务消费在 KvPanel/KvApiPanel，契约-实现-双端一致；mock 侧经 `mock.js:817 kv: kvMockGroup` 合入 |
| 全部 npm dependencies（16 个） | G9 逐包核对，无一死依赖 |
| `monaco-editor` 深路径 import（`monaco-editor/editor.js`） | GoMonacoEditor.vue:19-20，非标准入口消费，浅 grep 计 0 属误判，实际活跃 |
| `src/test/helpers.ts` + `api-mock.ts` | tests/ 下 51 与 44 个测试文件分别消费 |
| `src/router/index.ts` 的 legacy redirect 块 | tests/router.test.ts 全量覆盖旧路径跳转；AGENTS.md 明令「路由 redirect 保持现状」 |
| `ui/dist/`（4.8MB） | `.gitignore:20` 已排除；build.sh 生成后同步 `internal/web/dist` 供 go:embed——**目录存在是正常的**，勿手工清理正在使用的构建产物 |
| vite.config.ts 的 `/v1`、`/health`、`/go` 三条 proxy | 均活跃（/go 详见 I3） |
| `packages/js-sdk` 与 ui 无关的引用 | http-api.ts 不 import js-sdk，两侧独立演进，无清理交叉点 |

---

## 执行顺序

1. **前置**：确认当前工作区未提交变更（tests 迁移 + KV 功能）已提交——本计划所有改动建立在干净基线上。
2. **F 组**（独立 commit，先修测试再动代码）：F1 → F2 → F3。修完 `yarn test` 必须全绿（395/395），这是后续所有项的回归基线。
3. **G 组**（每项独立 commit，顺序 G1 → G5 → G6 → G7 → G2 → G3 → G4 → G8）：
   - G5/G6/G7 是类型错误修复（先修后删，保证 `tsc` 可用作后续验证工具）；
   - 每项完成后 `yarn build && yarn test`；
   - G4 涉及 4 处测试改写，预留最多时间；不想动测试可先降级跳过；
   - G8 改 package.json 需单独 commit 并标注。
4. **H 组**（批量 commit，每批 ≤10 文件）：H3（sidebar 单批最大收益）→ H1（整文件清单）→ H2（剩余导出裁剪）。
   - 每批后 `yarn build && yarn test`；build 对缺失导出零容忍，是主要验证手段。
5. **I 组**：文档与流程改进放最后（I1+I2 合并一个 commit；I4 待用户确认后落地 typecheck；I5 二选一处理；I3 可选）。
6. **J 组**：逐项在 issue/讨论中决策，不在本轮执行（J4 是疑似功能 bug，建议提级为直接修复）。

## 验证命令汇总

```bash
cd ui
yarn build                                  # 每批改动后
yarn test                                   # F 组修复后必须 395/395
yarn test 2>&1 | grep -E "FAIL|×"           # 快速失败定位
npx vitest run tests/KeyValue.test.ts       # F2 定点验证
npx tsc --noEmit -p tsconfig.json 2>&1 | grep "^src/" # G5-G7 后 src 内应零真实错误
grep -rn "DatabaseListResult" src/          # G1 后应为空
grep -rn "llm.providers\|auth.refresh\|agentThreads.run" src/ --include="*.ts" | grep -v test   # G2-G4 后应为空
grep -rn "gofunctions.update\|\.update(" src/services/http-api.ts # G5 后应无 update 方法
grep -c "export" src/components/ui/*/index.ts   # H 组前后对比导出总数
grep -n "@types/dompurify\|@types/marked" package.json   # G8 后应为空
git ls-files src | grep -vE "\.(vue|ts|js|css)$" # 应始终为空（防产物误入库）
```

## 风险与回滚

- G/H 组全部是「零消费删除」或「类型错误修复」，构建期即验证（vite 对缺失模块零容忍），运行期风险极低；
- 唯一的动态盲区是 `import.meta.glob`（仅 src/docs/catalog.ts 一处，已核实不涉清理对象）与路由懒加载（仅 pages/，已核实全部活跃）；
- H 组删文件若遗漏 index.ts 导出行，build 立即报错——**先删导出行再删文件**更安全；
- G5 删 `gofunctions.update` 前需人工确认无外部脚本（如 e2e、curl 示例文档）依赖该前端路径——它只是 `saveVersion` 的旧签名糖衣，风险极低；
- J1（mock 退役）是唯一有功能取舍的项，强制延后决策；J4（persist 缺陷）修复属功能变更，建议与清理分开 commit；
- 每项独立 commit，按 commit 单独 revert；
- `package.json` 仅 G8（删 stub 类型包）与 I4（加 typecheck script，待确认）两处触及；`tsconfig.json`、`vite.config.ts` 不动（I3 仅改注释除外）、不动 coverage 阈值。

---

## 修订记录

- **v1.0**（2026-09-29）：初版，F×3 / G×4 / H×4 / I×3 / J×3 / K×16。
- **v1.1**（2026-09-29 下午，第二轮复核补充）：
  - 新增 **G5**：`gofunctions.update` 契约漂移——`Api` 接口无此方法但 http-api/mock 双端实现了它，`tsc --noEmit` 报 TS2353（当前工具链无类型检查环节所以潜伏至今）；业务零消费（编辑路径走 saveVersion），删除并迁移 5 处测试引用。
  - 新增 **G6**：`http.ts:51` axios headers 联合类型上的 `.includes` 调用编译错（TS2339），运行时侥幸正确。
  - 新增 **G7**：`vite-env.d.ts:29` 相对路径 ambient 声明非法（TS2436），改走已有 `@docs` alias。
  - 新增 **G8**：`@types/dompurify`、`@types/marked` 均为官方废弃的空壳 stub 包（库自带类型），从 devDependencies 删除。
  - 新增 **G9**：16 个 dependencies 逐包消费核对，全部活跃无死依赖（monaco 深路径 import 易误判，已记录）。
  - 新增 **I4**：工具链缺类型检查环节——build/test 均不跑 tsc，导致 G5-G7 长期潜伏；建议加 `typecheck` script。
  - 新增 **I5**：`stores/auth.ts` 死状态字段 `loginReason`（写入无读取，LoginModal 不展示）、`booting`。
  - 新增 **J4**：`stores/settings.ts` persist() 不落盘 `projectDefaults`——用户设置的默认供应商/模型刷新即丢，疑似功能 bug。
  - **K 组扩充**：stores 三件套逐成员消费核对全活跃（唯二例外归 I5）、`http.ts` 全导出活跃、`kv` API 面活跃、`logLevelTextMap` 属导出面过宽非死代码、monaco 深路径消费防误判。
- **v1.2**（2026-09-29 傍晚，**执行完毕**）：
  - F/G/H/I 全组 + J4 执行完毕，J1-J3 延后；执行明细见顶部「执行状态」表。
  - 最终验证：`yarn build` 通过、`yarn test` 85 文件 / 395 测试全绿（两轮稳定）、`yarn typecheck` src 零真实错误。
  - 覆盖率 93.67% funcs / 91.68% branches 低于 95% 阈值——经回溯验证为**清理前基线即如此**，非本次清理引入，阈值未动（后续如需达标应补测试或调整阈值，独立决策）。
  - 执行 commit 链（5579782 之后）：dcef853(F) → 425b818(G1) → 511f25b(G5) → 3c70291(G6/G7) → 218d91a(G2) → c1eb8ee(G3) → 053fafa(G4) → 952312b(G8) → 4b44364(H3) → 7e44832(H1) → f0f2c4e(H2) → 28a031f/be245d5(工作区恢复) → b2309df(I) → d2c0d8c(J4)。
