# Proposal

## Why

当前只有「登录 / 权限位」，没有「这一行数据能不能看」。任何拥有项目读权限的成员都能读取该项目的全部集合文档，终端用户与云助手也只能靠项目级权限兜底。代码中 `access_rule`、`owner_field`、`RowRule` 全仓 0 命中，这是上线前最大的功能空洞（legacy plan §3-H2）。

## What Changes

- 项目级新增集合访问规则：每个集合可声明 `owner_field`（默认 `owner_id`）与访问模式 `off | owner_read | owner_write`。
- 集合文档 API 在服务端注入 owner 过滤：读只返回本人或管理员可见的行，写校验目标行 owner；越权返回 403 `forbidden_row`，不泄露行是否存在。
- 管理员（`database:admin` / `project:admin`）与系统内部主体默认绕过行级规则；终端用户以自身身份作为 owner，委托令牌继承降级后的身份。
- SQL 通道不做行级重写：行级规则只约束集合文档 API；SQL 仍按项目权限判定，并在文档中明确该限制。
- 聚合与导出按 owner 过滤；批量写入与导入在创建时写入 owner；未实现路径不得绕过（fail-closed）。
- 规则变更写审计（只记模式与 owner 字段，不记数据），并提供关闭开关以兼容存量项目。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `identity-access`: 增加行级访问规则与越权拒绝的授权语义。
- `data-service`: 增加集合文档的 owner 过滤与写校验。

## Impact

- 受影响代码：`internal/collection`（规则模型与过滤）、`internal/api/datahttp`（文档读写入口）、`internal/agentrun` / `internal/agentbridge`（主体传递）、`internal/systemdb` 或项目设置（规则持久化，不新增版本化迁移）、`ui/src`（集合设置与只读提示）、`packages/js-sdk`。
- 不改变：SQL 语义、系统库只读、公开 API 路径与错误码集合（只新增 `forbidden_row`）。
- 数据迁移：规则默认 `off`，存量项目行为不变。
