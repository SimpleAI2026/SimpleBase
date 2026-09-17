# SimpleBase JavaScript SDK 计划

> **状态**：已实现（MVP，本地 packages/js-sdk）  
> **日期**：2026-09-17  
> **目标**：提供类似 Supabase 的浏览器 / Node 公用 JS SDK——用一把 API Token（Bearer Key）读写当前项目的数据库与 S3；附带 examples 与写入 `docs/` 的使用文档（顺带验证 Wiki）。  
> **关联**：[`proto-http.md`](./proto-http.md)、[`ui-docs-wiki-plan.md`](./ui-docs-wiki-plan.md)、[`databases-and-s3-plan.md`](./databases-and-s3-plan.md)

---

## 0. 一句话目标

发布 **`@simplebase/sdk`**（工作名）：`createClient({ url, apiKey, projectId })` 后，通过同一 Token 调用 HTTP `/v1`，覆盖 **SQL / 文档集合 / S3 对象**，并在仓库内提供可运行 examples + Wiki 模块「SDK」。

---

## 1. 背景与动机

| 现状 | 缺口 |
|---|---|
| 控制台 UI 经 `ui/src/services/http.ts` 调 `/v1`，Bearer Key 写死/存 localStorage | 外部 App / 脚本无法复用同一套类型安全客户端 |
| 契约已在 `proto-http.md`：databases、query/execute、data/collections、s3/objects | 缺 npm 包、缺官方示例、缺面向开发者的文档页 |
| DevMode 种子 Key：`sb_live_dev_key_12345`，项目 UUID 固定 | SDK 文档可用该种子做「复制即跑」 |

对标 Supabase：`createClient(url, key)` → `client.from()` / `client.storage`；SimpleBase 对应 `client.sql` / `client.collection` / `client.storage`（命名见 §4）。

---

## 2. 范围

### 2.1 做（MVP）

1. **JS/TS SDK 包**（`packages/js-sdk` 或仓库根 `sdk/js`）：  
   - ESM + CJS 双构建（`tsup` / `unbuild`）  
   - 浏览器与 Node 18+（原生 `fetch`）  
   - TypeScript 类型完整导出  
2. **能力面（只包已落地 HTTP）**：  
   - 数据库：list / get /（可选 create，需 DatabaseAdmin）  
   - SQL：`query`（只读）、`execute`（写）、`batch`  
   - Document KV：collections CRUD、documents CRUD  
   - S3：list / upload / delete / presign  
3. **Examples**：至少 4 个可跑脚本/页面（见 §7）。  
4. **Wiki 文档**：新增 `docs/sdk/` 模块，从安装到 API 参考；用现有 `/docs` 验证渲染与深链。

### 2.2 不做（本计划）

- 不实现新的后端鉴权模型（继续现有 API Key + 权限位）。  
- 不包含 LLM / Cloud Agent / Metrics / Logs（可列 Phase 2）。  
- 不做 Realtime / WebSocket 订阅。  
- 不做 React/Vue 专用 hooks（examples 可用原生或轻量示例）。  
- 不发布到 npm 的自动化流水线可作为 Phase 2（MVP 可 `yarn link` / workspace 引用）。

---

## 3. 鉴权与安全模型

### 3.1 Token

与现网一致：

```http
Authorization: Bearer <API_KEY>
```

- Key 由控制台 / 系统库签发；权限位决定能否读/写库与 S3（`DatabaseRead` / `DatabaseWrite` / `DatabaseAdmin` / …）。  
- SDK **不**在客户端推导权限：401/403 原样抛出 `SimpleBaseError`。

### 3.2 初始化

```ts
import { createClient } from '@simplebase/sdk'

const sb = createClient({
  url: 'http://127.0.0.1:8080',           // 或 https://api.example.com
  apiKey: 'sb_live_dev_key_12345',
  projectId: '00000000-0000-0000-0000-000000000002',
  // optional:
  // databaseId: 'default',               // 默认库，可在调用时覆盖
  // fetch: customFetch,
  // headers: { 'X-Request-Id': '...' },
})
```

所有项目级路径前缀：`/v1/projects/:projectId/...`。

### 3.3 安全文案（必须写进 docs）

- **浏览器暴露 Key = 拥有该 Key 全部权限**。公开站点应使用**仅 Read** 的受限 Key，或只走后端 BFF。  
- 写操作、上传、删库 Key 禁止进前端生产包。  
- 对标 Supabase「anon key 进前端 + RLS」：SimpleBase **当前无行级 RLS**，文档必须明确「权限边界 = API Key 权限集」。

