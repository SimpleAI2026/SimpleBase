---
name: examples-business-cases-implementation
overview: 按现有业务案例计划，将 examples 中的演示替换为五个可运行的商城、小游戏、预约、社区和工单案例，集成可信 BFF、Go/JS SDK、数据库、KV、云函数、定时任务和私有前端构建物上传，并以自动测试和本地端到端验证交付。先保留或迁移原有压测资产及用户改动，不声称私有对象存储具备公开网站托管能力。
design:
  architecture:
    framework: html
  styleKeywords:
    - 现代业务界面
    - 清晰分层
    - 案例主题色
    - 轻量交互
  fontSystem:
    fontFamily: PingFang SC
    heading:
      size: 30px
      weight: 700
    subheading:
      size: 20px
      weight: 600
    body:
      size: 15px
      weight: 400
  colorSystem:
    primary:
      - "#2563EB"
      - "#0F766E"
    background:
      - "#F5F7FB"
      - "#FFFFFF"
    text:
      - "#172033"
      - "#526078"
    functional:
      - "#15803D"
      - "#B45309"
      - "#B42318"
todos:
  - id: audit-old-examples
    content: 使用 [subagent:code-explorer] 核对旧示例引用、用户改动和压测迁移路径
    status: completed
  - id: build-shop-slice
    content: 实现商城前后端、数据库与 KV，并验证构建和业务测试
    status: completed
    dependencies:
      - audit-old-examples
  - id: complete-shop-schedule
    content: 实现商城云函数发布、定时任务、幂等 worker 和私有产物上传
    status: completed
    dependencies:
      - build-shop-slice
  - id: build-remaining-cases
    content: 实现小游戏、预约、社区和工单的独立业务闭环及测试
    status: completed
    dependencies:
      - complete-shop-schedule
  - id: migrate-and-document
    content: 迁移压测资产、替换旧示例并更新案例文档和清理计划
    status: completed
    dependencies:
      - build-remaining-cases
  - id: verify-all
    content: 执行全仓构建测试及可用环境中的五例端到端验证
    status: completed
    dependencies:
      - migrate-and-document
---

## User Requirements

按照 `plan/planv4.0/examples-business-cases-plan.md` 实现代码，将 `examples/` 中的旧演示替换为约五个可运行的业务案例。本阶段先确认实施方案；获得确认前不修改文件。

## Product Overview

每个案例独立展示一个完整业务流程，包含可构建的前端、可信后端、初始化步骤和运行说明。案例覆盖商城、小游戏、预约、内容社区及工单服务。

## Core Features

- 每例实际使用用户数据库、项目 KV、云函数和定时任务，并通过 Go SDK 或 JS SDK 初始化可信后端。
- 构建前端并上传至项目对象存储；发布云函数源码、配置定时任务，提供验证与回滚流程。
- 将定时任务结果与真实业务数据关联，幂等写回数据库并刷新 KV；测试重复执行和失败恢复。
- 安全迁移旧示例及其引用，保留仍有用途的压测代码，不覆盖现有用户改动。

项目对象存储目前只能按私有对象能力验收上传与短期下载，不能将其描述为已上线的公开网站。

## Tech Stack Selection

- 沿用仓库的 Go 1.25、`packages/go-sdk`、`packages/js-sdk` 和现有 SimpleBase HTTP API。
- Go SDK 用于小游戏、内容社区的可信后端；JS SDK 用于商城、预约、工单的可信 Node 后端。
- 前端以轻量 HTML、JavaScript 和仓库现有构建工具实现，不新增第三方依赖；Go 云函数以单文件源码发布，由平台解释执行。

## Implementation Approach

先完成商城案例的端到端纵切，验证数据库事务、KV 权限、云函数版本发布、定时任务运行记录及前端构建上传，再将已验证的做法用于其余四例。每例保留独立业务模型和命名空间；仅抽取确有重复的部署辅助逻辑，避免为示例引入复杂框架。

定时任务的输入是固定 JSON，不能自行读取业务库。因此云函数的 `Tick` 返回短小时间窗；可信后端 worker 按运行记录读取真实数据，调用同一函数文件的 `Compute`，再按运行 ID 幂等写回。读取采用有界分页和批处理，避免逐条远程请求；运行记录接口最多返回 100 条且无游标，追赶不及时必须告警、停止自动回写并提供重算流程。

## Implementation Notes

