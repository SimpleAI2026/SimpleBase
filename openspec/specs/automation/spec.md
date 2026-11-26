# Automation Specification

## Purpose

定义云函数、事件触发器、定时任务与本地重放的执行语义，保证 HTTP、cron 与本地开发三种入口行为一致。

## Requirements

### Requirement: 云函数签名与活动版本

云函数源码 MUST 为 `package main`，导出函数 MUST 形如 `func Name(req T) R`（恰好一个入参与一个返回值）。`/go`、调试试跑、cron 与事件触发器 SHALL 共享同一个「活动版本」执行器与全局并发槽位，不得各自创建信号量。

#### Scenario: 非法函数签名
- **WHEN** 用户提交无入参、多返回值或非 `package main` 的函数
- **THEN** 保存或运行被拒绝并返回可定位的编译错误

#### Scenario: 并发上限
- **WHEN** 并发调用超过全局槽位
- **THEN** 请求排队或返回限流错误，不绕过槽位另起执行器

### Requirement: 触发器语义

事件触发器 SHALL 使用有界进程内队列，匹配后最多重试三次；系统 MUST NOT 声称触发器持久化或 exactly-once，也 MUST NOT 把事件正文写入日志。项目删除时应清理匹配的触发器。

#### Scenario: 队列已满
- **WHEN** 事件到达时队列已满
- **THEN** 返回队列满错误并记录元数据，不阻塞写入主路径

#### Scenario: 失败重试
- **WHEN** 目标函数连续失败
- **THEN** 最多重试三次后放弃，并记录不含事件正文的失败元数据

### Requirement: 定时任务语义

定时任务 SHALL 支持 `cron` 与 `interval` 两种触发：`interval` 以认领时刻推进下次执行，避免漂移。调度 tick SHALL 为 30 秒、并发上限 4、单次执行超时 10 秒；错误信息截断 300 字符、响应截断 4KB、输入截断 8KB。多实例认领 MUST 使用条件更新（CAS）。

#### Scenario: 间隔任务漂移
- **WHEN** 上一次执行延迟完成
- **THEN** 下一次执行从本次认领时刻重新计算间隔，不累积漂移

#### Scenario: 重复认领
- **WHEN** 两个调度者同时尝试认领同一任务
- **THEN** 只有一个认领成功，另一个跳过本次执行

### Requirement: 本地重放与 dev 一致

`simplebase dev` 与 `skills/simplebase/scripts/sbdev.py` MUST 保持同一套校验文案、动作序列与记账名（迁移 `<文件名>`、SQL 种子 `seed:<路径>`、集合种子 `seed-doc:<集合>/<id>`）。两端测试 MUST 逐字比对 `internal/sbcli/testdata/dev/*/expect.json`；服务端建文档忽略客户端 `id`，按 id 去重必须改用 `_migrations` 记账。

#### Scenario: 修改校验提示
- **WHEN** 开发者改动 `simplebase dev` 的一条校验文案
- **THEN** `sbdev.py` 同步修改，否则两端 expect.json 对账测试失败

#### Scenario: 重复 apply
- **WHEN** 对同一工作区重复执行 `dev apply`
- **THEN** 已记账的迁移与种子不重复执行，输出与首次一致
