# SimpleBase UI 改造计划：ant-design-vue → shadcn-vue（Vue 3 + Tailwind v4 + Reka UI）

> **日期**：2026-09-17
> **范围**：`ui/` 全部前端（约 8,000 行：页面 8 个、组件 20 个、布局 2 个、样式 5 文件）
> **目标**：将 ant-design-vue 4 全量替换为 shadcn-vue（Reka UI + Tailwind CSS v4），保留现有 Claude 橙暖色设计语言（tokens.css 品牌变量迁移到 Tailwind 语义 token）
> **参考**：`skills/shadcn/`（React 版规则，组件组合/表单/图标/chat 规则同样适用于 Vue 版，仅 CLI 与 import 语法不同）
> **重要前置事实**：
> 1. `internal/web/dist` 与 `ui/dist` 已加入 `.gitignore` 并 `git rm --cached`（本计划第 1 步已完成），embed 编译已验证通过。
> 2. shadcn 官方 CLI（skill 中 `npx shadcn@latest`）面向 **React**。Vue 项目必须使用 **`npx shadcn-vue@latest`**（社区官方维护，基于 Reka UI——Radix UI 的 Vue 移植），文档：v3.shadcn-vue.com。

---

## 0. 一句话目标

**零运行时 CSS-in-JS、组件源码进仓可改、设计 token 单一来源（Tailwind `@theme`）、暗色模式一行 class 切换**；同时兑现 cleanup-refactor-plan 中 Logs HTTP 化、LlmManager 删除等前端清理项，避免二次返工。

---

## 1. 现状盘点（改造依据）

### 1.1 ant-design-vue 使用面（rg 统计，改造工作量依据）

| antd 组件 | 用量 | shadcn-vue 对应 | 迁移难度 |
|---|---|---|---|
| `<a-button>` | 36 | `Button`（variant/size） | 低 |
| `<a-form-item>` / `<a-form>` | 19+7 | `Field`/`FieldLabel`/`FieldDescription`（无校验引擎，用自写校验或 vee-validate） | **高** |
| `<a-tag>` | 16 | `Badge` | 低 |
| `<a-input>` / `<a-textarea>` | 16+3 | `Input` / `Textarea` | 低 |
| `<a-card>` | 15 | `Card` + `CardHeader/Title/Description/Content/Footer` | 低 |
| `<a-tooltip>` | 9 | `Tooltip` + `TooltipProvider` | 低 |
| `<a-table>` | 8 | `Table`（简单）；分页/排序复杂表用 TanStack Table + `Table` 或自封装 | **中** |
| `<a-empty>` / `<a-result>` | 7+4 | `Empty` | 低 |
| `<a-select>` | 6 | `Select`（Reka；注意 v-model → `v-model` 保持） | 中 |
| `<a-radio-button>`/`-group` | 6+2 | `ToggleGroup`（2–7 项）或 `RadioGroup` | 低 |
| `<a-popconfirm>` | 3 | `AlertDialog` | 低 |
| `<a-input-number>` / `<a-switch>` | 3+3 | `Input type="number"` / `Switch` | 低 |
| `<a-drawer>` | 3 | `Sheet`（side="left/right"） | 中 |
| `<a-breadcrumb>` | 2+4 | `Breadcrumb` | 低 |
| `<a-auto-complete>` | 2 | `Combobox`（`Command` in `Popover`） | 中 |
| `<a-alert>` | 2 | `Alert` | 低 |
| `<a-layout>` 全家 | 7 | 自有 layout（`DefaultLayout.vue` 重写，不用 shadcn Sidebar 组件也行，但**建议用 `Sidebar`**） | **高** |
| `<a-menu>` / `<a-config-provider>` | 1+1 | `SidebarMenu` 系列 / **直接删除**（主题交给 Tailwind） | 中 |
| `<a-modal>` / `<a-upload>` / `<a-slider>` / `<a-skeleton>` / `<a-progress>` | 各 1 | `Dialog` / 自写 `Input type="file"` | 中 |
| `message.success/error/...` | **67 处** | `vue-sonner` 的 `toast()` | 中（全局替换） |
| `<a-space>` / `<a-row>`/`<a-col>` / `<a-divider>` / `<a-badge>` | 3/2/1/1 | `flex gap-*` 工具类 / Grid 类 / `Separator` / `Badge dot` | 低 |

