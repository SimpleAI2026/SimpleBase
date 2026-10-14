# 前端代码简化与清理计划（v6.0）

> 范围：`ui/src/`；不修改配置、依赖或后端。2026-10-13 已按 §6.1 推荐顺序执行第一轮，结果见 §7。
> 基线：`main` 对应 HEAD `5a03d47` + 2026-10-13 工作区未提交改动；实施前重新核对分支、源码与差异。
> 依据：[`code-simplifier`](../../skills/code-simplifier/code-simplifier.md)、根目录 `AGENTS.md`、`ui/AGENTS.md`；结构参考[后端简化计划](./backend-code-simplification-plan.md)。指定 skill 以仓库指导文件的形式读取、应用，不需要安装或调用额外服务。
> 证据口径：§2 候选表为静态阅读形成的候选；行号是编写计划时的定位锚点。实际执行与测试结果只以 §7 记录为准。

## 1. 目标与边界

降低阅读、修改和回归成本，保持可观察行为不变。优先明确分支、移除真实重复，再拆有完整业务意义的职责；不追求最少行数，不引入通用 CRUD 框架、事件总线或新状态管理方案。

### 1.1 必须保持

- API 路径、编码、请求字段、字段省略规则、默认值、错误文本、认证优先级、401 处理与返回契约。
- Vue 响应式对象身份、数组复制/原地修改方式、watch 的来源与调用时机、初始化顺序、pending/刷新/空态/错误态、toast 次数与先后、分页重置条件。
- 项目切换、助手切换、会话 URL query、旧响应校验、消息顺序、重试不重复追加 user 消息、工具卡匹配与确认交互。
- SSE 事件分发、续传游标、重连次数和延迟、取消与结束语义、计时器清理和外部回调顺序。
- 现有 DOM 语义、布局、键盘交互、插槽、props/emits、模板绑定以及确有调用方依赖的组件暴露字段。

### 1.2 项目硬边界

所有请求仍经 `services/api.ts` 的 `api`；页面不直接引入 axios 或 `http-api.ts`。跨层契约集中在 `services/types.ts`，对象形状用 `interface`，新代码不增加 `any`。纯类型收紧不顺便增加运行时校验。

项目只读走 project store 的 `isAdmin`；角色走 auth getters。保留系统项目只读 SQL 与查看数据入口。路由以当前 `ui/AGENTS.md` 为准：控制台入口落 `/console/agents`，旧路径 redirect 保留，标题使用 `meta.titleKey`，不能照搬根规范较早的 redirect 示例。

复用现有 `SbAsyncRegion`、`SbErrorState`、`SbEmptyState`、`SbTabsPanel`、`SbModal`、`ConfirmAction`、`TablePager`；不改 `components/ui/` fork 风格、不恢复已删除组件。保留页面宽度、顶栏顺序、`--page-pad` 与弹窗焦点环/窄屏布局。状态类匹配 reka 的真实 `data-state`；现存 `data-active` 不作为无效类批量删除。

组件外文案用 `@/locales` 的 `t()`；中文、英文 key 同构，未知后端错误文案不翻译。对话自有空态保持 `show-empty=false`，未建立资源仅按已登记错误码识别。

不安装依赖、不修改 `package.json`、锁文件、`tsconfig.json`、`vite.config.ts`、主题配置或后端；不启动服务、访问真实用户库、不执行数据库命令；不编辑 `internal/web/dist`。

### 1.3 skill 的适用解释

| 指定 skill 原则 | 前端落地 |
| --- | --- |
| §1 保持功能 | 先在旧入口补表征测试，再做一次等价变换；异常输入和副作用也属于行为。 |
| §2 项目标准 | 采用 Vue 3 / Pinia / 项目 API 与 i18n 约束，不套 React 组件模式；顶层具名 helper 优先 `function` 并写清返回类型，不批量改回调为具名函数或强加导入扩展名。 |
| §3 清晰度 | 优先展开多条件嵌套三元、整理同实体映射和完整表单职责，避免 dense one-liner。 |
| §4 平衡 | 已有三态、分页、表单和对话抽象保留；两行转发和外观相似但语义不同的代码不强抽。 |
| §5 范围 | 用户明确指定 `ui/src/`，覆盖“仅最近修改”的默认限制；实施仍一次一张任务卡，不全仓重排。 |

