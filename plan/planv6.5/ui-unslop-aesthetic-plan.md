# 前端审美改造：按 unslop 九遍法收敛 `ui/src`（planv6.5）

> **状态：** P0–P3 已执行（见文末「执行记录」）；P4 视觉回归（截图对比）未做
> **日期：** 2026-11-10
> **方法来源：** `skills/unslop/skills/unslop/SKILL.md`（CC BY 4.0，作者 Damir Sokolovsky / @heydamir，复制或改编须保留署名）及其 `references/{signs,tokens,visuals}.md`
> **审计方式：** 静态代码审计（ripgrep + 阅读关键文件）。**未运行应用、未截图**，视觉判断来自 class 与 token 推断；执行前须先截图建立基线（见 §6）
> **关联代码：** `ui/src/style.css`、`pages/Home.vue`、`pages/Dashboard.vue`、`layouts/{Default,Docs}Layout.vue`、`components/NavMenu.vue`、`components/TrendChart.vue`、`components/agent/*`、`locales/{zh-CN,en-US}.ts`、`ui/tests/Home.test.ts`

**Goal：** 把 `ui/src` 里「看起来像 AI 模板」的部分改成有取舍的设计，同时不改任何功能、信息架构与 `ui/AGENTS.md` 已定稿的布局规范。

**结论先行：** 控制台（`/console/*`）整体已经克制，属于 unslop 的 **light slop → Polish 模式**；公开首页 `Home.vue` 是 **heavy slop**，是本次改造的主战场。不要对控制台做大改。

---

## 0. 审计结果（证据）

### 0.1 总体分级

| 区域 | 级别 | 依据 |
| --- | --- | --- |
| `pages/Home.vue`（公开首页） | Heavy | 径向色晕 + 网格背景、blur 光晕、彩色阴影、大写加宽字距标签、角标装饰、假状态字样，集中在一个文件 |
| `layouts/DocsLayout.vue`、`DefaultLayout.vue` | Light | `backdrop-blur-xl` 粘性顶栏、`font-bold` 衬线 logo、「S」色块 logo |
| `pages/Dashboard.vue` | Light | 4 张等宽统计卡 + 彩色图标底块 + 大写标签，是典型模板形态 |
| `components/NavMenu.vue` | Light | 分组标题 `uppercase tracking-[0.08em] text-[11px]`，计数徽标为 `rounded-full bg-primary/10` |
| `components/TrendChart.vue` | 基本合格 | 用主题变量取色、两个系列、虚线网格；细节可调（见 §4） |
| 其余 `pages/*`、`components/*` | 基本合格 | 未发现渐变、光晕、emoji、`transition-all`、`hover:scale`、硬编码 Tailwind 色板（全 `src` 仅 1 处） |
| `components/ui/**` | 库代码 | 按 `ui/AGENTS.md` 视为本地 fork，不在本次范围；唯一例外见 §5 决策 4 |

### 0.2 逐条命中（对照 `signs.md`）

**Surfaces / Color**

| # | 位置 | 现象 | 对应 sign |
| --- | --- | --- | --- |
| C1 | `Home.vue:3` | `radial-gradient(... color-mix(--brand 13%) ...)` 径向色晕铺在首屏背景 | Radial color washes |
| C2 | `Home.vue:4` | 52px 线性渐变网格 + mask 渐隐，作为 hero 背景 | Floating/glowing grids |
| C3 | `Home.vue:49` | 预览窗后 `bg-brand/10 blur-3xl` 光晕 | Colored glows |
| C4 | `Home.vue:34`、`:50` | 彩色阴影 `shadow-[0_12px_32px_-16px_var(--primary)]`、`shadow-[0_24px_80px_-40px_...]` | Colored shadows |
| C5 | `Home.vue:72-73` | 预览窗对角两个 `border-primary/60` 角标 | Decoration with no meaning |
| C6 | `style.css` | `--brand: #3b82f6` 与 `--primary: #2563eb` 同时存在，两个蓝都在界面里露面（`Home.vue:42-44` 的圆点用 brand，其余用 primary） | 多个近似色 |
| C7 | `style.css` 深色 | `--background #0b1220`、`--card #111c2e`、`--border #29374b` 整套偏海军蓝；亮色 `#f8fafc` 为 slate 蓝灰 | 应用表面应保持中性，表面不应读成某种颜色 |
| C8 | 多处 | 大量半透明叠加：`bg-primary/10`、`border-primary/25`、`bg-card/95`、`bg-background/70` | 同色相多档色调 |

