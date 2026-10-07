# SimpleBase 文档写作约定

`docs/` 内每个模块目录是一组文档；根目录的 README 不进入导航。新增模块时登记到 `_meta.json`。

- 每篇必须以 frontmatter 开始，包含 `title` 和 `order`；可选 `group`（侧栏分组）和 `description`（检索摘要）。
- 一篇只写一个 H1，与 `title` 对齐；从 H2 开始组织小节，不跳标题级别。`database/ducklake.md` 是上游离线镜像，允许上游原有标题结构。
- 站内链接写 `/docs/<模块>/<页面>`，索引页可省略 `/index`；同模块可用 `./xxx.md`，其它模块可用 `../module/xxx.md`。指向仓库其他目录使用 GitHub 完整地址。
- 代码块必须标注语言：shell 用 `bash`、HTTP 报文用 `http`，配置用 `yaml` / `json`，否则使用 `text`。
- 概览页建议采用「是什么 → 何时用 → 最小示例 → 限制 → 相关链接」。
- 示例应对照当前服务端路由、SDK 类型与开发模式种子数据；有 S3、Cloud Key 等环境前提必须写明，勿假称已运行真实 Cloud。
- 大于 50 KiB 的参考资料需与懒加载策略保持一致，避免进文档首页 JS chunk。