本计划不重复 v5 的[UI 优化计划](../planv5.0/ui-optimization-v3-plan.md)、[代码与样式优化计划](../planv5.0/code-and-style-optimization-plan.md)、[加载态计划](../planv5.0/console-loading-states-plan.md)，也不替代 v6 的助手交互、模块提及、CLI 与文档预览功能计划。重叠文件先核对当前源码；缺陷修复与纯简化分开交付。

## 2. 当前结构与优先级

静态文件规模（不含 `components/ui/`）：`http-api.ts` 1356 行、`types.ts` 940 行、`AgentManager.vue` 856 行、`Databases.vue` 525 行、`useAgentConversation.ts` 235 行。语言树约 1500 行不等于职责混乱，不因长度拆语言包。规模仅用于选阅读热点，不能证明应重构。

| ID / 风险 | 当前源码证据 | 候选动作 | 行为锁与测试入口 |
| --- | --- | --- | --- |
| A1 / 低 | `http-api.ts:351-373`，`toCronJobItem` 的 `scheduleKind` 嵌套三元 | 改为显式条件分支或一个局部结果变量；修正误写为 GoFunctionItem 的映射注释。 | `interval`、`once`、`cron`、未知/缺失/null 全部维持原回退 cron；`http-api.test.ts`。 |
| A2 / 低，条件实施 | `GoFunctions.vue:216-230`，三个入口重复设置模式、目标、打开状态 | 仅在确实更清楚时用同页 `openFunctionModal` 共用三步赋值；保留原三个模板入口。 | create 清目标，edit/view 仅传 name 而非整个 record；赋值先于打开；`GoFunctions.test.ts`、`GoFunctionModal.test.ts`。helper 比原实现更绕则保留重复。 |
| B1 / 中 | `http-api.ts:283-310,351-388,413-481`，多个 `Record<string, any>` 映射 | 每次选一个实体，明确 wire 输入类型与返回类型，减少 any；先选 CronJob，再考虑 metrics/log。不批量自动 camelize。 | 数字转换、truthy/nullish 回退、别名优先级、undefined 字段；`http-api.test.ts`。输入收紧不能偷偷改变 runtime coercion。 |
| B2 / 中 | `AgentManager.vue:666-734`，create/edit 初始化、模块默认工具、切换与保存成一组职责 | 先提纯 create/edit 表单构造；确认收益后独立提取助手表单 composable，页面保留列表和选中助手编排。 | 工具数组复制、编辑不覆盖工具、空名称校验、sandbox 校验、保存 loading/关闭/重载顺序；`AgentManager.test.ts`。 |
| B3 / 中，条件实施 | `GoFunctions.vue:201-208` 与 `CronJobs.vue:224-231` 完全相同的 list→赋值→page=1→非空返回；均已有 useLoadState/usePagination | 先审计引用；仅当明确降低总认知成本，才增加一个窄范围列表加载适配。禁止重造加载状态机。 | 首次/刷新/失败、分页重置仅成功发生、project getter 求值时机；两个页面测试与 `useLoadState.test.ts`、`usePagination.test.ts`。两处七行重复不足以单独证明需要抽象。 |
| B4 / 中，后置 | `Databases.vue:383-525` 已有 Collection/SQL/KV 子组件，剩余创建状态和校验仍在页面 | 独立提取数据库创建表单职责，而非重拆整个页面；先判断现有 29 行 create 是否值得搬移。 | 名称 trim/长度/控制字符、collection 默认、空 SQL→undefined、双重错误展示、关闭后重载；`Databases.test.ts`。保留 expanded/watch/reload 语义。 |
| C1 / 高 | `http-api.ts:557-717`，事件分发与请求/重连闭包同处 | 首选只隔离单个无 IO 的事件转换；网络、控制器、游标仍由原闭包持有。完整流模块拆分另议。 | run header 与 run 帧回调、lastId、error 抑制 end、1/2/4 秒重连、after 编码、close abort；`agent-skills-stream.test.ts`、`http-api.test.ts`。 |
| C2 / 高 | `useAgentConversation.ts:99-174`，多个工具事件查卡/复制逻辑 | 仅提取可证明一致的 call_id 查找；无 ID 的 start/result 回退逻辑保持分离，不做万能 upsert。 | start 找最后同名；result 找最后同名且无 content；delta 仅累积 generating；状态先更新再外部回调；`useAgentConversation.test.ts`、`useAgentConversation-edge.test.ts`。 |
| C3 / 高 | `AgentManager.vue:549-643,755-845`，会话选择、创建、query 与发送/重试互相依赖 | 先画状态图，再选一个连续会话加载阶段提取；不一次拆整页，不把刷新与加载错误策略统一。 | project/agent 旧响应校验、page 失败回退 list、空列表创建、query 优先、停止顺序、mention 切换提示与 retry；`AgentManager.test.ts`、`agent-welcome-confirm.test.ts`、对话 composable 测试。 |

