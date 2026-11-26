# Dev Experience Specification

## Purpose

定义 SimpleBase 本地 CLI 的输出信封与退出码、TypeScript 与 Go SDK 的错误契约、四个站点示例清单的校验规则以及文档站链接要求，使本地开发、云端重放与样例验证在不同机器上可重复执行。

## Requirements

### Requirement: CLI 输出信封与退出码

`simplebase` CLI SHALL 使用统一信封：成功为 `{"ok":true,"data":…}`，失败为 `{"ok":false,"error":{code,message,http_status,request_id}}`，业务 `code` 原样透传。退出码 MUST 为 0（成功）、1（一般错误）、2（用法错误）、3（需要确认）、4（配置不安全）。

#### Scenario: 命令需要确认
- **WHEN** 破坏性命令在非交互模式下需要确认
- **THEN** 输出 `confirmation_required` 并以退出码 3 结束，不执行操作

#### Scenario: 服务端业务错误
- **WHEN** 服务端返回 400 与业务错误码
- **THEN** CLI 以退出码 1 结束并在 error 中保留服务端 code 与 request_id

### Requirement: SDK 错误与安全提示

TypeScript / Go SDK MUST 将 HTTP 错误映射为包含 `status`、`code`、`requestId` 与可选 `details` 的统一错误对象。浏览器环境使用 API Key 时 MUST 提示「该 Key 拥有其全部权限」，且 SDK MUST NOT 假定存在行级权限隔离。

#### Scenario: 捕获 SDK 错误
- **WHEN** 调用失败且服务端返回 `invalid_request`
- **THEN** 抛出的错误对象包含 status、code、requestId，调用方可按 code 分支

#### Scenario: 浏览器直连
- **WHEN** 应用在浏览器中携带 API Key 调用 SDK
- **THEN** 文档与类型注释提示 Key 权限范围，不宣称前端沙箱隔离

### Requirement: 示例工程清单一致

四个站点示例（`shop`、`community`、`iot-telemetry`、`ops-assistant`）MUST 显式声明 `data_model`、只声明一个数据库、`once` 任务写死 `run_at`；否则 `simplebase dev` 与 `sbdev.py` 的推断或校验必须失败。示例验证 MUST 在临时副本中执行，不得写入仓库内 `.simplebase/`。

#### Scenario: 清单缺字段
- **WHEN** 示例清单遗漏 `data_model` 或声明多个数据库
- **THEN** 两种入口均校验失败并给出一致提示

#### Scenario: 运行示例验证
- **WHEN** 执行 `examples/run_all.sh`
- **THEN** 使用参数化端口（默认 18xxx）与临时工作区，不修改仓库内样例目录

### Requirement: 文档站链接可验证

`docs/` 下的新增或修改文档 SHALL 通过文档站内容测试：frontmatter、目录登记、站内链接与锚点必须有效，不得链接到 `ui/` 之外的未发布目录（如 `plan/`）。

#### Scenario: 链接到 plan 目录
- **WHEN** 文档站页面包含指向 `plan/` 的链接
- **THEN** `docs-content.test.ts` 失败，需改为仓库内可访问地址或移除

#### Scenario: 新增文档
- **WHEN** 在 `docs/` 新增一篇文档
- **THEN** 同步登记目录并通过链接与锚点校验

### Requirement: 站点 CLI 与 SDK

`simplebase` CLI SHALL 提供 `file site enable|disable|status` 与 `file public <key> [--folder] --on|--off`；JavaScript SDK SHALL 提供 `s3.site.get`、`s3.site.set` 与 `s3.setPublic`。两种入口 MUST 与 HTTP API 使用同一语义与错误码。

#### Scenario: CLI 启用站点
- **WHEN** 管理员执行 `simplebase file site enable`
- **THEN** 站点开启，`status` 返回可发布状态与访问地址

#### Scenario: SDK 设置公开访问
- **WHEN** 调用 `s3.setPublic(key, true)`
- **THEN** 对象可经公开链接访问，返回结果与 HTTP API 一致

### Requirement: 部署清理需确认

`site deploy --prune` MUST 先列出将被删除的对象并要求显式确认；非交互模式 MUST 以退出码 3 结束且不删除任何对象。清理范围 MUST 限制在本次发布前缀内，MUST NOT 删除前缀外对象。

#### Scenario: 非交互清理
- **WHEN** 在 CI 中执行 `site deploy --prune` 而未提供确认
- **THEN** 输出待删清单并以退出码 3 结束，对象保持不变

#### Scenario: 确认清理
- **WHEN** 用户确认后执行
- **THEN** 只删除前缀内不属于本次产物的对象，结果列出删除数量
