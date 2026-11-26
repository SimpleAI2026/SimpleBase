# Design

## 规则模型

```text
CollectionAccessRule
  mode:        off | owner_read | owner_write
  ownerField:  默认 owner_id，必须是已存在的集合字段
```

模式语义：`owner_read` 只过滤读，写仍按项目权限；`owner_write` 同时过滤读并校验更新 / 删除 / 批量 / 导入的目标行 owner。两种模式都会在创建时写入 owner，非管理员不能伪造或转移 owner。

规则按「项目 + 集合」持久化，优先复用项目设置表（`sys_settings_project` 的 key），不新增系统表、不做版本化迁移。规则读取失败时 fail-closed：按 `off` 之外的最严格模式处理，但记录告警；规则解析错误直接拒绝请求。

## 强制点

- 入口：`api/datahttp` 的文档读写 handler 统一调用 `collection.ApplyAccessRule`，不在各 handler 复制判断。
- 领域：`internal/collection` 提供纯函数生成过滤条件与 owner 校验，便于金标测试。
- 主体：使用 `auth.Principal`；管理员判定沿用 `HasPermission`，终端用户与委托主体携带自身 owner 标识。
- 聚合 / 导出：与列表共用同一过滤条件，禁止只过滤列表。

## SQL 边界

不对任意 SQL 做行级重写（无法可靠覆盖 JOIN、子查询与函数）。SQL 通道维持项目权限判定，并在 `docs/` 与集合设置页明确提示；需要行级隔离的场景使用集合文档 API。

## 失败语义

- 越权：403 `forbidden_row`，不区分「不存在」与「无权」。
- 规则损坏：500 `access_rule_invalid`，不静默放行；读取失败为 `access_rule_unavailable`。
- 聚合 / 导出复用 `collection/query` 的条件拼接；批量 / 导入是只创建路径，逐条写入 owner。任何新入口若无法保证过滤，必须先返回 `access_rule_unsupported`，不得退回不过滤执行。
- 关闭开关：写回 `{"mode":"off"}` 即可恢复原行为，无需迁移数据。

## 兼容性

默认 `off`，不改变存量项目；新增字段与设置不影响现有 API 响应。SDK 只新增类型，不改变既有方法签名。