### 1.2 现有样式体系（迁移资产，不是垃圾）

| 文件 | 行数 | 去向 |
|---|---|---|
| `styles/tokens.css`（122 行 Claude 橙设计变量 + dark 主题） | 122 | **映射到 Tailwind v4 `@theme` + `.dark` 变量块**（见 §3.1），`tokens.ts`（antd 主题桥接）**删除** |
| `styles/antd-patch.css` | 53 | **删除**（antd 专属 hack） |
| `styles/base.css` | 75 | 保留部分全局 reset，合并进 `style.css` |
| `styles/utilities.css`（sb-card 等复用类） | 181 | 逐个评估：多数可被 `Card`/工具类替代，少数（`sb-terminal`、`sb-mono`）迁入 `@theme` 或保留 |
| 各页面 scoped 样式 | ~1,500 | 大部分替换为工具类；设计语言（暖米色、8pt、圆角标尺）由 token 承载 |

### 1.3 功能面（不动逻辑，只动皮）

- 8 个路由页面：Dashboard / Databases / S3 / AgentManager / Settings / Logs / DocsWiki /（FaaS 已列删除）
- 服务层 `services/*`（api/http/mock/types）**零改动**——这是本次改造最大的护城河：所有页面逻辑依赖 `Api` 接口，与 UI 库解耦
- 主题切换：`stores/settings.ts` 的 `data-theme` 机制 → 改写为给 `<html>` 加 `.dark` class（Tailwind 惯例）
- AiChat/AiChatComposer：参考 skill `rules/chat.md`（Message/Bubble/Scroller 模式）重写气泡与滚动锚定

---

## 2. 技术栈与依赖变更

### 2.1 package.json 净变化

```diff
  dependencies:
+   "reka-ui": "^2.x"            # shadcn-vue 底层（CLI 自动装）
+   "tailwindcss": "^4"           # CSS 引擎
+   "@tailwindcss/vite": "^4"     # Vite 插件（替代 postcss 配置）
+   "class-variance-authority": "^0.7"   # 组件 variant
+   "clsx" + "tailwind-merge"     # cn() 工具
+   "lucide-vue-next": "^0.4x"    # 图标（替代 @ant-design/icons-vue）
+   "vue-sonner": "^1.x"          # toast（替代 message.* 67 处）
+   "recharts"?? 否              # Dashboard 若需图表再引入 shadcn Chart（Vue 版 chart 基于 unovis，按需）
+   "tw-animate-css": "^1"        # shadcn-vue init 默认动画依赖
-   "ant-design-vue": "^4.0.0"
-   "@ant-design/icons-vue": "^7.0.0"
```

**安装顺序**（对照 v3.shadcn-vue.com/docs/installation/vite）：
1. `pnpm add tailwindcss @tailwindcss/vite tw-animate-css`（项目用 npm/yarn 则换 runner；当前项目无 lock 文件，沿用 npm）
2. `vite.config.ts`：加 `tailwindcss()` 插件 + `@` alias（`resolve.alias: { '@': path.resolve(__dirname, './src') }`）
3. `tsconfig.json` / `tsconfig.app.json`：`compilerOptions.paths: { "@/*": ["./src/*"] }`
4. `npx shadcn-vue@latest init`（选 Neutral base color；生成 `components.json` + `src/style.css` 变量）
5. 组件批量添加：`npx shadcn-vue@latest add button card input textarea select dialog sheet alert badge table tabs tooltip popover dropdown-menu switch separator skeleton sonner empty breadcrumb avatar progress scroll-area combobox toggle-group command form field label radio-group alert-dialog pagination chart`

