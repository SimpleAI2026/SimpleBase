# SimpleBase v5.0 前后端缺陷清理与样式优化计划

## 原始需求

> 1. 更新 `/Volumes/my/github/SimpleBase/AGENTS.md` 和加载 `/Volumes/my/github/SimpleBase/AGENTS.md`
> 2. 分析 ui 和后端代码，包括一些 bug 和样式等，现在要对代码优化和样式优化，给出 plan 放到 `/Volumes/my/github/SimpleBase/plan/planv5.0`，先不实现代码

---

> 日期：2026-10-08
> 范围：`ui/src/`（组件库样式钩子、页面交互、API 层、测试与覆盖率）、`internal/api`、`internal/systemdb`、`internal/auth`、仓库卫生
> 状态：**待实施**（本文件只做分析，不含代码改动）
> 方法：静态通读 + 构建产物比对（`ui/dist/assets/*.css`）+ vitest 探针实测 + 基线命令实测 + 依赖源码核对（reka-ui 2.10.5 / tailwindcss v4）
> 编号：§2–§3 为功能性缺陷，§4–§5 为样式与结构优化，§6–§9 为工程卫生与流程，§10 为排期验收
> 前置：`AGENTS.md`、`ui/AGENTS.md` 已按本次审查结论更新（新增 reka-ui 状态样式约束、typecheck/覆盖率的真实现状、`planv4.1` 归属说明）

---

## 0. 总览

| # | 主题 | 影响 | 优先级 | 改动面 | 风险 |
| --- | --- | --- | --- | --- | --- |
| 2.1 | **`Switch` / `Tabs` 等的状态样式钩子与 reka-ui 实际属性不匹配** | 运行时开关「点了没反应」，无任何报错；定时任务启停、SQL 事务开关、日志自动刷新、Agent 定时启用全部失效 | **P0** | 13 个 `ui/` 组件 + 5 个调用点 + 测试桩 | 低（纯类名替换，但必须先修测试桩否则无法验证） |
| 2.2 | 登录失败 / 改密失败（401）被当成「API Key 失效」，连带弹出设置弹窗 | 用户输错一次密码，同时弹登录框 + 设置框，且 `logout` 每次都打开登录框 | **P0** | `services/http.ts`、`stores/auth.ts` | 低 |
| 2.3 | admin（只读角色）UI 只读判定缺失：S3、云 Agent、日志、云函数页仍显示写入口 | 只读管理员可以点上传/删除/新建 Agent，再由后端 403 兜底 → 体验与语义不一致 | **P1** | 5 个页面 | 低 |
| 2.4 | 前端全局覆盖率 functions 94.24% 未达 95% 阈值（`yarn test` 以非零退出） | 测试命令常态失败，脱离 CI 可用状态 | **P1** | 测试 | 低 |
| 3.1 | **KV `EXISTS key` 返回库内键总数，而非指定键是否存在** | 客户端（Redis 语义依赖方）会拿到错误计数；这是数据层语义错误不是显示问题 | **P0** | `internal/api/kv_commands.go` | 低（有现成仓库方法） |
| 3.2 | `agent_handler` / `agent_schedule_handler` / `s3_handler` 写路径未检查 `writable` | 违反 `internal/AGENTS.md`「写操作 handler 必须检查 writable」；单写实例被置只读后这些入口仍可写 | **P1** | 3 个 handler + 测试 | 中（涉及行为变更，需补用例） |
| 4.1 | 样式钩子统一（与 2.1 同根因的样式面） | 弹层进出场动画、Tabs 选中态、Switch 轨道/滑块颜色全部静止无过渡 | **P1** | 同上 13 个组件 | 低 |
| 4.2 | 云沙盒页原生 `<select>`、未配置提示里的字面 `\n` | 与全站 shadcn/reka 体系不一致；提示文案显示为 `\n` 而非换行 | **P2** | `pages/Sandboxes.vue` | 低 |
| 4.3 | 全局 `@import` Google Fonts 外链 | 离线/内网/严格 CSP 环境下首屏阻塞在外部请求，字体回退不一致 | **P2** | `ui/src/style.css` | 低 |
| 4.4 | 表格/空态/骨架/移动端的局部一致性 | 视觉细碎问题 | **P2** | 若干页面 | 低 |
| 5.1 | 巨型文件（1426 行 `kv_commands.go`、1171 行 `http-api.ts`、1020 行 `app.go`） | 可读性与评审成本 | **P2** | 4 个文件 | 中（纯搬移，需保持导出面） |
| 5.2 | 死代码与未使用导入 | 噪声、误导后来者 | **P2** | 6 处 | 低 |
| 5.3 | SSE 直连 `fetch` 绕过 axios 401 刷新；`load()` 里 `Promise.allSettled` 后未处理的 rejection | 会话过期时流式请求直接报错；后台未捕获 Promise 告警 | **P2** | `services/http-api.ts`、`pages/Logs.vue` | 中 |
| 8.1 | `internal/` 37 个文件未过 `gofmt` | 格式漂移 | **P2** | 37 文件 | 低（建议按文件顺手格式化，不做一次性全仓重排） |
| 8.3 | 75 处 `NewHTTPError(status, "裸字符串")` 绕过领域错误 | 与「错误必须走领域错误」的约定冲突 | **P3** | `internal/api/*.go` | 低 |
| 9.1 | v4.1 成果（`scripts/`、`internal/testutil/`、`ui/src/components/agent/`、`useAgentConversation.ts`、e2e 测试）仍未跟踪 | 未提交代码只存在于工作区 | **P1（流程）** | 版本控制 | — |
| 9.2 | `fakellm-server`（8.5MB 构建产物）未被 `.gitignore` 覆盖 | 一次 `git add -A` 就会提交二进制 | **P2（流程）** | `.gitignore` | — |

**建议执行顺序**：`9.1`（先提交既有成果，避免后续改动混入未跟踪文件）→ `2.1`+`2.4`+`4.1`（同一根因、同一批文件，一起改一起测）→ `3.1` → `2.2` → `2.3`+`3.2`（权限与只读语义同一批）→ `4.2/4.3/4.4` → `5.x` → `8.x`。

---

## 1. 审查方法与基线

### 1.1 方法

1. **静态通读**：`ui/src/**`（pages / components / services / stores / composables）与 `internal/api`、`internal/systemdb`、`internal/auth`。
2. **构建产物比对**：`ui/dist/assets/index-*.css` 里 Tailwind 编译出的选择器 vs. 依赖源码里 reka-ui 真正渲染的属性——这是 §2.1 结论的直接证据。
3. **探针实测**：临时 vitest 用例挂载真实 `Switch` / `TabsTrigger`，打印 DOM 属性与 `update:*` 事件（用后即删，未留在仓库）。
4. **依赖源码核对**：`ui/node_modules/reka-ui/dist/**` 确认哪些 `data-*` 会被渲染。
5. **基线命令实测**：`go build`、`go vet`、`go test ./internal/... -short`、`yarn test`、`yarn typecheck`、`gofmt -l`。

### 1.2 实测基线（改动前必须复现）

