# Platform Architecture Specification

## Purpose

定义 SimpleBase 后端的分层、依赖方向与生命周期边界，使各业务域只能通过显式端口协作，并让架构约束可以被自动化门禁检查。

## Requirements

### Requirement: 唯一组合根与反向释放

系统 SHALL 由 `internal/app` 完成全部跨模块装配与生命周期管理，任何模块不得自行构造其他模块的具体实现。启动失败时 SHALL 按创建顺序的逆序释放已分配资源；关停时 SHALL 先取消在途 Agent run 与有执行循环的轮次，再在独立收尾预算内等待写库完成。

#### Scenario: 启动中途失败
- **WHEN** 任一模块构造或初始化失败
- **THEN** 已创建的对象按逆序关闭，进程以非零状态退出，且不遗留后台协程

#### Scenario: 服务关停
- **WHEN** 进程收到 SIGTERM 或收到关停调用
- **THEN** 在途 run 落 `canceled`，有执行循环的轮次落 `canceled`（其 running 任务落 `interrupted`），`waiting_human` 与 `awaiting_plan_confirm` 轮次状态不变，且收尾写库在超时内完成

### Requirement: 入口按限界上下文分包

HTTP 入口 SHALL 由入口内核（`internal/api`：路由、守卫、中间件、错误兜底、健康观测、内置 MCP 派发）、协议内核叶子包（`api/httpkit`：错误协议、context、SSE、审计辅助、计时）与业务域子包（`api/<context>http`）组成。入口内核的 `Dependencies` SHALL 直接引用业务子包类型，不设置 `*_compat.go` 类型或构造函数转发层。业务域子包 MUST NOT import 入口内核、组合根或同级子包，且 MUST NOT 自带路由注册。

#### Scenario: 新增业务 handler
- **WHEN** 开发者新增一个业务 handler
- **THEN** 它位于对应 `<context>http` 子包，`internal/api` 顶层生产文件清单不变，架构门禁通过

#### Scenario: 子包反向依赖
- **WHEN** 任一 `<context>http` 子包 import `internal/api`、`internal/app` 或同级子包
- **THEN** 架构门禁失败并输出 `importer → imported` 与文件

### Requirement: 应用服务与 HTTP 解耦

Agent 协作的应用服务 SHALL 位于 `internal/agentrun`，不得 import Echo、`internal/api*` 或 `internal/app`。HTTP 层只负责协议解析、权限与归属校验、SSE 帧编码与 DTO 映射，不得持有群引擎或运行时实例。

#### Scenario: 群会话消息
- **WHEN** 客户端向群会话发送一条消息
- **THEN** HTTP 层完成鉴权与归属校验后调用应用服务提交轮次，自身不执行调度或状态迁移

#### Scenario: 停机窗口的新请求
- **WHEN** 应用服务已进入停机状态
- **THEN** 新建 run、轮次与群定时的请求返回 503 `service_stopping`，且不产生持久化写入

### Requirement: 系统库使用面收敛

新代码 MUST 只通过消费方窄接口使用系统库，不得持有 `*systemdb.Store` 具体类型；仓储写方法的调用点 SHALL 收口到登记的唯一出口。

#### Scenario: 绕过唯一出口写状态
- **WHEN** `internal/agentrun` 之外的包调用 `UpdateAgentRunStatus`，或非 `agentrun/group_store.go` 调用 `UpdateAgentTurnStatus`
- **THEN** 架构门禁失败并指出违规文件

#### Scenario: 新增包使用系统库
- **WHEN** 新增一个包并直接声明 `*systemdb.Store`
- **THEN** 架构基线检查失败（新包该项基线为 0），除非改用窄接口

### Requirement: 架构门禁随测试执行

依赖方向与系统库使用面基线 SHALL 由随 `go test` 执行的门禁强制；基线只能下降，不得因重构而上升。门禁 SHALL 扫描宿主 `internal/` 与四个嵌套独立模块，并拒绝独立模块 import 根模块。

#### Scenario: 引入反向依赖
- **WHEN** 任一包 import 了规则禁止的包
- **THEN** `TestArchitectureGate` 失败并给出违例条目；修复后通过

#### Scenario: 更新基线
- **WHEN** 等价搬迁导致计数下降
- **THEN** 用 `go test ./internal/app -run TestArchitectureGate -update` 重新生成基线，评审只接受下降的 diff
