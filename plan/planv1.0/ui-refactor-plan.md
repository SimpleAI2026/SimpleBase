# SimpleBase UI 重构 / 优化 / 美化计划

> 目标目录：`ui/src`
> 更新日期：2026-07-25

## 一、总体目标

1. **API 抽象**：页面不直接依赖 axios，统一经由 `services/api` 访问后端，接口有完整 TypeScript 类型。
2. **Mock 先行**：通过环境变量 `VITE_USE_MOCK` 一键切换 mock / 真实后端，前端可脱离后端独立开发。
3. **美化与体验优化**：统一视觉语言（卡片、表格、终端、状态反馈），操作有 loading / message 反馈。

## 二、任务清单与状态

| # | 任务 | 产出 | 状态 |
|---|------|------|------|
| 1 | API 类型抽象层 | `services/types.ts`（`Api` 接口 + 领域模型类型） | ✅ 已完成 |
| 2 | http 封装强化 | `services/http.ts`（baseURL 环境变量、统一错误信息） | ✅ 已完成 |
| 3 | 真实后端实现迁移 | `services/http-api.ts`（实现 `Api` 接口） | ✅ 已完成 |
| 4 | Mock 数据实现 | `services/mock.js` + `mock.d.ts`（内存状态、增删查改闭环、日志流模拟） | ✅ 已完成 |
| 5 | API 入口按环境切换 | `services/api.ts`（导出 `api`、`isMock`） | ✅ 已完成 |
| 6 | 环境变量配置 | `.env.development`（mock 开）、`.env.production`（mock 关）、`.env.example`、`vite-env.d.ts` | ✅ 已完成 |
| 7 | 通用工具函数 | `utils/format.ts`（formatBytes / formatTime / formatJson） | ✅ 已完成 |
| 8 | Dashboard 美化 | 统计卡重构为数据驱动、新增近 7 天请求趋势 CSS 柱状图、Mock 标记 | ✅ 已完成 |
| 9 | DataManager 美化 | 列配置 + `bodyCell` 插槽、JSON 高亮预览、JSON 实时校验、删除二次确认、message 反馈 | ✅ 已完成 |
| 10 | S3Manager 美化 | 大小/时间格式化、删除二次确认、上传成功提示、Key 等宽字体 | ✅ 已完成 |
| 11 | ProjectManager 美化 | 新增/编辑合并为同一弹窗、项目头像、描述与创建时间列、loading 反馈 | ✅ 已完成 |
| 12 | FaaSManager 美化 | 部署 loading、版本 tag、调用结果面板、JSON 校验 | ✅ 已完成 |
| 13 | Logs 美化与抽象 | `api.logs.connect` 抽象（mock 可模拟推送）、级别着色、连接状态呼吸灯、日志上限截断 | ✅ 已完成 |
| 14 | 布局 Mock 标识 | `DefaultLayout.vue` 头部在 mock 模式下显示「Mock 数据」标签 | ✅ 已完成 |
| 15 | Lint / 构建验证 | `read_lints` 无错误 + `vite build` 通过 | ✅ 已完成 |
| 16 | 日志级别着色兼容多格式 | 前端已兼容 `[LEVEL]` / `LEVEL` / `level=xxx` 三种格式 | ✅ 前端就绪，待后端联调 |
| 17 | 趋势图对接真实 `/metrics/trend` 接口 | 前端已调用且独立容错（接口失败不影响统计卡） | ✅ 前端就绪，待后端实现 |
| 18 | 清新白主题色板 | `theme.css` 重写：清透蓝 `#4d7cfe` + 青 `#22d3ee`，偏白冷灰中性色 | ✅ 已完成 |
| 19 | 全局光影 | 背景径向光晕、柔和分层阴影、卡片 hover 浮起、品牌色 glow | ✅ 已完成 |
| 20 | 侧边栏亮色化 | 白色侧边栏 + 白色品牌区，菜单 `light` 主题，选中态渐变 + 光晕 | ✅ 已完成 |
| 21 | 头部毛玻璃 | 半透明白 + `backdrop-filter` 模糊 | ✅ 已完成 |
| 22 | 终端亮色化 | 日志终端改浅色渐变底，级别着色适配亮色 | ✅ 已完成 |
| 23 | 页面配色焕新 | Dashboard 统计卡/图表渐变、FaaS 调用结果面板亮色化 | ✅ 已完成 |
| 24 | 修复：品牌区白字白底不可见 | `.sb-brand-text` 改 `--sb-text`、品牌区分隔线改 `--sb-border-soft` | ✅ 已完成 |
| 25 | 修复：antd v4 主色未生效 | 改用 `ConfigProvider` `theme.token` 配置（v4 为 CSS-in-JS，`--ant-primary-color` 变量无效） | ✅ 已完成 |
| 26 | 修复：系统暗色模式串色 | `color-scheme: light` 固定亮色，避免原生控件/滚动条变暗 | ✅ 已完成 |
| 27 | 修复：浅色文字对比度不足 | `--sb-text-muted` 等浅灰在白底上对比度提升（≥ 4.5:1） | ✅ 已完成 |
| 28 | 修复：布局头部 Mock 标签缺失 | `DefaultLayout.vue` 头部补「Mock 数据」标签（任务 14 未落地，`isMock` 已导入未使用） | ✅ 已完成 |

