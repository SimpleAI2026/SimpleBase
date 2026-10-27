# planv6.1 能力示例体验问题修复计划

> 状态：**已实施完成**（2026-10-15）。实施结果见文末 §10。
> 来源：[`examples-automation-report.md`](examples-automation-report.md) §7「体验与缺陷」9 条与 §8「后续」5 条。
> 原则：先修让示例红灯的 P0，再做低风险的契约/文案修正，最后处理需要设计取舍的项。不新增第三方依赖、不改 `go.mod` / `package.json`。

## 1. 总览

| # | 优先级 | 问题 | 方案 | 动产品代码 | 阶段 |
|---|---|---|---|---|---|
| 1 | P0 | `schema rows` CLI/SDK 解码失败 | SDK `TableRows.Columns` 改为 `[]SchemaColumn`，补 `Total/Limit/Offset/Table`，补解码测试 | 是（go-sdk） | A |
| 2 | P1 | settings 响应字段是 `Key` / `ValueJSON` | `systemdb.SettingKV` 补 json tag（snake_case） | 是（systemdb） | A |
| 3 | P1 | 内置助手默认不可写，不好发现 | 提示词/拒绝文案/SKILL.md/控制台提示统一说明 | 是（文案 + UI） | B |
| 4 | P1 | 本地 gocloudagent shell 与 CLI 凭据隔绝 | `config_missing` 与 `--help` 补充说明 | 是（文案） | B |
| 5 | P2 | `function test` 比 `invoke` 多一层信封 | 文档与 skill 参考写明；CLI 不改形状 | 否（文档） | C |
| 6 | P2 | 定时任务运行状态是 `completed` | 文档与 skill 参考写明取值 | 否（文档） | C |
| 7 | P2 | `./build.sh` 不生成 `config.yaml` | 构建路径缺文件时给出明确提示，帮助文案限定 `dev` | 是（脚本） | C |
| 8 | P2 | 认证不在任何 skill 里 | 本轮不做，仅落设计结论 | 否 | D |
| 9 | P3 | 简写 LLM 超时固定 60s | 新增简写超时环境变量；文档建议命名 provider | 是（config） | D |

## 2. 现状核实

核对报告与代码，修正报告中两处不准确的符号名：

- 不存在 `CronJobRunSuccess`，运行状态常量只有 `running / completed / failed / canceled`（`internal/systemdb/cron_jobs.go:25-31`）。
- 不存在 `GetSchema`，SDK 方法名是 `Client.DatabaseSchema`（`packages/go-sdk/databases.go:108`）。

## 3. 阶段 A：契约修复（#1、#2）

### 3.1 #1 `schema rows` 解码（P0）

**根因**

| 位置 | 形状 |
|---|---|
| `internal/api/schema_rows.go:110-117` `schemaRowsResponse.Columns` | `[]schemaColumnJSON`（`name/type/nullable/sensitive`，定义在 `schema_handler.go:27-32`） |
| `packages/go-sdk/databases.go:147-151` `TableRows.Columns` | `[]string` |

`GET .../schema` 与 `GET .../schema/tables/:table/rows` 在服务端共用 `schemaColumnJSON`，SDK 里 `DatabaseSchema` 用 `SchemaColumn` 解得对，唯独 rows 写成 `[]string`。CLI 的 `cmdSchemaRows`（`internal/sbcli/commands.go:494-509`）直接透传 SDK 结果，解码错误被 `run.go` 统一映射成 `usage` / 退出码 1，所以现象是「用法错误」。

**方案选择**

| 方案 | 评价 |
|---|---|
| A. SDK 改成对象（采用） | 服务端、控制台、`/schema` 全部不动；列类型/可空/敏感标记对调用方有用；与 `SchemaColumn` 一致 |
| B. 接口改返回字符串列名 | 控制台表格已依赖对象列（含 `sensitive`），改接口会破坏前端，且丢信息 |

**改动**

