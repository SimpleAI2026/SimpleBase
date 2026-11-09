# SimpleBase UI：设置页 + 通用 AI Chat 组件计划

> 目标目录：`ui/src`
> 制定日期：2026-09-16
> 状态：前端实现中（本地 settings + AiChat）；**后端仍不实现**
> 关联：`ui-plan-v2.md`（总入口）、`ui-principles.md`、`proto-http.md`（契约）、`ui-style-plan.md`（主题 token）
> Chat 输入框视觉参考：见下文「Composer 视觉规格」（用户提供截图：圆角容器 + 左 `+` + 右麦克风 + 黑底上箭头发送）

## 一、需求摘要

1. **设置页**：可配置默认模型、主题（亮/暗/跟随系统）。
2. **厂商 Token Plan 预置**：内置常见 LLM 厂商模板，开发者只需填写 API Key（及可选 base URL / 默认模型），即可在项目内选用。
3. **通用 AI Chat 组件**：把对话能力从 `LlmManager.vue` 抽成可复用组件；输入框（Composer）按参考图样式实现。
4. **文档先行**：更新前端 API 文档（`proto-http.md` / `proto.http`）；后端对应接口本轮只记 TODO，不写 Go。

## 二、范围与不做的事

### 做（计划层）

- 页面信息架构、组件边界、本地状态 vs 服务端状态划分
- 预置厂商清单与配置字段
- Composer / Chat 组件 API（props / emits / slots）
- 前端期望的 HTTP 契约（写入 `proto-http.md` §3.9 / §6.7），标明 **后端未实现**
- 与现有 `LlmManager`、`stores/auth`、`stores/project`、暗色主题 Phase 5 的衔接

### 不做（本轮）

- 不改 `ui/src/**`、不改 `internal/**`
- 不新增 npm 依赖
- 不实现真实厂商 SDK 调用链路（仍走现有 `:p/llm/chat|stream` 网关；设置页配置的 key 在后端就绪前可仅本地持久化，见 §五）
- 不把密钥写入 git / mock 种子明文仓库（文档强调 localStorage / 服务端密文引用）

## 三、信息架构

### 3.1 路由与导航

| 路径 | 名称 | meta.title | 说明 |
|---|---|---|---|
| `/settings` | `settings` | 设置 | 已改为 deep-link → 右上角 `SbModal`（见 [`ui-settings-merge-plan.md`](./ui-settings-merge-plan.md)） |
| `/llm` | `llm` | LLM 对话 | 保留；内部改用通用 Chat 组件（实现阶段） |

建议设置页 Tab（单页多区块，不必拆子路由）：

1. **外观**：主题（`light` | `dark` | `system`）
2. **模型**：默认模型、默认供应商、温度 / max_tokens 默认值（可选）
3. **供应商与 Key**：预置厂商卡片列表 +「自定义 OpenAI 兼容」入口

### 3.2 状态分层

| 配置项 | 一期存放 | 二期（后端就绪后） | 说明 |
|---|---|---|---|
| 主题 | `localStorage` + `stores/settings` | 可继续本地 | 与 `ui-plan-v2` Phase 5 暗色模式合并 |
| 默认 model / provider | `localStorage`（按 projectId 分桶） | `GET/PUT :p/llm/settings` | 项目级默认，避免串项目 |
| 厂商 API Key | **一期仅本地**（加密可选，至少不明文进仓库） | `PUT :p/llm/providers/:id/credential` → 服务端 `CredentialRef` | 与现有 catalog `LLMProviderConfig.CredentialRef` 对齐 |
| 厂商启用 / 默认模型 | 本地草稿 | 随 settings / providers CRUD | 列表展示用预置 catalog |

> 一期原则：设置页可独立于后端使用（Mock / DevMode）；真实对话仍调用已有 `llm/providers|chat|stream`。本地填的 key **不自动注入**现有后端（后端当前不读浏览器里的厂商 key），避免假安全感——UI 需文案说明：「Key 将在后端凭证接口就绪后由服务端托管；当前对话仍使用服务端已配置的供应商」。

## 四、预置 Token Plan 厂商

