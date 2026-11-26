# Project Catalog Specification

## Purpose

定义项目与数据库目录的生命周期、单项目单库约束、删库语义与存储身份，使数据面与运维操作共享唯一事实来源。

## Requirements

### Requirement: 单项目单库

每个项目 SHALL 只有一个用户数据库；再次创建数据库 MUST 返回 `database_already_exists`。数据库状态机 SHALL 只包含创建中、就绪、降级、删除中与已删除，不向调用方暴露打开/关闭状态。

#### Scenario: 重复建库
- **WHEN** 项目已存在用户数据库时再次调用创建
- **THEN** 返回 `database_already_exists`，不创建第二个文件

#### Scenario: 降级
- **WHEN** 数据库进入降级状态
- **THEN** 状态保持降级并可用于诊断，不自动回退为就绪或删除

### Requirement: 删库只清数据面

删库 SHALL 只 DROP 用户表、视图与序列并清理集合元数据；`__` 前缀内部表（KV）、项目对象文件与备份 MUST 保留。整体删除项目文件只是没有 registry 时的退路，不得作为默认路径。

#### Scenario: 删除数据库
- **WHEN** 用户删除项目数据库
- **THEN** 用户表与集合元数据消失，KV 内部表、对象文件与备份仍可访问

#### Scenario: 删除系统库
- **WHEN** 对 `kind=system` 的数据库发起删除
- **THEN** 返回 `ErrSystemProtected`（403），不执行任何删除

### Requirement: 实例只读保护

当实例配置为只读（`instance.writable=false`）时，所有写操作 SHALL 返回 `ErrWriterUnavailable`（503），读操作保持可用。写路由 MUST 挂统一写门禁，handler 内部 MUST 再次检查 writable。

#### Scenario: 只读实例写入
- **WHEN** 客户端在只读实例上调用写接口
- **THEN** 返回 503 `writer_unavailable`，数据不发生变更

#### Scenario: 只读实例读取
- **WHEN** 客户端在只读实例上调用查询或列表接口
- **THEN** 正常返回数据

### Requirement: 存储身份与本地优先

`data_dir` 与 `cache_dir` MUST 物理分离，缓存淘汰不得删除唯一副本。存储身份 SHALL 记录在 `{data_dir}/_system/identity.json`；本地与 S3 身份不一致时 MUST 拒绝启动。切换存储模式不迁移历史数据，必须显式 reset。

#### Scenario: 目录混用
- **WHEN** `cache_dir` 与 `data_dir` 指向同一路径
- **THEN** 启动被拒绝并说明缓存淘汰会删除真实数据

#### Scenario: 存储身份不一致
- **WHEN** 已有 `identity.json` 记录 local 而本次配置为 s3（或相反）
- **THEN** 启动失败，提示先 reset 或恢复原存储配置