1. `packages/go-sdk/databases.go`
   - `TableRows` 改为：

     ```go
     type TableRows struct {
         Table   string         `json:"table"`
         Columns []SchemaColumn `json:"columns"`
         Rows    [][]any        `json:"rows"`
         Limit   int            `json:"limit"`
         Offset  int            `json:"offset"`
         Total   int64          `json:"total"`
     }
     ```

   - `SchemaColumn` 补 `Sensitive bool \`json:"sensitive,omitempty"\``（服务端系统库会返回）。
   - `ListTableRows` 保持签名不变（`limit`），避免破坏调用方；`offset` 作为后续独立项，不在本轮加。
2. `packages/go-sdk/schema_test.go`：新增 `TestListTableRowsDecodesColumnObjects`，用 `httptest` 返回真实形状（对象列 + `total`），断言 `Columns[0].Name/Type`、`Rows`、`Total`；再加一条列含 `sensitive:true` 的用例。
3. `internal/sbcli`：新增一条 `schema rows` 的 CLI 级用例（`cli_test.go`，沿用 `TestCLIResources` 的 fake server），断言退出码 0 与 `.data.columns[0].name`。
4. `examples/capabilities/03-sql/`：`run.sh` 的 `schema-rows` 步骤恢复为断言 `.data.columns[0].name` 与 `.data.rows | length`；`expected.md` 删除「当前仍失败」的说明。
5. `docs/sdk/go-database-sql.md`：若有 `ListTableRows` 示例，同步新类型。

**兼容性**：`TableRows.Columns` 类型变化是 SDK 的破坏性变更，但该方法此前对任何真实服务端都无法成功解码，不存在可用的存量调用方。在变更说明中标出即可。

**验收**：`go test ./packages/go-sdk/... ./internal/sbcli/... ./internal/api/...`；`./examples/run_all.sh --start --only 03-sql` 全绿，整套 `run_all.sh` 63/63。

### 3.2 #2 settings 字段 snake_case（P1）

**根因**：`internal/systemdb/settings.go:10-15` 的 `SettingKV` 无 json tag，被 `internal/api/system_handlers.go:115`、`:144` 直接 `c.JSON(map{"settings": rows})` 序列化，输出 `Key/ValueJSON/UpdatedAt`；而写入体 `system_handlers.go:123-126` 已经是 `key / value_json`，读写不对称。

**方案**：给 `SettingKV` 加 tag，不新增响应类型（两处 handler 与 CLI 都不用改）。

```go
type SettingKV struct {
    Key       string    `json:"key"`
    ValueJSON string    `json:"value_json"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

**影响面排查**（已核对）：`ui/src`、`packages/` 无消费 `ValueJSON` 的代码；Go 内部（`internal/gocloudagent/runtime.go`、`agent_handler.go`、`systemdb_test.go`）使用的是 Go 字段名，加 tag 不影响。唯一依赖旧字段名的是 `examples/capabilities/01-project/run.sh:25`。

**改动**

1. `internal/systemdb/settings.go` 加 tag。
2. `internal/api/system_handlers_test.go`：断言 GET `/settings` 响应的 JSON key 为 `key / value_json / updated_at`（补一条，防回退）。
3. `examples/capabilities/01-project/run.sh:25` 与 `expected.md`、`README.md`：`.Key` 改 `.key`。
4. `docs/` 若有 settings 响应示例同步（`docs/cli.md` 待实施时核对）。

**破坏性**：对 HTTP 调用方是破坏性变更（字段改名）。仓库内无消费者，且当前项目尚属内部版本；在发布说明中标注。

**验收**：`go test ./internal/systemdb/... ./internal/api/...`；示例 `01-project` 通过。

## 4. 阶段 B：助手可发现性与说明（#3、#4）

### 4.1 #3 内置助手默认不可写

**现状**

- 默认值：`internal/systemdb/agents.go:713-770` `defaultCloudAgentSpecs` 不设 `Writable`，库列默认 0（`migrate.go:670`）。该行为是安全默认，**不改默认值**。
- 拦截：`internal/gocloudagent/cli_tool.go:217-218` 拒绝文案「该助手未开启写权限，只能执行只读命令」，在确认流程之前触发。
- 提示词：`internal/gocloudagent/prompt.go:41-42` `capabilityWritable / capabilityReadonly`。
- skill 文档：`skills/simplebase/SKILL.md:27`。
- 控制台开关：`ui/src/components/modal/AgentFormModal.vue:88-99`，文案 `locales/zh-CN.ts:560-561`（`formWritable` / `formWritableHint`），页面 `ui/src/pages/AgentManager.vue`。