| 命令 | 结果 |
| --- | --- |
| `go build ./internal/... ./cmd/...` | ✅ 通过 |
| `go vet ./internal/...` | ✅ 无输出 |
| `go test ./internal/api ./internal/cloudagent ./internal/systemdb ./internal/llmgateway -short` | ✅ 全部 `ok`（api 21.7s / cloudagent 0.8s / systemdb 2.6s / llmgateway 0.7s） |
| `cd ui && yarn build` | ✅ 通过（`dist/` 内 `DocsWiki` 与 `GoFunctions` 各自有独立 CSS chunk） |
| `cd ui && yarn test` | ⚠️ **阈值失败**：statements 99.24%、branches 95.07%、**functions 94.24%（< 95%）**，退出码非零 |
| `cd ui && yarn typecheck` | ❌ **262 处 TS 错误**（241 × TS2307 + 21 处其他），几乎全部在 `tests/**` |
| `gofmt -l internal cmd gofunction packages` | ⚠️ **37 个文件**待格式化，全部集中在 `internal/` |
| `git status` | 32 个改动文件 + 9 项未跟踪（见 §9） |

### 1.3 结论置信度说明

本文所有 P0/P1 结论都有「文件:行 + 可复现命令或依赖源码」两级证据。少数只能靠推理判断的项集中在**附录 A**，标注为「待核实」，不做为实施依据。

---

## 2. 前端功能性缺陷

### 2.1 【P0】状态样式钩子与 reka-ui 实际属性不匹配（Switch / Tabs / 弹层）

#### 现象

- `CronJobs.vue` 的「启用」开关点击后**视觉不动**，但接口确实被调用、`record.enabled` 也确实变了（重新加载后状态正确）。
- `SqlWorkModal.vue` 的「事务」开关、`Logs.vue` 的「自动刷新」开关、`AgentScheduleModal.vue` 的「启用」开关同样「点了没反应」。
- `DataTabs.vue` / `KvPanel.vue` 的页签选中态在浅色下几乎看不出差异。
- 所有弹窗（Dialog / Popover / Select / Sheet / Tooltip / AlertDialog / Combobox）**没有进出场动画**，且部分只有淡入没有淡出。

#### 根因

Tailwind 的裸 `data-*:` 变体会被编译成属性存在性选择器（与属性值无关）。本仓库的产物里可见：

```
.data-checked\:bg-primary   →  [data-checked]{background-color:var(--primary)}
.data-unchecked\:bg-input   →  [data-unchecked]{background-color:var(--input)}
.data-active\:bg-background →  [data-active]{background-color:var(--background)}
.data-open\:animate-in      →  [data-open]{animation:enter …}
.data-closed\:animate-out   →  [data-closed]{animation:exit …}
```

而锁定版本 reka-ui **2.10.5**（`ui/yarn.lock` → `reka-ui@npm:2.10.5`）渲染的是带**值**的 `data-state`，从不渲染裸属性。逐组件核对源码：

| reka 组件 | 实际渲染 | 行号 |
| --- | --- | --- |
| `SwitchRoot` | `data-state="checked" \| "unchecked"` | `dist/Switch/SwitchRoot.js:99` |
| `SwitchThumb` | `data-state="checked" \| "unchecked"` | `dist/Switch/SwitchThumb.js:25` |
| `TabsTrigger` | `data-state="active" \| "inactive"` + `data-orientation` | `dist/Tabs/TabsTrigger.js:53` |
| `DialogContentImpl` / `DialogOverlayImpl` | `data-state="open" \| "closed"` | `DialogContentImpl.js:86` / `DialogOverlayImpl.js:35` |
| `TooltipContentImpl` | `data-state`（`delayed-open` 等） | `TooltipContentImpl.js:123` |
| `SelectContentImpl` / `PopoverContent` | `data-state="open" \| "closed"` | 同上模式 |

**两侧对不上**，所以这些类永远不生效。全仓构建产物中 `data-state=checked|active|open` 出现次数为 **0**，可以确认没有任何补偿映射。

#### 影响面（全部需要改）

失效变体所在文件（`grep -rn "data-active:\|data-checked:\|data-unchecked:\|data-open:\|data-closed:" ui/src/components/ui`）：

| 文件 | 失效钩子 |
| --- | --- |
| `ui/switch/Switch.vue:33` | `data-checked:bg-primary`、`data-unchecked:bg-input`、`dark:data-unchecked:bg-input/80` |
| `ui/switch/Switch.vue:39` | `dark:data-unchecked:bg-foreground`、`dark:data-checked:bg-primary-foreground`、`group-data-[size=*]/switch:data-checked:translate-x-*`、`group-data-[size=*]/switch:data-unchecked:translate-x-0` |
| `ui/tabs/TabsTrigger.vue:19,20,21,22` | `group-data-[variant=default]/tabs-list:data-active:shadow-sm`、`data-active:bg-background`、`data-active:text-foreground`、`dark:data-active:*`、`group-data-[variant=line]/tabs-list:data-active:bg-transparent`、`…:data-active:after:opacity-100` |
| `ui/dialog/DialogContent.vue:37`、`DialogOverlay.vue:17` | `data-open:*` / `data-closed:*` |
| `ui/alert-dialog/AlertDialogContent.vue:37,45` | 同上 |
| `ui/popover/PopoverContent.vue:37` | 同上 |
| `ui/select/SelectContent.vue:39` | 同上 |
| `ui/sheet/SheetContent.vue:44`、`SheetOverlay.vue:16` | 同上 |
| `ui/tooltip/TooltipContent.vue:27` | `data-open:*`（但同一行的 `data-[state=delayed-open]` 写法是**对的**，可作为参照样板） |
| `ui/combobox/ComboboxList.vue:28`、`ComboboxItem.vue:20` | `data-open:*`（`data-highlighted:*` 是对的，reka 确实输出该属性） |
| `ui/field/FieldLabel.vue:15` | `has-data-checked:*`（父级从来看不到 `[data-checked]`） |

调用点（业务侧写法同样错误，需一并改）：

- `ui/src/pages/CronJobs.vue:116-120` — `<Switch :checked="record.enabled" @update:checked="toggleEnabled(record)" />`
- `ui/src/pages/Logs.vue:51` — `<Switch :checked="autoRefresh" @update:checked="autoRefresh = $event" />`
- `ui/src/components/modal/SqlWorkModal.vue:41` — `<Switch :checked="transactional" @update:checked="transactional = $event" />`
- `ui/src/components/ai/AgentScheduleModal.vue:38` — `<Switch :checked="form.enabled" @update:checked="…" />`

> `Switch` 组件**没有** `checked` prop，也**不 emit** `update:checked`（`dist/Switch/SwitchRoot.js:15-66`：props 是 `modelValue`/`defaultValue`/`trueValue`/`falseValue`，`emits: ["update:modelValue"]`）。所以调用点必须改为 `v-model` 或 `:model-value` + `@update:model-value`。探针实测：绑定 `:checked` 时点击前后属性分别是 `unchecked`→`checked`（内部状态在变，因为 `useVModel` 走了非受控分支），但 `update:checked` 一次都没触发，父组件的值永远不会被更新。

#### 修复方案

1. 把 13 个 `ui/` 组件里的裸属性变体全部换成 `data-[state=…]` 形式：
   - `data-checked:` → `data-[state=checked]:`
   - `data-unchecked:` → `data-[state=unchecked]:`
   - `data-active:` → `data-[state=active]:`
   - `data-open:` → `data-[state=open]:`
   - `data-closed:` → `data-[state=closed]:`
   - 带分组前缀的（`group-data-[size=default]/switch:data-checked:translate-x-…`）要写成 `group-data-[size=default]/switch:data-[state=checked]:translate-x-…`，`group-data-[variant=line]/tabs-list:data-active:*` 同理。
   - `has-data-checked:*` → `has-data-[state=checked]:*`（`FieldLabel.vue`）。