### 2.2 关键差异（skill 规则的 Vue 适配）

skill（`skills/shadcn/SKILL.md`）规则按 React/tsx 写，但**原则层完全通用**，Vue 版对应：

| skill 规则 | Vue 写法 |
|---|---|
| `className` → | `class` |
| `cn()`（clsx+tailwind-merge） | `cn()` 放 `src/lib/utils.ts`（CLI 生成，`import { cn } from '@/lib/utils'`） |
| `data-icon="inline-start"` | 同名 attribute 直接用 |
| `Field` + `data-invalid` | shadcn-vue 的 `Form` 基于 vee-validate；**本项目表单极简（无复杂校验）**，建议用轻量 `Field`/`Label` 手写组合，不引 vee-validate |
| `sonner` toast | `vue-sonner`：`<Toaster />` 挂 App.vue，`toast.success('...')` 全局替换 `message.success` |
| Dialog `asChild` | Reka UI 同样支持 `as-child` |
| 图标 `lucide-react` | `lucide-vue-next`（`import { ReloadOutlined } from '@ant-design/icons-vue'` → `import { RefreshCw } from 'lucide-vue-next'`，注意图标名不同，需逐一替换） |

---

## 3. 主题与设计语言迁移（保品牌：Claude 橙）

### 3.1 tokens.css → Tailwind v4 语义 token

在 `src/style.css`（shadcn-vue init 产物）追加品牌映射（**保持现有视觉不变**）：

```css
@import "tailwindcss";
@import "tw-animate-css";
@custom-variant dark (&:is(.dark *));

:root {
  /* shadcn 语义变量 ← 现有 --sb-* 值 */
  --background: #f5f4ef;        /* 原 --sb-bg */
  --foreground: #1f1e1d;        /* 原 --sb-text */
  --card: #ffffff;              /* 原 --sb-surface */
  --primary: #d97757;           /* Claude 橙 */
  --primary-foreground: #faf9f5;
  --secondary: #eeece5;         /* 原 --sb-bg-soft */
  --muted: #e7e4db;             /* 原 --sb-bg-sunken */
  --muted-foreground: #706f6a;  /* 原 --sb-text-secondary */
  --accent: #eeece5;
  --destructive: #c0452f;       /* 原 --sb-danger */
  --border: rgba(31,30,29,0.12);
  --input: rgba(31,30,29,0.12);
  --ring: #d97757;
  --radius: 0.75rem;            /* 原 --sb-radius 12px */
}

.dark {
  --background: #1a1918;
  --foreground: #f3f1ea;
  --card: #242322;              /* 原 --sb-bg-elevated */
  /* ...对照 tokens.css dark 块逐项搬 */
}
```

`tokens.ts`（antd 主题桥接，67 行）与 `App.vue` 里的 `a-config-provider`/`darkAlgorithm` **全部删除**；主题切换收口为：

```ts
// stores/settings.ts applyThemeToDom 简化为
document.documentElement.classList.toggle('dark', dark)
```

### 3.2 复用类收编

`utilities.css` 的 `sb-card`/`sb-toolbar`/`sb-status-tag` 等 → 由 `Card` 组件 + `flex items-center gap-*` 工具类替代；`sb-terminal`（Logs 页）/`sb-font-mono`/`sb-json` 保留为 `@utility` 或组件内样式。**逐类登记迁移去向，不允许"删了不管"**。

---

## 4. 分阶段实施（每阶段可独立 PR、可独立验收）

### Phase 0：脚手架与双栈并存（0.5 d）

> 策略：**页面逐个切换**而非大爆炸重写。antd 与 shadcn 可短暂共存（体积浪费但风险最低），每迁完一批页面删一批 antd 组件用量。