**问题**：用户说「建一个库」得到 `write_not_allowed`，模型只会转述错误，用户不知道去哪里开。

**改动**

1. `cli_tool.go` 拒绝文案补出路：`该助手未开启写权限，只能执行只读命令。请在「云助手 → 编辑助手 → 允许写操作」中开启后重试`。`write_not_allowed` 错误码与顺序不变。
2. `prompt.go` 的 `capabilityReadonly`：要求模型遇到 `write_not_allowed` 时直接告诉用户去哪里打开开关，不要反复重试。
3. `skills/simplebase/SKILL.md:27` 明确写「未开写权限时写命令会直接失败，且不会弹出确认」。
4. 控制台：
   - 先读 `AgentManager.vue` / 对话页，确认现有只读助手是否有徽标；
   - 在助手列表与会话头部为 `writable=false` 的助手加一个「只读」标签，悬浮说明并带跳转编辑的入口（用现有 `SbModal` 编辑流程，不新增页面）；
   - 文案同时补 `zh-CN.ts` 与 `en-US.ts`，key 集合保持一致（`ui/tests/i18n.test.ts` 会卡）。
5. 测试：`internal/gocloudagent` 补拒绝文案断言（沿用现有 `cli_tool` 测试）；前端补只读标签渲染用例，断言打在真实 DOM 上，不桩 `Switch`。

**验收**：`go test ./internal/gocloudagent/...`；`cd ui && yarn test && yarn build`；`examples/capabilities/agent` 的 `readonly-guard` 仍通过。

### 4.2 #4 本地 gocloudagent shell 与 CLI 凭据隔绝

**现状**：`internal/gocloudagent/tools/tools.go:140` 用 `sanitizedEnv`（`:191-204`）启动 shell，丢弃 `SIMPLEBASE_` / `GOCLOUDAGENT_` 前缀与含 `TOKEN|SECRET|PASSWORD|API_KEY|…` 的变量。`simplebase` CLI 在子进程里取不到配置，回 `config_missing`（`internal/sbcli/run.go:133-135`，退出码 4），模型无法区分「没配置」与「被剥离」。

**不放宽安全策略**。只补信息：

1. `internal/sbcli/run.go:133-135`：`endpoint, token, and project are required` 之后追加提示 `（若在 gocloudagent 的 shell 中运行：该环境不继承 SIMPLEBASE_*，请使用 simplebase 配置文件）`。先核对该文案是否被测试或 `cli_tool.go:319-323` 的 `case 4` 依赖，再改。
2. `internal/gocloudagent/cli/command.go:336-376` `printUsage` 的环境变量段落后补一句「shell 工具不会把 SIMPLEBASE_* / GOCLOUDAGENT_* 传给子进程」。
3. `tools.go` 里 shell 工具对模型的 description 补同一句，并说明正确做法是事先写好 `simplebase` 配置文件、不要把文件内容打印出来。
4. `docs/workbench/agent-tools.md` 增加「本地 gocloudagent 的环境隔离」小节；`examples/capabilities/agent/README.md:27` 已提到本地 shell，同步指向该小节。
5. 测试：`tools_test.go` 现有 `TestSanitizedEnvStripsCredentials` 保持；新增对 `config_missing` 信息含提示的断言。

**验收**：`go test ./internal/sbcli/... ./internal/gocloudagent/...`。

## 5. 阶段 C：文档与脚本（#5、#6、#7）

### 5.1 #5 `function test` 与 `invoke` 形状

**现状**：`goFuncTestResponse`（`internal/api/gofunction_handler.go:554-563`）本身是信封 `{ok,status_code,duration_ms,version,active_version,function_name,data,error}`；CLI 再包一层 `writeOK`，所以函数返回值在 `data.data`。`invoke` 直接返回函数返回值。形状差异是接口设计使然，**不改**。

**改动**

1. `docs/automation/gofunction-invoke.md`：增加对照表（`invoke` → `.data`；`test` → `.data.data`，并列出信封字段）。
2. `skills/simplebase/references/gofunction.md:33` 的 `function test` 示例旁补注释。注意该文件含 `<!-- allow: -->` 注释，由 `EmbeddedSkillAllows` 对账，只加说明文字，不动 allow 注释。
3. sbcli 没有 `--help` 机制（`run.go:43-45` 仅一行 usage），不为此新增帮助框架；改在 `docs/cli.md` 的 function 小节写明。

