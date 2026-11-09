# 示例

能力示例（资源 CLI 与云助手）在 [`capabilities`](capabilities/README.md)。一条命令跑完 01–15：

```bash
./examples/run_all.sh --start
```

- `11-backup` 需要 `SIMPLEBASE_BACKUP_ENABLED=true`（默认关闭时 `/backups` 路由不挂载，用例按 SKIP 处理）：`SIMPLEBASE_BACKUP_ENABLED=true ./examples/run_all.sh --start`。
- `14-sandbox` / `15-llm` 是可选能力，前置不满足（本机无隔离、无 LLM provider）时打印 SKIP 并退出 0。
- 逐站端到端验证（skill 工作区重放 + A/B 双实例）见 [`scripts/examples-verify/verify.sh`](../scripts/examples-verify/verify.sh) 与 [执行报告](../plan/planv6.4/examples-skill-verification-report.md)。

## 业务案例

五个案例各有独立项目，使用同一套 `simplebase.json` 清单驱动初始化。BFF 只监听 `127.0.0.1`；前端不携带 Key。案例目录的 `.env` 仅存运行时最小权限 Key，初始化时超管 JWT 只存在进程内存中。

| 案例 | 项目 | 页面 | 重点 |
|---|---|---|---|
| [`shop`](shop/README.md) | `ex-shop1` | `:8092` | JS SDK、KV 购物车、事务下单、云函数算价、商品图片 |
| [`jd`](jd/README.md) | `ex-jd001` | `:8093` | 仿京东：搜索/秒杀/优惠券/结算抽屉、幂等下单、推荐与算价云函数、平台能力页 |
| [`community`](community/README.md) | `ex-cmty1` | `:8096` | Go SDK、文档/SQL 同库双模型、附件、热榜、LLM 摘要 |
| [`iot-telemetry`](iot-telemetry/README.md) | `ex-iot01` | `:8098` | 设备读数、5 分钟聚合、导出对象、interval/once、触发器审计 |
| [`ops-assistant`](ops-assistant/README.md) | `ex-ops01` | `:8097` | 工单事务、只读 Key 验证、SLA 函数、附件与触发器 |

五个案例都覆盖同一批平台能力，便于对照同一份清单在不同业务下的用法：

| 能力 | 五个案例是否都有 |
|---|---|
| 数据库（SQL 表 + 迁移） | 全部 |
| 集合文档 | 全部（`shop_events` / `jd_events` / `posts` + `platform_events` / `device_events` / `ticket_meta` + `ops_events`） |
| KV | 全部（string / hash / list / set / zset 均有覆盖） |
| 对象存储 | 全部（`products/` / `jd/` + `jd/reviews/` / `posts/` / `exports/` / `ops/attachments/`） |
| 云函数 | 全部（业务函数 + `audit` 触发器回调） |
| 定时任务 | 全部（`shop_daily` / `jd_daily` / `community_hot` / `iot_rollup` + `iot_export_once` / `ops_sla_scan`） |
| 事件触发器 | 全部（record_change + object_event + http 三类） |
| 触发器审计落库 | 全部（`audit_log` 表 + `/api/platform` + 页面展示） |

触发器只存在于系统库、不进 `dev apply`，由 `examples/lib/init/run.mjs` 按
（kind, target, func_file, func_export）幂等同步，并写出 `.simplebase/triggers.json` 快照供 BFF 展示
（管理面要求 `project:admin`，运行时 Key 读不到）。

```bash
./build.sh dev
./examples/init.sh [--url http://127.0.0.1:8080] [--user U --password P] \
  [--only shop,jd] [--project-prefix ex] [--skip-build] [--reset-data] [--dry-run]
```

初始化脚本把库、迁移、集合、云函数和定时任务交给 `simplebase dev apply`（需要先 `./build.sh` 生成根目录 `simplebase`，或用 `SIMPLEBASE_BIN` 指定）；项目创建、Key 签发、KV / 对象 / 助手种子和冒烟仍由脚本完成。`trigger_once` 有副作用，不进入 `dev apply`，由脚本在任务首次创建时触发一次。同一套机制见[本地开发、云端重放](../docs/getting-started/local-cloud-collab.md)。

默认种子超管 `simplebase2026`，密码可通过 `SIMPLEBASE_ADMIN_PASSWORD` 覆盖。每次执行签发新运行时 Key 并吊销 `.env` 记录的旧 Key（旧 Key 来自其他实例、在目标实例上已不存在时跳过，不中断初始化）；勿将 `.env`、JWT 或 Key 提交仓库。控制台打开 `http://127.0.0.1:5173/?project=ex-shop1`，其他项目以 ID 替换；各案例 README 有可直接核验的数据表、KV、对象和任务清单。

恢复手段：`./build.sh reset -y` **会清掉同一实例上的所有项目和本地数据**；随后 `./build.sh dev`、`./examples/init.sh` 重新创建。远程 S3 数据不随本地 reset 删除；谨慎操作。

平台缺口 2026-11-06 逐条复核（对照 [`examples-business-cases-plan` §6](../plan/planv4.0/examples-business-cases-plan.md#6-平台缺口与处理决策)）：G0（项目归属校验）、G3（KV 读命令权限）、G9（迁移工具，已由 `simplebase dev` 的 up-only 迁移补位）、G14（Realtime / 订阅已实现）不再构成限制；G5 已按该文档决策改为超管登录 + 每次初始化轮换运行时 Key。仍适用：G0b、G1、G2、G4、G6、G7、G8、G10、G11、G12、G13、G15、G16（各案例 README 的「限制」一节只引用这些编号）。