**Type**

| # | 位置 | 现象 |
| --- | --- | --- |
| T1 | `Home.vue:24`、`:80` | `uppercase tracking-[0.12em]` / `tracking-[0.2em]` 的 eyebrow 标签 |
| T2 | `Home.vue:52,60,65,67` | 预览窗里 `font-mono text-[10px]` 全大写 + `tracking-widest/wider`，10px 低于 11px 下限 |
| T3 | `Dashboard.vue:21` | 统计卡标签 `uppercase tracking-wide` |
| T4 | `NavMenu.vue:5` | 侧栏分组标题 `uppercase tracking-[0.08em]` |
| T5 | `ui/table/TableHead.vue:13` | 表头 `uppercase tracking-wider`（库代码，见 §5 决策 4） |
| T6 | `Home.vue:10,27`、`DefaultLayout.vue:9`、`DocsLayout.vue:8` | 衬线（Georgia 回退）上 `font-bold`/`font-semibold` + 负字距 `tracking-[-0.045em]`；标题字重 600/700 |
| T7 | 全 `src` | `text-[Npx]` 任意字号 12 个文件（Home 6、DocsSearch 4、GoFuncTestModal 4…）；`font-semibold/bold` 18 个文件 |
| T8 | `style.css:1` | 通过 Google Fonts 在线加载 Inter 且带 700 字重，离线/内网环境会回落系统字体 |
| T9 | 全 `src` | `tabular-nums` 仅 5 个文件使用，Dashboard 的请求数/耗时/配额等数字多数未对齐 |

**Icons / Details**

| # | 位置 | 现象 |
| --- | --- | --- |
| I1 | `Dashboard.vue:14-19, 228-250` | 统计卡左侧 48px 彩色图标底块（`bg-primary/12`、`bg-success/12`、`bg-destructive/10`）：图标装饰了数字 |
| I2 | `Dashboard.vue:11-27` | 4 张宽度相同、结构相同的卡并排，无主次 |
| I3 | `Home.vue:56`、`:67` | 伪标签 `PRODUCT PREVIEW`、每行硬编码 `READY`：预览里的状态是写死的，不是产品数据 |
| I4 | `Home.vue:85` | 能力卡的图标底块（`border-primary/20 bg-primary/10`），3 张卡同构 |
| I5 | `NavMenu.vue:28` | 导航徽标是 `rounded-full bg-primary/10` 药丸 |
| I6 | `locales/*`：`✓ 下次 {time}` | 少量字符图标混入文案，应改线性图标或去掉 |
| I7 | `DefaultLayout.vue:6`、`Home.vue:9`、`DocsLayout.vue:7` | logo 是「S」字母色块，三处各写一遍，尺寸/圆角/阴影不一致（`size-8 rounded-md` / `size-9 rounded-lg shadow-sm` / `size-8 rounded-lg shadow-sm`） |

**Copy**

| # | 位置 | 现象 |
| --- | --- | --- |
| P1 | `Home.vue:25, 52, 53, 56, 60, 65, 67, 80` | 8 处硬编码英文，**绕过 i18n**（`SIMPLEBASE / WORKSPACE`、`OVERVIEW`、`PRODUCT PREVIEW`、`AGENTS/DATABASE/STORAGE/FUNCTIONS`、`CAPABILITY MAP`、`READY`、`BUILT FOR YOUR WORKFLOW`），违反 `ui/AGENTS.md` 的唯一文案来源约定。`locales` 里已有 `capabilitiesEyebrow` 却没被使用 |
| P2 | `zh-CN.ts:266-289` | 「AI 工作台」「AI OS · 云助手」重复出现 3 次（标题高亮、tag、能力卡标题），属于「把 AI 贴满」；`tagObservability` 的 key 与文案「低成本」对不上 |
| P3 | `zh-CN.ts:269` | 单句超过 90 字的 hero 描述，混合产品能力、定价口径与免责 |
| P4 | `Home.vue:41-45` | 三个圆点标签「AI OS · 云助手 / SaaS 模块 / 低成本」是并列口号而非事实 |