所有候选初始状态为“待核对”；低风险也不自动获得实施授权。C 类默认只分析和补测试，由用户选定子项后再修改生产逻辑。

## 3. 不强行统一与已存在的抽象

- `useLoadState`、`usePagination` 已存在；不要把 pages 普遍改成全能 `useResource`。数据库页虽也用 `usePagination`，但 `doLoad` 成功后不手动重置 `page`，且有展开行 watch，不能与 GoFunctions/CronJobs 不加区别共用。
- CronJobs 的 toggle 原地 `Object.assign`，trigger 打开记录，delete 重载列表，各自 pending Set 与 toast 不同，不套统一异步 CRUD。
- `llmStream` 与 `streamAgentRun` 不能仅因都读 SSE 就合并；Agent 有 run header、游标、续传、确认事件与独立结束策略。
- Agent wire/UI 类型当前保留若干 snake_case；改成 camelCase 是跨层契约变更，另立计划，不混入 B1。
- `cleanup()` 清计时器并置空连接，不等于 `stop()` 的 close/cancel；不能合成统一收尾函数后改变 cancel 请求或 callback 顺序。
- `Databases` 的 activeDb/activeCollection 被多个弹窗共享，不用通用“弹窗 registry”替代；SQL 只读入口不得因统一写权限被隐藏。
- `useCronJobForm`、`SqlResultPanel`、`ModelDefaultsPanel` 已完成职责拆分，不恢复旧 SettingsPanel 转发壳，不凭旧计划重复抽取。
- Monaco、ECharts、Markdown 安全渲染的生命周期与性能优化暂不列为实施候选：本次未完整审计这些模块。无测量不宣称更快、bundle 更小。
- 原生下拉、状态样式缺陷、竞态或 i18n 修复单列行为修复任务；若项目规范要求触及遗留组件时迁移，拆成独立修复批次，不称纯等价简化。

## 4. 实施协议与任务卡

### P0：每项重新建立安全网

1. 确认 HEAD，读取根与 ui 规范；搜索函数、类型、模板和测试调用方，不依赖本文行号。
2. 核对允许文件的 `git status --short`、`git diff`、`git diff --cached`。当前已有大量修改和新文件，不覆盖、不回退、不自动 commit，不以 HEAD 文件替换工作区。
3. 确认目标是否已被并行工作完成；已存在记证据，重复不做。发现同一函数并行修改则暂停。
4. 阅读测试及 `src/test/setup.ts`、相关 mocks；只用本地 mock，不启动后端、不连接真实 API。确认本机已有依赖与命令可用，不自动安装。
5. 运行本项旧实现测试，记录退出码。组件交互不桩掉 Switch/Tabs 等真实语义；源码规则测试不能替代点击与事件断言。
6. 写输入→状态/副作用→输出的表征清单；旧实现不满足新增断言时先审计断言，若是旧缺陷则转修复任务。

固定顺序：定位 → 旧实现表征通过 → 单次变换 → 差异复核 → 本项测试 → 交接并停止。状态用“待核对 / 待表征 / 可实施 / 已验证 / 已存在 / 暂缓 / 阻塞”。

