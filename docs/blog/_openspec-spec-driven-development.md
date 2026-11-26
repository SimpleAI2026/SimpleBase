# OpenSpec 实战：先写规格，再让 AI 写代码

用 Claude Code、Codex 或 Cursor 写过跨天的功能，大概率会遇到同一类问题：需求只存在于几十轮对话里，改到第十个文件时，最初说好的行为已经没人记得；换个人接手，只能从 diff 里猜意图。

OpenSpec 要解决的就是这一段：**在写代码之前，先把「要建什么」变成仓库里的 Markdown 文件**，人和 AI 对着同一份文件说话。

项目地址：<https://github.com/Fission-AI/OpenSpec>（MIT，官网 <https://openspec.dev>）

## 它到底是什么

一句话：Spec-driven development（SDD）for AI coding assistants。

它不是代码生成器，也不接管 IDE。它提供三样东西：

- 一套目录约定：`openspec/specs/` 放当前规格，`openspec/changes/` 放在途变更。
- 一个 CLI（`openspec`），负责建骨架、生成给 agent 的指令、校验格式、归档合并。
- 一组 skills，把 CLI 编排成 agent 能直接触发的固定流程。

官方给的五条理念是 *fluid not rigid / iterative not waterfall / easy not complex / built for brownfield not just greenfield / scalable from personal projects to enterprises*。翻成工程语言：不设阶段门，任何产物随时可改；默认你改的是一个已经存在的仓库，而不是从零开始。

和同类方案的差别（官方口径）：GitHub 的 Spec Kit 完备但重，有刚性阶段门、大量 Markdown 和 Python 环境；AWS 的 Kiro 强但锁 IDE 与模型。OpenSpec 只要求一个 CLI 加你本来就在用的编辑器。

## 核心模型：两份真相

```text
your-project/
├── openspec/
│   ├── config.yaml                       # 项目上下文与产物规则
│   ├── specs/                            # 现状：已实现、需长期保持的行为
│   │   ├── data-service/spec.md
│   │   └── ops-governance/spec.md
│   └── changes/                          # 计划：还没落地的变更
│       ├── add-ops-maturity/
│       │   ├── proposal.md               # 为什么、改什么
│       │   ├── specs/ops-governance/spec.md   # delta：只写行为差异
│       │   ├── design.md                 # 怎么做
│       │   ├── tasks.md                  # 可勾选清单
│       │   └── .openspec.yaml            # schema / skip_specs 等元数据
│       └── archive/
│           └── 2026-11-26-add-row-level-access-control/
```

两个关键设计决定：

1. **change 里放的是 delta，不是整份 spec。** 只写 `## ADDED Requirements`（以及 MODIFIED / REMOVED / RENAMED），归档时才合进 `specs/`。好处是多个 change 并行时不会互相覆盖同一个 spec 文件。
2. **文件名固定。** proposal / specs / design / tasks 四件套由 schema 定义，CLI 和 agent 按同一套约定读写，不靠 prompt 里「记得先写设计文档」这种口头约定。

需求写法是纯 Markdown，没有专有语法：

```markdown
### Requirement: HTTP 就绪端点

系统 SHALL 提供 `GET /v1/system/readiness`，返回每个关键依赖的 ready 状态与失败原因。
该端点 MUST 无写副作用，依赖全就绪时返回 200，否则返回 503。

#### Scenario: 依赖不可用
- **WHEN** 任一依赖检查失败
- **THEN** 返回 503，指出失败依赖与原因，不泄露凭据或 DSN
```

`Requirement` 用 SHALL / MUST NOT 描述行为契约，`Scenario` 用 WHEN / THEN 描述可验证路径。这段文字既是你 review 计划的落点，也是 agent 后续自检的落点。

## 安装与初始化

```bash
npm install -g @fission-ai/openspec@latest    # 需要 Node.js >= 22.20
# 或者
brew install openspec
```

在项目里初始化：

```bash
openspec init --tools codebuddy --scope project
```

`--tools` 接受 30 多个编码工具（claude、codex、cursor、codebuddy、gemini、copilot 等），也可以传 `none` 只用 CLI。以 2.2.0 + codebuddy 实测，它只落这几样：

```text
openspec/config.yaml
openspec/specs/.gitkeep
openspec/changes/archive/.gitkeep
.agents/skills/openspec-*/SKILL.md        # 7 个 skill
.codebuddy/skills/openspec-*/SKILL.md     # 同一份，按工具目录再落一次
```

注意 2.x 的集成载体是 **skills**，不是插件，也不是 MCP；1.x 的 `/opsx:*` 命令体系已经退役。默认装 7 个：总入口 `openspec`，加上 `explore` / `propose` / `apply-change` / `update-change` / `archive-change` / `sync-specs`。另外 6 个（`new` / `continue` / `ff` / `bulk-archive` / `verify` / `onboard`）默认不装：

```bash
openspec config profile    # 切 default / expanded
openspec update            # 把新的 skills 写入各工具目录
```

`openspec update` 在日常里还有另一个用途：团队把 skills 提交进 Git 后，每个项目跑一次，保证本机装的 skill 与仓库里的版本一致。