**Layout / Visuals**

| # | 位置 | 现象 |
| --- | --- | --- |
| L1 | `Home.vue:22-75` | 左文右图的 split 结构本身是合理的，问题在右侧：它是一张**用占位字样拼出的假窗口**，不是真实产品画面 |
| L2 | `Home.vue:77-91` | 3 张同构能力卡并排，文案抽象 |
| V1 | `Home.vue` 全页 | 没有任何「真实的一个瞬间」：没有真实图表、真实资源列表或真实对象 |

**Motion**

| # | 位置 | 现象 | 判断 |
| --- | --- | --- | --- |
| M1 | `agent/ConversationView.vue:48-50` | 三点 `animate-bounce`（输入中指示） | 属于实时状态，可保留；但 bounce 是弹跳曲线，建议改为 opacity 脉冲。**低优先级** |
| M2 | `agent/AgentToolCard.vue:30` | 运行中文案 `animate-pulse` | 是真实运行状态，**保留** |
| M3 | `style.css:328-339` | `.sb-fade-*` 的 `transition: opacity, transform 0.18s ease` | 合规（只动 opacity/transform），仅缓动曲线可统一 |
| M4 | 全 `src` | 未发现 `transition-all`、`hover:scale`、视差、循环浮动 | 无需处理 |
| M5 | `DefaultLayout.vue:47`、`DocsLayout.vue:3`、`DocsWiki.vue:13` | 三处粘性顶栏 `backdrop-blur-xl` | 大模糊在弱设备上卡顿；顶栏改实色 + 发丝线即可 |

### 0.3 做得对、不要动的部分

- 主题全部走 CSS 变量，无硬编码色板；图表取色经 `cssVar(...)`。
- 无渐变文字、无 emoji、无 `!`、无结尾句号、无 `Search anything...` 式占位。
- 焦点环与 `prefers-reduced-motion` 已处理。
- 已定稿并受测试保护的规范（侧栏 `h-11`/`sm:h-9`、`gap-1.5`、主区铺满、右上角顺序、单行 subtitle）**全部保持**。侧栏 `sm:h-9` 即 36px，已落在 unslop 建议的 32–36 区间内。

---

## 1. 设计方向（草案，**须用户确认后再动手**）

按 SKILL 要求先写五行方向；这是从产品本身推导出来的，不是趋势。

- **Product：** 面向 SaaS 开发者的云端数据库服务，把数据库、对象存储、云函数和云助手放进同一个项目控制台。
- **Character：** 精确、克制、带一点工程仪表感。不使用 clean / modern / sleek 这类词。
- **World：** 机房与仪表盘，读数、状态灯、单文件快照。中性色阶偏冷灰（unslop 的 Ink / Graphite 之间），不偏蓝。
- **Moments：** ① Dashboard 的请求趋势主图（全屏最大、画得最用心）；② Home 右侧预览改成**真实控制台的一个缩小切片**（真实的请求趋势 + 一张项目资源表），不再用假字样拼窗口。
- **Signature：** 「状态点 + 等宽读数」。写入实例是否可写、数据库是否就绪、云助手是否运行中，统一用同一个 6px 状态点配 mono 数字，只出现在状态数据旁，不当装饰用。

**强调色：** 沿用现有蓝（`--primary #2563eb`，它是真实品牌色且不是渐变），合并掉 `--brand`，只用在主按钮、选中态和图表主系列。深色主按钮保留蓝，不改成近白（避免引入新的视觉语言）。

---

## 2. 改造范围与分期

按 SKILL 的九遍顺序（间距 → 色 → 图标 → 文案 → 字 → 布局 → 细节 → 视觉 → 动效）逐遍做，每一遍做完整个目标页面再进入下一遍。分四期，**P0 单独可交付**。

### P0 · 公开首页 `Home.vue`（Heavy）

