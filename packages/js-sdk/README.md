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

Docs: see [`docs/sdk/`](../../docs/sdk/) in the repo Wiki (`/docs`).