### 5.2 #6 定时任务运行状态

**现状**：唯一终态为 `completed`（`scheduler.go:273`）。`docs/automation/cronjob-api.md:18` 只写「含状态、耗时与错误」，未列取值。

**改动**

1. `docs/automation/cronjob-api.md` 补状态枚举：`running / completed / failed / canceled`，并注明没有 `success`。
2. `skills/simplebase/references/cron.md` 的 `cron runs` 处同样补取值。
3. `examples/capabilities/08-cron/run.sh:31-34` 恢复为只认 `completed`（现同时接受 `success`），让文档与示例一致；若担心外部兼容，保留 `success` 分支但加注释，由实施者二选一，倾向严格版。
4. 排查 `ui/src` 是否硬编码 `success` 判断运行状态（探查时未核实），若有则修正。

### 5.3 #7 `./build.sh` 不生成 `config.yaml`

**现状**：`ensure_dev_config()`（`build.sh:139-161`）只在 `cmd_dev`（`:400-401`）调用；`build.sh:59` 的帮助文本没有限定 `dev`，容易误导。默认构建只编译，不该写配置文件，**不改行为**。

**改动**

1. `build.sh:59` 帮助文案改为「`dev` 子命令缺少 config.yaml 时从 config.example.yaml 生成」。
2. 构建成功结尾的提示里追加一行：`首次运行请先 cp config.example.yaml config.yaml，或使用 ./build.sh dev`（仅在 `config.yaml` 不存在时输出）。
3. `README.md` / `docs/` 的「快速开始」核对是否提到这一点，缺则补。
4. `examples/run_all.sh:52-54` 的复制逻辑保留。

**验收**：`./build.sh --help` 输出；在无 `config.yaml` 的干净目录跑 `./build.sh --skip-ui`，提示出现。

## 6. 阶段 D：需取舍的项（#8、#9）与后续

### 6.1 #8 认证进 skill

**结论：本轮不做。** 现状是有意设计：`internal/gocloudagent/skills.go:17-19` 注释写明 user / apikey / settings 保留给人类操作者，`SKILL.md:28` 同步声明。助手代管用户与 Key 意味着创建用户、签发 Key 都是敏感写操作，需要独立的确认模型、审计字段与权限分层（超管才可用），应单列设计，不混入体验修复。

仅记录一条前置事实供将来设计：skill 清单有三处需同步——`internal/gocloudagent/skills.go`（`skillPrefixes` / `skillAliases`）、`ui/src/utils/agent-skills.ts`（手工同步副本）、`skills/simplebase/SKILL.md` 与 `references/*.md`。

### 6.2 #9 简写 LLM 超时

**现状**：无 `SIMPLEBASE_LLM_PROVIDERS` 时走简写，`internal/config/env.go:213-224` 把 `openai` provider 的 `Timeout` 固定为 60s；agent 单次运行上限 `SIMPLEBASE_LLM_AGENT_RUN_TIMEOUT` 默认 180s、最大迭代 8（`config.go:176-177`）。多轮工具调用时单次模型请求容易超过 60s。

**方案**：新增 `SIMPLEBASE_LLM_PROVIDER_TIMEOUT`，只作用于简写生成的 `openai` provider，未设置仍为 60s，向后兼容。

1. `internal/config/env.go:213-224`：读取该变量（沿用命名 provider 里 `..._TIMEOUT` 的解析函数与错误风格）。
2. `internal/config/env_test.go`：补三条——未设置为 60s、设置为 120s 生效、非法值报错。
3. `config.example.yaml` 与 `docs/` 的配置说明补充该变量，并建议多轮工具调用场景使用命名 provider（`SIMPLEBASE_LLM_PROVIDERS` + `..._TIMEOUT`）。
4. 不修改 `AGENT_RUN_TIMEOUT` / `MAX_ITERATIONS` 默认值；校验范围（`config.go:213-217`）不变。
5. 密钥规则不变：该变量不涉及密钥，且不得把密钥写入 YAML。

