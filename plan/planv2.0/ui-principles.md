# SimpleBase UI 设计原则与代码结构原则

> 适用范围：`ui/src` 全部代码
> 版本：v2.0（2026-09-15）
> 配套：`ui-plan-v2.md`（总计划）、`proto-http.md`（API 契约）

---

## 第一部分：UI 设计原则

### 1. 视觉体系：一套变量走天下

**原则**：任何颜色、间距、字号、圆角、阴影、动效常量，只允许在 `styles/tokens.css` 中定义一次。组件、页面、antd token 全部引用变量，禁止出现字面量。

```css
/* 正确 */
.card { border-radius: var(--sb-radius); color: var(--sb-text-secondary); }

/* 错误：改主题要全局搜索替换 */
.card { border-radius: 12px; color: #706f6a; }
```

同一条约束对 TS 代码生效：`App.vue` 的 antd token、Dashboard 统计卡配色等，一律从 `tokens.ts` 导出常量，与 CSS 变量同源派生。

**变量命名规范**：`--sb-<类别>-<语义>-<状态>`，如 `--sb-primary-hover`、`--sb-text-secondary`、`--sb-bg-soft`。禁止按外观命名（`--sb-orange` ✗）。

### 2. 品牌基调：Claude 暖色系，克制不花哨

- 主色 `#d97757`（Claude 橙），语义色仅 success/warning/danger/info 四种。
- 背景暖米色 `#f5f4ef`，卡片纯白，通过 `box-shadow`（暖灰基调）分层，不用彩色边框。
- 标题衬线（Georgia 栈）、正文无衬线（系统栈）、代码等宽（`ui-monospace` 栈）——三种字族各司其职，不混用。
- 装饰性元素（呼吸灯、光标闪烁、毛玻璃）只用于「状态表达」，不做纯装饰。

### 3. 间距与字号：8pt 栅格 + 字号标尺

- 间距只用 `--sb-space-1..8`（4/8/12/16/20/24/32/40px），任何 `margin: 13px` 这类非标值不允许出现。
- 字号只用 `--sb-fs-xs..3xl`（12/13/14/16/20/24/30px）。当前代码里 11.5px、12.5px、17px 等零散值全部归档到标尺。
- 行高三档：tight(1.3) / normal(1.5) / relaxed(1.7)。

### 4. 布局：内容有界，密度分层

- 主内容区 `max-width: 1440px` 居中，大屏不拉伸到全宽。
- 页面骨架固定为：`PageContainer`（衬线大标题 + 副标题）→ 工具栏卡片 → 内容卡片。所有页面同构，用户形成肌肉记忆。
- 移动端 ≤768px：Sider 降级 Drawer、toolbar 换行、表格横滚；≤480px 输入控件占满整行。断点只有这两个，不新增。

### 5. 交互与动效：反馈及时，动效克制

- 动效只出现在状态切换（hover/选中/加载/进出场），时长 ≤ 240ms，缓动统一 `cubic-bezier(.4,0,.2,1)`。
- 破坏性操作（删除文档/对象/数据库）必须有 `a-popconfirm` 或 Modal 二次确认。
- 所有异步操作必须有 loading 态（按钮 `:loading` / 表格 loading / 骨架屏三选一），失败必须有 `message.error` 且文案来自后端 `error.message`（见 proto-http.md 错误协议）。
- 空态不是死胡同：`SbEmptyState` 带 CTA（如「新建集合」直达操作）。

### 6. 可访问性底线

- 文字对比度 ≥ 4.5:1（暖色系下尤其注意 secondary 文字）。
- 全局 `:focus-visible` 使用 `--sb-ring`，键盘导航可见。
- `prefers-reduced-motion: reduce` 时关闭动画。
- 图标按钮必须配 tooltip 或 aria-label。

### 7. 数据展示分层

- 人读的数据格式化（`formatBytes`/`formatTime`/`formatJson`），机器读的保持原样。
- JSON/SQL/日志用等宽字体 + 浅底色块（`SbCodeBlock`），与正文视觉区隔。
- 表格默认 10 行/页 + `共 N 条`；长字段 `ellipsis` + tooltip；操作列固定右侧宽度。