## CLI 全貌

`openspec -h`（2.2.0）给出的命令，按用途分组：

| 用途 | 命令 |
| --- | --- |
| 初始化与升级 | `init`、`update`、`install` |
| 查看 | `list`、`show`、`view`、`status`、`context`、`doctor` |
| 新建 | `new change`、`change`、`spec` |
| 驱动 agent | `instructions`（artifact / apply / archive） |
| 校验 | `validate --strict` |
| 收尾 | `archive` |
| 定制 | `config`、`schema`、`templates`、`schemas` |
| 多仓与工作视图 | `store`（beta，独立规划仓）、`workset`（纯本地工作视图） |

真正把它和「写文档」区分开的是 `instructions`。CLI 不是给你一份空模板，而是把「这次该写哪个文件、必须包含哪几节、项目有哪些不可违反的约束」生成成一段带标签的指令：

```text
<artifact id="proposal" change="add-ops-maturity" schema="spec-driven">
<task>Create the proposal artifact for change "add-ops-maturity".</task>
<project_context>
SimpleBase：单写实例的云端数据库服务（Go 1.25 + Echo v4 + Vue 3）……
</project_context>
<output>Write to: openspec/changes/add-ops-maturity/proposal.md</output>
<instruction>
Sections:
- **Why**: 1-2 sentences on the problem or opportunity.
- **What Changes**: ...
- **Capabilities**: ...
- **Impact**: ...
</instruction>
</artifact>
```

加 `--json` 更实用。`openspec instructions apply --change <id> --json` 的核心是三块：`contextFiles`（这次该读哪几个文件）、`tasks`（每条带 `line` 与 `done`）、`progress`。agent 不需要猜任务清单在哪儿。

## 一次完整变更

以给服务加 HTTP 就绪端点为例，这是 SimpleBase 里真实存在的 change `add-ops-maturity` 的第 3 组任务。

**第一步，开变更。**

```bash
openspec new change add-ops-maturity
# 或者让 agent 触发 /openspec-propose "加 HTTP 就绪端点"
```

**第二步，看还缺哪件产物。**

```text
$ openspec status --change add-ops-maturity
Change: add-ops-maturity
Schema: spec-driven
Progress: 0/4 artifacts complete

[ ] proposal
[ ] specs
[ ] design
[ ] tasks
```

**第三步，逐件生成。**

```bash
openspec instructions proposal --change add-ops-maturity
```

把输出里的 `<instruction>` 当写作提纲，`<project_context>` 当约束，写完再回到第二步。想一次生成全套就用 `openspec-propose` skill；想一步一步来，就手动 `openspec new change` 建目录、再逐件跑 `instructions`。

**第四步，校验。**

```bash
openspec validate add-ops-maturity --strict
# Change 'add-ops-maturity' is valid
```

**第五步，实施。**

```bash
openspec instructions apply --change add-ops-maturity --json
```

agent 拿到 contextFiles 和未完成任务，按序改代码，每完成一条立刻把 `tasks.md` 里的 `- [ ]` 改成 `- [x]`。这行勾选就是进度条：

```text
$ openspec list
Changes:
  close-legacy-backlog                6/54 tasks    3h ago
  add-ops-maturity                    1/15 tasks    3h ago
  complete-bi-dashboard               0/12 tasks    3h ago
```

**第六步，归档。**

```bash
openspec archive add-ops-maturity
```

`archive` 会做四件事：校验变更、把 delta 合进 `openspec/specs/`、把目录移到 `changes/archive/YYYY-MM-DD-<id>/`、报告合并条数。输出形如（数字对应该 change 当前的 delta：4 个新增 Requirement、0 个修改）：

```text
Task status: 15/15 tasks

Specs to update:
  ops-governance: update
Applying changes to openspec/specs/ops-governance/spec.md:
  + 4 added
  ~ 0 modified
Totals: + 4, ~ 0, - 0, → 0
Change 'add-ops-maturity' archived as '2026-11-26-add-ops-maturity'.
```

纯文档、纯重构、工具链类变更可以加 `--skip-specs`；`-y` 会跳过确认，未完成任务也能归档（但会打 warning）。

### 校验器到底卡什么

这几个边界我逐条试过，值得提前知道：

```text
✗ Requirement 没有 #### Scenario      → ERROR，必须至少有一个场景
✗ 章节名写成 ## Requirements          → ERROR，不是 delta 章节，被忽略
✗ 整个 change 没有任何 delta          → ERROR，提示改用 skip_specs: true
⚠ Requirement 正文没有 SHALL / MUST   → WARNING（RFC 2119 建议），不阻断
⚠ proposal 的 Why 少于 50 字符         → WARNING，不阻断
```

两个容易被忽略的细节：SHALL / MUST 要写在 `### Requirement:` 标题下的第一行，只写在标题里不算；delta 的章节名必须是 `## ADDED/MODIFIED/REMOVED/RENAMED Requirements` 之一，写成 `## Requirements` 会让这条需求被忽略，归档时不会进主 spec。