## 二.五、清新白风格设计规范

| 要素 | 取值 | 说明 |
|------|------|------|
| 主色 | `#4d7cfe` 清透蓝 | 按钮、选中态、图表主色 |
| 点缀色 | `#22d3ee` 青 | logo 渐变、耗时卡 |
| 背景 | `#f6f8fc` + 双径向光晕 | 蓝/青淡光晕营造空气感 |
| 表面 | `#ffffff` | 卡片、侧边栏、头部 |
| 边框 | `#f1f4f9` 极浅 | 弱化分割感 |
| 阴影 | 三层柔和阴影 + `rgba(77,124,254,.14)` 光晕 | 卡片 hover 浮起 |
| 特效 | 头部毛玻璃 blur(10px)、菜单选中渐变发光、连接状态呼吸灯 | 点到为止 |

## 二.六、样式问题根因分析（2026-07-25 排查）

现象：部分区域全白、文字看不见；部分区域仍是黑色 / 默认色，风格不统一。

| # | 根因 | 位置 | 现象 |
|---|------|------|------|
| 1 | 暗色主题残留：品牌文字 `color:#fff`、分隔线 `rgba(255,255,255,.06)` 未随侧边栏亮色化同步修改 | `DefaultLayout.vue` `.sb-brand-text` / `.sb-brand` | 侧边栏品牌名「SimpleBase」白字白底完全看不见；分隔线不可见 |
| 2 | antd v4 主色覆盖方式错误：v4 采用 CSS-in-JS + design token，`--ant-primary-color` 是 v3 的变量，已无效 | `styles/theme.css:38-39` | 主按钮、链接、选中态仍是 antd 默认蓝 `#1677ff`，与设计稿 `#4d7cfe` 不一致，出现「有的地方颜色不对」 |
| 3 | 未声明 `color-scheme: light`：macOS 系统暗色模式下原生滚动条、表单控件按暗色渲染 | `styles/theme.css` `:root` | 「有些地方还是黑色」的混搭感 |
| 4 | 浅灰文字对比度不足：`--sb-text-muted #a3aec4` / `--sb-text-secondary #71809b` 在白底上偏淡 | `styles/theme.css` 及各页面 `.sb-hint` `.sb-empty` 等 | 提示文字「白成一片看不见」 |
| 5 | 任务 14 未真正落地：布局头部无「Mock 数据」标签，`isMock` 导入未使用 | `layouts/DefaultLayout.vue` | mock 模式无全局标识，且存在未使用导入 |

### 修复方案