### A1：首批推荐

允许仅修改 `http-api.ts` 的 `toCronJobItem` 与 `ui/tests/http-api.test.ts`。`CronJobs.test.ts`、`CronJobModal.test.ts`、`split-components.test.ts`、`loading-surfaces.test.ts` 也引用 `scheduleKind`，只读核对并纳入回归，不修改。

1. 查 cronjobs list/get/create/update 实际如何使用映射；测试从 `api.cronjobs` 原入口断言结果，不只测新 helper。
2. 覆盖 interval、once、cron、未知字符串、空值；保持原 scheduleKind fallback 及所有其他字段。
3. 用显式分支替换一个嵌套三元，优先局部变量，避免只为三值映射新增通用函数。
4. 对照 `||`、`??`、`!= null` 不变；仅修正对应误导注释，不重排整个文件。
5. 运行本项测试、检查实际差异后停止。

### A2：可选而非必做

只允许 `GoFunctions.vue` 与对应页面/弹窗测试。表征三个实际入口：mode、target、打开顺序、重复打开与 create 清空上次目标。若 helper 需要新增通用对象配置、分支或暴露更多状态，保持原实现。保留模板 handler 名，不把 test/version 弹窗一起纳入。

### B1：一个实体一张子卡

`B1-cron` 必须在 A1 已验证或明确暂缓后单独开始，不能与 A1 同批改同一函数。先确定实际输入契约；所需共享 wire interface 放 `types.ts`，私有类型不扩成全局 schema 框架。经原 API 入口测试缺失、null、零、false、空串、数字字符串和异常值的原转换；不得添加 trim、过滤、排序或深拷贝。

不能只将 any 改成断言以掩盖类型问题，也不能为通过类型检查把本来保留的 truthy raw 值统一 String 化。无法同时保证输入兼容与返回契约时暂缓，另立类型债务/协议修复任务。metrics/log 为后续独立卡，保留 snake/camel/Pascal 别名优先级。

### B2：先纯构造，再评估 composable

首子项仅处理 `openCreate/openEdit` 的表单构造，字段列表与数组复制不变；不移动 save IO。第二子项才考虑完整表单 composable，须先覆盖模块选择、取消重开、create/patch 成功失败、saving finally、列表重载失败及选中助手变化。

`AgentManager.test.ts` 当前直接读写 `vm.form.*`，并整体赋值 `vm.form = {...}`；因此 `form` 必须仍是页面实例上可读写、可整体替换的 ref，`editing`、`saving`、`modalOpen` 的访问方式也先检索测试后再定。公开字段名和模板事件保持；composable 组件外用全局 t。若需要把路由、会话、流连接也传入表单层，说明边界失效，停止提取。

### B3 / B4：先证明收益

B3 只评估两个已核实页面；列出减少的重复与新增接口、间接层、测试成本，收益不足则交付“暂缓”，不为了 DRY 必须新增文件。helper 不接管 watch、toast 或 useLoadState 并发协议。

B4 只选创建表单阶段，禁止同时搬移展开行、集合重载、SQL/KV 弹窗。`Databases.test.ts` 当前直接使用 `vm.newName`、`vm.create()`、`vm.createVisible`；提取后须保持这些实例访问兼容，或把测试接口调整另立任务，不能为迁就重构放宽断言。

### C1 / C2 / C3：高风险默认零生产修改

- C1 提供事件/请求轨迹：header run、run、token、tool、confirmation、usage、error/end、断流续传、abort。未知事件、分块边界、JSON 失败与无 body 也表征。`readSse` 当前只处理以 `\n\n` 结束的帧、流结束时丢弃残余缓冲、静默跳过非 JSON 帧、把 `id:` 写入 `_sse_id`；这些都按现状锁定。重连等待每次都注册一次性 abort 监听且计时完成后不移除，也只记录不清理。不能擅自重写 parser 或修改监听器，这些属于行为修复。
- C2 对同名多工具、缺失 call_id、重复 delta、已运行卡收到 call、空结果、乱序/确认事件建立测试。仅共享完全等价查找，保留 reactive reply 及数组重新赋值触发更新；内部更新仍先于调用方 callback。
- C3 先列 projectId/activeId/threadId/threads/query/draft/switchTarget/conn 状态转换。受控 Promise 验证异步旧响应；不能用 sleep 猜完成。不要统一 `loadThreads` 的 page→list fallback 与 `refreshThreadsForAgent` 的 toast 策略。

