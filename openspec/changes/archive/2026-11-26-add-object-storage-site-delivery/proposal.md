# Proposal

## Why

站点 P0 已经能发布，但真实交付还缺三件事：发布一个站点需要逐个上传文件（不支持文件夹）；单页应用刷新子路由 404（无 `spa_fallback`）；匿名站点没有出口统计、配额与管理员下线入口。`sbdev.py site deploy` 也只能提示前缀下的旧对象，不能受控清理。这些是 legacy planv6.5 P1/P2 与 planv6.6 SITE-5 的未完成项。

## What Changes

- 文件夹上传：控制台一次选择整个目录，按相对路径批量上传，逐文件复用现有 50MiB 上限与 Content-Type 校验。
- `spa_fallback`：站点可开启「未命中的无扩展名路径回退到 `index.html`」，静态资源仍返回 404。
- 站点配额与出口统计：按项目记录站点出口用量，超限返回明确错误；管理员可全局下线站点并写审计。
- CLI / SDK：`packages/js-sdk` 增加 `s3.site.get/set` 与 `s3.setPublic`；`sbcli` 增加 `file site enable|disable|status` 与 `file public <key> --on|--off`。
- `site deploy --prune`：在预览确认后删除前缀内不属于本次产物的对象（删除属破坏性操作，必须二次确认）。
- 文档：泛域名 / 路径模式部署、缓存策略与配额说明。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `file-sites`: 增加文件夹上传、SPA 回退、站点配额 / 出口统计与管理下线。
- `dev-experience`: 增加站点 CLI / SDK 命令与 `--prune` 的确认语义。

## Impact

- 受影响代码：`ui/src/pages/S3Manager.vue`、`internal/api/filehttp`、`internal/objectstore`、`internal/systemdb`（站点配额 / 出口用量，复用现有表优先）、`internal/sbcli`、`packages/js-sdk`、`skills/simplebase/scripts/sbdev.py`、`examples` 站点部署脚本、`docs/ops`。
- 不改变：对象键布局、发布权限、子域名 / 路径分流、控制台源的附件策略。
- 不做：自定义域名（CNAME + 证书）单独立项。