1. **品牌区**：`.sb-brand-text` 改 `color: var(--sb-text)`；`.sb-brand` 分隔线改 `1px solid var(--sb-border-soft)`。
2. **antd v4 主题统一**：删除 `theme.css` 中无效的 `--ant-primary-color` 变量；在 `App.vue` 外层包 `a-config-provider`，配置 `:theme="{ token: { colorPrimary: '#4d7cfe', colorInfo: '#4d7cfe', borderRadius: 8, colorBgLayout: '#f6f8fc', colorText: '#2b3a55' } }"`，表格/弹窗/分页等组件随之统一。
3. **固定亮色**：`:root` 增加 `color-scheme: light;`。
4. **对比度**：`--sb-text-secondary` 调深至 `#5b6b87`、`--sb-text-muted` 调深至 `#8593ad`（白底对比度 ≥ 4.5:1）。
5. **Mock 标签**：`DefaultLayout.vue` 头部右侧补 `<a-tag v-if="isMock" color="orange">Mock 数据</a-tag>`。

### 验证

`yarn dev` 逐页检查：侧边栏品牌名可见、主按钮为 `#4d7cfe`、系统暗色模式下界面不变色、提示文字清晰可读、`read_lints` 无未使用导入告警。

## 二.七、Apple 大厂风格二次美化计划（2026-07-25）

### 现状审查（问题清单）

当前主题偏「清新 SaaS 风」：高饱和渐变图标 + 彩色光晕阴影 + 品牌渐变 logo + 菜单选中态强渐变发光。与 Apple（apple.com / macOS System Settings / Human Interface Guidelines）风格的差距：

| # | 问题 | 位置 | Apple 风格应为 |
|---|------|------|----------------|
| 1 | 主色 `#4d7cfe` 偏卡通蓝，非苹方系统蓝 | `theme.css :root` | 采用 Apple 官网 CTA 蓝 `#0071e3` / 系统蓝 `#0a84ff` |
| 2 | 背景双径向彩色光晕，视觉噪音大 | `theme.css body` | 苹果官网/后台惯用纯色浅灰背景 `#f5f5f7`，不做彩色光斑 |
| 3 | 统计卡图标为高饱和渐变色块 + 彩色投影 | `Dashboard.vue .sb-stat-icon` | 扁平低饱和「色彩色块 + 同色系图标」（如 Apple 设置图标：浅色底 + 深色/彩色图标，无渐变无投影） |
| 4 | 侧边栏纯白无质感，缺少苹方磨砂层次 | `DefaultLayout.vue .sb-sider` | 半透明磨砂玻璃背景（`backdrop-filter: blur`），贴近 macOS 侧边栏 |
| 5 | 菜单选中态使用强渐变 + 彩色发光阴影 | `NavMenu.vue .ant-menu-item-selected` | 苹方风格：柔和的强调色淡色填充（如 `rgba(0,113,227,.12)`）+ 主色文字，去除渐变/发光 |
| 6 | 品牌 Logo 为高饱和渐变方块 | `DefaultLayout.vue .sb-brand-logo` | 单色 / 低饱和纯色圆角块，弱化「渲染感」 |
| 7 | 阴影普遍偏重、偏彩色（如 `rgba(77,124,254,.14~.32)`） | `theme.css` `--sb-shadow-lg` `--sb-glow`；`NavMenu.vue` | 阴影应克制、近灰阶、极浅（Apple 卡片阴影通常 `rgba(0,0,0,.04~.08)`），且从未被使用的 `--sb-shadow-lg`/`--sb-glow` 应实际启用或移除 |
| 8 | 圆角、间距、字重节奏未统一到「大厂克制感」 | 全局 | 卡片圆角统一加大到 14px，标题字重降至 600，字号/行高按苹方节奏微调 |
| 9 | 死代码：`--sb-sider-width`/`--sb-sider-collapsed-width` 已声明但 `DefaultLayout.vue` 中仍硬编码 `232`/`64` | `theme.css` + `DefaultLayout.vue` | 统一引用变量，避免后续修改遗漏 |
| 10 | 日志/结果面板配色与主题脱节（偏浅蓝小清新，非中性灰） | `Logs.vue` `.sb-terminal`、`FaaSManager.vue` `.sb-result` | 统一为中性浅灰底 + 深灰文字，贴近 Apple 终端/控制台配色 |