## 让 agent 真正遵守项目约束

这一节是 OpenSpec 在工程上最有价值的部分：`openspec/config.yaml` 不是一个装饰文件，它的三段内容都会被注进 `instructions`（我逐段验证过落在哪个标签里）。

```yaml
schema: spec-driven

context: |
  PROJECT-CONTEXT-MARKER：所有产物用简体中文。

rules:
  proposal:
    - RULE-MARKER：提案不超过 500 字

operations:
  apply:
    guidance:
      - GUIDANCE-MARKER：每完成一条任务立即勾选
```

对应的注入位置：

- `context` → `<project_context>`，出现在所有 artifact 的指令里
- `rules.<artifact>` → `<rules>`，只出现在对应产物的指令里
- `operations.apply.guidance` → 出现在 apply 指令里

SimpleBase 的真实配置（节选）：

```yaml
context: |
  SimpleBase：单写实例的云端数据库服务（Go 1.25 + Echo v4 + Vue 3）。所有产物用简体中文，change-id 用 kebab-case。

  新代码落点：有 HTTP/WS/SSH 帧 → api/<context>http；协议原语 → api/httpkit；
  多入口复用或跨请求生命周期 → internal/agentrun 或领域包；外部系统适配 → agentbridge/dbbridge。

  不可违反：不新增第三方依赖；不改公开 API 路径 / 错误形状 / SSE 帧格式；
  系统库只允许 SELECT；四个嵌套独立模块不得 import 根模块；依赖方向由 TestArchitectureGate 强制。

  纯重构 / 工具 / 文档类变更在变更目录的 .openspec.yaml 设 skip_specs: true，
  不为通过校验编造需求；行为变化必须有 delta spec。
```

这带来三个实际收益：

1. **约束只写一次。** 不用每次对话重新交代「不要新增依赖」。agent 生成 proposal 和 tasks 时自动带着它。
2. **不用靠 AGENTS.md 承担全部上下文。** `context` 只在生成产物时读一次，不像常驻规则文件那样每轮都占窗口。
3. **纯重构不必编需求。** 在 `.openspec.yaml` 里设 `skip_specs: true`，校验会明确给出 INFO 而不是让你造一条假 Requirement：

```text
Change 'pure-refactor' is valid
ℹ [INFO] file: skip_specs is set in .openspec.yaml: change declares no
  spec-level behavior changes, zero deltas accepted
```

## 和同类方案的差别，以及什么时候不该用

用下来，OpenSpec 真正区别于「让 AI 写个设计文档」的地方是 delta 和归档这两个机制：

- 一个 change 一个目录，天然跟着 feature 分支走；delta 让并行开发不打架。
- 归档把「计划」转成「现状」，规格库不会变成一堆过期文档。
- 没有阶段门：design 可以后补，tasks 可以边做边加，适合棕地迭代。

它的代价也要说清楚：**它只保证结构对，不保证内容好。** Requirement 写成废话照样能过 `validate`，`- [x]` 勾得虚也一样能归档。所以人的 review 不能省。

以下场景我会建议不用：

- 改文案、修 typo、一次性脚本。写 proposal 的时间比改代码长。
- 需求还在探索。先让 agent 用 `explore` 聊清楚，再开 change。
- 团队没人看 spec。OpenSpec 的价值一半在「人看计划再放手」，全自动 apply 等于没有。

## 落地清单

1. `openspec init --tools <你的工具> --scope project`，把 `openspec/` 提交进 Git。
2. 先只写 `context`，跑两三个 change 之后，再按实际痛点补 `rules`。
3. 一个 change 只做一件事。跨三个模块的改动拆成三个 change。
4. Requirement 写行为（SHALL / MUST NOT），接口签名和文件路径留给 design 与 tasks。
5. tasks 每条都要能验证（命令加期望输出），否则归档时无法判断「做完了」。
6. 纯重构 / 文档 / 工具链变更设 `skip_specs: true`，别为过校验编需求。
7. CI 跑 `openspec validate --all --strict`；想拦「任务没勾完就归档」，另跑一条 `openspec validate --archived`。
8. 长任务开始实施前清空上下文（官方建议），中途用 `openspec status --json` 重新读状态，别靠记忆。
9. 不想上报就 `openspec config set telemetry.enabled false`，或设 `OPENSPEC_TELEMETRY=0` / `DO_NOT_TRACK=1`；它只采命令名与版本号。

## 最后

OpenSpec 没有发明新格式：Requirement 与 Scenario 是行为驱动设计的老写法，proposal / design / tasks 是 RFC 和任务清单的老组合。它的价值在两件事上：

一是**把「对齐」变成 commit**。需求不再是聊天记录，而是可以 review、可以 diff、可以归档的文件。

二是**给 AI 一个确定的读写契约**。`openspec instructions` 告诉它写哪个文件、写哪几节、项目红线是什么；`openspec status` 告诉它还差什么；`openspec archive` 负责把计划变成现状。中间没有一步靠 prompt 猜。

项目地址：<https://github.com/Fission-AI/OpenSpec>