2. 调用点 4 处改为 reka 契约：`<Switch :model-value="x" @update:model-value="x = $event" />` 或 `v-model="x"`；`Switch` 组件依赖 `modelValue === trueValue` 判等，业务侧传 boolean 即可。
3. **先修测试桩**（见 §2.4 与 §7.3），否则改完样式也无法被现有测试发现。
4. 验收必须在**真实浏览器**里目视：开关轨道变蓝、滑块右移、弹窗有淡入淡出、Tabs 选中项有底色。`yarn build` 通过不代表样式生效（这类 bug 的隐蔽性正源于此）。

**为什么之前测试全绿**：`ui/src/test/helpers.ts:100-104` 的 `Switch` 桩声明的是 `props: ['checked'] / emits: ['update:checked']`——恰好是业务侧的**错误**用法。测试打的是桩，桩照着错误契约实现，于是「测试通过 + 线上失效」并存。

---

### 2.2 【P0】登录 / 改密失败被当作「凭据失效」，误弹设置弹窗

#### 现象

密码输错一次，会同时弹出登录框和设置弹窗（设置默认落在「连接」页），用户被要求去配 API Key。

#### 根因

`ui/src/services/http.ts:89-117` 的 401 拦截器**不区分请求类型**：

```ts
if (status === 401 && !cfg.__retried && getAccessToken()) { … tryRefresh() … }
if (status === 401) {                       // ← 登录/改密请求也会走到这
  import('../stores/auth').then(({ useAuthStore }) => { useAuthStore().markUnauthorized() })
  onUnauthorized?.()
}
```

而 `markUnauthorized()`（`stores/auth.ts:75-84`）会**同时**做两件事：

```ts
this.openLogin('登录态已失效，请重新登录')
this.openSettings({ tab: 'connection' })
```

于是「用户主动登录失败」与「token 过期」走了同一条路径。`stores/auth.ts:122` 的 `logout()` 末尾也调 `openLogin()`，用户点「退出登录」会被立刻弹回登录框（这可能是刻意的，但需确认产品意图）。

#### 修复方案

1. 在拦截器里加白名单跳过：`/v1/auth/login`、`/v1/auth/refresh`、`/v1/auth/change-password` 等匿名端点收到 401 时**不触发** `markUnauthorized()`/`onUnauthorized`，只把错误原样抛给调用方（`LoginModal` 已有 `Alert` 展示错误）。
2. `markUnauthorized()` 拆成两件事：`clearSession()`（清 token + 标记 `lastUnauthorizedAt`）与 `openLogin()`；设置弹窗只在「API Key 通道被拒」时打开，或干脆由用户从顶栏齿轮自行打开。
3. 确认 `logout()` 是否应主动弹登录框；若保留，需避免与「跳转登录页」的其它路径叠加。
4. 补测试：`http.ts` 单测断言登录 401 不调用 `markUnauthorized`；`auth.test.ts` 断言 `markUnauthorized` 只开一个弹窗。

---

### 2.3 【P1】admin（只读角色）前端只读判定缺失

`stores/project.ts:61` 的 `isAdmin` 只表示「当前是 admin/系统项目」，`stores/auth.ts:40` 的 `isAdminRole` 才表示「当前用户是只读管理员」，二者语义完全不同（`ui/AGENTS.md` 已明确）。

现状：只用 `isAdmin`（系统项目）判定，漏了 `isAdminRole`/`canWrite`：

| 页面 | 现状 | 问题 |
| --- | --- | --- |
| `pages/S3Manager.vue` | 完全没有只读判定 | 只读管理员看到「上传对象」「删除」 |
| `pages/AgentManager.vue` | 完全没有只读判定 | 只读管理员看到「新建/编辑/删除 Agent」、可发消息触发 LLM |
| `pages/Logs.vue` | 没有 | 「保留策略」可保存（后端是 `ProjectAdmin`，只读角色会被拒） |
| `pages/GoFunctions.vue` | 只用 `isAdminProject` | 只读管理员看到「新建/测试/编辑/删除」 |
| `pages/CronJobs.vue` | 只用 `isAdminProject` | 同上 + 开关可点 |
| `pages/Sandboxes.vue` | 用了 `auth.canWrite` | **正确样板**，其余页面照它改 |

后端权限映射（`internal/auth/role.go:43-61`）已保证 admin 只拿到 `DatabaseRead` + `LLMInvoke`，所以现在是「后端拦住、前端仍展示」，属于体验层不一致，不是越权。

#### 修复方案

1. 统一在页面里用 `authStore.canWrite`（未登录 API Key 通道保持可写）与 `projectStore.isAdmin`（系统项目）的**或**来判只读；建议抽一个 `useWritable()` composable 收敛判断，避免散落（同时满足 `ui/AGENTS.md`「不得散落比较 role 字符串」）。
2. 逐页处理：S3（隐藏上传/删除）、Agent（隐藏新建/编辑/删除/定时，composer 置禁用并提示）、Logs（保留策略只读展示）、GoFunctions（隐藏新建/测试/编辑/删除）、CronJobs（等同 GoFunctions，开关禁用）。
3. 不要改动 `isAdmin` 的既有语义（系统库展示仍依赖它）。
4. 补测试：每个页面加一条「只读角色下写入口不存在」的断言。

---

### 2.4 【P1】前端覆盖率与类型检查不达标（`yarn test` / `yarn typecheck` 常态失败）

#### 2.4.1 覆盖率：functions 94.24% < 95%

`ui/vite.config.ts` 的 `coverage.thresholds` 四项都是 95，实测（2026-10-08）：

```
Statements   : 99.24% ( 11114/11199 )
Branches     : 95.07% ( 4422/4651 )
Functions    : 94.24% (  802/851 )   ← 未达阈值
Lines        : 99.24% ( 11114/11199 )
ERROR: Coverage for functions (94.24%) does not meet global threshold (95%)
```

低于 100% 且函数数缺口明显的文件（按 `yarn test` 报告）：

| 文件 | 未覆盖函数 |
| --- | --- |
| `pages/KeyValue.vue` | 50%（`panel?.reload()` / `panel?.openCreate()` 两个内联回调，13、25 行） |
| `pages/GoFunctions.vue` | 85%（`openView` 在 94-95 行附近的分支） |
| `pages/CronJobs.vue` | 94.73% |
| `layouts/DefaultLayout.vue` | 83.33%（194-195 行） |
| `components/settings/SettingsPanel.vue` | 90%（311 行） |
| `components/modal/UserFormModal.vue` | 66.66%（122-128、146 行） |
| `components/docs/DocsArticle.vue` | 明细见报告 |

**方案（不建议调低阈值）**：把这些函数补上直接断言——多数只需渲染后触发一次交互即可覆盖（例如 `KeyValue.vue` 点「刷新/新建 Key」断言透传给 `KvPanel` 的调用）。补完再跑一次，确认四项全绿。若确有个别函数属于 `/* v8 ignore */` 场景，按现有 `MessageScroller.vue:33,40` 的写法显式豁免，并在注释里写清原因。

#### 2.4.2 类型检查：262 处错误

```
241 × TS2307  Cannot find module '@/components/…vue'
 12 × TS2339  Property does not exist
  2 × TS2352 / 2 × TS7006 / 1 × TS2755 …
```

