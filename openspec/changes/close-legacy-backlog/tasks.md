# Tasks

> 每条先复核代码现状，再决定「实施 / 已做关闭 / 明确不做」。来源：原 `legacy-plan-consolidation-plan` §3（原文见 git 历史）。

## 1. A 组：数据保留与系统库卫生（A1/A2 已拆出）

- [ ] 1.1 A3：在真实 COS 复测九行验收指标（文件数、冷/热登录与日志、`wait_total`、`snapshot moved during copy`），记录数字

## 2. B 组：读性能 / DuckLake（B2 已并入 add-ops-maturity）

- [ ] 2.1 B1：用户库后台维护（flush + merge）、远端读缓存、`ListDocuments` 分页（默认 50 / 上限 500）、COPY 前 `WaitCount`
- [ ] 2.2 B3：集合名→建表状态缓存，写路径失效；连续 100 次写不重复建表
- [ ] 2.3 B4：确认生产镜像预装 DuckDB 扩展；冷启无 `INSTALL`
- [ ] 2.4 B5：把 `godatabase/bench` 与真实 COS 复测对齐成一张表（本地盘 / 假 S3 / 真实 COS）

## 3. C 组：多实例一致性（先复核，禁止按旧文直接改）

- [ ] 3.1 C1：复核启动探针与 `PutIfAbsent` 单测；两实例同库启动时第二个明确拒绝
- [ ] 3.2 C2：`pruneVersions` 失败上抛，`syncOnce` 不再忽略；`dirty==0` 不走全量 COPY
- [ ] 3.3 C3：`local-state.json` 水位与 `syncOnce` 顺序；重启后水位不倒退、重复同步幂等
- [ ] 3.4 C4：系统库 `locator.json` 跨节点恢复；若不做，明确降级为单实例限制并统一文档/告警/`replicas=1` 口径

## 4. D 组：功能缺口

- [ ] 4.1 D1：逐条决策 examples 的 G0b/G1/G2/G4/G6–G8/G10–G13/G15/G16，做或明确不做并更新 README
- [ ] 4.2 D2：gofunction 版本七项（test 独立超时 + semaphore、cron 记账、复制响应、保存后生效、历史版只读源码、`/go?version=`、细分错误码）
- [ ] 4.3 D3：provider-catalog 路由、LLM stream 落库、前端接系统 API、系统库备份 / 只读开关、S3 索引 reconcile
- [ ] 4.4 D4：真实 Cloud 沙盒生命周期与 `network=none` 冒烟；后续四项按需立项
- [ ] 4.5 D5：补 `docs/sdk/sandboxes.md`、`reference/errors.md`、`reference/config.md` 并过文档站校验
- [x] 4.6 D6：确认 `SIMPLEBASE_CATALOG_DATABASE_ID` 无引用后退役，写 breaking 发布说明

## 5. E 组：前端

- [x] 5.1 E1：补 `http-api-converters.ts` 与 `services/` 分支，覆盖率回到 90% 阈值以上
- [ ] 5.2 E2：统一工具栏间距、表尾 padding、Dashboard/CronJobs 状态文案
- [x] 5.3 E3：拆 `AgentManager.vue` 与 `http-api.ts`（单文件 <600 行），typecheck 0 错误
- [ ] 5.4 E4：ui-unslop P4 截图回归（桌面 / 窄屏、浅色 / 深色）并记录差异

## 6. G 组：评测与 Agent / CLI 能力

- [ ] 6.1 G1：CLI `max_tokens` 配置与参数，超限走 `ErrMaxTokensExceeded`
- [ ] 6.2 G2：定义 no-progress 停止判据并补用例
- [ ] 6.3 G3：云端 summarization + 按轮截断，替代固定 `maxHistoryChars`
- [ ] 6.4 G4：L2 specialists（默认关闭）+ 分流用例
- [ ] 6.5 G5：长工具执行期间推送中间帧
- [ ] 6.6 G6：`verify_l3` 覆盖「命令被挂载挡住」的真实 deny
- [ ] 6.7 G7：评测报表打印 token / 费用，并加预算护栏
- [ ] 6.8 G8：`workflow_dispatch` 评测入口与结果上传
- [ ] 6.9 G9：supervisor/advisor P1–P5（默认关闭 + 用量上限）
- [ ] 6.10 G10：正式废弃 E5 `eval_test.go` 方案并清理文档引用