---

## 4. SDK 表面设计（API 草图）

### 4.1 顶层

```ts
type SimpleBaseClient = {
  readonly projectId: string
  /** 切换默认 databaseId（不改 Key） */
  database(id: string): DatabaseRef
  /** 默认库；若未配置 databaseId 则第一次 SQL/data 前需 .database(id) */
  sql: SqlApi
  collection(name: string): CollectionApi
  storage: StorageApi
  databases: DatabasesApi
  raw: RawHttp  // escape hatch: request(path, init)
}
```

### 4.2 Databases

```ts
sb.databases.list(opts?: { limit?: number; cursor?: string })
sb.databases.get(databaseId: string)
sb.databases.create({ name: string })           // DatabaseAdmin
sb.databases.open(databaseId) / .close(...)    // Admin
sb.databases.remove(databaseId)                // soft delete
```

映射：`GET/POST/DELETE :p/databases...`（见 proto §3.1）。

### 4.3 SQL

```ts
const db = sb.database('default')

await db.sql.query<Row>('SELECT id, data FROM users WHERE id = ?', ['u1'], { maxRows: 100 })
await db.sql.execute('INSERT INTO users (id, data, created_at) VALUES (?, ?, ?)', [id, json, ts])
await db.sql.batch([{ sql, args }], { transactional: true })
```

映射：`POST :p/databases/:id/query|execute|batch`。  
强制参数化；SDK 对空 SQL / 非数组 args 做本地校验。

### 4.4 Collections（Document KV）

```ts
const users = db.collection('users')

await users.list()                              // GET .../data/collections/users
await users.insert({ id?: string, ...fields })  // POST documents
await users.update(id, fields)                  // PUT document
await users.remove(id)                          // DELETE
await db.collections.create('users')            // POST collections
await db.collections.list()
```

映射：proto §3.3。集合名校验：`[a-zA-Z_][a-zA-Z0-9_]*`（与后端一致，实现时再对齐源码）。

### 4.5 Storage（S3）

```ts
await sb.storage.list({ prefix?: string, refresh?: boolean })
await sb.storage.upload(key, fileOrBlobOrUint8Array, { contentType?: string })
await sb.storage.remove(key)
await sb.storage.presign(key)  // → { url }
```

映射：`GET/POST/DELETE :p/s3/objects`、`GET :p/s3/presign`。  
浏览器用 `FormData`；Node 用 `Blob` / `File` / `Buffer` 转 FormData（或 undici FormData）。

### 4.6 错误

```ts
class SimpleBaseError extends Error {
  status: number
  code: string          // invalid_api_key | sql_not_allowed | ...
  requestId?: string
  details?: unknown
}
```

解析后端统一 `{ error: { code, message }, request_id }`（以 proto 为准）。

---

## 5. 包结构

```
packages/js-sdk/                 # 推荐 monorepo workspace
  package.json                   # name: @simplebase/sdk
  tsconfig.json
  tsup.config.ts
  src/
    index.ts                     # createClient 导出
    client.ts
    http.ts                      # fetch wrapper + auth header
    errors.ts
    databases.ts
    sql.ts
    collections.ts
    storage.ts
    types.ts
  README.md                      # 指向 docs/sdk
examples/
  node-sql/                      # tsx 脚本：query + insert
  node-documents/                # collection CRUD
  node-storage/                  # upload + list + presign
  browser-quickstart/            # 静态 HTML + ESM CDN 或 Vite 小页
docs/sdk/                        # Wiki 模块（见 §8）
  index.md
  install.md
  quickstart.md
  auth.md
  database-sql.md
  documents.md
  storage.md
  errors.md
  examples.md
```

根 `package.json` workspaces 增加 `packages/js-sdk`、`examples/*`（若尚无 workspace 则 MVP 可先独立 `sdk/js` + 相对路径 examples）。

---

## 6. 实现要点

| 项 | 决策 |
|---|---|
| HTTP | 原生 `fetch`；可注入 |
| 序列化 | JSON；SQL args 按后端已有编码（number/string/bool/null） |
| 上传 | `multipart/form-data` 字段名与后端一致：`key` + `file` |
| Tree-shaking | 子路径导出可选：`@simplebase/sdk/storage`（Phase 2） |
| 测试 | Vitest：mock fetch 契约测试；可选对本地 `./build.sh dev` 的 smoke（标记 integration） |
| 版本 | 0.1.0 与 HTTP 契约同步；破坏性变更跟 proto major |

