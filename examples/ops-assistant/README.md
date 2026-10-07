# 运维助理

```bash
./examples/init.sh --only ops-assistant
set -a; . examples/ops-assistant/.env; set +a
go run ./examples/ops-assistant/server
bash examples/ops-assistant/scripts/verify-perms.sh
```

## 验证清单

控制台 `ex-ops01` 有 `ops` 库（工单、事件、SLA 标记与 `_migrations`）、`ticket_meta` 集合、KV 待办列表、`sla.go` v1 和 `ops_sla_scan` completed 记录；两把 Key 分别用于服务和只读请求。服务提供 `/api/tickets` 和 `/api/health`。

`verify-perms.sh` 预期只读 SQL/KV、函数调用与文件列表成功，写操作及跨项目访问返回 403。若本地运行的是 M1 之前的后端，请重启 `./build.sh dev`，否则只读 KV 和跨项目用例不会通过。

LLM provider 未配置时，agent 未创建，标记为“未验证”。限制：G0b、G1、G2、G4、G6、G11、G12（`plan/planv4.0/examples-business-cases-plan.md` §6）。