新增 run generation、忽略迟到事件、取消去重、并发请求取消、修改重连策略均是行为变更，发现需要时另立缺陷任务，不借简化实现。

## 5. 验证矩阵（实施时执行）

在 `ui/` 执行，下列定向测试不带 coverage，只用于快速回归，不替代全量覆盖率门：

```sh
# A1 / B1
yarn vitest run tests/http-api.test.ts tests/http-api-kv.test.ts tests/http-api-auth.test.ts tests/CronJobs.test.ts tests/CronJobModal.test.ts tests/split-components.test.ts tests/loading-surfaces.test.ts
# A2 / B3
yarn vitest run tests/GoFunctions.test.ts tests/GoFunctionModal.test.ts tests/CronJobs.test.ts tests/useLoadState.test.ts tests/usePagination.test.ts
# B2 / C3
yarn vitest run tests/AgentManager.test.ts tests/agent-welcome-confirm.test.ts tests/useAgentConversation.test.ts tests/useAgentConversation-edge.test.ts
# B4
yarn vitest run tests/Databases.test.ts tests/CollectionPanel.test.ts tests/SqlWorkModal.test.ts
# C1 / C2
yarn vitest run tests/agent-skills-stream.test.ts tests/http-api.test.ts tests/useAgentConversation.test.ts tests/useAgentConversation-edge.test.ts
# 所选阶段完成后
yarn build
yarn test
yarn typecheck   # 记录实际结果；已知基线可能失败，不得为通过而改配置
```

仓库根目录针对实际改动文件运行 `git diff --check -- <文件...>`；新文档/未跟踪文件还需单独做空白与链接检查。编辑器诊断仅检查实际编辑的 TS/Vue 文件。

全量 `yarn test` 的 statements/lines/functions/branches 四项阈值均为 95%（已核对当前 vite 配置），不得降低阈值、扩大排除列表或新增 `v8 ignore` 来过门；`http-api.ts` 已有 `v8 ignore` 区段，搬移映射时不得扩大其覆盖范围。新拆组件/composable 会计入覆盖率。

项目规范记载历史 functions 94.24% 与 typecheck 的 *.vue/TS2307 问题，**不是本次测量结果**；实施必须重新运行并记录实际基线。已有失败与新增回归分开报告，不能宣称所有命令通过。修复类型基线若需改配置，先另立授权任务，不自动修改。

布局/交互涉及的批次加跑 `ui-state-attributes`、`modal-input-layout`、`template-event-wiring`、`i18n`、`router` 测试；测试已存在不代表覆盖完整，先读断言。视觉验收检查窄屏、焦点、只读、首条消息与空态；纯 helper 变更不要求每项启动完整应用。

本次仅写 Markdown：做文档链接/编号/候选证据一致性检查即可，不生成 build/coverage，不把文档验收当作前端代码验证。

## 6. 交付与停止条件

### 6.1 推荐顺序

`P0 → A1 → A2收益评估 → B1-cron → B2纯表单构造`；B3/B4 以收益和安全网决定，不强制执行。C 类后置且需要具体子项选择。一次会话只交付一个子项，不默认推进整计划。

### 6.2 完成定义

- 被实施候选有旧入口表征、旧/新差异复核、真实命令和退出码；减少的是重复/认知负担而非单纯行数。
- 权限、路由、契约、错误、状态/事件/资源时序与响应式更新不变；无依赖或配置修改，无真实数据访问。
- 本项回归通过，阶段 build/test 通过；环境阻塞和历史失败明确标记。只做静态阅读不能记“已验证”。

### 6.3 立即停止当前子项

需要改变 props/emits、API 字段、snake/camel 契约、默认值、权限、错误、异步时序或引入运行时校验；旧实现表征失败且确为缺陷；发现目标函数并行修改；测试需要真实服务/凭据或安装依赖；失败定位超出允许范围。

