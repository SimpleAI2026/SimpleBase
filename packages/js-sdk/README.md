# @simplebase/sdk

JavaScript / TypeScript client for SimpleBase HTTP `/v1` (SQL, document collections, S3).

## Install

```bash
# from this monorepo (until published)
cd packages/js-sdk && yarn && yarn build
```

```ts
import { createClient } from '@simplebase/sdk'

const sb = createClient({
  url: 'http://127.0.0.1:8080',
  apiKey: process.env.SIMPLEBASE_API_KEY!,
  projectId: process.env.SIMPLEBASE_PROJECT_ID!,
  databaseId: process.env.SIMPLEBASE_DATABASE_ID
})
```

## 云沙盒 e2e

```ts
const result = await sb.sandboxes.run({
  files: [{ path: '/workspace/test.py', content: 'assert 1+1==2\nprint("ok")' }],
  command: 'python3 test.py'
})
if (result.timed_out || result.exit_code !== 0) throw new Error(result.stderr)
```

`run` 同步执行并清理临时沙盒（`keep:true` 可保留）。[查看完整云沙盒文档](../../docs/automation/sandbox-e2e.md)。

## 文档集合

`sb.database(id).collection(name)` 支持 `list({limit,cursor,offset,sort,filter,fields,count})`、`get`、`insert`、`update`、`patch`、`batch`、`remove`、`truncate` 和 `getSchema/putSchema/deleteSchema`。列表同时返回兼容的 `rows` 与结构化 `items/next_cursor/total`。filter 只接受已在 Schema 注册的字段。

`import({format,body,dryRun})` 上传原始 CSV/NDJSON；`export()` 返回原始 `Response`，调用方可直接消费 `body` 流。`dryRunView(query)` 校验逻辑视图并推断字段。

Docs: see [`docs/sdk/`](../../docs/sdk/) in the repo Wiki (`/docs`).
