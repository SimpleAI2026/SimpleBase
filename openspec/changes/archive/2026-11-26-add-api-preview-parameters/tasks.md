# Tasks

## 1. 后端参数元数据（APV-1/2）

- [x] 1.1 `buildOpenAPI` 为已知路径参数输出来源扩展字段（project / database / agent / collection）
- [x] 1.2 未知参数不输出扩展字段；既有 OpenAPI 字段与快照测试不变
- [x] 1.3 用例：文档包含来源标记，未知参数无标记

## 2. 控制台预览（APV-3/4）

- [x] 2.1 `ApiPreview.vue` 在 `watch(selectedKey)` 中先清空再注入默认值
- [x] 2.2 `projectID` 取当前项目；数据库 / 助手 / 集合以下拉列出项目资源，失败退化为输入框
- [x] 2.3 `resolvedPath` 替换全部同名占位符；缺失必填参数时阻止发送并高亮
- [x] 2.4 文案与提示补 zh-CN / en-US 两棵树

## 3. 验证

- [x] 3.1 前端用例：默认值注入、切端点清空、资源失败降级、同名参数替换、缺失参数阻止
- [x] 3.2 后端用例：OpenAPI 扩展字段与既有字段兼容
- [x] 3.3 `yarn typecheck` + `yarn test` + `go test ./internal/api/...` 通过

## 完成情况核对（2026-11-26）

- 已实现：`internal/api/route_meta.go` 的 `x-sb-source` 与 `paramSource`；`route_meta_test.go` 覆盖来源标记与未知参数省略。
- 已实现：`ui/src/pages/ApiPreview.vue` 的 `injectDefaults`、`watch(selectedKey)` 清空后注入、`resolvedPath.replaceAll` 与必填校验；`useApiParamOptions.ts` 提供项目 / 库 / 集合 / 助手等下拉。
- 已实现：`ui/tests/ApiPreview.test.ts` 与 `useApiParamOptions.test.ts`；`yarn typecheck`、`yarn test` 通过。
- 结论：10/10，可以 `openspec archive add-api-preview-parameters`。