不得删测试、放宽断言、批量重排、restore/reset/stash 用户修改。无法证明等价时保留旧复杂度，记录“暂缓”是有效交付。

### 6.4 交接模板

```text
任务 ID / 子项：
状态：待核对 / 待表征 / 可实施 / 已验证 / 已存在 / 暂缓 / 阻塞
基线：HEAD 与允许文件已有差异
允许文件 / 实际修改文件：
源码证据：函数、模板入口、调用方
旧入口表征：真实测试名、旧实现结果
等价复核：输入/默认/错误/对象身份/顺序/次数/清理
验证：完整命令、退出码；失败或未运行原因
减少的复杂度 / 保留的必要复杂度：
下一步：仅建议，不自动继续
```

> 下一模型任务指令：阅读本计划边界、P0 和 `<ID/子项>`，本次只执行该子项。先核对工作区差异和旧入口测试，通过后才做允许的单次等价变换；不修改配置、不安装依赖、不连接运行服务、不改变权限/契约/错误/状态和资源时序。证据不足按停点暂缓，交接真实结果后停止。

## 7. 执行记录（2026-10-13 第一轮）

用户要求按推荐顺序逐步执行，因此本轮在同一会话内连续完成多个子项；每项仍单独做旧实现表征 → 单次变换 → 回归，结论如下。

### 7.1 状态总表

| ID | 状态 | 实际修改 | 说明 |
| --- | --- | --- | --- |
| P0 | 已验证 | — | HEAD `5a03d47`（main）。允许文件 `http-api.ts`、`types.ts`、`AgentManager.vue`、`GoFunctions.vue` 及对应测试均已有未提交改动，本轮仅在其上做增量编辑，未 restore/reset。旧实现定向基线：11 个测试文件 / 129 用例通过。 |
| A1 | 已验证 | `services/http-api.ts`、`tests/http-api.test.ts` | 新增私有 `toCronScheduleKind(raw: unknown)`，以 if 分支替代 `toCronJobItem` 的嵌套三元；修正被误写为 GoFunctionItem 的注释。表征覆盖 interval / once / cron / `Interval` / unknown / null / 缺失，旧实现先通过。 |
| A2 | 暂缓 | — | 三个入口各 3 行赋值，差异在 mode 与 target（create 清空、edit/view 只传 name）。共用 helper 需传 mode + 可选 target，读起来不比现状清楚，按 §4 A2 停点保留重复。 |
| B1-cron | 已验证 | `services/types.ts`、`services/http-api.ts`、`tests/http-api.test.ts` | 新增 `CronJobWire` / `CronJobRunWire` interface（对照 `internal/api/cronjob_handler.go` 的 DTO），替换 `toCronJobItem` / `toCronJobRunItem` 及两处 list 回调的 `Record<string, any>`。经 String/Number/Boolean 兜底的字段标 `unknown`，原样透传的时间字段标 `string \| null`，运行时转换零改动。`http-api.ts` 中 `any` 出现次数 37 → 33。表征覆盖 0 / null / 空串 / 数字字符串 / 非法 trigger。 |
| B2（纯表单构造） | 已验证 | `pages/AgentManager.vue`、`tests/AgentManager.test.ts` | 提取 `createAgentForm` / `editAgentForm`，返回复用 `AgentFormModal.vue` 已导出的 `AgentFormState`；`openCreate` / `openEdit` 仍按“editing → form → modalOpen”顺序赋值。`form` 仍是页面实例上可整体替换的 ref。表征断言字段、默认值、数组复制（不回写 modules / agent）、无模块回退与 undefined 字段。 |
| B2（完整 composable） | 暂缓 | — | 纯构造已解决主要重复；`saveAgent` 依赖 `loadAgents` / `activeId`，且测试直接读写 `vm.form` / `vm.editing`，再抽 composable 收益不足以覆盖接口与测试兼容成本。 |
| B3 | 暂缓 | — | GoFunctions / CronJobs 各 7 行同构 `load()`，已复用 `useLoadState` + `usePagination`；新增适配层会多一层间接且覆盖率计入新函数，收益不足。 |
| B4 | 暂缓 | — | `Databases.vue` 的 `create` 仅 29 行，且测试直接使用 `vm.newName` / `vm.create()` / `vm.createVisible`；提取后为保持兼容需原样转发，收益不足。 |
| C1–C3 | 已验证（第三轮） | `services/http-api.ts`、`services/types.ts`、`composables/useAgentConversation.ts`、`pages/AgentManager.vue` 及对应测试 | 用户明确选定后执行，各项仍先旧实现表征再等价提取（见 7.4）。 |
| B1-metrics（第二轮） | 已验证 | `services/types.ts`、`services/http-api.ts`、`tests/http-api.test.ts` | 新增 `MetricsSummaryWire` / `TrendPointWire`；`toMetricsSummary`、`toTrendPoints` 入参改为 `unknown`，内部以 wire 类型读取。`toTrendPoints` 的嵌套三元 + `as any` 拆为私有 `trendPointList`（裸数组 / `{points}` / 其他→空）。snake 优先 camel、`??` 只跳过 null/undefined、`optionalNumber` 空串/非法值→null、非对象回退默认（含 overflow 30000）均已在旧实现表征后保持。 |
| B1-log（第二轮） | 已验证 | 同上 | 新增 `LogEventWire` / `LogRetentionWire`（snake / camel / Pascal 三套别名，优先级不变）；`fields_json` / `request_id` 仍原样透传、null 归一 undefined。表征覆盖 `0`/空串不被后续别名覆盖、null 跳到下一别名、null 元素、非对象保留默认 14 天。 |