**根因**：`ui/src/vite-env.d.ts` 只声明了 `*.md?raw`、`*.md`、`@docs/_meta.json` 三个模块，**没有 `*.vue` 的模块声明**；`tsconfig.json` 的 `include` 覆盖了 `tests/**/*.ts`，而测试大量直接 `import X from '@/components/…vue'`，于是 241 处 TS2307。剩下 21 处是测试里对 `wrapper.vm` 的属性访问（例如 `tests/simple-components.test.ts:115` 访问 `menuItems`，而 `NavMenu.vue` 的 `menuItems` 未 `defineExpose`）。

**方案**：

1. 在 `vite-env.d.ts` 补标准的 Vue SFC shim：

   ```ts
   declare module '*.vue' {
     import type { DefineComponent } from 'vue'
     const component: DefineComponent<{}, {}, any>
     export default component
   }
   ```

2. 剩下 21 处按情况处理：需要断言的内部状态用 `defineExpose` 暴露（`MessageScroller` 已有先例），或改成断言 DOM/emit。
3. 把 `yarn typecheck` 接入提交前自查（`AGENTS.md` 已写明现状）。

> 备选：把 `tests/**` 从 `tsconfig.json` 的 `include` 移出。**不推荐**——会失去测试代码的类型保护，且 `vite.config.ts` 在 `ui/AGENTS.md` 里属「不得擅改」清单。

---

## 3. 后端功能性缺陷

### 3.1 【P0】KV `EXISTS` 语义错误：返回全库键数

#### 证据

`internal/api/kv_commands.go:98-111`：

```go
"EXISTS": {
    parse: func(c echo.Context, argv []string) (func(tx *kv.Tx) (any, error), error) {
        if err := kvNeedAtLeast(c, argv, 1); err != nil {
            return nil, err
        }
        keys := append([]string(nil), argv...)
        return func(tx *kv.Tx) (any, error) {
            n, err := tx.Key().Count(kvCtx(c), "")   // ← pattern 为空 → Count 内部当成 "*"
            _ = keys // 精确计数在下方实现              // ← 注释说"在下方实现"，实际上没有
            return n, err
        }, nil
    },
    empty: func(ctx context.Context) (any, error) { return 0, nil },
},
```

`internal/database/kv/key.go:155-158`：

```go
func (r *KeyRepo) Count(ctx context.Context, pattern string) (int64, error) {
    if pattern == "" {
        pattern = "*"
    }
```

所以 `EXISTS a` 返回的是**库内键总数**，而不是 `a` 是否存在；`EXISTS a b c` 也只会返回同一个总数。Redis 语义要求返回「存在的键个数，最多等于参数个数」。

#### 影响

- 客户端按 Redis 语义使用 `EXISTS` 会得到错误结果（例如把 `EXISTS x` 的返回值当 0/1 判断时，只要库非空就恒为真）。
- `_ = keys` 是显式丢弃参数的写死实现，属于未完成代码遗留。

#### 旁证：现有测试**没有**覆盖这个语义

`internal/api/kv_handler_test.go:343-350` 的注释写着「DEL → 1；EXISTS → 0」，但**只断言了 DEL 的返回值**，EXISTS 从未被调用。所以测试全绿而语义是错的。

#### 修复方案

1. `internal/database/kv/key.go` 增加精确存在性方法，例如 `Exists(ctx, keys ...string) (int64, error)`——语义为「对每个 key 查 `kv.keys` 是否存在且未过期，累加计数」，复用 `getMetaAny`/`getMeta` 避免重复实现 TTL 判定。
2. `kv_commands.go` 的 `EXISTS` 改调该方法；删除 `_ = keys` 与误导性注释。
3. 补测试：`EXISTS a b` 在只存在 `a` 时返回 1；`EXISTS missing` 返回 0；含过期键时不计数；无 schema 空库走 `empty` 返回 0。
4. 顺手检查同文件里其它「近似实现」——`grep -n '_ = ' internal/api/kv_commands.go` 只命中这一处，但 `Count(kvCtx(c), "")` 还出现在 `kv_commands.go:285`（属于 `DBSIZE`，语义正确，无需改）。

---

### 3.2 【P1】`agent_handler` / `agent_schedule_handler` / `s3_handler` 写路径未检查 `writable`

`internal/AGENTS.md` 与根 `AGENTS.md` 都要求「写操作 handler 必须检查 `writable`（实例级只读返回 `ErrWriterUnavailable`）」。实测各 handler 中 `writable` 出现次数：

| handler | `writable` 命中 | 结论 |
| --- | --- | --- |
| `data_handler.go` | 7 | ✅ |
| `cronjob_handler.go` | 7 | ✅ |
| `gofunction_handler.go` | 10 | ✅ |
| `kv_handler.go` | 8 | ✅ |
| `database_handler.go` | 6 | ✅ |
| `sql_handler.go` | 7 | ✅ |
| `sandbox_handler.go` | 4 | ✅ |
| `agent_handler.go` | 3 | ⚠️ 只用在 `ListAgents`（`agent_handler.go:138`）与 `PatchThread`（`:346`），**未覆盖 `CreateAgent`/`PatchAgent`/`DeleteAgent`/`CreateThread`/`DeleteThread`/`CreateRun`** |
| `agent_schedule_handler.go` | 0 | ❌ 全部写接口无检查（`CreateSchedule` / `PatchSchedule` / `DeleteSchedule` / `TriggerScheduleRun`） |
| `s3_handler.go` | 0 | ❌ `UploadObject` / `DeleteObject` 无检查 |

路由侧（`internal/api/router.go:352-360`、`380-400`）这些接口都只挂了 `require(auth.DatabaseWrite)`，没有实例级只读保护。

> 注意区分：`auth.DatabaseWrite` 是**角色**权限（superadminl1/user 有、admin 无），`writable` 是**实例**开关（`config.instance.writable=false` 时整个实例只读）。前者已生效，缺的是后者。

#### 修复方案

1. 给 `cloudAgentHandler` 的写方法加 `if h.writable != nil && !*h.writable { return WriteError(c, ErrWriterUnavailable 对应错误) }` 前置检查（沿用 `PatchThread:346` 的现成写法）。
2. `agentScheduleHandler` 与 `S3Handler` 增加 `writable` 字段（构造处参照 `NewGoFunctionHandler(deps.System, deps.Config.Instance.Writable, deps.Audit)` 的传参风格），并在写方法前置检查；读方法保持不变。
3. `CreateRun` 属于「写消息 + 调 LLM」，是否纳入只读拦截需产品确认（建议纳入：只读实例不应产生新的 LLM 调用与落库）。
4. 补用例：为每条写路径加「`writable=false` → 期望错误码」的 handler 测试（现有 `cronjob_handler_test.go` / `gofunction_handler_test.go` 里有同类样板可照抄）。

---

## 4. 样式优化

### 4.1 【P1】样式钩子统一

与 §2.1 同一根因、同一批文件，实施时**一次改完**，不要拆成两个 commit。除属性名替换外，顺带确认这些视觉细节：

- `Switch` 轨道（`bg-primary` / `bg-input`）、滑块位移与颜色（`translate-x`）在明暗主题下都正确；
- `Switch` 缩略图尺寸类用的是 `group-data-[size=default]/switch`，需保证组名与 `data-size` 对上（`Switch.vue:33` 里 `data-[size=default]` 是有效写法，保留）；
- `TabsTrigger` 的 `group-data-[variant=line]/tabs-list:data-active:after:opacity-100` 下划线指示器要真的出现；
- 弹层动画（`animate-in` / `animate-out` / `slide-in-from-*`）依赖 `tw-animate-css`（已在 `style.css` 顶部 `@import`），替换后确认进出场都有。