| 遍 | 动作 | 解决 |
| --- | --- | --- |
| 1 间距 | 把 `h-[760px]`、`h-[850px]`、`lg:min-h-[660px]`、`max-w-[520px]` 等任意值换成 4px 阶梯；hero 上内边距统一 96/128（桌面）、48/64（移动） | T7 |
| 2 色 | 删 C1、C2、C3、C4、C5；预览窗改为 `bg-card` + 发丝线 + 中性小阴影；`border-primary/25` 之类半透明叠加合并为发丝线 `border-border`；圆点统一用 primary | C1–C6、C8 |
| 3 图标 | 能力卡去掉图标底块，仅留线性图标（lucide 16/18px、1.5 描边、muted 色） | I4 |
| 4 文案 | 全部硬编码英文迁入 `locales`（两棵树同构）；`capabilitiesEyebrow` 要么启用为小写 sentence-case 标签，要么删除；hero 描述拆成「一句价值 + 一句成本口径」；「AI OS」「AI 工作台」收敛为一处；`tagObservability` 的 key 与文案对齐 | P1–P4 |
| 5 字 | 标题字重 600/700 → 500；去掉标题负字距 `-0.045em`（超大标题上限 `-0.03em`）；eyebrow 改 sentence case 且去加宽字距；`text-[10px]` 一律提到 ≥ 12px | T1、T2、T6、T7 |
| 6 布局 | 保留左文右图；右侧预览占 ≥ 一半宽；主按钮 1 个 + 次按钮 1 个（已满足）；删除三枚圆点口号行 | L1、P4 |
| 7 细节 | 预览窗圆角按嵌套公式（外 16 / 内 8）；伪标签 `PRODUCT PREVIEW`、写死的 `READY` 全部删除 | I3 |
| 8 视觉 | 预览改为真实控制台切片：复用 `TrendChart`（用固定的、看起来合理的示例点位，不请求后端）+ 一张 3–4 行的资源表。不生成图片 | V1、L2 |
| 9 动效 | 仅保留颜色过渡；首屏最多一次 6–12px 的淡入上移，stagger ≤ 6 项 | M4 |

> Home 不是应用页，允许把表面向「世界」微调，但本方案不这么做：保持与控制台同一套中性色阶，避免首页与控制台像两个产品。

### P1 · Token 层 `style.css`

1. 合并 `--brand` 到 `--primary`（先全仓检索 `bg-brand` / `text-brand` / `--color-brand` 的引用，再替换并删除）。
2. 亮/暗色阶改为一条低饱和中性斜坡：背景、卡片、凸起面、发丝线、三档文字色（primary / secondary / muted）。**具体色值在方向确认后用 `tokens.md` 的 Ink 或 Graphite 起步**，保持文字对比度：muted ≥ 4.5:1，accent 上文字 ≥ 4.5:1。
3. `--radius` 从 `0.75rem` 回到一组明确阶梯：控件 8–10、卡片 12–16、面板 14–20；弹窗内嵌套用「外半径 − 内边距」。
4. 新增动效 token：一条缓动曲线 `cubic-bezier(0.22, 1, 0.36, 1)`，三档时长 150 / 200 / 300ms；`.sb-fade-*` 改用该曲线。
5. 字体：见 §5 决策 2。无论选择哪个，统一为「一个界面字族 + 一个等宽字族」，数字场景加 `tabular-nums`，去掉 Google Fonts 在线依赖。

### P2 · 控制台外壳与 Dashboard（Polish）

| 对象 | 动作 |
| --- | --- |
| Logo（I7） | 抽成一个共享的小组件，三处复用；纯文字 `SimpleBase` + 一个纯色平面标记，去掉 `shadow-sm` 与 `font-bold` |
| 顶栏（M5） | 三处粘性顶栏去掉 `backdrop-blur-xl`，改实色背景 + 发丝线 |
| `NavMenu`（T4、I5） | 分组标题改 sentence case、常规字距、`text-xs`；计数徽标改成安静的 mono 文本，不用药丸；**不动 `h-11`/`sm:h-9`/`gap-1.5`** |
| Dashboard 统计卡（T3、I1、I2） | 去掉图标底块；卡内只留「标签 / 大数字（`tabular-nums`）/ 变化」；4 张等宽卡改成「3–4 张紧凑数字行 + 下方一张宽图」的节奏，**请求趋势主图成为屏幕上最大的元素** |
| Dashboard 其余数字（T9） | 请求数、耗时分位、配额、资源数量统一 `tabular-nums` 与同一数字格式 |
| 状态语义色 | success / destructive / warning 保留，但色调向中性斜坡靠拢，仅承载含义 |
| 资源汇总表 | 保持；行高统一，数量列右对齐 |

