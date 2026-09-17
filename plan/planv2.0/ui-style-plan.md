# SimpleBase UI 样式优化与美化计划 v2.0

> 目标目录：`ui/src`
> 技术栈：Vue 3.4 + TypeScript + Vite 5 + shadcn-vue（Reka UI + Tailwind CSS v4）+ pinia
> 制定日期：2026-07-29
> 前置版本：`plan/planv1.0/ui-refactor-plan.md`（已完成 API 抽象、Mock 层、Claude 暖橙主题）

## 目录

- [一、现状分析](#一现状分析)
- [二、问题清单](#二问题清单)
- [三、设计系统 v2.0](#三设计系统-v20)
- [四、实施任务清单](#四实施任务清单)
- [五、分阶段落地路线](#五分阶段落地路线)
- [六、验收标准](#六验收标准)
- [七、风险与不做的事](#七风险与不做的事)

## 一、现状分析

### 1.1 文件结构

```
ui/src
├── App.vue                    23 行   ConfigProvider + token（仅 9 个 token）
├── main.ts                    13 行   antd 全量引入
├── layouts/DefaultLayout.vue  236 行  Sider + Drawer + Header + Content
├── components/
│   ├── NavMenu.vue             83 行  硬编码 6 个菜单项
│   └── PageContainer.vue       35 行  与 theme.css 中 .sb-page 规则重复
├── pages/                      6 个页面，各 158~244 行，模板/脚本/样式混写
├── services/                   api 抽象层（v1.0 产物，本次不动）
├── styles/theme.css           313 行  唯一全局样式，含变量 + 工具类 + 响应式
└── utils/format.ts             格式化工具
```

### 1.2 已经做对的部分

| 项 | 说明 |
|---|---|
| CSS 变量体系 | `--sb-*` 命名统一，色板/圆角/阴影/过渡集中在 `:root` |
| antd token 联动 | `App.vue` 用 `ConfigProvider.theme.token` 覆盖主色，v4 正确姿势 |
| `color-scheme: light` | 规避 macOS 暗色模式串色 |
| 页面骨架一致 | 6 个页面统一 `PageContainer` + `a-card.sb-card` + `.sb-toolbar` |
| 移动端适配 | 768 / 480 两个断点，Sider 降级为 Drawer |
| 视觉细节 | 毛玻璃头部、菜单选中态、日志呼吸灯、流式光标闪烁 |

### 1.3 视觉语言现状

主色 `#d97757`（Claude 橙），背景 `#f5f4ef` 暖米色，标题衬线体 Georgia，
正文 -apple-system。整体是「Claude 官网风」，方向正确但执行不彻底。

## 二、问题清单

按严重程度排序，每条给出位置、现象、根因。

### P0 — 影响一致性与可维护性

| # | 问题 | 位置 | 现象 / 根因 |
|---|------|------|------------|
| 1 | 设计 token 与 antd token 双份维护、数值散落 | `styles/theme.css:8-46`、`App.vue:10-22`、`Dashboard.vue:96-121` | 主色 `#d97757` 在三处硬编码；Dashboard 统计卡的 `rgba(217,119,87,.12)` 等 8 个色值直接写在 TS 里，改主题需改 3 个文件 |
| 2 | `PageContainer` 与 `theme.css` 规则重复且冲突 | `PageContainer.vue:14-34` vs `theme.css:91-145` | 两处都定义 `.sb-page` / `.sb-page-title`，`h2` 字号一个 20px（scoped 生效）一个 24px、`font-family` 一处有衬线一处没有，实际渲染取 scoped，全局那份是死代码 |
| 3 | 缺少通用组件，样式在 6 个页面里复制 | `DataManager.vue:226-243`、`FaaSManager.vue:198-217`、`S3Manager.vue:150-157`、`Logs.vue` | `.sb-json` / `.sb-result` 两份几乎相同的代码块样式；`.sb-json-error` 在 2 个文件重复；等宽字体栈 `'JetBrains Mono', Menlo, Consolas` 在 5 处重复 |
| 4 | 等宽字体未真正引入 | 全局 | 声明了 `JetBrains Mono` 但项目未安装/未引入 webfont，实际 fallback 到 Menlo，跨平台（Windows）掉到 Consolas，代码块观感不统一 |
| 5 | 无暗色模式 | `theme.css:5` | `color-scheme: light` 强制亮色。Claude 官网本身有暗色，管理后台长时间看日志的场景更需要 |

### P1 — 影响观感与体验

| # | 问题 | 位置 | 现象 / 根因 |
|---|------|------|------------|
| 6 | 间距无标尺 | 全局 | 出现 `4/6/8/10/12/14/16/18/20` 九种间距，`--sb-*` 里没有 spacing 变量，卡片 padding（18px/20px/12px）、toolbar gap（12px/8px/6px）各页面不一致 |
| 7 | 字号无标尺 | 全局 | 出现 `11.5/12/12.5/13/14/16/17/20/24/26` 十种字号，无 `--sb-font-size-*` 变量 |
| 8 | `!important` 泛滥 | `theme.css:104-126`、`NavMenu.vue:58-82`、`DefaultLayout.vue:119-186` | 共 30+ 处 `!important` 覆盖 antd。根因是没用 antd 的组件级 token（`theme.components.Card/Menu/Layout/Table`），只能靠 CSS 强压 |
| 9 | Dashboard 图表是手写 div 柱状图 | `Dashboard.vue:40-61,179-215` | 无坐标轴、无网格线、无渐变，`barHeight` 里 `140px` 硬编码与 CSS 里的 `height:140px` 重复；数据为空时布局塌陷 |
| 10 | 表格未做视觉优化 | 6 个页面的 `a-table` | 全部用 antd 默认样式：无斑马纹、无粘性表头、行高偏挤、hover 反馈弱；分页配置 `{ pageSize:10, size:'small', showTotal }` 在 3 个页面复制 |
| 11 | 加载态只有 spin，无骨架屏 | `Dashboard.vue:5`、各页表格 | 首屏空白后突然填充，抖动明显 |
| 12 | 空态千篇一律 | 各页 `#emptyText` | 只有 `a-empty` + 一行文字，没有引导操作按钮（如「新建集合」） |
| 13 | 移动端仅靠媒体查询压缩 | `theme.css:215-313` | 表格在窄屏靠横向滚动，`white-space: nowrap` 导致 Key 列不可读；Header 面包屑在 Drawer 打开时无遮罩层级处理 |
| 14 | 焦点可见性缺失 | 全局 | 定义了 `--sb-glow` 但只在注释里提到，无 `:focus-visible` 规则；键盘 Tab 导航不可见 |
| 15 | 无内容宽度上限 | `DefaultLayout.vue:217-220` | `.sb-content { margin: 18px }` 在 27 寸屏上表格拉到 2000px+，阅读困难 |

### P2 — 细节与代码卫生

| # | 问题 | 位置 | 现象 / 根因 |
|---|------|------|------------|
| 16 | 未使用的导入 | `NavMenu.vue:42` | `ProjectOutlined` 导入未使用 |
| 17 | Sider 宽度用 `getComputedStyle` 同步读 CSS 变量 | `DefaultLayout.vue:91-98` | 在移动端 `--sb-sider-width` 被媒体查询改成 `0px`，`parseInt('0px')=0` 导致切回桌面端侧边栏宽度为 0；且只在 setup 执行一次，不响应断点变化 |
| 18 | 阴影色值不一致 | `Logs.vue:128` | `box-shadow: 0 0 6px rgba(48,209,88,.6)` 是绿色（旧 iOS 绿），与 `--sb-success #3f8a5a` 不符 |
| 19 | 硬编码灰色 | `Logs.vue:124`、`theme.css:83-87`、`Logs.vue:147` | `#9ca3af`、`#d8d5cc`、`#b5b2a8`、`#9a7b1a` 未纳入变量 |
| 20 | `sb-mock-tag` 在 2 处重复定义 | `DefaultLayout.vue:213`、`Dashboard.vue:176` | 相同规则重复 |
| 21 | 过渡用 `all` | `theme.css:38` | `--sb-transition: all 0.22s ease` 触发不必要重排，应按属性列举 |
| 22 | 图表 CSS 定义在错误位置 | `theme.css:300-312` | `.sb-chart-*` 的 480px 断点规则写在全局，但组件本体样式在 `Dashboard.vue` scoped 里，跨文件割裂 |
| 23 | 页面标题 + 图标三处维护 | `NavMenu.vue:9-32`、`DefaultLayout.vue:100-107` 的 `titleMap`、各页 `PageContainer title` | 同一份中文标题写三遍（菜单项、面包屑映射、页头），`router/index.ts` 无 `meta`，新增页面要改 3 个文件且容易不同步 |

## 三、设计系统 v2.0

### 3.1 设计原则

1. **单一数据源**：所有视觉常量只在 `styles/tokens.css` 定义一次，antd token 与页面样式都引用它。
2. **组件优先**：重复出现 2 次以上的样式块，抽成组件或全局工具类，不在 scoped 里复制。
3. **克制的动效**：只在状态切换（hover / 选中 / 加载 / 进出场）上加动效，时长 ≤ 240ms。
4. **可访问性兜底**：对比度 ≥ 4.5:1、`:focus-visible` 可见、`prefers-reduced-motion` 降级。
5. **不引入重型依赖**：图表用轻量 SVG 自绘，不引 ECharts；不引 UnoCSS/Tailwind。

### 3.2 Token 分层

拆分 `styles/theme.css`（313 行）为四个文件，职责单一：

```
styles/
├── tokens.css      设计变量（颜色/间距/字号/圆角/阴影/层级/动效）
├── base.css        reset、html/body、滚动条、focus-visible、reduce-motion
├── antd-patch.css  antd 组件样式补丁（尽量少，能用 token 解决的不写 CSS）
└── utilities.css   .sb-page / .sb-toolbar / .sb-card / .sb-code / .sb-terminal 等工具类
```

`main.ts` 按顺序引入：`tokens → base → antd reset → antd-patch → utilities`。

### 3.3 变量规范

| 类别 | 变量 | 取值 |
|------|------|------|
| 主色 | `--sb-primary` / `-hover` / `-active` / `-light` / `-border` | `#d97757` / `#c15e3c` / `#a94e30` / `rgba(217,119,87,.12)` / `rgba(217,119,87,.28)` |
| 语义色 | `--sb-success` / `-warning` / `-danger` / `-info` | `#3f8a5a` / `#c99a2c` / `#c0452f` / `#4c7d9e` |
| 语义色浅底 | `--sb-success-bg` / `-warning-bg` / `-danger-bg` / `-info-bg` | 对应色 12% 透明度，供 tag / 统计卡 / 日志行复用 |
| 中性 | `--sb-bg` / `-bg-soft` / `-bg-sunken` / `-surface` / `-surface-hover` | `#f5f4ef` / `#eeece5` / `#e7e4db` / `#fff` / `#faf9f5` |
| 边框 | `--sb-border` / `-border-soft` / `-border-strong` | 12% / 8% / 20% 的 `#1f1e1d` |
| 文字 | `--sb-text` / `-text-secondary` / `-text-muted` / `-text-inverse` | `#1f1e1d` / `#5f5e59` / `#8a8983` / `#faf9f5` |
| 间距 | `--sb-space-1..8` | `4 / 8 / 12 / 16 / 20 / 24 / 32 / 40` px（8pt 栅格，4px 为半格） |
| 字号 | `--sb-fs-xs..3xl` | `12 / 13 / 14 / 16 / 20 / 24 / 30` px |
| 行高 | `--sb-lh-tight` / `-normal` / `-relaxed` | `1.3 / 1.5 / 1.7` |
| 圆角 | `--sb-radius-xs..full` | `4 / 8 / 12 / 16 / 999` px |
| 阴影 | `--sb-shadow-xs..lg` + `--sb-ring` | 4 级暖灰阴影 + `0 0 0 3px var(--sb-primary-light)` |
| 字体 | `--sb-font-sans` / `-serif` / `-mono` | 系统栈 / Georgia 栈 / `ui-monospace, SFMono-Regular, Menlo, Consolas, monospace` |
| 层级 | `--sb-z-sider` / `-header` / `-drawer` / `-modal` | `10 / 20 / 900 / 1000` |
| 动效 | `--sb-ease` / `--sb-dur-fast` / `-dur` / `-dur-slow` | `cubic-bezier(.4,0,.2,1)` / `120ms` / `200ms` / `320ms` |
| 布局 | `--sb-sider-w` / `-sider-collapsed-w` / `-header-h` / `-content-max-w` | `232 / 64 / 56 / 1440` px |

### 3.4 暗色主题

`tokens.css` 中用 `[data-theme='dark']` 覆盖同名变量，仅换值不换名：

| 变量 | 亮色 | 暗色 |
|------|------|------|
| `--sb-bg` | `#f5f4ef` | `#1a1917` |
| `--sb-bg-soft` | `#eeece5` | `#232120` |
| `--sb-surface` | `#ffffff` | `#26241f` |
| `--sb-text` | `#1f1e1d` | `#f0eee6` |
| `--sb-text-secondary` | `#5f5e59` | `#a8a5a0` |
| `--sb-border-soft` | `rgba(31,30,29,.08)` | `rgba(255,255,255,.08)` |
| `--sb-primary` | `#d97757` | `#e08a6d`（暗底提亮，保证对比度） |

配套：`color-scheme` 随主题切换、antd `ConfigProvider` 传 `theme.algorithm = darkAlgorithm`、
主题选择持久化到 `localStorage`，首次访问跟随 `prefers-color-scheme`。