### 4.2 【P2】`Sandboxes.vue` 尾部粗糙

`ui/src/pages/Sandboxes.vue` 的完成度明显低于其它页面：

1. **原生 `<select>`**（21-22 行）— 与全站 reka `Select` 不一致，且未使用 `Select` / `SelectTrigger`。建议换成 `ui/select` 组件，或统一用 `ToggleGroup` 做状态/来源过滤。
2. **字面 `\n`**（16 行）— `<pre>` 里写的是 `sandbox:\n  enabled: true\n…`，模板里 `\n` 不会被解释成换行，用户看到的是字面反斜杠 n。应改用多行文本（真换行）或 `<br>`。
3. **表格缺分页与列宽策略** — 其它列表页统一用 `TablePager` + `usePagination` + `sb-col-*` 列宽档位，这里既没有分页也没有用列宽工具类，列数到 8 时窄屏会挤成一团。
4. **操作列按钮无 icon、无 Tooltip** — 与 §4.4 的统一口径对齐。
5. **状态文案未本地化** — `{{ item.status }}` 直接输出 `running`/`stopped`，其它页面都走 `lib/status.ts` 的中文映射。建议在 `lib/status.ts` 增加 `sandboxStatusText`/`sandboxStatusVariant`。

### 4.3 【P2】全局字体外链

`ui/src/style.css:1`：

```css
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap');
```

问题：`@import` 必须在样式表最前，会**串行阻塞**首屏；离线/内网/严格 CSP 环境下 `--font-sans` 首选项拿不到，回退字体（PingFang SC 等）与设计稿不一致。控制台是内嵌在 Go 二进制里分发的，外链字体与「单文件交付」的定位也不搭。

**方案**（三选一，建议 b）：

- (a) 删除该行，`--font-sans` 直接用系统字体栈（当前已有完整回退链）；
- (b) 保留 Inter 但改为 `index.html` 里 `<link rel="preconnect">` + `<link rel="stylesheet">`，避免 CSS 串行阻塞；内网部署可再考虑自托管字体（新增静态资源，涉及构建产物体积，需评估）；
- (c) 保持现状，只在文档里声明「需要外网」。

### 4.4 【P2】局部一致性清单

| 位置 | 问题 | 建议 |
| --- | --- | --- |
| `GoFunctions.vue:154-159,175` | 导入了 `CopyIcon` / `EyeIcon` / `PencilIcon` / `Trash2Icon` / `TooltipProvider`，模板里只用了 `CodeIcon` / `PlusIcon` / `RefreshCwIcon`；删除按钮是纯文字 | 删掉未使用导入；`TooltipProvider` 若确无 Tooltip 用法也删 |
| `KeyValue.vue` / `S3Manager.vue` / `GoFunctions.vue` / `CronJobs.vue` | 页头「刷新 + 新建」的 `CardAction` 复制粘贴了 `v-if="!isAdmin"` / `v-else` 两份，仅差一个按钮 | 抽成一个 `CardHeaderActions` 或把新建按钮做成 `v-if`（减少重复分支，也便于 §2.3 统一加只读判定） |
| `CronJobs.vue:260-271` | `trigger()` / `triggering` 是死代码（「立即执行」走 `openRuns(record, { trigger: true })` 由弹窗内部触发） | 删除，或把「立即执行」改回直接调用它（二选一，别保留两条链路） |
| `AgentManager.vue:360-365` | `renameThread()` 无任何调用点，且 `ThreadSwitcher` 没有重命名入口 | 要么在 `ThreadSwitcher` 补重命名入口，要么删掉函数 |
| `Logs.vue:54` / `KeyValue.vue` | 为空态/移动端细节做了 `md:hidden` 分支，但 `Logs.vue` 没有移动端卡片布局（直接横向滚动） | 与 `Databases.vue` 的移动端卡片保持同口径，或明确接受横向滚动 |
| `DataTabs.vue` | 只有一个 `TabsTrigger`（「集合文档」） | 单页签的 Tabs 是冗余包装，可考虑回归直接渲染 `CollectionPanel`（同时减少一层 reka 依赖；若为未来加页签预留则保留并在注释写明） |
| `pages/*.vue`（整体） | 每个页面都重复「`useAsyncAction` + `usePagination` + 页头刷新按钮」的样板 | 长期可抽 `useListPage`；v5.0 只需先统一为同一写法，不做大重构 |
| `ui/` 组件零引用普查 | **已实测：当前没有零引用组件**。`avatar`/`label`/`progress`/`scroll-area`/`slider`/`empty`/`sheet`/`combobox`/`breadcrumb`/`toggle`/`toggle-group`/`command`/`input-group` 等看似只被 1-2 处引用的目录，逐一回溯后都有真实业务链路（如 `sheet` ← `KvDetailSheet` ← `KvPanel.vue`；`empty` ← `SbEmptyState`；`toggle`/`toggle-group` ← `TrendChart.vue`、`SqlWorkModal.vue`） | 无需删除。仅保留 `ui/AGENTS.md` 的既有约定：将来删组件前先做同样口径的全局引用确认 |

### 4.5 【P2】响应式与可访问性抽查

- `AgentManager.vue` 的左右栅格固定为 `md:grid-cols-[280px_minmax(0,1fr)]`，`min-h-[560px]`；窄屏下对话区会压得很低，建议给一个 `lg:` 断点以上的高度策略。
- `AgentComposer.vue:11,47` 有两个 `disabled` 的占位按钮（附件、语音「即将支持」）。要么实现、要么去掉——常驻禁用按钮在无障碍检查里会被标记，也让界面显得未完成。
- 弹层类组件依赖 reka 的焦点管理，本次改动**不要**动 `DialogContent` 的 `DialogTitle`/`DialogDescription` 结构（缺失会导致 a11y 告警）。

---

## 5. 代码结构优化

### 5.1 【P2】巨型文件拆分

| 文件 | 行数 | 现状 | 建议 |
| --- | --- | --- | --- |
| `internal/api/kv_commands.go` | 1426 | 一个 `var kvCommandTable = map[string]kvCmdSpec{…}` 从 86 行排到约 1200 行，62 条命令 | 按数据类型拆文件：`kv_commands_string.go` / `_key.go` / `_hash.go` / `_list.go` / `_set.go` / `_zset.go`，各自 `init()` 或由一个 `registerKVCommands` 注册进同一张表 |
| `ui/src/services/http-api.ts` | 1171 | 39 个映射函数 + `streamAgentRun`（100 行）+ 全部接口定义 | 按域拆：`services/http/databases.ts` / `agents.ts` / `sandbox.ts` / `cronjobs.ts` / `logs.ts` / `auth.ts`，`http-api.ts` 只做聚合与 `export const httpApi: Api`。**必须保持 `api.ts` 的导出面不变**（页面统一 `from '../services/api'`） |
| `internal/app/app.go` | 1020（`assembleDeps` 322 行） | 唯一装配点，按序创建 20+ 依赖 | 拆成 `assembleCoreDeps` / `assembleAgentDeps` / `assembleSandboxDeps`（保持线性调用顺序与失败回收），降低单函数认知负担 |
| `internal/api/agent_handler.go` | 903 | HTTP 层里含 `normalizeAgent`（263-307）、`streamRun`（694-794）、run 状态收尾 | 把 `streamRun` 的 SSE 帧构造抽到 `internal/api/agent_sse.go`；`normalizeAgent` 里的业务校验考虑下沉到 `internal/cloudagent` |
| `pages/AgentManager.vue` | 555 | 列表 + 对话 + 弹窗 + 定时 + 会话切换全在一页 | 把「新建/编辑 Agent 弹窗」抽成 `components/agent/AgentFormModal.vue`（现在内联在 86-144 行） |
| `components/modal/CronJobModal.vue` | 464 | 调度类型（cron/interval/once）三种表单 | 三种调度各抽一个子组件，父组件只做切换与提交 |