### P3 · 图表与细节

- `TrendChart`：一个主系列用 accent，错误系列用语义红；网格线弱化为发丝线；轴标签与 tooltip 用 mono + `tabular-nums`，字号 ≥ 11px；`areaStyle` 保留低透明度或去掉；图例改为小色点而不是旋转 45° 的菱形。
- 所有 `rounded-full` 药丸：保留头像/状态点，计数与标签改为 4–6px 圆角或纯文本。
- 键盘快捷键（若有）改小键帽样式。
- `ConversationView` 的 `animate-bounce`（M1）改为 opacity 脉冲，错开 150ms；`AgentToolCard` 的 `animate-pulse`（M2）保留。
- 清理 `locales` 中的字符图标（I6）。
- 其余 `text-[Npx]` 任意值（T7）逐个核对：能落到 12 / 13 / 14 的收敛，不能的（如 `GoFuncTestModal` 的终端式布局）保留并注明原因。

### P4 · 验证与回归

见 §6。

---

## 3. 不做的事（边界）

- 不改 `ui/AGENTS.md` 里已定稿的布局规范与右上角顺序，不加 `max-w-*`。
- 不改 `package.json` / `tsconfig.json` / `vite.config.ts`，不新增依赖（字体方案受此约束，见决策 2）。
- 不动 `src/components/ui/**`，除非决策 4 获批，且只改单个文件的 class。
- 不引入新的装饰来替代旧装饰：不换成 glass / grain / noise / neon / bento。
- 不生成 AI 图片：Home 的「瞬间」由代码里的真实控制台切片承担；若日后要做场景图，另开计划并先确认工具与费用。
- 不改信息架构、不删功能、不动路由与权限判定。
- 新增 `interface`，不用 `type`；新文案同时补两棵语言树。

---

## 4. 逐文件改动清单（预估）

| 文件 | 改动 | 优先级 |
| --- | --- | --- |
| `ui/src/pages/Home.vue` | 重写 template 的视觉层；预览改真实切片；文案外置 | P0 |
| `ui/src/locales/zh-CN.ts`、`en-US.ts` | 新增 Home 预览与标签文案，收敛重复口号；删除废 key | P0 |
| `ui/tests/Home.test.ts` | 现有断言绑定了 `AGENTS云助手`、`01云助手READY` 等**即将删除的假字样**和 `.space-y-3` 结构，须按新结构重写 | P0 |
| `ui/src/style.css` | token 合并、色阶、圆角阶梯、动效 token、字体 | P1 |
| 共享 `BrandLogo.vue`（新增） | 统一三处 logo | P2 |
| `layouts/DefaultLayout.vue`、`DocsLayout.vue`、`pages/DocsWiki.vue` | 顶栏去模糊；logo 复用 | P2 |
| `components/NavMenu.vue` | 分组标题与徽标 | P2 |
| `pages/Dashboard.vue` | 统计卡与网格节奏；`tabular-nums` | P2 |
| `components/TrendChart.vue` | 图例、轴字体、配色细节 | P3 |
| `components/agent/ConversationView.vue` | 输入中指示动效 | P3 |

---

## 5. 需要你拍板的决策

1. **设计方向**：§1 的五行是否认可？尤其是「状态点 + 等宽读数」作为签名细节，以及是否沿用现有蓝作为唯一强调色。SKILL 要求方向确认前不动手，因为中途改方向等于返工。
2. **字体**：仓库约束是不新增依赖。可选：
   - A. 继续 Inter，但改为本地自托管文件（需要把字体文件放进 `ui/public` 或 `src/assets`，属于新增资源但不是依赖）；
   - B. 只使用系统字体栈，去掉 Google Fonts，零新增；
   - C. 换 Geist / IBM Plex Sans，需要自托管文件。
   - 建议 **B 起步**，改动最小且离线可用；如果想有更强的个性再做 A 或 C。
