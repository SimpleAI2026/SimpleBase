# Tasks

## 1. 规则与持久化

- [x] 1.1 在 `internal/collection` 定义 `CollectionAccessRule`（模式、owner 字段、校验与过滤纯函数），补表驱动测试
- [x] 1.2 复用项目设置表持久化规则，提供读取 / 写入 / 删除与默认 `off`；不新增系统表、不做 DDL
- [x] 1.3 规则解析失败 fail-closed，新增 `access_rule_invalid` 错误映射与审计

## 2. 入口强制

- [x] 2.1 列表 / 按 id / 聚合 / 导出注入 owner 过滤，分页与总数同口径
- [x] 2.2 创建时写入当前主体 owner；更新 / 补丁 / 删除 / 批量 / 导入校验目标行 owner
- [x] 2.3 越权统一返回 403 `forbidden_row`，不泄露行是否存在
- [x] 2.4 管理员与系统内部主体绕过规则；终端用户与委托主体使用自身 owner

## 3. 设置与界面

- [x] 3.1 集合设置 API 增加规则读写与开关，仅项目管理员可改
- [x] 3.2 控制台集合设置增加规则编辑与只读提示；关闭规则需二次确认
- [x] 3.3 文档写明「行级规则只约束集合文档 API，SQL 不受约束」

## 4. 验证

- [x] 4.1 单测：本人读写成功、他人读写 403、管理员绕过、批量与导入逐行校验
- [x] 4.2 单测：规则损坏 fail-closed；关闭开关后恢复原行为
- [x] 4.3 路由级用例：终端用户与委托令牌越权被拒，审计只记规则摘要
- [x] 4.4 `go test ./...`、`TestArchitectureGate`、前端 `typecheck` + `test` 通过

## 完成情况核对（2026-11-26）

已实现（7）：
- 1.1 `internal/collection/access.go`：模式与 owner 字段校验、`CanRead` / `CanWrite`、`StampOwner` / `PreserveOwner`；`access_test.go` 覆盖。
- 1.2 规则存项目设置 `collection.access.<name>`；`DataHandler.WithAccessRules` + `internal/app` 注入 systemdb 读写；无新表 / DDL。
- 1.3 非法规则 fail-closed：读取 / 解析失败返回 `access_rule_invalid` / `access_rule_unavailable`，不静默放行。
- 2.3 越权统一 403 `forbidden_row`，不区分「不存在」与「无权」。
- 2.4 管理员（`database:admin` / `project:admin`）绕过；owner 使用 `Principal.Actor()`（`user:` / `key:`）。
- 3.1 `GET/PUT/DELETE .../:collection/access`；GET 可委托，PUT / DELETE 仅 project:admin + writeGate。
- 3.3 `docs/data/sql.md` 新增「行级访问规则」，写明模式、owner 注入、SQL 不受约束、批量 / 导入 / 导出 / 聚合暂 fail-closed。

- 2.1 列表 / 按 id / 聚合 / 导出均已注入 owner 过滤（聚合走 `ParseAggregateQueryFiltered`，导出拼入 SQL 条件）。
- 2.2 创建 / 批量 / 导入写入 owner；更新 / 补丁 / 删除校验并保持 owner。
- 3.2 集合 Schema 弹窗新增「行级访问规则」区块（模式 / owner 字段 / 保存），文案补 zh-CN / en-US。
- 4.1 / 4.2 用例覆盖：创建 / 批量 / 导入写 owner，更新 / 删除越权 403，管理员绕过，非法规则 fail-closed，关闭规则不注入过滤。
- 4.4 `go test ./internal/...` 仅 `TestNewLocalStorageAppUsesSystemDatabase` 既有失败；`yarn test` 通过（97.62 / 93.41 / 91.43）。

- 4.3 `TestDelegation_RouteTable` 覆盖委托令牌可访问 GET、拒绝 PUT / DELETE；`TestEndUserTokenRejectedOnCollectionAccessRoute` 覆盖终端用户 token 对三条路由均返回 401。规则变更审计已接入 `audit.Service`，Detail 只记 `mode` 与 `owner` 字段。