### 8. 中间态优先于终态

加载中、空数据、错误、无权限四态必须与正常态同优先级设计。当前「空白 → 突然填充」的抖动体验通过骨架屏消除。

---

## 第二部分：代码结构原则

### 1. 分层架构：单向依赖

```
pages（编排） → composables（复用逻辑） → stores（状态） → services（API） → http（传输）
        ↘ components（展示） ↗
```

- **pages 只做编排**：组组件、调 composable、绑定数据。不写重复样式、不直接 import axios。
- **services 是唯一出口**：页面永远 import `api`（来自 `services/api.ts`），永远不碰 `http` 实例。新增接口 = 改 `types.ts`（接口签名）→ 改 `http-api.ts`（实现）→ 改 `mock.js`（兜底）三件套同步完成。
- **types.ts 是契约**：`Api` 接口的每个方法签名与 `proto-http.md` 的接口表一一对应。后端加路由后，此处不同步视为 bug。

### 2. services 层规则（继承 v1.0，强化）

- 传输层（`http.ts`）职责：baseURL、超时、Authorization 注入、错误归一化（后端 `{error:{message}}` → `Error(message)`）、HTML fallback 防御。不放业务逻辑。
- 命名转换集中在一处：UI 用 camelCase，后端 DTO 用 snake_case，映射函数（如 `toLlmPayload`）写在 `http-api.ts` 顶部，禁止散落在调用处。S3 的 `lastModified` 是后端 camelCase 例外，同样在此层标注。
- 每个方法返回**已解包的数据**（`r.data.rows` 而不是 `r`），让页面拿到即可用；流式/WS 返回 `{ close() }` 句柄。
- 形状适配也在此层：如 SQL `QueryResponse.rows` 是二维数组，需要对象数组的页面由 adapter 按 `columns` zip 后返回，页面不做数据搬运。
- **services 层不 import store**：`projectId` 一律作为方法**首个参数**显式传入（与现有 `s3.list(projectId, …)`、`llm.chat(projectId, …)` 保持一致），由页面/composable 从 `stores/project.ts` 读取后传下来。这样 services 保持无状态、可单测，也避免 store ↔ service 循环依赖。现存 8 处 `proj-01` 字面量即此原则的反例。
- 路径参数必须 `encodeURIComponent`。

### 3. 组件规则

- **重复两次即抽组件**：同样的样式/结构出现第 2 次，就必须抽成 `components/` 下的组件或 `utilities.css` 工具类（现行 `.sb-json` 五处复制即反例）。
- 组件命名：`Sb` 前缀表项目通用组件（`SbCodeBlock`、`SbEmptyState`）；业务组件用名词短语（`ProjectPicker`、`ApiKeyDrawer`）。
- 组件 props 用 TypeScript interface 定义，事件用 `defineEmits<{...}>` 泛型签名，禁用 `any`。
- 组件不含业务请求；数据由页面/composable 传入。

### 4. 路由与元信息：单一数据源

- 页面标题、菜单名、面包屑、图标全部收敛到 `router/index.ts` 的 `route.meta`（`{ title, icon, hidden }`）。NavMenu 遍历路由渲染，`DefaultLayout` 的 `titleMap` 删除。
- 新增页面 = 加一条路由 + 建一个文件，不改三个地方。

### 5. 状态管理：pinia，按域拆分

- `stores/project.ts`：当前项目 ID（持久化到 localStorage）。**后端无 `GET /v1/projects` 接口**，无法枚举项目，因此 store 只保存用户输入的 ID，不做远程拉取。
- `stores/auth.ts`：API key（localStorage）、401 状态、key 配置抽屉开关。
- store 被页面读取后把值传给 services（见 §II.2），store 本身不发请求。
- 跨页面共享且需要持久的才进 store；页面私有状态用 `ref` 即可，不为用 store 而 store。

### 6. 组合式函数（composables）

- 重复的异步模式（loading + try/catch + message.error/success）抽 `useAsyncAction`，页面不再手写 try/catch 模板（当前 6 个页面 20+ 处重复即反例）。
- composable 命名 `useXxx`，返回 `{ data, loading, error, run }` 形态，职责单一。