> 如果评审认为不该扩 env 面，降级方案是只改文档，保留现状。这属于 P3，可后置。

### 6.3 报告 §8 后续项的归属

| 报告后续项 | 本计划处理 |
|---|---|
| 1. 修 `schema rows` 契约并加解码测试 | §3.1 |
| 2. settings 改 snake_case | §3.2（选 systemdb 加 tag，而不是 CLI 归一化） |
| 3. 云助手帮助写清四点（默认不可写、shell 不继承 `SIMPLEBASE_*`、`function test` 多一层、状态为 `completed`） | §4.1、§4.2、§5.1、§5.2 |
| 4. 沙盒示例待 `sandbox.enabled` 与 Key 具备后再加 | 不做；将来加 `09-sandbox` 时在 `run_all.sh` 里按 `sandbox.enabled` 探测，未开启时跳过且不计失败 |
| 5. 业务案例保持用 `examples/init.sh` | 不做；不并入 `run_all.sh` |

## 7. 实施顺序与工作量

| 阶段 | 内容 | 预计 |
|---|---|---|
| A | #1 + #2（含测试、示例回归） | 0.5 天 |
| B | #3 + #4（文案 + 前端只读标签 + 双语文案） | 0.5–1 天 |
| C | #5 + #6 + #7（文档、脚本提示） | 0.5 天 |
| D | #9（可选）；#8 仅留结论 | 0.25 天 |

阶段 A 完成后即可让能力示例全绿，应单独成 PR；B、C 可合并；D 单独评审。

## 8. 总验证清单

```bash
go build ./internal/... ./cmd/...
go test ./internal/systemdb/... ./internal/api/... ./internal/sbcli/... \
        ./internal/gocloudagent/... ./internal/config/... ./packages/go-sdk/...
go vet ./packages/go-sdk/... ./internal/sbcli/...
cd ui && yarn test && yarn build         # 仅阶段 B 需要
./examples/run_all.sh --start            # 目标：63/63 通过；agent 8 条仍通过
```

## 9. 风险与注意事项

- #1、#2 都是对外字段/类型的变更。要在变更说明里写清，并确认没有仓库外消费者后再合并。
- 改动命中 `internal/` 下可能未过 `gofmt` 的文件时，只格式化被改文件，不全仓重排。
- 新增写路径无；本计划不涉及系统库写入，无需新增「系统库拒绝」用例。系统库 rows 的 `sensitive` 标记在 SDK 新增字段中保持透传。
- 前端新增文案必须 `zh-CN.ts` 与 `en-US.ts` 同时补，文案里的裸 `@` 写成 `{'@'}`。
- 改 skill 参考文档时不得改动 `<!-- allow: -->` / `readonly` 注释，否则 `EmbeddedSkillAllows` 对账会变。
- 本计划不触发任何数据库结构变更，无迁移。

## 10. 实施结果（2026-10-15）

阶段 A–D 全部落地，#8（认证进 skill）按计划不做。

### 代码改动