- 修改前检查旧示例的追踪状态、未提交改动和引用；先迁移 `examples/gofunction/` 的压测用途，再删除被替代的旧目录。
- JS SDK 没有专用 KV 接口，可信端使用 `raw.request`；Go 可信端使用标准库 HTTP 适配器。KV 路由当前要求写权限，不能依赖文档中的只读 Key 描述。
- 云函数创建新版本时显式设置 `activate:false`，分别试跑后激活；失败保留旧版本。云函数源码不导入 Go SDK，不包含密钥。
- 浏览器不持项目 Key；后端本地默认仅监听环回地址。业务写入由数据库约束和事务保障幂等，KV 只保存临时态或可重建缓存。
- 前端产物按案例及不可变构建 ID 上传，设置正确 MIME；不修改私有 bucket 权限，也不编辑 `internal/web/dist`。

## Architecture Design

浏览器前端仅调用案例后端；后端通过 SDK 访问用户数据库，通过项目 API 访问 KV 和云函数。部署脚本负责前端对象上传、云函数版本发布及定时任务配置；SimpleBase 调度器执行云函数后，可信 worker 消费运行记录并完成业务回写。五例共享这一数据流，但各自隔离表、集合、KV 键、函数名、任务名和对象前缀。

## Directory Structure

以下列出计划中的文件责任；具体脚本扩展名和测试入口以商城纵切验证过的现有工具链确定，不创建空文件或空目录。

- `examples/shop-service/` **[NEW]**：`README.md` 记录商城运行与回滚；`server/` 实现 JS SDK 初始化、商品与订单 API、库存事务和 worker；`web/` 实现商品、购物车、订单界面；`functions/ex_shop_metrics.go` 实现 `Tick`、`Compute`；`deploy/` 实现发布、上传及验证；相邻测试覆盖库存、幂等和任务回写。
- `examples/mini-game-service/` **[NEW]**：相同目录职责；Go SDK 后端实现对局、积分和排行榜，云函数源码负责赛季排名计算；测试覆盖重复结算、并列排名及缓存恢复。
- `examples/booking-service/` **[NEW]**：JS SDK 后端实现时段、暂留、确认和取消；云函数源码计算过期候选；测试覆盖容量约束、令牌过期与重复清理。
- `examples/community-service/` **[NEW]**：Go SDK 后端使用文档集合与 SQL 记录内容及互动；云函数源码计算热榜；测试覆盖重复点赞、跨模型不一致和缓存重建。
- `examples/ticket-service/` **[NEW]**：JS SDK 后端实现建单、状态推进和超期视图；云函数源码计算 SLA；测试覆盖版本冲突、重复请求和状态复核。
- `examples/browser-quickstart/`、`examples/node-documents/`、`examples/node-sql/`、`examples/node-storage/` **[REMOVE after replacement]**：确认新案例覆盖原有教学入口且无未保存改动后移除。
- `examples/gofunction/` **[MOVE after reference check]**：先将仍需保留的压测实现迁出业务案例目录，更新引用并验证，再移除旧路径。
- `docs/sdk/examples.md`、`docs/sdk/index.md`、`plan/planv4.0/cleanup-plan.md` **[MODIFY]**：更新案例入口、命令和旧示例保护说明；搜索发现的其他旧路径引用逐一核对后同步更新。
- `.gitignore` **[MODIFY if needed]**：仅在现有规则未覆盖案例构建物、测试产物时补充针对性忽略规则。

实施前须验证独立云函数 `.go` 源码目录与根模块 `go build ./...` 的兼容性；若会被误编译，采用经解释器试跑验证的构建排除方式，不以破坏全仓构建换取示例布局。

## Design Style

五个前端采用一致的桌面优先布局与案例专属主题色。页面自上而下包含顶部导航、业务概览、主要操作区、数据或结果区，以及底部状态与帮助区；窄屏时将多列内容收为单列。表单反馈、加载、空态和错误提示清晰可见，轻量悬停与状态过渡不影响操作。

商城突出商品卡片与购物车；小游戏突出游玩区与排行榜；预约展示时段和状态；社区突出内容流与热榜；工单突出列表、详情和超期提醒。界面只请求可信后端，不呈现或保存项目密钥。

# Agent Extensions

- **code-explorer**（SubAgent）：实施前复核旧示例引用、构建入口和受迁移影响的文件；预期产出可核对的迁移清单，避免误删压测资产或遗漏文档入口。