### 7. 样式组织

```
styles/
├── tokens.css      变量（唯一允许出现颜色/字号字面量的文件）
├── base.css        reset、body、滚动条、focus-visible、reduce-motion
├── antd-patch.css  antd 覆盖（能用组件 token 解决的不写在这里）
└── utilities.css   .sb-page/.sb-toolbar/.sb-card/.sb-code 全局工具类
```

- `main.ts` 引入顺序：tokens → base → antd reset → antd-patch → utilities。
- 页面 `<style scoped>` 只写该页私有布局；可复用样式进 utilities 或组件。
- `!important` 只允许出现在 `antd-patch.css`（对抗 antd 优先级），业务样式禁用。目标是 antd-patch ≤ 60 行。
- antd 定制优先级：组件 token（`ConfigProvider.theme.components`）> antd-patch.css > scoped 强压。当前 30+ 处 `!important` 是没走组件 token 的结果。

### 8. TypeScript 纪律

- 严格模式；组件/函数签名禁 `any`（现行 `onClick(e: any)`、`handleUpload({...}: any)` 均需修正）。
- 后端响应类型与 `proto-http.md` 契约同步维护；可选字段标 `?`。
- `mock.js` 是 js（历史遗留），新增 mock 建议渐进迁 `.ts`，最终 `mock.d.ts` 删除。

### 9. 命名与文件

- 文件名：组件 PascalCase、composable camelCase、service/store camelCase。
- 页面文件 = 路由 name 对应（`SqlConsole.vue` ↔ `name: 'sql'`）。
- CSS 类名 `sb-` 前缀 + kebab-case；禁止 ad-hoc 类名（`.foo`、`.wrap`）。
- 删除即删除：未使用的 import、空文件（`mock.d.ts`）、死代码不留尸。

### 10. 工程约束

- 不新增运行时依赖（图表自绘、状态用 pinia、请求用 axios，均有）；devDependencies 变更需说明。
- `package.json`/`tsconfig.json`/`vite.config.ts` 属配置文件，修改需在 plan 中立项。
- 构建产物 `ui/dist` 由 `build.sh` 复制到 `internal/web/dist` 后 embed，前端不感知部署形态。
- 开发代理约定：`/v1`、`/health`、`/ws` 转发 `:8080`（vite.config.ts 已配）；新前缀要同步补代理。

### 11. 对「后端不存在的能力」的处理

- 不为后端没有的接口造假 UI。当前 FaaS、实时日志 WS、指标汇总三项在后端均不存在，处理方式统一为：菜单隐藏或页面显式提示「后端未支持」，Mock 模式下仍可演示。
- 页面依赖的后端行为若有隐式约束（如文档 API 只操作项目第一个库），必须在 UI 文案中明示，不让用户猜。
- 后端返回的非预期错误码（如参数校验落到 500 `internal_error`）由前端**提前本地校验**规避，而不是把「internal error」直接抛给用户。

---

## 附：现行代码与本原则的差距索引

| 原则 | 现状违反点 | 修复阶段（见 ui-plan-v2.md） |
|---|---|---|
| §I.1 单一变量源 | `App.vue`/`Dashboard.vue`/`theme.css` 三处色值 | Phase 1 |
| §II.2 services 规则 | `http-api.ts` 8 处 `proj-01` 硬编码；`Api.db.*` 签名缺 projectId | Phase 1 |
| §II.4 路由元信息 | 标题三处维护 | Phase 1 |
| §II.3 组件复用 | `.sb-json` 等 5 处复制；`.sb-page-title` 全局/scoped 双份 | Phase 4 |
| §II.6 composable | 20+ 处 try/catch 模板 | Phase 1 |
| §II.8 TS 纪律 | 多处 `any` | Phase 4 |
| §II.11 不造假 UI | Dashboard/FaaS/Logs 三页调用不存在的接口 | Phase 3 |
| §I.5 二次确认 | 已达标 | — |
| §I.4 布局有界 | 无 max-width | Phase 4 |