**默认 databaseId**：`createClient` 可传；未传时 `sb.sql` 抛「请先 database(id)」或读环境变量 `SIMPLEBASE_DATABASE_ID`。

---

## 7. Examples（验收可跑）

均使用 DevMode 种子（文档写明）：

| Example | 演示 |
|---|---|
| `examples/node-sql` | `query` 选一行 + `execute` 插入 |
| `examples/node-documents` | 建集合（若无）+ insert/update/list/delete |
| `examples/node-storage` | upload 小文件 → list prefix → presign → delete |
| `examples/browser-quickstart` | 输入 url/key/projectId，按钮触发 list databases + list objects |

每个 example 含 `.env.example`：`SIMPLEBASE_URL` / `SIMPLEBASE_API_KEY` / `SIMPLEBASE_PROJECT_ID` / `SIMPLEBASE_DATABASE_ID`。

---

## 8. Wiki 文档（`docs/sdk`）— 验证文档站

### 8.1 `_meta.json` 增加模块

```json
{ "id": "sdk", "title": "JS SDK", "order": 4 }
```

### 8.2 页面

| 文件 | 内容 |
|---|---|
| `index.md` | SDK 是什么、与控制台关系、功能矩阵 |
| `install.md` | npm/yarn/pnpm、CDN（若做 IIFE） |
| `quickstart.md` | 5 分钟：createClient → sql.query |
| `auth.md` | Bearer Key、权限、前端安全 |
| `database-sql.md` | databases.* + sql.* |
| `documents.md` | collection API |
| `storage.md` | S3 API |
| `errors.md` | SimpleBaseError 与常见 code |
| `examples.md` | 链到仓库 `examples/` |

文档内代码块与 SDK 最终签名一致；相对链接用 `./quickstart.md` 以便 Wiki 改写。

### 8.3 验收（文档站）

1. 顶栏「使用文档」新标签打开后，顶模块可见 **JS SDK**。  
2. 左侧列出上表页面；深链 `/docs/sdk/quickstart` 刷新可用。  
3. 代码块、表格渲染正常。

---

## 9. 实施阶段

### Phase 0 — 骨架（0.5d）

- 建 `packages/js-sdk`、tsup、`createClient` + `raw.request`。  
- 错误解析单测。

### Phase 1 — 数据面（1–2d）

- databases + sql + collections + storage。  
- Vitest 契约 mock。  
- `examples/node-sql`、`node-documents`。

### Phase 2 — Storage example + 浏览器（0.5–1d）

- `examples/node-storage`、`browser-quickstart`。  
- README。

### Phase 3 — Wiki（0.5d）

- `docs/sdk/*` + `_meta.json`。  
- 人工点一遍 `/docs/sdk/...`。

### Phase 4 — 可选增强

- npm publish CI；LLM/Agent 客户端；Realtime；`@simplebase/sdk/react`。

---

## 10. 验收标准（MVP = Phase 0–3）

1. `import { createClient } from '@simplebase/sdk'` 在 Node example 中可对本地 dev 服务完成：list DB → SQL query → collection insert → storage upload/list。  
2. 错误 Key 得到 `SimpleBaseError` 且 `code=invalid_api_key`（或现网等价）。  
3. `docs/sdk` 在 Wiki 可浏览；至少 quickstart / sql / storage 三页。  
4. `packages/js-sdk` 构建产物含 `.mjs` + `.cjs` + `.d.ts`。  
5. 不引入新的后端路由（除非发现契约 bug，另开修复）。

---

## 11. 风险与对策

| 风险 | 对策 |
|---|---|
| 浏览器 Key 泄漏 | docs/auth 醒目警告；example 默认读环境变量 |
| FormData 在 Node 差异 | 统一用 `Blob` + undici；测 Node 18/20 |
| ducklake 大文档拖慢 Wiki 包 | SDK 文档保持短页；与 database/ducklake 长文分离 |
| API 字段漂移 | SDK 类型从 proto 摘录；变更走 proto PR |

---

## 12. 决议摘要

1. 包名工作名 **`@simplebase/sdk`**，`createClient({ url, apiKey, projectId })`。  
2. MVP 覆盖 **SQL + Documents + S3**，鉴权复用现有 Bearer API Key。  
3. Examples 可跑；**使用文档进 `docs/sdk`** 验证 Wiki。  
4. 本轮**只出本 plan**；实现另开任务按 Phase 0→3 推进。