3. **深色底色**：现在是海军蓝（`#0b1220`）。是否同意改为近中性的深灰蓝？这会是用户可见度最高的一处变化，会影响所有页面截图。
4. **`TableHead.vue` 的 `uppercase tracking-wider`**：它在 `components/ui/` 下，按约定是库代码。是否允许就这一处改成 sentence case？不改的话，表头会和侧栏分组标题风格不一致。
5. **Home 预览的数据来源**：用固定示例点位（建议，纯展示、不请求后端），还是读取真实演示项目？前者无副作用。
6. **宣传用语**：「低成本」「AI OS」是营销口径。保留的话建议写成可核对的具体陈述（例如「持久化数据存放在对象存储」），你来确认是否有可引用的事实依据。

---

## 6. 验证方案

**基线（动手前）：** 在亮/暗两种主题下，对 `/`、`/console/dashboard`、`/console/agents`、`/console/databases`、`/docs` 各截 1 张 1440 宽、1 张 390 宽，存入 `output/ui-unslop/before/`。

**每期结束：**

```bash
cd ui && yarn build        # 必须通过
cd ui && yarn test         # 必须通过；coverage 阈值 95%，新增组件要补测（BrandLogo、Home 新结构）
```

- `ui/tests/i18n.test.ts`：两棵语言树 key 集合必须一致，裸 `@` 要写成 `{'@'}`。
- `ui/tests/ui-state-attributes.test.ts`、`modal-input-layout.test.ts`：不应受影响，须保持通过。
- `yarn typecheck` 当前已知有 241 处 TS2307（缺 `*.vue` shim），**以 `yarn build` 为准**，不拿它作为本次验收项。

**视觉验收（对照 SKILL checklist）：**

- [ ] 间距全部落在 4px 阶梯；同类行同高
- [ ] 一条中性色阶 + 三档文字色 + 一个强调色；无渐变、光晕、彩色阴影、色块背景
- [ ] 无 emoji / 字符图标；图标一套、一个尺寸、一个描边
- [ ] 无大写加宽字距标签；标题字重 ≤ 500；小字号无负字距；≥ 11px
- [ ] 数字 `tabular-nums` 且格式统一
- [ ] 圆角嵌套成立
- [ ] 动效只动 `color / background / border / opacity / transform`，一条缓动曲线
- [ ] Home 预览是真实产品切片，没有占位标签
- [ ] **模板检验：** 把产品名换成另一个云数据库产品，首页还成立吗？成立就继续加重方向（文案、预览、签名细节）
- [ ] 亮/暗、桌面/移动均无回归；`prefers-reduced-motion` 仍生效

**回滚：** 每期单独提交；P1 的 token 变更是全局的，若截图对比出现意外，只回退 `style.css` 即可还原整体观感。

---

## 7. 建议执行顺序

1. 确认 §5 决策 1–3（其余可在对应期开始前再定）。
2. 截基线。
3. 先做 P1 token，再做 P0 Home（Home 依赖新 token）。
4. P2 → P3。
5. 全量验证并更新本文档状态。

## 8. 执行记录（2026-11-10）

§5 的决策在用户未逐项回复的情况下，按计划里的建议值执行：沿用现有蓝为唯一强调色；字体用系统字体栈（决策 2 选 B）；深色改为近中性（决策 3 同意）；Home 预览用固定示例数据（决策 5）；不动 `components/ui/**`（决策 4 未批准）。

**已完成**

