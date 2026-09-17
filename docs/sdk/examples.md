---
title: 示例
order: 9
---

# 示例

先构建 SDK：

```bash
cd packages/js-sdk && yarn && yarn build
```

| 目录 | 内容 |
|---|---|
| `examples/node-sql` | CREATE / INSERT / SELECT |
| `examples/node-documents` | collection CRUD |
| `examples/node-storage` | upload / list / presign / delete |
| `examples/browser-quickstart` | 静态页演示（勿放写权限 Key） |

```bash
cd examples/node-sql
yarn
export SIMPLEBASE_DATABASE_ID=<your-db-uuid>
yarn start
```