1. 装 Tailwind v4 + `@tailwindcss/vite` + alias 配置。
2. `shadcn-vue init` + `style.css` 品牌变量（§3.1）。
3. `main.ts`：**暂保留** `app.use(Antd)`；新增 `import './style.css'`。
4. App.vue 挂 `<Toaster richColors position="top-center" />`（vue-sonner）。
5. 建 `src/lib/utils.ts`（CLI 自动）+ `src/components/ui/`（CLI 添加组件）。
6. 验收：`yarn build` 通过；antd 页面视觉无回归；新 Button/Card 可在任一页面试点渲染。

### Phase 1：骨架与导航（1 d）

| 改造对象 | 方案 |
|---|---|
| `DefaultLayout.vue`（276 行） | `a-layout/sider/header` → **shadcn `Sidebar`**（`SidebarProvider`/`Sidebar`/`SidebarHeader`/`SidebarContent`/`SidebarTrigger` + `SidebarMenu/SidebarMenuItem/SidebarMenuButton`）；移动端抽屉 → `Sheet`。折叠/展开状态用 `useSidebar()` |
| `NavMenu.vue`（102 行） | `SidebarMenuButton :is-active` + `lucide-vue-next` 图标；`routeOrder` 逻辑保留；顺带执行 cleanup 计划 F9（删 `llm` 死键） |
| 面包屑 | `Breadcrumb`/`BreadcrumbList/Item/Link/Separator` |
| `GlobalProjectSwitcher.vue`（261 行） | `DropdownMenu` + `Command`（项目搜索）；`CreateProjectModal` → `Dialog` + `Form` |
| `ApiKeyDrawer.vue`（98 行） | `Sheet` + `Field`/`Input`/`Button` |
| `PageContainer.vue` / `ProjectScope.vue` | 纯容器：工具类重写（`mx-auto w-full max-w-[1440px] flex flex-col gap-6`） |
| `SbEmptyState.vue` | 换 shadcn `Empty`（`EmptyDescription` + action slot） |

**验收**：全站骨架渲染正常、移动端抽屉可用、暗色切换正常（`html.dark`）。

### Phase 2：核心数据页（1.5 d）

| 页面 | 要点 |
|---|---|
| `Databases.vue`（427 行，最重） | `a-table` → `Table`（列表 8 列内、客户端分页用简单 `Table` + 自写分页或 `Pagination` 组件）；状态 `a-tag` → `Badge` variant 映射（9 个状态）；操作列 `a-popconfirm` → `AlertDialog`；打开/关闭/删除按钮 → `Button variant="outline"/destructive"` |
| `SqlWorkModal.vue`（375 行） | `SbModal` → `Dialog`（`DialogContent` + `sr-only` `DialogTitle`）；SQL 结果 `SbCodeBlock` 保留（改内部样式为工具类） |
| `CollectionPanel` / 文档 KV/List 模态 | `Dialog` + `Table` + `Badge`；`SbModal.vue` 重写为 shadcn `Dialog` 薄封装（保持 props 兼容，减小调用方 diff） |
| `S3Manager.vue`（233 行） | `Table` + 上传（自写 file input + `Button`）+ `AlertDialog` 删除确认 + 对象 `Empty` 态 |

### Phase 3：Dashboard / Settings / Logs（1 d）

| 页面 | 要点 |
|---|---|
| `Dashboard.vue`（286 行） | 统计卡 `Card`+`CardHeader/CardTitle/CardDescription`；配额 `Progress`；趋势图若后端 metrics 已接（cleanup 计划 R8）→ `npx shadcn-vue@latest add chart`（Vue 版基于 unovis），否则维持隐藏 |
| `Settings.vue`（334 行） | 主题 `ToggleGroup`（light/dark/system 3 项）；模型默认值 `Field`+`Input`+`Slider`+`Input(type=number)`；厂商 Key 卡 `Card` 网格（`grid` + `gap-3`）；编辑抽屉 `Sheet`；**顺带执行 cleanup 计划 U5**（删过时 alert，接 `llmSettings` API） |
| `Logs.vue`（201 行） | **顺带执行 cleanup 计划 R4**：重写为 HTTP 查询页（`GET :p/logs`），`Table`/日志级别 `Badge` + 过滤 `Input`/`Select`；删 WS 全部代码 |
| `message.*` 67 处 | 全局替换 `toast.success/error/warning`（vue-sonner），`message` import 清除 |

