# 全局项目切换器 + 创建项目

> 目标：项目切换只出现在顶栏；各业务页统一消费 `projectStore`；可创建项目。
> 配套：[`ui-plan-v2.md`](./ui-plan-v2.md)、[`ui-principles.md`](./ui-principles.md)、[`proto-http.md`](./proto-http.md)
> 状态：**implemented**
> 制定日期：2026-09-17
> 默认项目 ID：`internal/catalog.DevProjectID` = `00000000-0000-0000-0000-000000000002`（`systemdb/seed.go` DevMode「商城后台」）

## 背景

v2 UI 已有 `stores/project.ts` 与各页 `<ProjectPicker />`，但：

- 每页各自放一份项目输入框，切换入口不唯一。
- 缺省 ID 仍是历史占位 `proj-01`，与系统库种子 UUID 不一致。
- 后端已有 `GET /v1/projects`，没有 `POST /v1/projects`。
- 侧栏「设置」排在「日志」前面。（已过时：设置已并入右上角 `SbModal`，见 [`ui-settings-merge-plan.md`](./ui-settings-merge-plan.md)）

## 目标

1. 顶栏右侧全局项目下拉 + 创建项目（**唯一**切换入口）。
2. Dashboard / Databases / S3 / LLM / Settings / Logs 全部使用 `projectStore` 当前项目，去掉页内 `ProjectPicker`。
3. 侧栏顺序：… → LLM → Logs → **Settings 最后**。（已过时：Settings 已离开侧栏，见 [`ui-settings-merge-plan.md`](./ui-settings-merge-plan.md)）
4. `POST /v1/projects`；默认项目 ID 对齐系统种子 UUID。

## 阶段

### Phase A — Layout（前端）

1. `GlobalProjectSwitcher` 放在 `DefaultLayout` header-right（Mock / 连接设置 / GitHub / 刷新左侧）。
   - 展示当前项目 **名称**，短 ID 次要。
   - 下拉：`GET /v1/projects` + 本地历史；可搜索。
2. 从 Databases、S3Manager、LlmManager、Settings 等页面移除 `<ProjectPicker />`。
3. `NavMenu` `routeOrder`：`settings` 移到 `logs` 之后。
4. 扩展 `project` store：缓存 `projects[]`、`projectName`、`loadProjects()`；`DEFAULT_PROJECT_ID` 改为种子 UUID。兼容本地残留 `proj-01`。
5. 切换时 `setProject`；已 `watch(projectId)` 的页面自动重载。Dashboard / Logs / LLM 补 watch。

### Phase B — 创建项目

1. 后端 `POST /v1/projects` body `{ name, id? }` → 201 `{ id, name, created_at }`；省略 id 则生成 UUID；冲突 409；接入 catalog / systemdb。
2. 更新 `proto-http.md` / `proto.http` / mock / `http-api` types。
3. `CreateProjectModal`（`SbModal`）：名称必填，id 可选；成功后选中新项目并刷新列表。
4. `ProjectAdmin` 可访问本租户下新创建的项目（不仅是 API key 绑定的那一个）。

### Phase C — 收尾

1. `ApiKeyDrawer` 不再提供第二套项目 ID 编辑；当前项目只读。
2. 未选择项目时 `ProjectScope` 空态 +「创建项目」CTA。
3. `yarn build` + 相关 `go test` 通过。
4. 本文状态 → implemented；开 PR。

## 验收（G1–G6）

| # | 标准 |
|---|---|
| G1 | 顶栏右侧有全局项目切换器（名称 + 短 ID），是唯一切换入口 |
| G2 | Databases / S3 / LLM / Settings 等页无 `ProjectPicker`；数据请求使用 `projectStore` 当前项目 |
| G3 | 侧栏顺序为 Dashboard → Databases → S3 → LLM → Logs → Settings（已过时：Settings 已离开侧栏，见 [`ui-settings-merge-plan.md`](./ui-settings-merge-plan.md)） |
| G4 | `POST /v1/projects` 成功 201、冲突 409；缺省项目 ID 为 `00000000-0000-0000-0000-000000000002` |
| G5 | 新建项目弹窗成功后选中该项目并刷新列表；页内 `watch(projectId)` 会重载 |
| G6 | ApiKeyDrawer 项目只读；无项目时有空态；`yarn build` 与相关 Go 测试通过 |

## 技术约束

- 沿用 Ant Design Vue 4 + 中文文案。
- 提交不含密钥；不重新引入 Turso。
- 种子 UUID 以 `internal/catalog/reserved.go` / `internal/systemdb/seed.go` 为准。
- services：先改 `types.ts`，再改 `http-api.ts` / `mock.js`。
