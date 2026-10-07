---
title: SDK
order: 1
---

# SDK

SimpleBase 提供 JavaScript SDK（`@simplebase/sdk`）和 Go SDK（`packages/go-sdk`），统一使用 Bearer API Key 调用 `/v1`。服务端可选择对应语言的客户端；浏览器中仅使用受限读 Key，写权限 Key 不得嵌入公开前端。

## 文档目录

1. [安装](/docs/sdk/install)
2. [快速开始](/docs/sdk/quickstart)
3. [鉴权与安全](/docs/sdk/auth)
4. [数据库与 SQL](/docs/sdk/database-sql)
5. [文档集合](/docs/sdk/documents)
6. [对象存储](/docs/sdk/storage)
7. [错误处理](/docs/sdk/errors)
8. [示例](/docs/sdk/examples)

Go SDK 从 [安装](/docs/sdk/go-install) → [快速开始](/docs/sdk/go-quickstart) → [数据库与 SQL](/docs/sdk/go-database-sql) 阅读。JavaScript 包路径：`packages/js-sdk`。可运行示例见 [examples/README.md](https://github.com/linkxzhou/SimpleBase/blob/main/examples/README.md)；仓库中按业务场景组织 `shop`、`community`、`iot-telemetry` 与 `ops-assistant`。
