# 数据库管理页：SQL 控制台 + 数据管理合一

> 目标：把独立的「SQL 控制台」与「数据管理」折进 **数据库管理** 页，用统一 `SbModal` 家族承载查询 / 集合 / 文档操作。
> 配套：[`ui-plan-v2.md`](./ui-plan-v2.md)、[`ui-principles.md`](./ui-principles.md)、[`proto-http.md`](./proto-http.md)
> 状态：**implemented**（PR https://github.com/linkxzhou/SimpleBase/pull/2）

## 背景

v2 UI 已有独立路由：

- `/databases` 库生命周期
- `/sql` SQL query / execute / batch
- `/data` 集合与文档（项目级，隐式第一个库）

问题：多库时 SQL / 文档操作与当前行绑定的库容易错位；侧栏入口过多；文档 API 不带 `databaseId`。

## 目标

1. 数据库列表成为唯一控制台：行内 **SQL**、展开集合、**新建集合**、查看/新增文档。
2. 去掉侧栏「SQL 控制台」「数据管理」；`/sql`、`/data` 重定向到 `/databases`。
3. 统一弹窗：`SbModal` 包装 `a-modal`（宽度 / footer / `destroyOnClose`）。
4. 文档 API 显式带 `databaseId`，**禁止**在调用方已提供 `databaseId` 时静默落到「第一个库」。

## 交互与文案（中文）

| 场景 | 文案 |
|---|---|
| 查询成功但 0 行 | 暂时未查询到数据 |
| 尚未执行过查询 | 温和空提示（如「执行后结果将显示在这里」） |
| 文档列表为空 | 暂时未查询到数据 |
| 非就绪库操作 | 按钮禁用 + tooltip「数据库未就绪」 |
| 导航标题 | 数据库管理 |
| 行按钮 | SQL、新建集合 |
| 集合行 | 查看数据、新增文档 |

集合名校验：字母开头，`[A-Za-z][A-Za-z0-9_]*`（与后端 `{0,62}` 上限对齐）。

## 阶段

### Phase 1 — Modal 基座 + SQL

1. `ui/src/components/modal/SbModal.vue`：包装 `a-modal`，默认宽度、footer、`destroyOnClose`。
2. `SqlWorkModal.vue`：tab **查询 | 执行 | 批量**；调用 `api.sql.*(projectId, databaseId, …)`；结果在同一弹窗内、编辑器下方。
3. `Databases.vue` 行操作增加 **SQL**（仅 `status === ready` 可点），打开绑定该库的 `SqlWorkModal`。
4. 新建数据库对话框迁到 `SbModal`。
5. 路由：菜单隐藏 `/sql`；`/sql` → `/databases`。逻辑从 `SqlConsole.vue` 迁入弹窗，不留第三份拷贝。

### Phase 2 — 集合

1. 行展开：圆 + plus / minus；展开区为该库的 `CollectionPanel`。
2. 行按钮 **新建集合** → `CreateCollectionModal`。
3. 后端优先：
   - `GET/POST /v1/projects/:p/databases/:dbId/data/collections`
   - 文档挂同一前缀
   更新 Go handler、`http-api.ts`、types、mock。允许薄封装：路径带 `databaseId` 时必须选中该库。
4. 集合行：**查看数据** | **新增文档**。

### Phase 3 — 文档

1. `DocumentListModal`：按 db + collection 分页；空 →「暂时未查询到数据」。
2. `DocumentKvModal`：Key `a-input` + Value `a-textarea`；提交 `{ [key]: parsedValue }`（能 `JSON.parse` 则解析，否则字符串）；插入后刷新列表。
3. 去掉 `/data` 菜单，重定向到 `/databases`；删除或掏空 `DataManager.vue`。

### Phase 4 — 收尾

1. 导航标题「数据库管理」。
2. `yarn build` 通过。
3. 本文状态 → implemented / PR。
4. 开 PR（截图说明 + 测试计划）。

## 技术约束

- 沿用 Ant Design Vue 4 + `PageContainer`。
- 非就绪库：禁用 SQL / 集合操作并给 tooltip。
- 提交不含密钥；不重新引入 Turso。
- UI 文案以中文为准。
- services：先改 `types.ts`，再改 `http-api.ts` / `mock.js`。

## 验收（A1–A9）

| ID | 标准 |
|---|---|
| A1 | 侧栏只有「数据库管理」，无独立 SQL / 数据管理入口 |
| A2 | 访问 `/sql`、`/data` 均落到 `/databases` |
| A3 | 就绪库行 **SQL** 打开绑定该库的工作台；非就绪禁用 + tooltip |
| A4 | SQL 结果在同一弹窗、编辑器下方；成功空结果「暂时未查询到数据」；未执行时温和空提示 |
| A5 | 展开行为圆 plus/minus；展开区 `CollectionPanel`；**新建集合** 弹窗校验集合名 |
| A6 | 集合/文档请求走 `/databases/:dbId/data/...`；提供 `databaseId` 时不得静默用第一个库 |
| A7 | 集合行「查看数据」「新增文档」；文档列表可分页；空列表「暂时未查询到数据」 |
| A8 | KV 弹窗提交 `{ [key]: parsedValue }`，插入后列表刷新 |
| A9 | 导航标题「数据库管理」；`yarn build` 通过；弹窗均走 `SbModal` |

## 不做

- 不恢复 Turso。
- 不把 SQL 结果拆到独立路由或第二个弹窗。
- 不为未就绪库假装能查集合。
