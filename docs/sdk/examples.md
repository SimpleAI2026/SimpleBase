---
title: 示例
order: 9
group: JavaScript
---

# 示例

完整初始化说明见 [`examples/README.md`](https://github.com/linkxzhou/SimpleBase/blob/main/examples/README.md)。

| 目录 | 内容 |
|---|---|
| [`examples/shop`](https://github.com/linkxzhou/SimpleBase/blob/main/examples/shop/README.md) | JS SDK、电商事务和缓存 |
| [`examples/community`](https://github.com/linkxzhou/SimpleBase/blob/main/examples/community/README.md) | Go SDK、文档和 SQL、对象存储 |
| [`examples/iot-telemetry`](https://github.com/linkxzhou/SimpleBase/blob/main/examples/iot-telemetry/README.md) | HTTP 脚本、interval/once 任务 |
| [`examples/ops-assistant`](https://github.com/linkxzhou/SimpleBase/blob/main/examples/ops-assistant/README.md) | Go SDK、最小权限、可观测 |

```bash
./build.sh dev
./examples/init.sh
```

`.env` 仅供可信服务与脚本读取，不应放进浏览器代码。
