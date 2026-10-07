# 业务案例

四个案例各有独立项目，使用同一套 `simplebase.json` 清单驱动初始化。BFF 只监听 `127.0.0.1`；前端不携带 Key。案例目录的 `.env` 仅存运行时最小权限 Key，初始化时超管 JWT 只存在进程内存中。

| 案例 | 项目 | 重点 |
|---|---|---|
| [`shop`](shop/README.md) | `ex-shop1` | JS SDK、购物车、事务下单、实时云函数、商品图片 |
| [`community`](community/README.md) | `ex-cmty1` | Go SDK、文档/SQL 双模型、双数据库、附件、热榜 |
| [`iot-telemetry`](iot-telemetry/README.md) | `ex-iot01` | 无 BFF，HTTP/控制台、遥测汇总、interval/once、worker |
| [`ops-assistant`](ops-assistant/README.md) | `ex-ops01` | 工单事务、只读 Key 验证、SLA 函数、健康指标 |

```bash
./build.sh dev
./examples/init.sh [--url http://127.0.0.1:8080] [--user U --password P] \
  [--only shop,community] [--project-prefix ex] [--skip-build] [--reset-data] [--dry-run]
```

默认种子超管 `simplebase2026`，密码可通过 `SIMPLEBASE_ADMIN_PASSWORD` 覆盖。每次执行签发新运行时 Key 并吊销 `.env` 记录的旧 Key；勿将 `.env`、JWT 或 Key 提交仓库。控制台打开 `http://127.0.0.1:5173/?project=ex-shop1`，其他项目以 ID 替换；各案例 README 有可直接核验的数据表、KV、对象和任务清单。

恢复手段：`./build.sh reset -y` **会清掉同一实例上的所有项目和本地数据**；随后 `./build.sh dev`、`./examples/init.sh` 重新创建。远程 S3 数据不随本地 reset 删除；谨慎操作。已知平台缺口（G0b–G16）及回避方式集中列于 [`examples-business-cases-plan`](../plan/planv4.0/examples-business-cases-plan.md#6-平台缺口与处理决策)。