### 7.2 验证结果

| 命令（`ui/` 下） | 退出码 | 结果 |
| --- | --- | --- |
| A1 / B1 定向：`yarn vitest run tests/http-api.test.ts tests/http-api-kv.test.ts tests/http-api-auth.test.ts tests/CronJobs.test.ts tests/CronJobModal.test.ts tests/split-components.test.ts tests/loading-surfaces.test.ts` | 0 | 7 文件 / 72 用例通过（改前、改后各跑一次） |
| B2 定向：`yarn vitest run tests/AgentManager.test.ts tests/agent-welcome-confirm.test.ts tests/useAgentConversation.test.ts tests/useAgentConversation-edge.test.ts` | 0 | 61 用例通过；新增用例先在旧实现通过 |
| `yarn build` | 0 | 通过；仅有既有 chunk 体积提示 |
| `yarn test`（含 coverage 门） | 0 | 107 文件 / 729 用例通过；Statements 99.51% / Branches 95.18% / Functions 95.12% / Lines 99.51%，四项均达 95% |
| `yarn typecheck` | 2 | 4 处错误，均在 `tests/`，`src/` 为 0：3 处 TS2339（`agent-components`、`agent-split-components`、`useAgentConversation` 测试的 `.exists`），1 处 TS2345（`http-api.test.ts` 的 `agentThreads.page(pid, 1)`，HEAD 版本已有同一行）。均非本轮引入，按计划不改配置或顺手修复 |
| `git diff --check -- <本轮 5 个文件>` | 0 | 无空白问题 |

`ui/AGENTS.md` 记载的 functions 94.24% 与 241 处 TS2307 是历史基线，本轮实测已不同（见上表），留给维护者更新规范，本轮不改规范文件。

**第二轮（B1-metrics / B1-log）验证**：

| 命令（`ui/` 下） | 退出码 | 结果 |
| --- | --- | --- |
| `yarn vitest run tests/http-api.test.ts tests/Logs.test.ts tests/Dashboard.test.ts tests/TrendChart.test.ts` | 0 | 37 用例通过；新增表征用例均先在旧实现通过 |
| `yarn build` | 0 | 通过 |
| `yarn test` | 0 | 729 用例通过；Statements 99.52% / Branches 95.21% / Functions 95.12% / Lines 99.52%；`http-api.ts` 行 100% / 分支 96.46% |
| `yarn tsc --noEmit` | 2 | 仍为同 4 处既有测试错误（`http-api.test.ts` 的 `page(pid, 1)` 因前方新增用例行号移到 532），`src/` 为 0 |
| `git diff --check -- <3 个改动文件>` | 0 | 无空白问题 |

