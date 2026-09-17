---
title: 安装
order: 2
---

# 安装

当前尚未发布到 npm。在仓库内构建后通过 `file:` 引用：

```bash
cd packages/js-sdk
yarn
yarn build
```

在应用或 example 的 `package.json`：

```json
{
  "dependencies": {
    "@simplebase/sdk": "file:../../packages/js-sdk"
  }
}
```

要求 **Node ≥ 18**（或带原生 `fetch` / `FormData` / `Blob` 的浏览器）。

```ts
import { createClient } from '@simplebase/sdk'
```