### Phase 4：AiChat / Cloud Agent（1 d）

对照 skill `rules/chat.md` 的组合模式（Vue 适配）：

- `AiChat.vue`（258 行）：消息列表 → 自写 `MessageScroller` 语义组件（Reka 无官方 chat 原生件，参照 skill 规则：滚动容器 + `Message` 行 + `Bubble` 表面 + 流式光标 `▍` 保留）；工具调用卡片 → `Card` 变体
- `AiChatComposer.vue`（305 行）：`@` mention → `Popover` + `Command`（mention 选择器）；发送按钮流式时变 `Square`（stop）图标——遵循 skill「Button 无 isPending，用 `disabled` + `Spinner`/`data-icon`」
- `AgentManager.vue`（380 行）：左栏 agent 列表 `Card` + `Badge`（module）+ `DropdownMenu`（编辑/删除）；新建/编辑 `Dialog` + `Field`（name/module/system_prompt/tools）；`ToggleGroup` 选 module；对话区复用新 AiChat
- `DocsWiki` / docs 组件（3 文件 ~490 行）：`DocsLayout` → `Sidebar` 变体或 `ScrollArea` + 树；`DocsModuleTabs` → `Tabs`；文章 `prose`（需 `@tailwindcss/typography` 插件，或保留现有渲染样式）

### Phase 5：收尾与卸载 antd（0.5 d）

1. `rg '<a-' src/` → **0 命中**；`rg "ant-design-vue" src/ package.json` → 0 命中。
2. `main.ts` 删 `app.use(Antd)` 与 `antd/dist/reset.css` import。
3. `pnpm remove ant-design-vue @ant-design/icons-vue`（package.json 净化）。
4. 删除 `styles/antd-patch.css`、`tokens.ts`；`tokens.css` 中已被 Tailwind 变量吸收的项清理，仅保留 Composer 等专有变量（迁 `@theme`）。
5. 执行 cleanup 计划遗留项：FaaSManager/faas 域删除、LlmManager 删除、NavMenu `llm` 键删除（与本改造同 PR 收口）。
6. `yarn build && npx vue-tsc --noEmit` 全绿；`./build.sh dev` 走查全部页面。
7. 更新 `ui/README`（如有）与 `plan/planv2.0/ui-plan-v2.md` 的技术栈章节。

---

## 5. 风险与对策

| 风险 | 对策 |
|---|---|
| shadcn-vue 是社区项目（非官方 shadcn），组件覆盖度略低于 React 版 | 已核对：本项目用到的 30+ antd 组件在 shadcn-vue 全部有对应（Table/Dialog/Sheet/Command/Sidebar/Sonner 均在）；`MessageScroller` 等 chat 原生件缺 → 自写 |
| Tailwind v4 + 现有 scoped 样式冲突（优先级） | 迁移期 antd 与 tailwind 共存时用 `@layer` 控制；切换完成的页面立即删除对应 scoped 样式 |
| `a-table` 功能差距（排序/筛选/分页/列宽） | 本项目表格均为简单列表（无服务端排序、无列宽拖拽）；分页用自封装 `Pagination` + composable `usePagination`（已有） |
| 双栈期包体积膨胀（antd ~1MB + tailwind） | 过渡期 ≤ 5 个 PR；Phase 5 一次性移除 antd；构建产物 dist 已 gitignore，不污染仓库 |
| 表单校验（antd form rules → 无） | 现有校验均为必填/trim 级别，手写 `Field data-invalid` + `aria-invalid`（skill forms.md 规则）足够，不引 vee-validate |
| 暗色主题回归 | token 值从 tokens.css 逐项对照搬运；验收清单含 dark 模式全页截图比对 |
| Vue 版 CLI 与 skill 文档（React）混淆 | 本 plan §2.2 已列差异表；执行时只用 `shadcn-vue` CLI，skill 规则文件作组合模式参考 |