**约束**：拆分是**纯搬移**，不夹带行为改动；每个文件拆完立刻跑 `go build` / `yarn test`，避免「重构 + 修 bug」混在一个 commit 里（本次 §2.1/§3.1 都是行为修复，务必独立提交）。

### 5.2 【P2】死代码与未使用导入

| 位置 | 内容 | 处理 |
| --- | --- | --- |
| `internal/api/agent_handler.go:245-261` | `normalizeAgent` 方法体（`method normalizeAgent [245-261]`）与 `function normalizeAgent [263-307]` 同名；方法疑似只是薄封装 | 确认是否可合并，减少一层跳转 |
| `internal/api/kv_commands.go:106` | `_ = keys // 精确计数在下方实现` | §3.1 修复后删除 |
| `ui/src/pages/CronJobs.vue:219,260-271` | `triggering` + `trigger()` 无调用 | 删除或改回直接调用 |
| `ui/src/pages/AgentManager.vue:211,360-365` | `nextCursor` 只写不读；`renameThread()` 无调用 | `nextCursor` 若计划做「加载更多」则补 UI，否则删；`renameThread` 二选一 |
| `ui/src/components/NavMenu.vue:122-131` | `menuItems` 仅被 `tests/simple-components.test.ts:115` 使用，生产代码只用 `menuGroups` | 删 `menuItems`，测试改断言 `menuGroups`（顺带修 §2.4.2 那处 `wrapper.vm` 类型错误） |
| `ui/src/pages/GoFunctions.vue:154-159,175` | 4 个图标导入 + `TooltipProvider` 未使用 | 删除 |
| `ui/src/composables/useAiChat.ts` | `useAiChat` 已无生产调用点（`AgentManager` 只用其 `ChatMsg` 类型） | 保留 `ChatMsg` 定义并迁到 `services/types.ts`（或新建 `composables/agent-types.ts`），删除 `useAiChat` 函数及其测试；`ChatMsg` 目前被 4 个文件 import，迁移时要一次性改完 |

> `ChatMsg` 迁移会牵动 `ConversationView.vue`、`AgentManager.vue`、`useAgentConversation.ts`、`tests/useAgentConversation.test.ts`，属跨文件改动，单独一个 commit。

### 5.3 【P2】API 层健壮性

1. **SSE 绕过 401 刷新**：`ui/src/services/http-api.ts:161`（`llmStream`）与 `:496`（`streamAgentRun`）用原生 `fetch`，不走 axios 拦截器。token 过期时流式请求直接抛错，用户拿不到「自动刷新后重试」的待遇（`http.ts` 只对 axios 请求做 refresh）。方案：抽一个 `authedFetch()`，在 401 时复用 `http.ts` 的 `tryRefresh()` 逻辑后重试一次。
2. **未处理的 Promise rejection**：`ui/src/pages/Logs.vue:208-210` 的 `Promise.all([api.logs.list(...), needRetention ? api.logs.getRetention(...) : Promise.resolve(null)])` 走 `try/catch` 是安全的；但 `useAgentConversation.ts:144`（`api.agentThreads.cancel(...).catch(() => undefined)`）和 `AgentManager.vue:523,538` 的 `.catch(() => undefined)` 属于**主动吞错**，建议至少 `console.debug` 或改为静默但记录到某个诊断通道，避免线上问题无从追查。
3. **`any` 收敛**：`ui/src/services/http-api.ts` 里的映射函数签名普遍是 `(raw: Record<string, any>)`（共 35 处 `any`）。建议为每个后端响应定义 `interface XxxRaw { … }`，映射函数入参改为 `unknown` + 局部断言，逐步把 `any` 降到 0（`ui/AGENTS.md` 已要求「不写 any 落盘新代码」）。
4. **`data as any` 兜底**：`http-api.ts:177` 的 `(data as any)?.error?.message || (data as any)?.message || …` 可用一个 `extractErrorMessage(data: unknown): string` 收敛，避免同类写法在 `http.ts:94` 与 `http-api.ts` 各写一份（两处逻辑应当一致）。

---

## 6. 权限与只读语义的统一（跨前后端）

§2.3（前端只读展示）与 §3.2（后端 `writable` 检查）是同一件事的两端，建议同一批实施并共同验收：

| 层级 | 判定依据 | 覆盖范围 |
| --- | --- | --- |
| 角色权限（已有） | `auth.Role.CanWrite()` / `PermissionsForRole`：admin 只有 `DatabaseRead`+`LLMInvoke` | 全部 `require(auth.DatabaseWrite/ProjectAdmin)` 路由 |
| 实例只读（缺） | `Config.Instance.Writable` | `agent_*`、`agent-schedules`、`s3` 写接口（§3.2） |
| 前端体验层（缺） | `authStore.canWrite` 或 `projectStore.isAdmin` | S3 / Agent / Logs / GoFunctions / CronJobs（§2.3） |

验收口径：

- 只读**角色**登录 → 所有写按钮不可见，直接调接口返回 403；
- 只读**实例**（`instance.writable=false`）→ 所有写接口返回 503/`ErrWriterUnavailable`，UI 至少提示明确错误而不是静默失败。

---

## 7. 测试与覆盖率

### 7.1 typecheck 与 §2.4.2 同项

`vite-env.d.ts` 补 `*.vue` shim 后，241 处 TS2307 应一次性消失；剩余 21 处逐个处理。目标是让 `yarn typecheck` 与 `yarn build`、`yarn test` 一起进入提交前自查（`AGENTS.md` 已更新）。

### 7.2 覆盖率补测

按 §2.4.1 的清单补；注意 `ui/vite.config.ts` 的 `exclude` 已包含 `src/components/ui/**`、`src/test/**`、`*.d.ts`、`main.ts`、`types.ts`——**不要**为了让数字好看而往 `exclude` 里加业务文件。

### 7.3 测试桩会掩盖真实缺陷（重要教训）

`ui/src/test/helpers.ts:100-104`：

```ts
Switch: {
  props: ['checked'],
  emits: ['update:checked'],
  template: '<button type="button" class="switch" @click="$emit(\'update:checked\', !checked)">sw</button>'
}
```

这个桩用的是 reka-ui **已废弃**的 `checked` 契约。后果：§2.1 的开关在真实 DOM 里失效，但 `CronJobs.test.ts` / `Logs.test.ts` / `SqlWorkModal.test.ts` / `AgentScheduleModal.test.ts` 全都通过。同类风险：`Slider` 桩、`Input`/`Textarea` 桩（手写 `v-model` 实现，与真实组件行为可能漂移）。

**方案**：

