# SimpleBase 文档写作约定

`docs/` 下的每个目录是文档站的一个 Tab，当前固定为**六大模块**，顺序与控制台侧栏分组对应；根目录的 README 不进入导航。新增模块前先确认是否真的需要新 Tab——Tab 过多会让导航栏拥挤，优先把新内容放进已有模块的子主题里。

| 模块 id | Tab | 包含的子主题（`group`） | 对应控制台入口 |
|---|---|---|---|
| `getting-started` | 入门 | 上手、核心概念 | — |
| `workbench` | 工作台 | 云助手、监控大盘 | 工作台 |
| `data` | 数据 | 数据管理、Realtime、Key-Value、对象存储 | 数据 |
| `automation` | 自动化 | 云函数、事件触发、定时任务、云沙盒 | 自动化 |
| `ops` | 运维 | 日志与审计、部署与故障处理、备份恢复 | 运维 |
| `sdk` | SDK | JavaScript、Go | — |

模块清单与顺序登记在 `_meta.json`；`ui/tests/docs-order.test.ts` 会校验 Tab 数量与顺序，改动时同步更新。

## 页面约定

- 每篇必须以 frontmatter 开始，包含 `title` 和 `order`；可选 `group`（侧栏子主题分组）和 `description`（检索摘要）。
- 同一模块内用 `group` 把页面归到子主题，**同一 `group` 的页面 `order` 必须连续**，否则侧栏会出现两个同名分组标题（测试会拦）。
- 模块首页文件名为 `index.md`，`order: 0`，不带 `group`，用一张表指向各子主题。
- 一篇只写一个 H1，与 `title` 对齐；从 H2 开始组织小节，不跳标题级别。`data/ducklake.md` 是上游离线镜像，允许上游原有标题结构。
- 文件名用「子主题 + 用途」命名，如 `sandbox.md`（概览）、`sandbox-api.md`、`sandbox-e2e.md`，不要再嵌套子目录。

## 链接与代码

- 站内链接写 `/docs/<模块>/<页面>`，模块首页可省略 `/index`；同模块可用 `./xxx.md`，其它模块可用 `../module/xxx.md`。指向仓库其他目录使用 GitHub 完整地址。
- 代码块必须标注语言：shell 用 `bash`、HTTP 报文用 `http`，配置用 `yaml` / `json`，否则使用 `text`。
- 概览页建议采用「是什么 → 何时用 → 最小示例 → 使用场景 → 限制 → 相关链接」。
- 示例应对照当前服务端路由、SDK 类型与开发模式种子数据；有 S3、bubblewrap 等环境前提必须写明，勿假称已在不具备的环境中运行过。
- 大于 50 KiB 的参考资料需与懒加载策略保持一致，避免进文档首页 JS chunk（目前仅 `data/ducklake.md`）。

## 旧路径

文档合并前的路径（如 `/docs/ops/dashboard`、`/docs/kv`、`/docs/sandbox/api`）由 `ui/src/docs/catalog.ts` 的 `legacyDocRedirects` 重定向到新位置，新增迁移时在那里补一行。
