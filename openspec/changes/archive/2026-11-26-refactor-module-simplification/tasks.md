# Tasks

## 1. 第 0 批：基线与特征测试

- [x] 1.1 记录工作区状态（不覆盖未提交改动），以及 build / vet / test / 门禁 / `gofmt -l` 基线，包括已有失败项
- [x] 1.2 `gosandbox/proto`：每个 `Kind` 补往返测试和字节快照（golden），覆盖超长字段的错误文本
- [x] 1.3 `gosandbox/host`：为 `BwrapArgs` 补典型策略的 golden argv
- [x] 1.4 `gocloudagent/plan`：`Validate` 补多错误同时存在的输入，锁定先返回哪个错误
- [x] 1.5 `internal/config`：确认 env / YAML 每个域都有覆盖用例，缺的补上；`Redacted()` 补整张 map 的快照

## 2. 第 1 批：internal 迁移残留

- [x] 2.1 A-01 `app/assemble_api.go` 和 `internal/api` 同包测试改为直接引用 `<context>http`；删除四个 `*_compat.go`，`sandboxDeps` 移到 `router.go`
- [x] 2.2 同步 `kernel_residue_test.go` 清单、根 `AGENTS.md`（更正文件数）、`internal/README.md`、`internal/AGENTS.md`、平台架构 spec 中的文件清单描述
- [x] 2.3 A-02 `applyEnv` 按域拆分
- [x] 2.4 A-03 `applyYAML` 按域拆分
- [x] 2.5 A-04（可选）`defaults` / `Redacted` 按域拆分

## 3. 第 2 批：gocloudagent

- [x] 3.1 C-01 `group/engine.go` 按职责拆成 5 个同包文件（纯搬移，单独提交）
- [x] 3.2 C-02 先用 fake Store / Runner 复现取消、存储失败、awaiting_plan_confirm 暂停三种交错，再把 `execute` 的循环体抽出来；跑 `-race`
- [x] 3.3 C-03 去掉 `truncate` 一行转发，group 的上限改成具名常量
- [x] 3.4 C-04 `plan.Validate` 按阶段抽函数
- [x] 3.5 C-05 `cli/group_runtime.go` 拆出 `group_runner.go` / `group_prompt.go`（纯搬移）
- [x] 3.6 跑宿主侧 `go test -race ./internal/agentrun/ ./internal/api/ ./internal/app/`

## 4. 第 3 批：gosandbox

- [x] 4.1 S-01 `proto` 编解码按 Kind 拆成成对的私有函数，1.2 的 golden 用例必须保持字节一致
- [x] 4.2 S-04 `BwrapArgs` 分段抽取，1.3 的 golden argv 保持一致
- [x] 4.3 S-03 `group` / `sandboxhttp` 的手写 `itoa` 换成 `strconv.Itoa`；host 侧先查清手写的原因
- [x] 4.4 Linux 分支：`GOOS=linux go vet ./...`，需要真实 bwrap 的用例在 Linux CI 上验证，并记录未验证的平台（仓库没有会跑 gosandbox 的 Linux CI：已改在本机 Linux 虚拟机 + 真实 bwrap 上做 base / work 对照验证，逐用例无差别；另发现 5 个范围外的既有问题，见 design「4.4 补做」）

## 5. 已批准项（2026-11-26 批准，结论见 design「已批准项结论」）

- [x] 5.1 S-02 新建 `gosandbox/internal/ptyx`（linux / darwin / 其他平台），agent 和 host 改为调用它，删除原来的两份 `openPTY` / `setWinsize`
- [x] 5.2 X-02 `godatabase/blob` 导出 `JoinKey` 并补测试；`internal/objectstore` 删除副本，改为调用 `blob.JoinKey`
- [x] 5.3 S-03 host 侧手写 `itoa` 随 5.1 删除

## 6. 收尾

- [x] 6.1 全量复跑第 0 批的命令，与基线对比；已有失败单独列出
- [x] 6.2 检查最终 diff：没有混入无关改动、没有残留 import 或包装函数、导出符号没有变化（嵌套模块可对比 `go doc` 或 `apidiff`，工具不可用时写明）
- [x] 6.3 每项回填「完成 / 部分 / 保留 / 取消」、实际执行的命令和未验证的边界；不提前打勾
- [x] 6.4 `openspec validate refactor-module-simplification --strict`，完成后 `openspec archive`（`--strict` 通过；已归档为 `changes/archive/2026-11-26-refactor-module-simplification`。改动本身未由代理提交：除 C-01 外都还在工作区，提交分组建议见 design「6.2」）