内置静态 catalog（前端常量，如 `ui/src/constants/llmProviders.ts`，实现阶段再加）：

| id | 显示名 | 协议形态 | 必填 | 可选 | 默认模型示例 |
|---|---|---|---|---|---|
| `openai` | OpenAI | OpenAI Chat Completions | `api_key` | `base_url`, `organization` | `gpt-4o-mini` |
| `anthropic` | Anthropic | Messages API | `api_key` | `base_url` | `claude-sonnet-4-5` |
| `azure_openai` | Azure OpenAI | Azure OpenAI | `api_key`, `endpoint`, `deployment` | `api_version` | （部署名） |
| `google` | Google Gemini | Google Generative Language | `api_key` | `base_url` | `gemini-2.0-flash` |
| `deepseek` | DeepSeek | OpenAI 兼容 | `api_key` | `base_url` | `deepseek-chat` |
| `moonshot` | Moonshot (Kimi) | OpenAI 兼容 | `api_key` | `base_url` | `moonshot-v1-auto` |
| `zhipu` | 智谱 | OpenAI 兼容 | `api_key` | `base_url` | `glm-4-flash` |
| `dashscope` | 阿里云百炼 | OpenAI 兼容 | `api_key` | `base_url` | `qwen-plus` |
| `openrouter` | OpenRouter | OpenAI 兼容 | `api_key` | `base_url` | `openai/gpt-4o-mini` |
| `custom_openai` | 自定义 OpenAI 兼容 | OpenAI 兼容 | `api_key`, `base_url` | `default_model` | 用户自填 |

UI 行为：

- 卡片展示：logo/首字母、名称、协议标签、Key 已填/未填状态（只显示「已配置」掩码，不回显完整 key）
- 点击「配置」→ Drawer/Modal：填 key + 可选字段 → 保存到本地 settings store
- 「设为默认」写回默认 provider；模型下拉合并「该厂商推荐模型 + 自由输入」

## 五、通用 AI Chat 组件

### 5.1 组件树（实现阶段目标）

```
components/ai/
├── AiChat.vue              # 编排：消息列表 + Composer + 发送/停止
├── AiChatMessageList.vue   # 气泡列表（可抽自现 LlmManager）
├── AiChatMessage.vue       # 单条 user/assistant
└── AiChatComposer.vue      # 输入框（按参考图）
```

可选 composable：`composables/useAiChat.ts`（messages、sending、stream abort、调用 `api.llm.*`）。

### 5.2 `AiChat` 公共 API（约定）

```ts
props: {
  projectId: string
  model?: string
  provider?: string          // 仅 UI 展示；实际请求仍走网关默认供应商，直至后端支持指定 provider
  streaming?: boolean        // default true
  placeholder?: string
  disabled?: boolean
  showToolbar?: boolean      // 模型选择等是否内嵌；Settings 页改默认时可为 false
}
emits: {
  'update:model': [string]
  sent: [{ role, content }]
  error: [unknown]
  finished: [{ content, model?, provider? }]
}
slots: {
  toolbar?: () => VNode     // 覆盖顶部工具条
  empty?: () => VNode
}
```

`LlmManager.vue` 退化为薄页面：`PageContainer` + `ProjectPicker` + `<AiChat :project-id="..." />`。

### 5.3 Composer 视觉规格（对照参考图）

容器：

- 宽：占满内容区可用宽度；高度随多行文本增高（min ≈ 单行+图标行，max 约 8～12 行后内部滚动）
- 圆角：大圆角（建议 `border-radius: 24px`～`28px`）
- 背景：浅底（亮色模式接近 `#f5f5f5` / 白底+浅灰描边；暗色模式用 token）
- 边框：1px 浅灰；聚焦时可用主题色弱描边或阴影，避免厚边框

布局（单层 flex）：

| 区域 | 内容 | 说明 |
|---|---|---|
| 左下 | 圆形 `+` 按钮 | 附件/扩展菜单占位；一期可 `disabled` + tooltip「即将支持」 |
| 中 | 多行 textarea | 无边框；placeholder 可选；Enter 发送，Shift+Enter 换行 |
| 右下 | 麦克风图标 | 语音输入占位；一期同样可禁用 |
| 右下（最右） | 实心黑圆 + 白色上箭头 | 主发送按钮；发送中可切换为停止（危险色）或保持黑圆+方停图标 |