---

## 6. 验收标准（DoD）

1. `rg '<a-' ui/src/` → 0；`rg "ant-design-vue|@ant-design" ui/ --glob '!package-lock.json'` → 0。
2. `yarn build` 产物中不含 antd chunk；总 JS gzip 体积较改造前**下降**（antd 移除 > shadcn 引入）。
3. 8 个路由页面 + 2 布局全部 shadcn 渲染；四档宽度（375/768/1440/2560）不塌陷。
4. 暗色模式：`html.dark` 一处切换全站生效（含 sonner/Dialog/Sheet）。
5. 主题品牌色 `#d97757`（Claude 橙）与暖米色背景视觉等价改造前（截图比对）。
6. 功能回归：登录 Key 配置 → 项目切换 → 建库 → SQL → 文档 KV → S3 上传 → Agent 对话（流式）→ 设置页改主题，全链路手测通过。
7. `vue-tsc --noEmit` 0 错误。
8. cleanup-refactor-plan 中前端项（FaaS/LlmManager/Logs HTTP/Settings alert）全部关闭。

---

## 7. 与既有计划的关系

| 文档 | 关系 |
|---|---|
| `cleanup-refactor-plan.md` | 本计划 Phase 2–4 顺带执行其前端清理项（F1–F9、U4/U5）；执行顺序：先本计划 Phase 0/1，清理项随对应 Phase 落地 |
| `ui-plan-v2.md` | 技术栈章节改写（antd → shadcn-vue）；其 P1–P4 交互原则不变，视觉规范由 token 承接 |
| `ui-style-plan.md` / `ui-principles.md` | 设计原则保留；实现层从「antd token 覆盖」换为「Tailwind 语义 token」 |
| `ui-settings-chat-plan.md` / `cloud-agent-plan.md` | Phase 4 落地其 UI 部分；后端 API 无任何改动 |
| `proto-http.md` | 零改动（服务层不动） |

---

## 8. 排期与估算

| 阶段 | 内容 | 估算 |
|---|---|---|
| **P0** | 脚手架（Tailwind v4 + shadcn-vue init + Toaster + 试点 Button/Card） | 0.5 d |
| **P1** | 布局/导航/全局组件（Sidebar/Sheet/Empty/Breadcrumb） | 1 d |
| **P2** | Databases + SQL/文档模态 + S3（最重的数据页） | 1.5 d |
| **P3** | Dashboard/Settings/Logs + message→toast 全局替换 | 1 d |
| **P4** | AiChat/AgentManager/docs | 1 d |
| **P5** | 卸载 antd + 清理样式 + 验收 | 0.5 d |

**总估：5.5 人天**。P0 完成后每个 Phase 独立 PR，可随时暂停交付（双栈并存期保持可发布）。

---

## 9. 附录：组件安装清单（一次性）

```bash
npx shadcn-vue@latest add alert alert-dialog avatar badge breadcrumb button card chart checkbox combobox command dialog drawer dropdown-menu empty field form input label pagination popover progress radio-group scroll-area select separator sheet skeleton slider sonner switch table tabs textarea toggle-group tooltip
```

> 按需精简：`chart`（P3 视 metrics 接入再装）、`form`（若不用 vee-validate 可不装，用 field/label 手写）、`checkbox`/`radio-group`（当前用量为 0，装了备用可移除）。