| 期 | 内容 | 文件 |
| --- | --- | --- |
| P1 | 去掉 Google Fonts；合并并删除 `--brand`；亮/暗色阶改为低饱和中性斜坡（`#fafafa`/`#171717`，`#141415`/`#ededed`），强调色仅 `--primary`；默认过渡曲线 `cubic-bezier(0.22,1,0.36,1)` / 150ms；`.sb-fade-*` 统一曲线；新增 `.sb-dot-pulse`（减少动态时静止可见） | `style.css` |
| P1 | Monaco 主题跟随新色阶 | `GoMonacoEditor.vue`（含测试断言） |
| P0 | Home 去掉径向色晕、网格、`blur-3xl` 光晕、彩色阴影、角标、大写字距标签、三枚口号圆点、假状态字样；预览改为真实控制台切片（`TrendChart` + 资源表，示例数据）；全部英文硬编码迁入 i18n；标题字重 500、去掉衬线与负字距；hero 描述缩短且只保留可核对的陈述 | `Home.vue`、`locales/*` |
| P2 | 抽出 `BrandLogo`，三处复用；两个粘性顶栏 + DocsWiki 页签栏去掉 `backdrop-blur-xl`；侧栏分组标题 sentence case、导航徽标改安静的 mono 文本、选中态去阴影（`h-11` / `sm:h-9` / `gap-1.5` 未动） | `BrandLogo.vue`（新）、`DefaultLayout.vue`、`DocsLayout.vue`、`DocsWiki.vue`、`NavMenu.vue` |
| P2 | Dashboard 统计卡：去图标底块与大写标签，改为「状态点 + 标签 + `tabular-nums` 大数字」；分位数、资源数量字重降为 500 | `Dashboard.vue` |
| P3 | `TrendChart`：图例改小圆点；两个系列同为实线与圆点；网格改实线发丝线；轴字体 mono、12px；新增 `hideControls` | `TrendChart.vue` |
| P3 | 输入中三点 `animate-bounce` → 透明度脉冲；项目切换器药丸/强调色图标 → 安静文本；文档侧栏/目录/页选择器大写字距标题 → sentence case；`GoFuncTestModal` 5 处大写字距标题；全站 `text-[10px]`、`text-[11px]`（`components/ui/**` 之外）统一到 `text-xs` | 多个组件 |
| 测试 | 重写 `Home.test.ts`（真实预览切片、无假字样、无光晕/渐变/模糊）；新增 `BrandLogo.test.ts`；`TrendChart.test.ts` 补 `hideControls` 与线型用例；`DocsWiki.test.ts` 同步边框断言 | `ui/tests/*` |

**验证结果**

- `yarn build`：通过。
- `vitest`：855 通过、1 失败。唯一失败是 `docs-content.test.ts` 的断链（`getting-started/local-cloud-collab.md` 里的 `/docs/cli` 指向不存在的 `docs/cli`）。我用 `git stash` 回到改动前复跑，确认**该失败在改动前就存在**，与样式无关，未处理。
- 本次涉及文件的覆盖率：`BrandLogo` / `Home` / `NavMenu` / `DocsLayout` 100%，`TrendChart` 语句 100% / 分支 91.66%，`Dashboard` 语句 98.33% / 分支 77.77%（未覆盖行是既有加载分支，非本次新增）。全量覆盖率阈值未能读到：vitest 在有失败用例时不输出覆盖率，需先修掉上面的文档断链后再核对。

**与计划的差异**

- 计划 §2 P0 写的「首屏最多一次淡入上移」没有做，首页目前没有任何入场动画，只保留颜色过渡。
- 计划 §3 说 `TrendChart` 的 `areaStyle` 可保留低透明度，已从 0.08 降到 0.06。
- `GlobalProjectSwitcher` 里表示系统项目的 `★` 被 `GlobalProjectSwitcher.test.ts` 断言为文本，所以只把药丸样式改成安静文本，字符本身保留；是否改成线性图标留待后续。
- 计划里的 `--radius` 阶梯没有调整：`0.75rem` 下卡片 12px、控件 10/8px，已落在建议区间内。

**未完成 / 待你确认**

1. **P4 视觉回归**：没有跑应用、没有截图。亮/暗两套新色值（尤其深色的 `#141415` 底与 `#60a5fa` 主色对比度）、Home 预览的实际观感都只做了代码层核对，需要你本地看一眼。`./build.sh dev` 起服务后检查 `/`、`/console/dashboard`、`/console/agents`、`/docs`。
2. **`TableHead.vue` 的 `uppercase tracking-wider`**（决策 4）仍未改，表头与新的 sentence case 标题风格会不一致。
3. **`internal/web/dist` 未同步**：我只跑了 `yarn build`（输出到 `ui/dist`），没有执行 `./build.sh`，所以 embed 目录仍是旧产物。要在 Go 二进制里看到新样式，需要你跑 `./build.sh`。
4. 「低成本」「AI OS」等营销用语（决策 6）：本次只去重，没有核实事实依据。

署名：本计划采用的审计框架与改造流程来自 unslop（Damir Sokolovsky，[@heydamir](https://x.com/heydamir)，CC BY 4.0）。