1. 删掉 `Switch` 桩，测试直接用真实 `Switch`（reka 在 jsdom 下可渲染，§1.1 的探针已验证）。
2. 若某些组件必须桩（例如 `Monaco` 这种重型依赖），桩的 props/emits 必须与**锁定版本的依赖源码**一致，并在桩上方注释版本号与依据文件（如 `// reka-ui 2.10.5 dist/Switch/SwitchRoot.js`）。
3. 为 `Switch` 补一条**真实组件**的回归用例：点击后 `data-state` 从 `unchecked` 变 `checked`，且 `update:modelValue` 被触发一次。这条用例能直接锁死 §2.1 不复发。

### 7.4 后端测试

- §3.1 补 `EXISTS` 语义用例（含过期键与空库）；
- §3.2 补三条写路径的 `writable=false` 用例；
- 新增/修改 `LLMService` 等跨层接口时，`internal/api/*_test.go` 的 `fakeGW`（`adapters_test.go:377`）与 `fakeLLMSvc`（`llm_handler_test.go:54`）必须同步实现新方法，否则整包编译失败——本次工作区改动里已经有 `ProviderModels` 的先例可参照。

---

## 8. 代码卫生

### 8.1 【P2】gofmt

```
$ gofmt -l internal cmd gofunction packages
internal/api/agent_more_test.go
internal/api/api_keys_handler_test.go
internal/api/auth_handler.go
internal/api/db_stats_cache.go
internal/api/e2e_apikeys_check_test.go
internal/api/gofunction_handler.go
internal/api/kv_commands.go
internal/api/kv_init.go
internal/api/llm_handler_test.go
internal/api/sql_types.go
internal/api/stage_timing_test.go
internal/app/app.go
internal/app/identity.go
internal/app/start_test.go
internal/auth/cache_test.go
internal/catalog/cache.go
internal/catalog/tenant_ctx.go
internal/cloudagent/coverage_test.go
internal/cloudagent/sandbox_tools_test.go
internal/config/sandbox_test.go
internal/config/yaml.go
internal/cronjob/scheduler_test.go
internal/database/coverage_test.go
internal/database/ducklake/inspect.go
internal/database/ducklake/runtime_test.go
internal/database/kv/executor.go
internal/database/kv/schema.go
internal/database/lease/lease_test.go
internal/database/registry/registry.go
internal/llmgateway/coverage_test.go
internal/llmgateway/service_test.go
internal/objectstore/probe.go
internal/observability/metrics.go
internal/observability/stage_timing_metrics.go
internal/systemdb/async_flush.go
internal/systemdb/cron_jobs.go
internal/testutil/fakellm/server.go
```

差异绝大多数是 `"a " + b` → `"a " + b` 这类冗余拼接空格（见 `kv_commands.go:1067` 的 `gofmt -d` 输出），**纯格式**。

**方案建议**：不要一次性全仓 `gofmt -w`（会产生一个 37 文件的巨大 diff，淹没其它改动）。改为：

- 本次涉及的改动文件顺手 `gofmt -w`；
- 单独开一个 `style(gofmt):` 前缀的 commit 批量格式化剩余文件（无行为变更，便于 `git blame` 时用 `--ignore-rev` 或 `.git-blame-ignore-revs` 跳过）。

### 8.2 【P3】错误统一

`grep -rn 'NewHTTPError(http.Status[A-Za-z]*, "' internal/api/*.go | grep -v _test | wc -l` → **75 处**。`agent_handler.go` 占比最高（`"project context missing"`、`"agent not found"`、`"thread not found"`、`"invalid limit"`、`"invalid cursor"`、`"title must be 1-80 chars"`…）。

`internal/AGENTS.md` 要求「错误统一走 `WriteError` + 领域错误…不得裸返回字符串」。方案：

1. 先把高频复用的几个（project context missing、not found 系列、invalid cursor/limit）提取成 `error.go` 里的领域错误构造函数；
2. 其余按模块逐个替换，每个模块一个 commit；
3. 补测：这些错误码的 HTTP status 与响应体 `error.code` 断言（`error_test.go` 已有样板）。

### 8.3 【P3】其它

- `internal/api/kv_commands.go` 命名前缀统一为 `kv*` 是好的，但 `kvArgErr` / `kvInvalidArgument`（后者在 `kv_handler.go:382`）职责重叠，建议合并成一个入口。
- `ui/src/services/http.ts:104-107` 与 `:110-115` 两段 401 处理重复（`import('../stores/auth')` 各写一遍），抽一个 `notifyUnauthorized()`。

---

## 9. 版本控制与交付卫生

### 9.1 【P1，流程】既有成果未跟踪

`git status --short` 显示工作区里有一批「已删除 + 已修改 + 未跟踪」的混合状态。其中**未跟踪**的项（`??`）：

```
?? fakellm-server                        ← 8.5MB 构建产物，不应入库
?? internal/api/agent_e2e_errors_test.go
?? internal/api/agent_e2e_harness_test.go
?? internal/api/agent_e2e_stream_test.go
?? internal/api/agent_e2e_usage_retry_test.go
?? internal/api/sse_test_helpers_test.go
?? internal/testutil/                    ← fakellm 测试基建
?? scripts/                              ← test.sh / smoke.sh / llm-integration.sh
?? ui/src/components/agent/              ← AgentCard/ThreadSwitcher/ConversationView/AgentComposer/AgentToolCard
?? ui/src/composables/useAgentConversation.ts
```

这些是 **v4.1 云 Agent 优化的实际成果**（`plan/planv4.0/cloud-agent-optimization-plan.md` 的 D 组之外部分）。它们目前只存在于工作区：任何 `git clean` / 误删 / 切分支都会丢失，而且后续 §2/§3 的改动会与它们混在同一个 diff 里，评审时无法区分「既有未提交」与「本次修复」。

**必须先做的事**：把这些成果按逻辑分组提交（建议：`test(api): add cloud agent e2e harness`、`chore(scripts): add backend test entrypoints`、`refactor(ui): extract agent conversation layer`），再开始 v5.0 的任何改动。

### 9.2 【P2，流程】构建产物未被忽略

`fakellm-server`（8.5MB，`go build ./scripts/fakellm-server` 的产物）未被 `.gitignore` 覆盖。检查：`.env`、`config.yaml`、`simplebased` 都已在 `.gitignore`（第 1、13、14 行），只有 `fakellm-server` 漏了。

方案：在 `.gitignore` 增加 `/fakellm-server`（与现有 `/simplebased` 同风格）。顺带确认 `ui/dist/`、`ui/coverage/`、`output/` 是否已忽略。

### 9.3 【P2，流程】`internal/web/dist` 与 embed

`AGENTS.md` 已声明 `internal/web/dist` 由 `./build.sh` 生成、不得手改。本轮改前端后需要跑一次 `./build.sh` 或 `./build.sh --ui-only` 同步，否则后端嵌入的仍是旧前端。**这一点在 §2.1 的验收里尤其重要**：只看 `yarn build` 不足以验证用户实际看到的效果，要确认 embed 目录也更新了。

---

## 10. 排期与验收

### 10.1 建议的 commit 顺序