### 新任务清单

| # | 任务 | 产出 | 状态 |
|---|------|------|------|
| 29 | 色板替换为 Apple 系统色 | `theme.css`：主色 `#0071e3`、中性灰阶（`#f5f5f7` 背景、`#1d1d1f`/`#6e6e73`/`#86868b` 文字） | ✅ 已完成 |
| 30 | 背景去彩色光晕，改纯色浅灰 | `theme.css body` | ✅ 已完成 |
| 31 | 阴影体系收敛为中性灰阶、启用/清理死变量 | `theme.css` `--sb-shadow*` `--sb-glow` | ✅ 已完成 |
| 32 | 侧边栏磨砂玻璃化 | `DefaultLayout.vue .sb-sider` | ✅ 已完成 |
| 33 | 菜单选中态改为淡色填充 | `NavMenu.vue` | ✅ 已完成 |
| 34 | 品牌 Logo 扁平化 | `DefaultLayout.vue .sb-brand-logo` | ✅ 已完成 |
| 35 | 统计卡图标改扁平色块 | `Dashboard.vue` | ✅ 已完成 |
| 36 | 布局尺寸变量统一引用，消除硬编码 | `DefaultLayout.vue`（`siderWidth`/`siderCollapsedWidth` 读取 CSS 变量） | ✅ 已完成 |
| 37 | 日志终端 / 调用结果面板改中性配色 | `Logs.vue` `FaaSManager.vue` | ✅ 已完成 |
| 38 | antd 主题 token 同步新色板 | `App.vue` | ✅ 已完成 |
| 39 | Lint / 构建验证 | `read_lints` 无错误 + `yarn build` 通过 | ✅ 已完成 |
| 40 | 文案调整：「FaaS 函数」统一改为「云函数」 | `NavMenu.vue` / `DefaultLayout.vue` / `FaaSManager.vue` 页面标题（路由名 `faas`、组件名 `FaaSManager` 及内部 API 字段保持不变，避免影响契约） | ✅ 已完成 |

## 二.八 LLM Gateway API 接入计划

> 后端 `internal/api/llm_handler.go` 已实现 `chat` / `stream`(SSE) / `providers` 三个接口，前端 services 层新增 `llm` 命名空间与之对应。

### 任务清单

| # | 任务 | 产出 | 状态 |
|---|------|------|------|
| 41 | services 层新增 LLM 类型与接口 | `types.ts`：`LlmMessage` / `LlmChatRequest` / `LlmChatResponse` / `LlmStreamHandlers` 等 + `Api.llm` | ✅ 已完成 |
| 42 | http 实现：chat / providers（axios）+ stream（fetch SSE 逐帧解析，可中断） | `http.ts` 导出 `baseURL`；`http-api.ts` `llmStream` | ✅ 已完成 |
| 43 | mock 实现：供应商列表、整段回复、逐块流式推送 | `mock.js` `llm` 命名空间 | ✅ 已完成 |
| 44 | LLM 对话页面（项目 ID、模型可选、流式开关、气泡会话、可停止） | `pages/LlmManager.vue` | ✅ 已完成 |
| 45 | 路由与菜单接入 | `router/index.ts` `/llm`；`NavMenu.vue`「LLM 对话」 | ✅ 已完成 |
| 46 | 契约文档更新 | `plan/ui-api-contract.md` LLM 三个接口表 | ✅ 已完成 |
| 47 | 构建与类型验证 | `vite build` 通过，`tsc --noEmit` src 零错误 | ✅ 已完成 |

