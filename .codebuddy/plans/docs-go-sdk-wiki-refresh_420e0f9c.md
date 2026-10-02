---
name: docs-go-sdk-wiki-refresh
overview: 校准现有文档与当前项目行为，新增与 JS SDK 核心能力对齐的 Go SDK 及使用文档，并改善文档站页面视觉与响应式体验。保留既有 `/docs/sdk/*` 链接与仓库单模块约束。
design:
  architecture:
    framework: vue
    component: shadcn
  styleKeywords:
    - 技术文档
    - 清晰层次
    - 轻柔渐变
    - 响应式阅读
  fontSystem:
    fontFamily: PingFang SC
    heading:
      size: 30px
      weight: 700
    subheading:
      size: 18px
      weight: 600
    body:
      size: 15px
      weight: 400
  colorSystem:
    primary:
      - "#167A94"
      - "#155E75"
    background:
      - "#F7FAFC"
      - "#FFFFFF"
    text:
      - "#172638"
      - "#526579"
    functional:
      - "#15803D"
      - "#B45309"
      - "#B42318"
todos:
  - id: audit-contracts
    content: 使用 [subagent:code-explorer] 核对 API 契约、文档过时点及现有测试约束
    status: completed
  - id: build-go-sdk
    content: 实现 packages/go-sdk 的请求层、数据库、SQL、文档和存储操作
    status: completed
    dependencies:
      - audit-contracts
  - id: test-go-sdk
    content: 补齐 packages/go-sdk 协议、错误、取消及上传测试
    status: completed
    dependencies:
      - build-go-sdk
  - id: refresh-docs
    content: 更新 docs/ 现行指南并撰写 docs/sdk/go-*.md，保留原有 JS 链接
    status: completed
    dependencies:
      - audit-contracts
      - build-go-sdk
  - id: polish-docs-ui
    content: 美化 DocsWiki.vue 并更新文档目录与页面测试
    status: completed
    dependencies:
      - refresh-docs
  - id: verify-changes
    content: 运行 Go 测试、前端测试及构建，核对文档链接和 SDK 示例
    status: completed
    dependencies:
      - test-go-sdk
      - polish-docs-ui
---

## User Requirements

- 参考项目 README 和开发规范，更新 `docs/` 中的使用文档，使现行功能、部署方式和限制说明保持一致。
- 在 `packages/` 增加可供 Go 项目使用的 Go SDK，并在现有 `docs/sdk/` 中补充对应指南。
- 美化文档站页面，改善桌面和移动端的阅读与导航体验。

## Product Overview

文档站同时提供现行产品指南、JavaScript SDK 指南和 Go SDK 指南。现有 JavaScript 文档链接继续可用；历史迁移资料保留，但明确标识为存档。

## Core Features

- Go SDK 覆盖数据库管理、SQL、文档集合和对象存储的常用操作，并提供鉴权、错误处理示例。
- SDK 文档包含安装、快速开始、主要操作及错误处理。
- 文档页保留模块切换、文章导航和前后篇入口，以更清晰的层次呈现内容。

## Tech Stack Selection

- 沿用仓库现有的 Go 单模块结构：`packages/go-sdk` 作为根模块下的包，不新增独立 `go.mod` 或第三方依赖。
- 文档继续使用 `docs/` 下的 Markdown；文档站沿用 Vue 3、TypeScript、Tailwind CSS 和现有组件。
- Go SDK 使用标准库 HTTP 客户端与 `httptest` 测试；不引入新的服务端接口。

## Implementation Approach

以现有 JS SDK 的公开能力和实际 HTTP 协议为边界设计 Go SDK，而不是一次覆盖 README 所列全部服务。共享的请求层集中处理 Bearer 鉴权、项目路径、编码、响应解码和结构化错误；数据库、SQL、集合与存储按领域提供方法。请求支持 `context.Context` 与可注入的 HTTP 客户端；上传采用流式请求，避免无必要地把文件整体读入内存。

文档先核对 README、规范及对应实现，再修正现行指南和导航。保留 `/docs/sdk/*` 原有页面路径，在同一 SDK 模块增加 Go 专属页面，避免迁移链接。视觉改造限定在 `DocsWiki.vue` 的页面编排，不改变路由、文档解析或已定稿的基础组件风格。

## Implementation Notes