| # | commit 主题 | 内容 | 前置 |
| --- | --- | --- | --- |
| 1 | `chore(repo): track v4.1 agent work, ignore fakellm-server` | §9.1 分组提交 + §9.2 | — |
| 2 | `fix(ui): correct reka-ui state attribute hooks` | §2.1 + §4.1（13 个 `ui/` 组件 + 4 个调用点） | — |
| 3 | `test(ui): use real Switch component, add state regression` | §7.3 | 2 |
| 4 | `test(ui): add vue shim and fix typecheck` | §2.4.2 / §7.1 | — |
| 5 | `test(ui): close function coverage gap` | §2.4.1 / §7.2 | 3,4 |
| 6 | `fix(api): implement precise KV EXISTS` | §3.1 + 用例 | — |
| 7 | `fix(ui): do not treat login 401 as credential loss` | §2.2 | — |
| 8 | `fix(api): enforce writable on agent, schedule and s3 writes` | §3.2 + 用例 | — |
| 9 | `fix(ui): honour readonly role on list pages` | §2.3 + 用例 | 2（开关禁用依赖 §2.1） |
| 10 | `style(ui): polish sandbox page, fonts, shared header actions` | §4.2–§4.5 | 9 |
| 11 | `refactor: split oversized modules` | §5.1（可再拆多个 commit） | 全部功能修复完成 |
| 12 | `style(gofmt): format internal packages` | §8.1 | 11 |
| 13 | `refactor(api): replace raw string errors with domain errors` | §8.2 | 11 |

第 2/3/5、第 6、第 8、第 9 各是「修复 + 用例」的完整闭环，单独可回滚。

### 10.2 验收清单

**必须全部满足才能算完成：**

1. `go build ./internal/... ./cmd/...` 通过；`go vet ./internal/...` 无输出。
2. `go test ./... -short` 通过（新增用例含 §3.1 与 §3.2）。
3. `cd ui && yarn build` 通过；`cd ui && yarn test` **四项覆盖率全部 ≥ 95%** 且退出码为 0。
4. `cd ui && yarn typecheck` 无错误。
5. **真实浏览器人工验证**（不可用自动化替代）：
   - 定时任务页点「启用」→ 开关轨道变蓝、滑块右移，刷新后状态保持；
   - 日志页「自动刷新」开关可切换并开始轮询；
   - SQL 弹窗「事务」开关可切换；
   - Agent 定时弹窗「启用」可切换；
   - 打开/关闭任一弹窗有淡入淡出；`Select` / `Popover` / `Tooltip` 均有进出场动画；
   - 数据库页展开行的「集合文档」页签有选中底色。
6. 明暗两套主题下（顶栏太阳/月亮切换）上述控件都正常。
7. 只用 `instance.writable=false` 起的实例上，`POST /v1/projects/:id/agents`、`/agent-schedules`、`/s3/objects` 均返回只读错误。
8. 用 admin 角色登录，S3 / Agent / 日志 / 云函数 / 定时任务页看不到写入口。
9. `./build.sh`（或 `--ui-only`）后 `internal/web/dist` 已同步，嵌入式控制台表现与 dev 一致。

**明确不在本版范围（留给 v5.1+）：**

- `scheduler.go` 的 `stopCh`/`done` 竞态与 `IsRunning()` 无锁读（**附录 A-1**，需单独做并发审计）；
- 通知/告警类产品功能（`notifyWrite` 只做缓存失效，无外部 webhook）；
- `ui/` 组件的删除动作（**实测当前没有零引用组件**，见 §4.4；不做删除以保持与 §2.1 的批量改动解耦）；
- Cloud Agent 的 hooks / 审批闸 / 多 Agent team（`TeamSupported` 目前恒为 `false`，属预留）。

---

## 附录 A：待核实项（不作为实施依据）

| # | 项 | 现状 | 需要什么才能定性 |
| --- | --- | --- | --- |
| A-1 | `internal/cloudagent/scheduler.go` 的 goroutine 生命周期 | `stopCh` / `done` 的收发与 `IsRunning()` 读路径未见同步原语；`plan/planv4.0/cloud-agent-optimization-plan.md` 提到过一处 `close(nil)` 竞态已修，但当前代码未见锁 | `go test -race ./internal/cloudagent` + 专门的生命周期用例；需要一轮并发审计 |
| A-2 | KV 的 `time.Now()` 与 `nowMs()` 是否统一 | `kv_commands.go:1421` 的 `nowMs()` 与仓库层的 `r.t.nowMs()` 可能不是同一时钟来源 | 读 `internal/database/kv/executor.go` 的 `nowMs` 定义与注入方式；若不一致会影响 TTL 判定一致性 |
| A-3 | `AccessLogMiddleware` 与 `perfStageMiddleware` 在系统库忙时是否互相放大 | v4.0 已处理「每请求写日志」，但 `stage_timing.go`（387 行）的埋点成本未量化 | 压测对比 `--http` 场景开/关埋点的 p95 |
| A-4 | `db_stats_cache.go` 的失效策略 | 缓存不命中时是否逐个数据库串行统计 | 读 `db_stats_cache.go` 与 `db_stats.go`，或看压测 |
| A-5 | 前端 `pages/*.vue` 是否都处理了项目切换 | `Databases.vue` / `Logs.vue` / `S3Manager.vue` 有 `watch(projectId)`；`AgentManager.vue` 有 `watch(() => project.id)`；`KeyValue.vue` 靠 `ProjectScope` 的 `:key` 重挂载 | 逐个确认，或统一改为 `ProjectScope` 的 key 策略 |
| A-6 | `@lucide/vue` 的按需引入是否生效 | 全仓直接从 `'@lucide/vue'` 具名导入；未见 `unplugin-vue-components` 配置 | 看 `yarn build` 的 chunk 体积；若整包引入会导致首屏偏大 |
| A-7 | `ui/dist` 中 `DocsWiki` / `GoFunctions` 的独立 CSS chunk 成因 | 两个页面各带一个 CSS chunk，说明有页面级样式被 Vite 单独提取 | 确认是预期（Monaco 等动态依赖）还是样式泄漏 |

---

## 附录 B：证据索引

用于复核本文档的关键结论：

| 结论 | 复核命令 |
| --- | --- |
| §2.1 裸属性变体不生效 | `grep -o '\[data-checked\]\|\[data-active\]\|\[data-open\]' ui/dist/assets/index-*.css`；`grep -rn '"data-state"' ui/node_modules/reka-ui/dist/Switch/SwitchRoot.js` |
| §2.1 `Switch` 无 `checked` prop | `sed -n '15,66p' ui/node_modules/reka-ui/dist/Switch/SwitchRoot.js` |
| §2.2 401 未区分端点 | `sed -n '89,117p' ui/src/services/http.ts`；`sed -n '75,84p' ui/src/stores/auth.ts` |
| §3.1 `EXISTS` 语义 | `sed -n '98,111p' internal/api/kv_commands.go`；`sed -n '155,158p' internal/database/kv/key.go`；`grep -n 'EXISTS' internal/api/kv_handler_test.go` |
| §3.2 `writable` 缺口 | `for f in internal/api/*_handler.go; do echo "$(grep -c writable $f) $f"; done` |
| §2.4.1 覆盖率 | `cd ui && yarn test`（末尾 Coverage summary） |
| §2.4.2 typecheck | `cd ui && yarn typecheck 2>&1 | grep -c 'error TS'` |
| §7.3 测试桩 | `sed -n '100,104p' ui/src/test/helpers.ts` |
| §8.1 gofmt | `gofmt -l internal cmd gofunction packages` |
| §8.2 裸字符串错误 | `grep -rn 'NewHTTPError(http.Status[A-Za-z]*, "' internal/api/*.go | grep -v _test | wc -l` |
| §9 未跟踪文件 | `git status --short | grep '^??'` |
