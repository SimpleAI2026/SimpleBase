# Sandbox Specification

## Purpose

定义云沙盒的生命周期、隔离、错误码与终端访问契约，确保隔离不可用时 fail-closed 且对外错误稳定。

## Requirements

### Requirement: 生命周期与资源限额

沙盒默认 SHALL 为 1 CPU / 256MB，空闲 5 分钟回收，最长运行 30 分钟；单次 exec 超时 30 秒（可配置到 300 秒），输出上限 65536 字节，单文件上限 1MiB，每项目最多 5 个，回收扫描 5 分钟。

#### Scenario: 超过项目配额
- **WHEN** 项目已有 5 个沙盒时再创建
- **THEN** 返回 `limit_exceeded`（429），不创建新沙盒

#### Scenario: 空闲回收
- **WHEN** 沙盒空闲超过 5 分钟
- **THEN** 被回收；后续 exec 返回 `gone` 而不是在宿主上继续执行

### Requirement: 错误码与退出语义

沙盒错误 MUST 使用固定集合：`sandbox_unavailable`(503)、`not_found`(404)、`name_conflict`(409)、`limit_exceeded`(429)、`invalid_spec`(400)、`invalid_path`(400)、`file_too_large`(413)、`busy`(409)、`gone`(410)、`backend_error`(502)，且 MUST NOT 透传底层 SDK 路径。命令非零退出 SHALL 返回 200 并由 `exit_code` 表达；超时 SHALL 返回 200 + `timed_out=true` + `exit_code=-1`。

#### Scenario: 命令失败
- **WHEN** 沙盒内命令以非零码退出
- **THEN** HTTP 返回 200，响应包含真实 `exit_code` 与输出

#### Scenario: 命令超时
- **WHEN** 命令执行超过超时
- **THEN** 返回 200，`timed_out=true`，`exit_code=-1`，不吞掉已产生的输出

### Requirement: 隔离与出网控制

本机执行 MUST 经 `gosandbox/host` 的隔离（bwrap + Landlock + seccomp 或 Seatbelt）；隔离开启但不可用时 MUST 返回 `isolation_unavailable`，不得退回裸执行。`egress` 档 MUST 使用沙盒自己的网络命名空间，并用黑洞路由拦截云元数据、私网、CGNAT、宿主回环与链路本地地址；pasta 不可用时 MUST 无出网（fail-closed）。沙盒环境变量 MUST 拒绝 `SIMPLEBASE_`、`MSB_`、`AWS_` 前缀。

#### Scenario: 隔离不可用
- **WHEN** 宿主缺少 bwrap/Landlock 或策略加载失败
- **THEN** 创建或执行返回 `isolation_unavailable`，不启动未隔离进程

#### Scenario: 沙盒读取云凭据
- **WHEN** `egress` 档沙盒访问 `169.254.169.254` 或 `10/8` 私网
- **THEN** 连接失败（黑洞路由），沙盒内错误可见且宿主记录规则命中元数据

### Requirement: 终端与 E2B 兼容面

终端 SSH 与 E2B 兼容路由 MUST 保持锁定集合不变。终端票据 MUST 一次性使用；SSH 认证器 MUST 在路由构造前装配完成，不得在 router 构造期回填。跨项目访问 MUST 被拒绝，审计只记录命令 SHA-256。

#### Scenario: 票据重放
- **WHEN** 已使用的终端票据被再次提交
- **THEN** 认证失败，不建立新会话

#### Scenario: 跨项目终端
- **WHEN** 用户用自己项目的票据连接其他项目的沙盒
- **THEN** 连接被拒绝，不泄露目标沙盒信息