- 核对数据库级文档路由、SQL 参数、集合响应、S3 multipart 字段、预签名响应及错误结构后再固定 Go 的公开方法和示例；不照搬 JS 的动态类型。
- URL 路径段和查询参数分别编码；限制错误响应读取量，不在错误或测试输出中泄露 API Key、SQL 参数或上传内容。
- 保留 `docs/ops/migration.md` 等历史资料与大型 DuckLake 规格原文；从入门推荐路径中移除过时迁移步骤，并在入口注明历史属性。
- 文档由现有 Vite glob 收录，新增 Go 页面无须新路由；注意新增 Markdown 对前端构建体积的影响，不复制大型规格。
- 对象形状若需新增 TypeScript 定义，一律使用 `interface`。不手工编辑 `internal/web/dist`。

## Architecture Design

Go 调用方 → `packages/go-sdk` 领域方法 → 共享 HTTP 请求层 → 现有 `/v1/projects/:projectID` API。文档 Markdown → `ui/src/docs/catalog.ts` → `DocsWiki.vue` 中现有侧栏、移动端选择器和文章组件。服务端及其权限保护逻辑保持不变。

## Directory Structure Summary

以下为计划修改或新增的文件；实施时仅在核对实际协议后确定公开方法签名。

- `packages/go-sdk/client.go` **[NEW]**：配置与客户端入口；校验地址、项目 ID 和凭据，复用可注入 HTTP 客户端。
- `packages/go-sdk/http.go`、`errors.go` **[NEW]**：项目路径、请求及响应处理、结构化错误；支持上下文取消、编码和安全的错误读取。
- `packages/go-sdk/databases.go`、`sql.go`、`documents.go`、`storage.go` **[NEW]**：分别实现与现有 JS SDK 核心能力对齐的领域操作；明确请求、响应结构及上传资源关闭责任。
- `packages/go-sdk/*_test.go` **[NEW]**：使用 `httptest` 验证请求协议、鉴权、参数编码、取消、错误、空响应和上传，不依赖真实数据库或 S3。
- `packages/go-sdk/README.md` **[NEW]**：包的导入方式、最短可运行示例和能力边界。
- `docs/_meta.json` **[MODIFY]**：将现有“JS SDK”模块标题调整为兼容双语言的“SDK”，保留模块 ID `sdk`。
- `docs/sdk/index.md` **[MODIFY]**：增加 JavaScript 与 Go 两条阅读路径，保持现有 JS 页面链接。
- `docs/sdk/go-install.md`、`go-quickstart.md`、`go-database-sql.md`、`go-documents.md`、`go-storage.md`、`go-errors.md` **[NEW]**：按实际 Go SDK 接口编写安装、鉴权与主要能力示例，并标注权限及限制。
- `docs/getting-started/index.md`、`docs/database/index.md`、`docs/ops/index.md` **[MODIFY]**：校准产品入口、现行数据库能力与部署阅读顺序；区分现行指南和历史迁移资料。
- `docs/ops/deployment.md`、`docs/gofunction/overview.md`、`docs/cronjob/overview.md` **[MODIFY]**：对照 README、相关规范和实现，定点修正过时的运行、配置或能力表述，不作无依据的全文重写。
- `ui/src/pages/DocsWiki.vue` **[MODIFY]**：优化模块栏、阅读区、移动端选择器及状态提示的视觉层次；保留现有导航行为。
- `ui/src/pages/DocsWiki.test.ts`、`ui/src/docs/catalog.test.ts` **[MODIFY]**：验证新页面结构、移动端导航及新增 Go 文档可发现性；保留既有路由断言。

## Design Style

采用克制的技术文档风格：暖白与深蓝灰为底，青蓝色突出当前位置，轻柔渐变和细边框增强层次。仅在文档页加入适度的过渡与悬停反馈，保持正文长时间阅读的舒适度，并尊重减少动态效果设置。

## Page Blocks

1. **顶部导航**：沿用 `DocsLayout.vue` 的品牌、返回控制台及外部链接，不重复设置顶栏。
2. **模块切换栏**：保留吸顶标签；提高选中态辨识度，横向空间不足时仍可滚动，键盘焦点清晰可见。
3. **阅读引导区**：在当前模块与文章内容之间呈现简洁的模块说明和当前位置，不压过文章标题。
4. **正文工作区**：桌面端清楚区分目录与文章；移动端突出“选择文档页面”入口，并保证长标题、代码块和窄屏内容可读。
5. **文章导航与状态**：沿用文章组件的前后篇入口；空目录或缺失页面使用一致、易辨认的提示样式。

仅调整文档站局部容器与留白，不改控制台主内容宽度规范；不新增独立 CSS 文件或组件库。

# Agent Extensions

- **code-explorer**（SubAgent）：定向复核跨文件的 SDK 协议、文档入口和文档站测试。预期结果：实施前确认请求与响应契约、过时内容及受影响路径，避免仅凭 README 推断接口。