## 7. H 组：生产就绪剩余项（H1/H2/H6 已拆出）

- [ ] 7.1 H3：云函数出网控制与函数级 secrets，secret 不进日志 / 审计
- [ ] 7.2 H4：触发器持久队列 + 项目删除 / 导出 / 成员角色 API
- [ ] 7.3 H5：examples `16-production-profile` / `17-backup-restore` / `18-secrets` / `19-end-users` / `20-alerts`

## 8. I 组：平台能力缺口

- [ ] 8.1 I1：PocketBase 对照 6 项（relation/expand、索引 API、schema 导入 diff、集合重命名、路径级限流、SMTP）逐项决策
- [ ] 8.2 I2：沙盒违规监控 `Result.Violations`、祖先 pin、`--args fd` 与 P2 项
- [ ] 8.3 I3：`gitdir:` 跟随 + Linux CI 真 bwrap 用例
- [ ] 8.4 I4：`docs/automation/sandbox-api.md` 与 `sandbox.md` 对齐 E2B 路由与差异表
- [ ] 8.5 I5：云函数单一路径（cron 与 HTTP 共用 Runner，删旧兜底）
- [ ] 8.6 I6：gosandbox `egress` 档公网可达与宿主回环隐藏的 Linux 真机补测（pasta 可用环境）

## 9. J 组：examples 与本地开发

- [ ] 9.1 J1：`.simplebase/` 加入 `.gitignore`
- [ ] 9.2 J2：文档标注已失效的 `sb_live_*` Key，勿复用
- [ ] 9.3 J3：local-dev P1（`/health/live` 带 version + major 校验、漂移检测、`pull`、CI 示例）
- [ ] 9.4 J4：local-dev P2（多环境 profile、种子审批、离线 tarball、控制台「从工作区导入」）
- [ ] 9.5 J5：三条设计限制写清楚（plan 不能预演建库后步骤、引擎检查是项目级、expect.json 三处同改）
- [ ] 9.6 J6：认证（user/apikey）进 skill，先定敏感写操作确认流
- [ ] 9.7 J7：`14-sandbox` 示例在 bwrap + Landlock 的 Linux 上复跑

## 10. K 组：工程债

- [ ] 10.1 K1：删除 `sys_databases.storage_prefix` 与 `catalog.Database.StoragePrefix`，删库退路改用 `sys_projects.database_path`
- [ ] 10.2 K2：删除或实现 `Executor.Compile` / `Runtime.Cleanup`，README 标注
- [ ] 10.3 K3：`internal/api/perfbench` 移出到独立包
- [x] 10.4 K4：`TestQuotaHandler` / `TestAuditHandler` 的 missing_project 断言改为 400
- [ ] 10.5 K5：gofunction handler 4 处 `system_project_protected` 构造收敛
- [x] 10.6 K6：复核前端 typecheck 残留测试类型错误并修复
- [ ] 10.7 K7：gosandbox README 补 bwrap + helper 组合耗时与 `memory.max` 实测
- [ ] 10.8 K8：gofunction A4 慢路径或调整验收线并注明归因
- [x] 10.9 K9：修正 `gofunction/bench_report.md` 指向不存在的计划路径

## 完成情况核对（2026-11-26）

已关闭（6）：
- 4.6 D6：`docs/ops/deployment.md` 已写明 `catalog.database_id` / `SIMPLEBASE_CATALOG_DATABASE_ID` 删除且无效果。
- 5.1 E1：`yarn test` 覆盖率 statements 97.68% / branches 93.44% / functions 91.71%，高于 90% 阈值。
- 5.3 E3：`AgentManager.vue` 553 行、`http-api.ts` 15 行（桶文件），均 <600。
- 10.2 K2：`gofunction/README.md` 已标注为预留空实现，但 API 仍在，未删除 / 未实现，保持待办。
- 10.4 K4：`quota_audit_handler_test.go` 的 `missing_project` 已断言 400 `invalid_request`。
- 10.6 K6：`yarn typecheck` 0 错误。
- 10.9 K9：`gofunction/bench_report.md` 的失效链接已移除。

未关闭：其余 48 项保持待办；C 组（C1–C4）仍标注「未核实」，开工前必须先读当前实现。