### S3 对象存储后端对接（任务 48–53）

| # | 任务 | 产出 | 状态 |
|---|------|------|------|
| 48 | FileStore 抽象与实现 | `internal/objectstore/filestore.go`（FileStore 接口 + S3 实现 + DevMode 内存实现 + ValidateFileKey） | ✅ 已完成 |
| 49 | S3 HTTP Handler | `internal/api/s3_handler.go`（list/upload/delete/presign + 项目隔离） | ✅ 已完成 |
| 50 | 路由注册与错误映射 | `router.go` 挂载 `/v1/projects/:id/s3/*` 路由；`error.go` 映射 `ErrInvalidKey` → 400 | ✅ 已完成 |
| 51 | App 装配 | `app.go` DevMode 用 `NewMemoryFileStore`，生产用 `NewS3FileStore` | ✅ 已完成 |
| 52 | UI 对接 | `types.ts`/`http-api.ts`/`mock.js`/`S3Manager.vue` 全部加 `projectId` 参数 | ✅ 已完成 |
| 53 | 测试与契约文档 | `filestore_test.go` + `s3_handler_test.go` 全绿；`ui-api-contract.md` 更新 | ✅ 已完成 |

### 设计规范（Apple 大厂风格）

| 要素 | 取值 | 说明 |
|------|------|------|
| 主色 | `#0071e3` | Apple 官网 CTA 蓝，按钮/选中态/链接 |
| 强调色（浅色填充） | `rgba(0,113,227,.1)` | 菜单选中态、图标底色 |
| 背景 | `#f5f5f7` 纯色 | 无彩色光晕，克制 |
| 表面 | `#ffffff` | 卡片、侧边栏、头部 |
| 文字主色 | `#1d1d1f` | 接近黑但不是纯黑 |
| 文字次要 | `#6e6e73` | |
| 文字弱化 | `#86868b` | |
| 边框 | `rgba(0,0,0,.06)` 极浅灰 | 弱化分割感 |
| 圆角 | 14px（卡片）/ 10px（按钮、输入框） | 统一节奏 |
| 阴影 | `0 1px 2px rgba(0,0,0,.04)` + `0 8px 24px rgba(0,0,0,.06)`，无彩色发光 | 克制、分层 |
| 侧边栏 | `rgba(255,255,255,.7)` + `backdrop-filter: blur(20px)` | 磨砂玻璃质感 |
| 图标色块 | 浅色底（强调色 10% 透明度）+ 同色系深色图标，无渐变无投影 | 扁平化 |

## 三、架构说明

```
services/
├── types.ts      # Api 接口 + MetricsSummary / DbRow / S3Object / Project / FaasFunction 等类型
├── http.ts       # axios 实例（VITE_API_BASE_URL）、统一错误处理、wsBase
├── http-api.ts   # Api 的真实后端实现
├── mock.js       # Api 的 Mock 实现（纯 JS，内存状态）
├── mock.d.ts     # mock.js 的类型声明
└── api.ts        # 入口：VITE_USE_MOCK=true ? mockApi : httpApi
```

页面统一 `import { api } from '../services/api'`，不感知实现来源。

## 四、环境变量

> 后端对接契约详见 `plan/ui-api-contract.md`。

## 四.五、后端对接契约

详见 `plan/ui-api-contract.md`：列出了 UI 期望的全部 REST 路径、WS 协议、错误与日志格式约定。UI 端 `services/http-api.ts` 即按此契约实现。

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `VITE_USE_MOCK` | `true` 使用内置 mock 数据 | 开发 `true` / 生产 `false` |
| `VITE_API_BASE_URL` | 后端 API 基础路径 | `/api` |
| `VITE_WS_BASE_URL` | WebSocket 基础地址 | 同源推导（http→ws） |

## 五、验证方式

```bash
cd ui
yarn dev      # mock 模式下启动，界面右上角显示「Mock 数据」
yarn build    # 生产构建（mock 关闭）
```
