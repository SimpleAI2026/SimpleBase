# Design

## 参数来源元数据

- `buildOpenAPI` 的路径参数输出可选 `x-sb-source`：`{kind, depends_on?}`。
- `kind` 取值：`project`、`databases`、`collections`、`tables`、`agents`、`agent-threads`、`cron-jobs`、`gofunctions`、`function-triggers`、`mcp-servers`、`sandboxes`、`api-keys`、`backups`。
- 只标注已知参数；未知参数省略字段，既有 OpenAPI 消费者不受影响。`collection` / `table` 通过 `pathContains` 限定到对应资源路径，并声明依赖 `databaseID`。

## 控制台注入流程

1. 选中端点或切换项目时先清空旧参数、旧下拉状态与结果。
2. `injectDefaults()` 读取 `x-sb-source`：`project` 取当前项目；其他 kind 走 `useApiParamOptions` 的加载器，按「项目 + kind + 依赖值」缓存，失败不缓存。
3. 资源列表失败时保留空值并把控件退化为自由输入，不阻塞预览。
4. `resolvedPath` 用 `replaceAll` 替换所有同名占位符；必填参数为空时阻止发送并高亮。

## 回滚

`x-sb-source` 是附加字段，旧客户端忽略即可；前端可单独回退下拉与默认值逻辑，不影响请求路径与后端。
