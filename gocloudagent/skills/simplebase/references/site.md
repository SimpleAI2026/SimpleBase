# site
<!-- allow: site -->
<!-- readonly: site status -->

命令：`site status|enable|publish`。把项目对象存储里 `web/` 前缀下的静态页面发布到 `<项目ID>.<站点域名>`。站点面只读（只能 GET/HEAD），没有 `/api`。

## status

```
simplebase site status
```

返回 `enabled`、`baseUrl`、`publishable`、`reason`。`publishable=false` 时（`reason` 为 `sites_disabled`、`system_project` 或项目 ID 不可作域名）不能发布，如实告诉用户并跳过站点步骤，不要重试。

## enable / publish

先用 `object upload` 把页面传到 `web/index.html`，再依次执行：

```
simplebase site enable                       ← 需要确认（对外公开）
simplebase site publish --prefix web/        ← 需要确认（对外公开）
```

- 发布前缀固定为 `web/`：缺省 `--prefix` 或传空串返回 `site_root_publish_forbidden`，传 `web/` 以外的前缀返回 `site_prefix_not_allowed`。不要尝试公开对象根或其它目录。
- 两条命令都需要界面确认；headless 返回 `confirmation_unavailable`，不要重试。
- `site_not_publishable`：项目当前不可发布，按 `status` 的 `reason` 告知用户。
- `writer_unavailable`：实例只读，不能发布。

## 页面约束

- 单个 `web/index.html`，内联 CSS 与 JS，Hash 路由，不依赖构建，不引用外链 CDN。
- 体积不超过 200KiB（`stdin` 上限 256KiB）；`object upload` 的内容放 `stdin`。
- 页面里不得出现任何 Key 或 token（如 `sb_live_`）。站点面没有数据接口：需要展示数据时，把结果写成 `web/data/*.json`，页面用相对路径 `fetch` 读取。