`http-api.ts` 的 `any` 出现次数累计 37 → 27（本轮 33 → 27）。

### 7.4 第三轮（C2 / C1 / C3）执行记录

用户明确选定 C 类后执行；顺序 C2 → C1 → C3，每项先在旧实现跑表征再等价提取，网络闭包、reactive 语义与回调时序零改动。

| ID | 状态 | 实际修改 | 关键结论 |
| --- | --- | --- | --- |
| C2 | 已验证 | `composables/useAgentConversation.ts`、`tests/useAgentConversation-edge.test.ts` | 新增模块级私有 `findToolCard(cards, callId, fallback?)`：有 call_id 精确匹配，无 id 且给了回退条件时取逆序首个满足者；delta/call 不带回退，start 按 `name`、result 按 `name && !content` 回退，confirmation 保持原样精确匹配。新增 6 个表征用例锁定：delta 仅在 generating/无状态时累积且数组引用不变、call 替换参数并只在 generating/空状态升 pending、无 id 的 call/start/result 各自追加、result 无匹配时 push、确认事件不落卡仍进 awaiting_confirm、回复数组重新赋值先于外部回调。87 用例定向回归通过。 |
| C1 | 已验证 | `services/http-api.ts`、`services/types.ts`、`tests/http-api.test.ts` | 仅提取无 IO 的 usage 帧 → 指标转换：新增 `AgentUsageEventWire` 与私有 `toAgentRunMetrics`，`|| 0` 兜底语义原样保留（非数字字符串仍得 NaN，已按现状表征并注释）。新增稀疏事件表征用例锁定 12 类事件的回调实参（缺字段、空 call_id、`"true"`/数字真值、未知 type 忽略、`end.reason` 非字符串归 undefined 等）。事件分发、重连、abort 闭包未动，完整流模块拆分仍另议。 |
| C3 | 已验证 | `pages/AgentManager.vue`、`tests/AgentManager.test.ts` | 先列状态转换再只提取会话加载映射：新增页面内 `toChatMessage(m, agentModule)` 纯映射（role 归一、assistant 补 module、BUG-06 失败/取消态还原），`loadThreadMessages` 保留竞态守卫（`threadId !== id` 丢弃旧响应）。新增 2 个表征用例：三角色消息映射（system 归 user、error_code 走 fallback 文案——传普通对象非 Error，按现状锁定）与慢请求期间切线程后旧响应不覆盖。`loadThreads` 的 page→list 回退与 `refreshThreadsForAgent` toast 策略未统一，发送/重试链路未动。 |

第三轮验证（`ui/` 下）：

| 命令 | 退出码 | 结果 |
| --- | --- | --- |
| C2 定向：`yarn vitest run tests/useAgentConversation*.test.ts tests/AgentManager.test.ts tests/agent-welcome-confirm.test.ts tests/agent-toolcard-variants.test.ts tests/agent-components.test.ts` | 0 | 87 用例通过 |
| C1 定向：`yarn vitest run tests/http-api.test.ts` | 0 | 14 用例通过 |
| C3 定向：`yarn vitest run tests/AgentManager.test.ts` | 0 | 40 用例通过 |
| `yarn vitest run`（全量） | 0 | 107 文件 / 738 用例通过（较上轮 +9 表征用例） |
| `yarn build` | 0 | 通过；仅既有 chunk 体积提示 |
| `yarn tsc --noEmit -p .` | 0 于 `src/` | 同前 4 处既有测试错误，`src/` 为 0，本轮未新增 |


### 7.3 下一步建议

- B1-metrics / B1-log / C1 / C2 / C3 已完成。剩余同类候选（`toLlmSettings`、Agent 系列映射）因 Agent 类型保留 snake_case UI 契约（§3），需先单独评估，不默认继续。
- C 类已完成（见 7.4）；完整流模块拆分（C1 的进一步）与整页状态机重构（C3 的进一步）仍属另议范围。
- typecheck 的 4 处测试类型错误另立修复任务，不归入纯简化。
