# simplebase skill 维护规范

> 给修改本 skill 的人和模型看，执行任务时不需要读。目录结构遵循 `skills/skill-creator` 的约定：`SKILL.md`（YAML frontmatter 加正文）负责触发和路由，模块细节按需放在 `references/` 里加载。

## 1. 目录布局

```
skills/simplebase/
├── SKILL.md                 ← frontmatter(name, description) + 索引正文，allow 注释为空
└── references/
    ├── <skill-id>.md        ← 每个模块一份，宿主按加载的模块注入
    └── authoring.md         ← 本文件（未登记的 id，不参与对账、不注入）
```

- `<skill-id>` 必须登记在 `gocloudagent/skills.go` 的 `skillPrefixes` 中（`SkillsForModule` 据此按助手 module 加载），并补到 `SKILL.md` 的模块表里。`@` 不再参与技能解析，不需要别名表。
- 宿主注入时会去掉 YAML frontmatter（`readSkill`），所以 frontmatter 只给 skill 加载器看，不占 prompt 预算。
- 修改后执行 `python -m scripts.quick_validate ../simplebase`（在 `skills/skill-creator` 目录下）校验 frontmatter。

## 2. frontmatter

- `name: simplebase`，必须是 kebab-case，且和目录名一致。
- `description` 写清“做什么 + 何时触发”，可以适当主动一些，以免漏触发；长度不超过 1024 字符，不能出现尖括号。

## 3. allow / readonly 注释

```markdown
# database
<!-- allow: database,sql,schema,collection,document -->
<!-- readonly: database list,sql query -->
```

- allow 条目必须和 `skillPrefixes[<skill-id>]` 完全一致（顺序不限），否则 `TestEmbeddedAllowsMatchPrefixes` 失败。顶层 `SKILL.md` 必须写 `<!-- allow: -->`。
- readonly 必须和 `sbcli.CommandReadOnly` 的判定一致；正文中出现的每条只读命令都要登记。

## 4. 命令示例

```
simplebase <resource> <action> [--flag value] ...   ← 说明
```

- 行首必须是 `simplebase `，前两个词要能被 `sbcli.DispatchKnown` 解析（对应 `TestSkillCommandsDispatchable`）。
- 行尾说明用 `← `，不用 `#`。只写真实存在的 flag，必填项必须出现在示例里。
- 文件内容统一用 `--file -` 经 `stdin` 传入，不写宿主机路径。

## 5. 内容

- 每条命令依次写：用途 → 必填 flag → 最小示例 → 是否需要确认 → 常见错误码。
- 说明原因，少用全大写的 MUST/NEVER。错误码直接写 `error.code` 原文，并附处理建议；拿不准的就不写，不要自己编。
- 破坏性命令要写明“需要界面确认”，并说明 headless 场景返回 `confirmation_unavailable`、不要重试。

## 6. 体积

- 单份模块文档建议 1–3KiB，测试要求 600B 到 `maxSkillChars`（8192）之间。
- 多份文档拼接后超过 8192 字符会被截断（`gocloudagent/prompt.go`），尾部的示例会丢失，因此不要堆砌内容。