交互：

- 空内容禁用发送（或点按无效果）
- 流式生成中：发送钮变为停止，调用现有 `AbortController`
- 不引入新图标库；优先 ant-design-vue icons（`PlusOutlined` / `AudioOutlined` / `ArrowUpOutlined` / `BorderOutlined`）

样式实现约束（对齐 `ui-principles`）：

- 颜色/圆角/间距全部走 `tokens.css`，禁止在组件内写死 `#000` 以外无法 token 化的特例（发送钮可用 `--sb-composer-send-bg`）
- 与现有 Claude 暖橙主题协调：发送钮可用近黑，不必强行橙色，以免抢对话气泡

## 六、设置页 UI 草图

```
设置
├─ 外观
│   └─ 主题：○ 浅色  ○ 深色  ○ 跟随系统
├─ 模型默认值
│   ├─ 默认供应商 [select]
│   ├─ 默认模型   [select/input]
│   └─ 温度 / Max Tokens（可选高级折叠）
└─ 供应商 API Key
    ├─ [OpenAI]     已配置 · 设为默认 · 编辑
    ├─ [Anthropic]  未配置 · 配置
    └─ …预置列表…
```

## 七、与现有计划的关系

| 现有项 | 关系 |
|---|---|
| `ui-plan-v2` Phase 5 暗色模式 | 主题设置是其产品入口；本计划定义设置页，实现时可与 Phase 5 合并交付 |
| `LlmManager` / F4 messages 校验 | Chat 组件实现时一并带上本地校验 |
| catalog `LLMProviderConfig` | 后端二期 CRUD 对齐该模型；前端预置 id 与之兼容 |
| `proto-http.md` §3.5 | 对话仍用现网关；新增 §3.9（设置/凭证）为**后端 TODO** |

建议在 `ui-plan-v2.md` 增加 **Phase 6（可并行）**：设置页 + AiChat 组件（依赖 Phase 1 stores；暗色依赖 Phase 5 tokens）。

## 八、实施阶段（供日后开发，本轮不执行）

### P0 — 文档与类型契约（本轮完成）

- [x] 本文档
- [x] `proto-http.md` 增补 §3.9 / §6.7
- [x] `proto.http` 增补注释样例
- [x] `ui-plan-v2.md` 挂载本计划入口

### P1 — 前端实现（未开始）

1. `stores/settings.ts`（主题 + 按项目默认模型 + 本地厂商草稿）
2. `pages/Settings.vue` + 路由 / 菜单
3. `components/ai/*` + `useAiChat`
4. `LlmManager` 改为使用 `AiChat`
5. Mock：settings 本地；llm 行为不变
6. 文案：说明 Key 托管状态与网关关系

### P2 — 后端（未开始，见 proto-http §6.7）

1. 项目级 LLM settings CRUD
2. 厂商凭证写入（只收 key，存 `CredentialRef`，禁止回显明文）
3. chat/stream 支持显式 `provider` / 覆盖默认模型
4. providers 列表返回「已配置 / 未配置」状态（不含 secret）

## 九、验收标准（实现阶段）

1. `/settings` 可切换主题，刷新后保持；`system` 跟随 OS
2. 预置厂商均可保存 key；列表仅显示掩码状态
3. `AiChat` 可在 `LlmManager` 使用；Composer 视觉符合 §5.3（对照参考图）
4. 未实现附件/语音时有明确禁用态，不假装可用
5. `yarn build` / `vue-tsc` 通过；无密钥进仓库
6. 后端未就绪时，设置页不因缺失接口白屏（纯本地存储路径）

## 十、开放问题

1. 一期本地 key 是否用 Web Crypto 包装后再存 localStorage，还是明文仅开发机可用并强提示？
2. chat 请求是否允许前端直连厂商（绕过 SimpleBase 网关）？**默认否**——保持配额/审计在网关。
3. Azure / 自定义厂商的字段校验规则是否要做厂商插件化？一期用静态 schema 即可。