| 问题 | 文件 | 改动 |
|---|---|---|
| #1 | `packages/go-sdk/databases.go` | `TableRows` 改为 `{Table, Columns []SchemaColumn, Rows, Limit, Offset, Total}`；`SchemaColumn` 增 `Sensitive` |
| #1 | `packages/go-sdk/schema_test.go` | 新增 `TestListTableRowsDecodesColumnObjects`（列对象、`total`、`sensitive`、空库 ID 报错） |
| #1 | `internal/sbcli/cli_test.go` | 伪服务端补 `/rows` 路由；`TestCLIResources` 新增 `schema rows` 断言 |
| #1 | `examples/capabilities/03-sql/{run.sh,expected.md}` | `schema-rows` 恢复断言，删除「当前失败」说明 |
| #2 | `internal/systemdb/settings.go` | `SettingKV` 加 `json` tag（`key` / `value_json` / `updated_at`） |
| #2 | `internal/api/system_handlers_test.go` | `TestSettingsHandlers` 断言 snake_case 且旧字段名不泄漏 |
| #2 | `examples/capabilities/01-project/{run.sh,expected.md}` | `.Key` → `.key` |
| #3 | `internal/gocloudagent/cli_tool.go` | 拒绝文案补「编辑助手 → 允许写操作」出路 |
| #3 | `internal/gocloudagent/prompt.go` | `capabilityReadonly` 要求直接说明去哪里开启，不重试 |
| #3 | `skills/simplebase/SKILL.md` | 写明写命令直接失败、不弹确认 |
| #3 | `ui/src/components/agent/AgentCard.vue`、`ui/src/pages/AgentManager.vue` | `writable=false` 时显示「只读」标签（列表 + 会话头部），头部标签点击直达编辑 |
| #3 | `ui/src/locales/{zh-CN,en-US}.ts` | `components.agentCard.readonly` / `readonlyHint` |
| #3 | `ui/tests/agent-components.test.ts` | 只读标签仅在 `writable === false` 出现（旧后端无字段时不误报） |
| #4 | `internal/sbcli/run.go` | `config_missing` 文案说明 shell 不继承 `SIMPLEBASE_*`、改用配置文件 |
| #4 | `internal/gocloudagent/cli/command.go` | `--help` 新增 shell 环境隔离一节 |
| #4 | `internal/gocloudagent/tools/tools.go` | shell 工具 description 说明剥离范围与 `config_missing` 含义 |
| #4 | `docs/cli.md`、`docs/workbench/agent-tools.md`、`examples/capabilities/agent/README.md` | 补同一说明 |
| #5 | `docs/automation/gofunction-invoke.md`、`docs/cli.md`、`skills/simplebase/references/gofunction.md` | 写明 `test` 的 `data.data` 信封 |
| #6 | `docs/automation/cronjob-api.md`、`skills/simplebase/references/cron.md`、`docs/cli.md` | 状态枚举 `running/completed/failed/canceled`，明确没有 `success` |
| #6 | `examples/capabilities/08-cron/run.sh` | 恢复只认 `completed` |
| #7 | `build.sh` | 帮助文案限定 `dev`；构建完成时若缺 `config.yaml` 给出提示 |
| #9 | `internal/config/env.go` | 简写 provider 支持 `SIMPLEBASE_LLM_PROVIDER_TIMEOUT`，非法/非正值回落 60s |
| #9 | `internal/config/config_more_test.go` | 新增 `TestLoadLLMProvidersFlatEnvTimeout`（默认 60s / 120s 生效 / 三种非法值回落） |
| #9 | `config.example.yaml`、`.env.example` | 补变量说明与多轮工具调用建议 |

### 与计划的偏差

1. `#9` 的测试落在 `internal/config/config_more_test.go`（计划里写的是 `env_test.go`，实际该包把 LLM env 用例放在 `config_more_test.go`）。
2. `#2` 未新增独立响应类型，只给 `SettingKV` 加 tag——与计划一致，实施时确认了 `ui/`、`packages/` 无消费者。
3. `#4` 的错误文案改为单一字符串拼接（原文案与新增提示合并成一句话），`cli_tool.go` 的 `case 4` 直接把 stderr 透传，未受影响。
4. `#3` 会话头部的只读标签用 `Badge` 且可点击直达编辑弹窗，比计划里「加标签 + 悬浮说明」多了一个入口。

### 验证

```bash
gofmt -l internal/gocloudagent internal/sbcli internal/config packages/go-sdk   # 无输出
go build ./internal/... ./cmd/... ./packages/...
go vet ./packages/go-sdk/... ./internal/sbcli/... ./internal/config/...
go test ./...                                   # 除既有 skills/simplify-code-eval-std 夹具外全绿
cd ui && npx vitest run --coverage              # 107 files / 744 tests 通过
cd ui && yarn build                             # 通过
./build.sh                                      # 通过，含 gocloudagent 二进制
./build.sh --help                                # 帮助文案已限定 dev
```

前端覆盖率：statements 99.17% / branches 95.31% / functions 95.00%（三项均达 95% 阈值）。

### 未做

- `./examples/run_all.sh --start` 未在本机执行：需要真实服务与 LLM 密钥，且会向库中写示例资源，留给有能力启动服务的环境验证。
- `#8` 认证进 skill：按计划仅保留结论（见 §6.1）。
- 沙盒示例（报告 §8 第 4 条）：维持不做。
