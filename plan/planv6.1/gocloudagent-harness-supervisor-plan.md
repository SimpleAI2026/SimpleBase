# gocloudagent Harness 方案：强模型监督 + 弱模型执行（Supervisor/Advisor + GLM-5.3-Flash Worker）

> 版本：planv6.1 草案 · 日期：2026-10-18 · 拟放置路径：`plan/planv6.1/gocloudagent-harness-supervisor-plan.md`
> 状态：调研 + 设计，**未改动仓库代码**。基于 `linkxzhou/SimpleBase` main 分支（`internal/gocloudagent`、`internal/llmgateway`、`plan/planv6.1/*`）阅读。
> 引用规则：所有链接在 2026-10-18 用 curl/WebFetch 访问过（HTTP 200）。openai.com 两篇站点屏蔽了 curl（403），内容通过 WebSearch 摘要核实，表中标为“经搜索核实”。数字只取原文给出的；拿不准或没读正文的标为 **未核实**。

---

## 0. 摘要（TL;DR）

1. 业界已经有“便宜 executor 主跑，强模型按需出主意”的成熟做法：Anthropic **Advisor tool / Advisor strategy**、Aider **Architect/Editor**、Anthropic 多智能体研究系统（Opus lead + Sonnet sub-agents），还有学术界的 COPE、FrugalGPT/RouteLLM/AutoMix 级联路由。收益大小**强烈依赖“弱模型是否真的去咨询”和“有没有可靠的失败信号”**。
2. gocloudagent 当前的失败（TB 0/8、120s 超时、只 `read_file` 就停、不写回、断言失败、helper NameError）有相当一部分是 **harness 问题**：隐式 `max_tokens=4096` 截断、缺“完成前必须验证”的停止准则、超时预算。这些改进比引入强模型更便宜，应当先做（P0）。
3. 推荐的架构是**外层循环（Supervisor Loop）包住现有的 `SessionRunner`**，不建议用 eino `supervisor.New`/`NewAgentTool` 让强模型当主 agent。强模型只在 4 个时点出现：**开局规划、确定性触发的卡住干预、完成前验收、（可选）失败后升级重跑**。weak worker 另外可以通过 `consult_supervisor` 工具主动咨询，调用次数有上限。
4. 默认关闭（`agent.supervisor.enabled=false`）。强模型仍走 SimpleBase gateway 和服务端 provider 凭证，CLI 不持有 key（延续 D-D1）。跨 provider（例如 Flash 在 SiliconFlow、监督者在另一家）需要 gateway 支持 **model→provider 路由**，目前 `prepareRequest` 总是走默认 provider。
5. 用 A/B 四臂评测（weak-only / strong-only / supervised / cascade）决定是否默认开启，按模型分别记录 tokens 和 ¥ 成本。

---

## 1. 调研综述

### 1.1 模式分类表

| # | 模式 | 代表方案 | 链接 | 成本/效果数据（仅原文给出） | 对 gocloudagent 的适用性 |
|---|---|---|---|---|---|
| 1 | **Advisor（执行者按需咨询强模型）** | Anthropic Advisor tool（beta `advisor-tool-2026-03-01`） | https://platform.claude.com/docs/en/agents-and-tools/tool-use/advisor-tool | advisor 看完整 transcript，无工具，通常返回 400–700 文本 tokens。支持 `max_uses` 和每次 `max_tokens`（文档建议 2048，可把输出压到约 1/7）。advisor 用量在 `usage.iterations` 单独上报。Haiku 在第 2 轮被提示咨询约 +7pp；“写文件前必须先咨询”的 Hard rule 让 Haiku coding 约 +7.5pp | **高**。与 GLM-5.3-Flash 的“只读就停”问题直接对应：可要求“首次写之前 / 宣告完成之前必须咨询” |
| 1b | Advisor strategy（博客） | Sonnet/Haiku executor + Opus advisor | https://claude.com/blog/the-advisor-strategy | Sonnet+Opus advisor：SWE-bench Multilingual +2.7pp，每任务成本 −11.9%。Haiku+Opus advisor：BrowseComp 41.2% vs 单跑 19.7%，比 Sonnet 单跑便宜 85% | 高。证明“弱执行 + 强建议”可以同时提升效果、降低成本 |
| 1c | Advisor 的反面证据 | Anthropic “Optimizing for cost and intelligence” | https://platform.claude.com/docs/en/about-claude/models/optimizing-for-cost-and-intelligence | 内部 coding 基准 +1.7pt，成本约 2.1x，平均约 1.4 次咨询/尝试。咨询很少时收益落在噪声内甚至为负（GPQA 0 次咨询；Chartography −7pt）。引用 DeepSWE 客户端 advisor loop：低 effort Sonnet 5 + advisor 比单独 executor +23pt。“失败后用高 effort 重跑”比全量高 effort 便宜约 40%，通过率相同（前提是有失败信号）。把 max_tokens 提到 64k 在某 repo 基准上最高 +22pt。prompt caching 是最大的成本杠杆 | **关键警示**：必须用**确定性触发**强制咨询，不能完全依赖弱模型自觉。另外 max_tokens 本身就是大杠杆 |
| 1d | Claude Code advisor / subagents | 每个 subagent 单独配置 `model:` | https://code.claude.com/docs/en/advisor ，https://code.claude.com/docs/en/sub-agents | 文档未给量化数据 | 配置形态可以参考（每个角色独立选模型） |
| 2 | **Orchestrator-Worker（强 lead + 弱 workers）** | Anthropic Multi-agent Research System | https://www.anthropic.com/engineering/multi-agent-research-system | Opus 4 lead + Sonnet 4 子代理在内部评测上比单 Opus 4 高 90.2%。token 用量解释了 80% 的方差。agent 约用 chat 的 4x tokens，多智能体约 15x。原文指出 coding 的可并行性较弱 | **中低**。gocloudagent 任务（HumanEval/MBPP/TB/SWE）大多是串行单仓库任务，并行拆分收益有限，token 却会成倍增加 |
| 2b | 多智能体何时不划算 | Kim et al., *Towards a Science of Scaling Agent Systems* | https://arxiv.org/abs/2512.08296 | 260 个配置、6 个 benchmark：单 agent 基线超过一定水平后，协作收益递减。工具密集型任务有多智能体开销。没有集中式验证的架构更容易传播错误。相对单 agent 的变化范围从 +80.8%（可分解金融推理）到 −70.0%（串行规划） | 支持“**集中式验证（强模型验收）**”，不支持“多 worker 并行拆分” |
| 3 | **Architect/Editor（强模型想，弱模型改）** | Aider architect mode（2024-09） | https://aider.chat/2024/09/26/architect.html | o1-preview + DeepSeek/o1-mini（whole 格式）85.0% vs o1-preview 单跑 79.7%。o1-mini + DeepSeek 71.4% vs 61.1%。gpt-4o-mini 自配对 60.2% vs 55.6% | 高。“规划/诊断交给强模型，落盘编辑交给便宜模型”。注意：即便同模型分成两步也有提升，说明部分收益来自**流程拆分本身** |
| 4 | **Planner-Executor / 级联协作** | COPE（KAIST，arXiv 2506.11578） | https://arxiv.org/abs/2506.11578 | 三阶段：小模型单跑 → 大模型规划、小模型执行 → 大模型全做。按置信度升级，代码任务用测试执行作信号（具体节省比例 **未核实**） | 高。与“先 weak 跑、失败再让 strong 规划”的升级链完全对应 |
| 4b | Plan-and-Act | arXiv 2503.09572 | https://arxiv.org/abs/2503.09572 | WebArena-Lite 57.58%，WebVoyager 81.36% | 中。证明显式 planner 有用，但场景是 web agent |
| 4c | 强弱协作仓库级代码生成 | Gandhi et al., EMNLP 2025 | https://aclanthology.org/2025.emnlp-main.1043/ | 以约 40% 更低成本达到强模型水平。pipeline 和基于上下文的策略最高效 | 高。直接是 repo-level code 场景 |
| 4d | 产品实践：Plan mode | Cursor planning、Devin 2.0、Codex subagents | https://docs.cursor.com/agent/planning ，https://cognition.ai/blog/devin-2 ，https://developers.openai.com/codex/subagents | 页面可访问，正文细节 **未核实** | 仅作形态参考 |
| 5 | **路由 / 级联（Routing & Cascade）** | FrugalGPT（2305.05176） | https://arxiv.org/abs/2305.05176 | 与 GPT-4 同效果时成本最多降 98%（HEADLINES 98.3%），或同成本 +4% 准确率 | 中高。“先 Flash，失败再 strong 重跑”是最简单的级联 |
| 5b | RouteLLM（2406.18665） | 训练路由器 | https://arxiv.org/abs/2406.18665 | 成本降低 2x 以上，MT Bench 最高 3.66x | 中。要训练路由器、要数据，短期不做 |
| 5c | AutoMix（2310.12963，NeurIPS 2024） | 自验证 + POMDP 路由 | https://arxiv.org/abs/2310.12963 | 性能相当时成本降低 50% 以上 | 中。代码任务的“自验证”可以直接用测试结果代替 |
| 6 | **Critic / Verifier / Judge** | OpenHands critic（TD 训练的 Qwen2.5-Coder-32B） | https://www.openhands.dev/blog/sota-on-swe-bench-verified-with-inference-time-scaling-and-critic-model ，https://huggingface.co/OpenHands/openhands-critic-32b-exp-20250417 ，https://docs.openhands.dev/sdk/guides/critic | SWE-bench Verified：单轨迹 60.6% → 5 次尝试 + critic 选择 66.4% | 中。需要多次采样（成本 ×N），适合 SWE 类任务的可选模式 |
| 6b | Agent-as-a-Judge（2410.10934） | 用 agent 评估 agent | https://arxiv.org/abs/2410.10934 | DevAI 55 个任务，与人工一致率约 90%（LLM-as-a-Judge 约 70%） | 高。强模型“完成前验收”时应带工具（读文件/跑测试），不能只看文本 |
| 6c | Trae Agent（2507.23370） | 生成 / 剪枝 / 选择 模块化 agent | https://arxiv.org/abs/2507.23370 | SWE-bench Verified Pass@1 75.20%，比基线平均 +10.22% | 中。selector 模式可以用于 cascade 的多候选选择 |
| 7 | **Test-time scaling / PRM** | Large Language Monkeys（2407.21787） | https://arxiv.org/abs/2407.21787 | DeepSeek-Coder-V2 在 SWE-bench Lite 上 1 次 15.9% → 250 次 56% | 中。说明弱模型 coverage 高，瓶颈在**选择/验证** |
| 7b | CodeMonkeys（2501.14723） | 串行 + 并行扩展，模型生成测试投票 + 选择轨迹 | https://arxiv.org/abs/2501.14723 | SWE-bench Verified 57.4%，预算约 2300 USD。对已有 top 提交集成做选择得 66.2% | 低（成本过高），但“编辑时同时生成测试脚本”值得借鉴到 worker prompt |
| 7c | S*（2502.14382） | 代码生成混合 test-time scaling + 执行驱动选择 | https://arxiv.org/abs/2502.14382 | 3B 模型超过 GPT-4o-mini。GPT-4o-mini+S* 在 LiveCodeBench 上比 o1-preview 高 3.7% | 中。对 HumanEval/MBPP 类函数级任务有效 |
| 7d | PRM / 过程监督 | Let's Verify Step by Step（2305.20050，2023） | https://arxiv.org/abs/2305.20050 | MATH 子集 78%，过程监督优于结果监督，发布 PRM800K | 低（需训练 PRM）。思想上对应“每步/关键节点监督” |
| 7e | Test-time compute 最优分配 | Snell et al.（2408.03314） | https://arxiv.org/abs/2408.03314 | 原文结论：按难度自适应分配 test-time compute 比固定策略更好（具体倍数 **未核实**） | 支持“按触发/难度升级”而非全量升级 |
| 8 | **Speculative / draft-then-verify（agent 级）** | Speculative Actions（2510.04371） | https://arxiv.org/abs/2510.04371 | 下一动作预测准确率最高 55%，延迟最多降 20% | 低。目标是降延迟，不是提通过率，暂不做 |
| 9 | **Weak-to-Strong** | Burns et al.（2312.09390） | https://arxiv.org/abs/2312.09390 | 训练范式（弱监督强），方向与本方案相反 | 不适用（本方案是强监督弱），仅作概念区分 |
| 10 | **Stuck detection / 干预触发** | OpenHands StuckDetector | https://github.com/OpenHands/software-agent-sdk/blob/main/openhands-sdk/openhands/sdk/conversation/stuck_detector.py | 规则：同一 action+observation 重复 4 次等，扫描最近 20 个事件 | 高。gocloudagent 已有 `newRepeatDetector(12, 4)`，可以把“直接报错停止”改成“先触发 supervisor 干预” |
| 11 | **Harness engineering** | OpenAI “Harness engineering”（2026-02-11） | https://openai.com/index/harness-engineering/ （站点屏蔽 curl，经搜索核实） | “Humans steer. Agents execute”，agent 之间互相 review 的循环（无可引用的量化数据） | 高（理念）：supervisor 就是“steer”的自动化 |
| 11b | Codex agent loop | OpenAI “Unrolling the Codex agent loop”（2026-01-23） | https://openai.com/index/unrolling-the-codex-agent-loop/ （同上，经搜索核实） | 无可引用数据 | 参考 loop 结构、prompt caching |
| 11c | Effective harnesses for long-running agents（Anthropic，2025-11-26） | initializer + coding agent，feature_list（全部 `passes:false`），progress 文件，git commit，端到端验证 | https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents | 失败模式：过早宣告胜利、未测试就标记完成 | **高**。与 gocloudagent “读完就停 / 不写回”同类，解法是结构化的待办 + 强制验证 |
| 11d | Harness engineering（Fowler / Böckeler） | guides（前馈）+ sensors（反馈），computational vs inferential | https://www.martinfowler.com/articles/harness-engineering.html | 无量化数据 | 高（分类框架）：优先用 computational sensor（测试、lint）触发，inferential（LLM 评审）为辅 |
| 11e | Lilian Weng “Harness Engineering for Self-Improvement”（2026-07-04） | 综述 | https://lilianweng.github.io/posts/2026-07-04-harness/ | 具体数据 **未逐条核实** | 背景阅读 |
| 12 | **Context engineering** | Anthropic “Effective context engineering for AI agents”（2025-09-29） | https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents | 无量化数据（compaction、结构化笔记、sub-agent 隔离上下文） | 高。supervisor 的输入应是**压缩后的轨迹摘要 + 关键 diff/测试输出**，而不是全量 transcript |
| 13 | **极简 harness 基线** | mini-swe-agent（核心 <100 行，仅 bash） | https://github.com/SWE-agent/mini-swe-agent | SWE-bench “Bash Only” 榜的标准 harness | 对照组：证明 harness 简单也能做到强。复杂化需要数据支撑 |
| 13b | SWE-agent ACI（2405.15793） | Agent-Computer Interface 设计 | https://arxiv.org/abs/2405.15793 | SWE-bench pass@1 12.5%，HumanEvalFix 87.7%（2024） | 工具接口设计（编辑/搜索工具形态）影响很大 |
| 13c | Agentless（2407.01489） | 无 agent 的定位-修复-验证流水线 | https://arxiv.org/abs/2407.01489 | SWE-bench Lite 32.00%（96/300），GPT-4o 每 issue $0.70 | 流程固定化也是一种“监督”，可以用于 SWE 类任务 |

社区讨论（X，仅作舆情参考，不作证据）：
- advisor strategy 实践分享：https://x.com/otisxyz/status/2047333126628839589
- 质疑 harness 过度复杂：https://x.com/okleusi/status/2108228359088124055
- 放弃 orchestrator-worker 拆分的经验：https://x.com/piyush_yip/status/2094397461385412830
- harness engineering 思维：https://x.com/iwhaleocean/status/2063813051141570698
- 网传“Karpathy 评 Opus 5.5 harness engineering”等转述贴：**未核实，不引用**。

### 1.2 调研结论提炼

- **R1（触发比模型更重要）**：advisor 的收益来自“被咨询”。咨询 0 次就没有收益，甚至可能为负（1c）。弱模型往往不会主动求助，所以需要**确定性 harness 触发**（1、1c、10）。
- **R2（验证驱动）**：所有显著的级联/升级收益都依赖可靠的失败信号（测试、执行结果）（4、5c、1c “失败后重跑”）。gocloudagent 的 benchmark 都有可执行测试，这是天然优势。
- **R3（集中式验收 > 并行拆分）**：2b 显示集中式验证能减少错误传播，工具密集的串行任务不适合多 worker（2、2b）。
- **R4（流程拆分本身就有价值）**：Aider 同模型自配对也有提升（3），Anthropic 长任务 harness 使用同一模型的不同 prompt（11c）。部分收益不需要强模型。
- **R5（先修 harness 再加模型）**：max_tokens 提高（1c 最高 +22pt）和 prompt caching 是“免费午餐”。gocloudagent 隐式 4096 截断应当先修。
- **R6（成本可控）**：advisor 每次输出 400–700 tokens，并有 `max_uses`/`max_tokens` 上限（1）。按 SiliconFlow 价格，GLM-5.3 约为 Flash 的 10x，少量咨询的增量成本可控（见 §5.4 估算）。

---

## 2. gocloudagent 现状与痛点映射

### 2.1 现状（来自代码阅读）

- 运行时：eino v0.9.19 `adk.NewChatModelAgent` + `adk.NewRunner(EnableStreaming:true)`，`ToolsConfig{ExecuteSequentially:true, UnknownToolsHandler}`，`MaxIterations`。
- 模型：`agentcore.NewModel(Client,"",model)` 通过 `HTTPClient` 调 `POST {endpoint}/v1/projects/{projectID}/v1/chat/completions`（Bearer SimpleBase token），并有 usage callback。CLI 不持有 provider key（D-D1）。
- 护栏：
  - `newRepeatDetector(12, 4)`：(tool,args) 的 SHA-256 在窗口内重复超过 4 次即报错停止。
  - `MaxRunSeconds` 全局 wall-clock。
  - compaction。
  - shell 单命令 120s / 32KiB。
- 结束原因：`stop / max_iterations / length / canceled / error`。`end` 事件带 `usage{prompt,completion,total,reasoning}`、`model`、`duration_ms`。
- Gateway：`llmgateway.prepareRequest` 总是 `getClient(ctx, projectID, "")`，即默认 provider。请求里的 `model` 只在默认 provider 内覆盖（受 `AllowedModels` 约束）。实例 provider 通过 `SIMPLEBASE_LLM_PROVIDERS` / `SIMPLEBASE_LLM_PROVIDER_<NAME>_*` 配置。
- 已知 harness 问题（optimization plan）：litellm `applyDefaults` 隐式 `max_tokens=4096`；`finish_reason=length` 导致工具参数 JSON 截断（smoke 中 risk-scorer-replay 即此）。

### 2.2 基线（PR #51，以用户给出为准）

| Suite | 结果 | 备注（smoke-report 同期数据） |
|---|---|---|
| HumanEval | 154/164 | 复跑一次为 158/164；prompt 1.07M / completion 116k tokens |
| MBPP sanitized | 348/427 | 同类复跑：61 个断言失败、12 个超时、2 个运行时错误，约 ¥2.28 |
| Terminal-Bench（8 题） | 0/8 | layout-config-recreation 918s / 43 步；risk-scorer-replay 工具参数 JSON 截断 |
| SWE-bench Verified 前 10 | 5/10 | 2 题无 patch、2 题测试失败、1 题 gateway 超时；约 ¥0.63，1843s |

### 2.3 痛点 → 根因 → 对策映射

| 失败现象 | 可能根因（H=harness，M=模型能力） | 首选对策（不需强模型） | Supervisor 能补什么 | 优先级 |
|---|---|---|---|---|
| 120s 超时（MBPP 12 题 / TB 长任务） | H：单命令 120s 太死；M：写出死循环或慢算法 | 按 suite 放宽单命令超时；超时信息结构化回传（建议加 `timeout` 标签） | 触发 T4（超时/预算将尽）：强模型诊断“算法复杂度/死循环/阻塞 IO”，给出改写方向 | P0 + P1 |
| 只调 `read_file` 就停、不写回（SWE 2 题无 patch） | M：弱模型过早结束（=11c 的“过早宣告胜利”）；H：停止准则缺失 | 停止准则：有写任务时，没有 write/edit 就不允许 `stop`（optimization #13）；worker prompt 加入“先计划→写→测”清单 | **T1 开局计划**（明确要改的文件/函数）+ **T5 无写入即停检测** + **T6 完成前验收** | P0 + P1 |
| 断言失败（MBPP 61 题、SWE 2 题测试失败） | M：理解题意/边界错误；H：未运行测试就交付 | 强制“交付前运行可见测试/样例”（computational sensor） | T6 验收：强模型对照题目和测试输出给出修正点；cascade：失败后用强模型重跑 | P1 |
| helper 函数 NameError | M：漏写依赖函数/只输出片段；H：`length` 截断导致代码不完整 | 修 `max_tokens` 默认值；交付前 `python -c "import …"` 冒烟 | T6 验收中的静态检查（未定义名） | P0 |
| 工具参数 JSON 截断（TB） | H：`max_tokens=4096` + `finish_reason=length` | 显式传更大 `max_tokens`；length 时提示“分块写文件” | T3 length 触发：强模型给分块写入策略 | **P0** |
| 重复动作（TB 43 步） | M：卡在循环；H：detector 直接报错终止 | detector 先 warn 再 error（两段阈值） | **T2 卡住干预**：warn 时让 supervisor 看最近 N 步并给新方向 | P1 |
| gateway 超时（SWE 1 题） | H：上游超时/重试 | optimization #2/#3（重试、超时透传） | 无 | P0 |

> 结论：**P0 harness 修复应在 supervisor 之前完成，并单独测一轮基线**，否则 A/B 会把 harness 收益算到 supervisor 头上。

---

## 3. 推荐架构

### 3.1 总体形态：外层 Supervisor Loop + 可选 Advisor 工具

```
                ┌────────────────────── SupervisorRunner (cli/supervisor.go) ──────────────────────┐
 prompt ──────▶ │ [S0 Plan]  strong.Generate(task + repo 概要) → plan(checklist, 目标文件, 验证命令) │
                │      │                                                                          │
                │      ▼                                                                          │
                │ [W  Run]   SessionRunner(weak=GLM-5.3-Flash).Run(prompt + plan)                 │
                │      │     ├─ middleware: TriggerMonitor (BeforeModelRewriteState / WrapInvokableToolCall) │
                │      │     │     T2 重复 warn / T3 length / T4 预算将尽 / T5 只读不写 …          │
                │      │     │     → strong.Review(轨迹摘要) → 注入 <supervisor_guidance> 消息   │
                │      │     └─ tool: consult_supervisor(question)  (可选, max_uses)              │
                │      ▼                                                                          │
                │ [S1 Accept] reason==stop 时: 跑 verify 命令 + strong.Judge(diff, 测试输出)       │
                │      ├─ PASS → 结束                                                             │
                │      └─ FAIL → 注入修正意见, 续跑 W（最多 accept_rounds 次）                    │
                │ [S2 Escalate] (可选 cascade) 仍失败 → strong 作为 worker 重跑 / 接手               │
                └───────────────────────────────────────────────────────────────────────────────────┘
 end 事件: usage(worker) + supervisor_usage + supervisor_calls + triggers_fired + cost_cny
```

**为什么用外层循环，而不是 eino 的 `supervisor.New` / `NewAgentTool`：**

| 方案 | 优点 | 缺点 | 结论 |
|---|---|---|---|
| A. eino `supervisor.New`（强模型做主 agent，弱模型作为子 agent） | 现成 | eino 文档本身标注不推荐。强模型会进入每一轮循环，token 最贵的模型消耗最多，违背“弱干活” | 不采用 |
| B. `adk.NewAgentTool(strongAgent)` 作为 worker 的工具（advisor 式） | 简单，等价于 Anthropic advisor | 完全依赖弱模型主动调用。R1 表明弱模型调用率低时没有收益 | **作为补充**（`consult_supervisor`） |
| C. **外层循环 + 确定性触发 + 中间件注入** | 触发可控、可测、可预算。不改变 worker 主循环。对现有 `SessionRunner` 侵入小 | 要自己写触发器和摘要器 | **主方案** |
| D. 纯 cascade（weak 失败 → strong 重跑） | 最简单，FrugalGPT 型 | 需要可靠失败信号。线上任务（非 benchmark）没有测试时无法判定 | 作为 C 的最后一级（S2） |

### 3.2 角色职责

| 阶段 | 角色 | 输入（context engineering） | 输出 | 默认 max_output_tokens |
|---|---|---|---|---|
| S0 Plan | strong | 任务 prompt + 仓库文件树（截断）+ 已知验证命令（如能发现 `pytest`/`go test`） | JSON：`{steps[], target_files[], verify_cmds[], risks[]}`。写入 worker 首条 user 消息的 `<plan>` 块 | 1024 |
| 干预 Review | strong | 触发原因 + 最近 K 步动作/观察的压缩摘要（每条观察截断到 1–2KB）+ 当前 plan + git diff stat | 文本建议（≤ 600 tokens，参考 advisor 400–700）：诊断 + 下一步 1–3 个具体动作 | 768 |
| consult 工具 | strong | worker 提问 + 同上摘要 | 文本建议 | 768 |
| S1 Accept | strong（**带只读工具**，参考 Agent-as-a-Judge） | 任务 + plan + `git diff` + verify 命令输出（可截断） | JSON：`{verdict: pass/fail, issues[], fix_hints[]}` | 1024 |
| S2 Escalate | strong 作为 worker | 原任务 + 弱模型失败摘要 + diff | 完整运行 | 跟随 worker 配置 |
| W Run | weak（GLM-5.3-Flash） | 原 prompt + plan + 注入的 guidance | 工具调用 / 代码 | 建议显式 ≥ 16k（修 4096 问题） |

### 3.3 触发策略（deterministic first）

| ID | 触发条件 | 数据来源（现有代码） | 动作 | 默认 |
|---|---|---|---|---|
| T1 | 运行开始 | — | S0 Plan | on（enabled 时） |
| T2 | 重复动作达到 warn 阈值（建议 3，error 阈值保持 4→6） | `repeatDetector`：增加 `warn` 回调，不再立即报错 | Review → 注入 guidance，计数器清零 | on |
| T3 | `finish_reason=length` 或工具参数 JSON 解析失败 | runner 中 reason=length / UnknownTools/参数错误 | Review（重点：分块写文件、减少单次输出） | on |
| T4 | wall-clock 已用 ≥ 60% 或 iterations 已用 ≥ 60% | `MaxRunSeconds`、`MaxIterations`、事件计数 | Review（重新聚焦/裁剪范围） | on |
| T5 | 连续 N（默认 8）次只读工具调用、无 write/edit/shell 写入；或 `stop` 时任务需要改动但 diff 为空 | `WrapInvokableToolCall` 统计工具类型；`git diff --stat` | Review；`stop` 时拒绝结束并注入“必须落盘” | on |
| T6 | worker 返回 `stop` | runner reason | S1 Accept：先跑 verify_cmds（computational），失败或无命令时再让强模型判断（inferential） | on |
| T7 | verify/测试命令失败 ≥ 2 次且错误相同 | shell 工具输出哈希 | Review（换思路） | on |
| T8 | worker 调用 `consult_supervisor` | 工具 | Review | off（Phase 3 再开） |
| T9 | S1 判 fail 且 `accept_rounds` 用尽 | — | S2 Escalate（cascade） | off |

全局约束：`max_calls`（默认 6）、`min_steps_between_interventions`（默认 5，防抖），同一触发 ID 最多 2 次。

### 3.4 在 eino 上的落地（包、接口、文件）

> 以下为设计，**未改代码**。文件均位于 `internal/gocloudagent/` 下。

1. **`cli/config.go`**：新增

   ```go
   type SupervisorConfig struct {
       Enabled          bool     `json:"enabled"`            // 默认 false
       Model            string   `json:"model"`              // 如 "zai-org/GLM-5.3"
       BaseURL          string   `json:"base_url,omitempty"` // 可选；为空=同一 SimpleBase endpoint
       ProjectID        string   `json:"project_id,omitempty"`
       Triggers         []string `json:"triggers"`           // ["plan","repeat","length","budget","noop","accept","verify_fail"]
       MaxCalls         int      `json:"max_calls"`          // 默认 6
       MaxOutputTokens  int      `json:"max_output_tokens"`  // 默认 768
       AcceptRounds     int      `json:"accept_rounds"`      // 默认 1
       Escalate         bool     `json:"escalate"`           // cascade，默认 false
       ConsultTool      bool     `json:"consult_tool"`       // advisor 工具，默认 false
       BudgetCNY        float64  `json:"budget_cny"`         // 单任务监督成本上限，0=不限
       PriceInPerM      float64  `json:"price_in_per_m"`     // 用于估算，可选
       PriceOutPerM     float64  `json:"price_out_per_m"`
   }
   // Config 增加: Supervisor SupervisorConfig `json:"supervisor"`
   ```

   `MergeEnv` 增加 `GOCLOUDAGENT_SUPERVISOR_ENABLED / _MODEL / _BASE_URL / _TRIGGERS / _MAX_CALLS / _MAX_OUTPUT_TOKENS / _BUDGET_CNY`。**不新增任何 API key 字段**：token 复用现有 SimpleBase `Token`，配置文件仍要求 0600。服务端（SimpleBase 实例）的 provider 环境变量 `SIMPLEBASE_LLM_PROVIDER_<NAME>_API_KEY` 保管真实 key。CLI flag 可选 `--supervisor-model`。

2. **新文件 `cli/supervisor.go`**：

   ```go
   type Supervisor interface {
       Plan(ctx context.Context, in PlanInput) (Plan, Usage, error)
       Review(ctx context.Context, in ReviewInput) (Guidance, Usage, error)   // 干预 / consult
       Accept(ctx context.Context, in AcceptInput) (Verdict, Usage, error)
   }
   type llmSupervisor struct { model model.ToolCallingChatModel; cfg SupervisorConfig; budget *Budget }
   type SupervisorRunner struct { Worker *SessionRunner; Sup Supervisor; Cfg SupervisorConfig }
   func (r *SupervisorRunner) Run(ctx context.Context, prompt string) (RunResult, error)
   ```

   - `llmSupervisor` 直接用 `agentcore.NewModel(Client, "", cfg.Model)` 调 `Generate`。Plan/Review 不带工具；Accept 可以选择用 `adk.NewChatModelAgent` 加只读工具（read_file/grep/run verify），`MaxIterations` 设为 4–6。
   - `Budget`：记录 `calls`、`prompt/completion tokens` 和估算 ¥。超限后所有触发降级为 no-op，并写 `warn` 事件。

3. **`cli/runner.go`**：
   - `SessionRunner` 增加 `Handlers []adk.ChatModelAgentMiddleware` 和 `InjectQueue`（supervisor 产出的待注入消息）。
   - 新增中间件 `triggerMonitor`：
     - `WrapInvokableToolCall`：统计只读/写工具、shell 退出码、错误哈希（T5/T7），记录到共享 `RunState`。
     - `BeforeModelRewriteState`：如果 `InjectQueue` 非空，向 `state.Messages` 追加一条 `user` 消息 `<supervisor_guidance>…</supervisor_guidance>`。这样不打断 eino 的 Run，在同一会话里生效。为兼容性用 user 而不是 system 角色。
     - 触发时同步调用 `Sup.Review`。worker 是串行执行（`ExecuteSequentially:true`），同步调用不引入并发问题。
   - `repeatDetector` 增加两段阈值：warn → 回调 T2，error → 现有逻辑。
   - `stop` 拦截：`Run()` 拿到 reason=stop 后返回给 `SupervisorRunner`。由外层决定是 Accept 后结束，还是追加修正消息、用同一 Session 续跑（复用 `Session` 历史，相当于新的一轮 user turn）。
   - `RunResult` 增加 `SupervisorUsage Usage`、`SupervisorCalls int`、`Triggers map[string]int`、`Verdict string`、`Escalated bool`。

4. **`cli/repl.go`（`buildRunner` / headless `Run`）**：`cfg.Supervisor.Enabled` 时构造 `SupervisorRunner`，否则保持原路径（零行为变化）。`end` 事件增加 `supervisor_usage{prompt,completion,total}`、`supervisor_model`、`supervisor_calls`、`triggers`、`cost_cny_est`。

5. **`cli/runner.go` 的 `defaultInstruction`**：supervisor 开启时追加：“收到 `<supervisor_guidance>` 时优先执行其中的动作；有改动需求的任务在宣告完成前必须写入文件并运行验证”。`consult_supervisor` 开启时再追加 Anthropic 推荐的时机提示（定位完成后、写第一处代码前、卡住时、宣告完成前）。

6. **`tools/`**：可选 `consult_supervisor` 工具（`ConsultTool=true`）。实现上调 `Sup.Review`，受 `MaxCalls` 共享预算约束。也可以直接用 `adk.NewAgentTool` 包一个无工具的 strong `ChatModelAgent`，但自写工具更容易做预算和摘要控制，推荐自写。

7. **`internal/llmgateway`（跨 provider 时才需要）**：当前 `prepareRequest` 总是 `getClient(ctx, projectID, "")`。
   - **同 provider 方案（零 gateway 改动，推荐 Phase 1 使用）**：监督者选 SiliconFlow 上的 `zai-org/GLM-5.3` 或 `deepseek-ai/DeepSeek-V4-Pro`，加入默认 provider 的 `AllowedModels`，请求体 `model` 覆盖即可。
   - **跨 provider 方案（Phase 4）**：在 `ProviderConfig` 增加 `Models []string`（或全局 `model→provider` 映射），`prepareRequest` 先按 `req.Model` 查 provider，未命中再走默认。key 仍只在服务端环境变量里。需要单测覆盖 allowlist 和 fallback 交互（参考 `model_resolution_test.go`）。
   - `BaseURL` 字段只用于“监督者走另一个 SimpleBase 实例/项目”的场景，鉴权仍是 SimpleBase token，**不允许直连上游 provider**，以免把 key 放到 CLI。

### 3.5 上下文传递（context engineering）

- **轨迹摘要器**（`supervisor.go: summarizeTranscript`）：最近 K=12 步，每步保留 `tool + 参数摘要(≤200B) + 观察首尾(各≤1KB) + exit code`。再加上 `git diff --stat` 与 diff 截断（≤8KB）、plan、触发原因。目标是单次 Review 输入 ≤ 12k tokens。
- 复用已有 compaction 结果作为“远期历史”摘要，避免把全量 transcript（可达数十万 tokens）发给强模型。
- `Redact`：送给 supervisor 的内容同样经过现有 `redact`。
- 前缀稳定：supervisor 的 system prompt 和 plan 固定放在前部，以利用 provider 侧 prompt cache（SiliconFlow GLM-5.3 cache 价 ¥2/M，是否自动命中 **未核实**）。

### 3.6 预算控制

- 硬上限：`max_calls`、每次 `max_output_tokens`、`budget_cny`（按配置价格估算）、强模型 wall-clock 计入 `MaxRunSeconds`。
- 软策略：同一触发 ID 最多 2 次；两次干预之间至少隔 5 步；预算用尽后仅保留 T6 的 computational 验证（跑测试，不调强模型）。
- 上报：`end` 事件分模型记录 token 和估算 ¥，供评测脚本汇总。

---

## 4. 分阶段实施与验收

| Phase | 内容 | 涉及文件 | 验收标准 |
|---|---|---|---|
| **P0 Harness 修复（无强模型）** | ① 显式 `max_tokens`（如 16k，可配），length 时提示分块。② 停止准则：有写需求但无 diff 时拒绝 stop（注入一次提醒）。③ repeatDetector 两段阈值。④ 单命令超时按 suite 可配，超时结构化回传。⑤ 交付前 verify 提示 | runner.go（含 `defaultInstruction`）, repeat_detector.go, tools/tools.go, config.go | 单测通过。重跑 smoke/core 得到**新 weak-only 基线**。TB 不再出现参数 JSON 截断。SWE “无 patch”样本 ≤ 1/10 |
| **P1 Supervisor 骨架 + 验收（T1/T6）** | `SupervisorConfig`，`supervisor.go`（Plan/Accept），`end` 事件扩展，同 provider 强模型 | config.go, supervisor.go, repl.go, runner.go | `enabled=false` 时 jsonl 输出与 P0 逐字段一致（新增字段除外）。`enabled=true` 时 HumanEval smoke 正常完成，`supervisor_usage` 非零，`max_calls` 生效 |
| **P2 干预触发（T2–T5、T7）** | `triggerMonitor` 中间件 + 注入队列 + 摘要器 | runner.go, supervisor.go | 构造的“卡住”测试用例（重复读同一文件 / 只读不写）能触发并注入 guidance。预算超限会降级。触发次数写进 end 事件 |
| **P3 Advisor 工具 + Cascade（T8/T9）** | `consult_supervisor` 工具；S2 escalate | tools/, supervisor.go | 工具调用计入共享预算；cascade 只在 verdict=fail 时发生；评测中 consult 率可统计 |
| **P4 Gateway 跨 provider 路由（可选）** | `model→provider` 映射 | internal/llmgateway/service.go 等 | 单测覆盖：未映射模型走默认 provider；allowlist 仍生效；key 不出现在任何响应或日志中 |
| **P5 评测与默认值决策** | §5 A/B | eval/gocloudagent-bench/* | 产出报告；满足 §5.5 的判定标准才考虑默认开启（即便开启也只在 `--profile` 级别） |

---

## 5. 评测方案

### 5.1 对照组（四臂 + P0 对照）

| 臂 | Worker | Supervisor | 说明 |
|---|---|---|---|
| A0 | GLM-5.3-Flash（PR #51 harness） | — | 历史基线 |
| A1 weak-only | GLM-5.3-Flash（P0 harness） | — | **主对照**，隔离 harness 收益 |
| A2 strong-only | GLM-5.3（或 DeepSeek-V4-Pro） | — | 上限/成本参照 |
| A3 supervised | GLM-5.3-Flash | GLM-5.3（T1–T7） | 本方案 |
| A3' supervised+consult | 同上 + consult 工具 | | 观察工具式 advisor 的增量 |
| A4 cascade | A1 失败题 → A2 重跑 | | FrugalGPT/“失败后重跑”对照 |
| A5 self-supervised | Flash | Flash（同模型充当 supervisor） | 隔离“流程拆分”收益（参考 Aider 自配对） |

### 5.2 数据集

使用现有 `eval/gocloudagent-bench/`：
- HumanEval 164（run_core.py）
- MBPP sanitized 427（run_mbpp.py）
- Terminal-Bench 8 题（run_terminal_bench.py / harbor_agent.py），资源允许时扩到更多题
- SWE-bench Verified 前 10 → 扩到 50（run_swebench.py）

HumanEval/MBPP 天花板较高，主要看成本和延迟。TB/SWE 是主要信号。

### 5.3 指标

- pass rate（以及 SWE 的 resolved、无 patch 数）
- tokens 按模型分开：worker prompt/completion/reasoning，supervisor prompt/completion
- ¥ 成本：按 SiliconFlow 价目（Flash ¥0.80/¥2.80/M，cache ¥0.23；GLM-5.3 ¥8/¥28/M，cache ¥2；DeepSeek-V4-Pro ¥12/¥24/M；Kimi-K2.7-Code ¥6.5/¥27/M，取自 https://siliconflow.cn/pricing ，2026-10-18 抓取）
- 端到端延迟 p50/p95，以及超时率
- supervisor 调用次数/题、各触发器命中率、consult 率、Accept 判定的准确率（与真实测试结果对照：假通过率 / 假拒绝率）
- 每题“被救回”（A1 失败、A3 通过）与“被搞坏”（A1 通过、A3 失败）的数量

### 5.4 成本粗估（说明性，非测量值）

假设每题 3 次 supervisor 调用，每次输入 10k、输出 0.7k tokens，用 GLM-5.3：每题约 30k×¥8/M + 2.1k×¥28/M ≈ ¥0.24 + ¥0.06 = **¥0.30/题**。作为对照，smoke 中 SWE 10 题 Flash 总成本约 ¥0.63（约 ¥0.06/题）。也就是说，**SWE 类任务开启 supervisor 会让单题成本变成原来的数倍**，必须用通过率增量来换。HumanEval/MBPP 类短任务应只开 T6 或完全不开。以上都是估算，以实测为准。

### 5.5 判定标准（建议）

- 在 TB+SWE 上，A3 相对 A1 通过率提升 ≥ 2 题（或 ≥ 10pp），且“被搞坏”数 ≤ 1；
- A3 成本 ≤ A2 的 50%，且通过率 ≥ A2 的 90%（即“接近强模型、明显更便宜”）；
- 如果 A5（同模型自监督）与 A3 差距在噪声内，结论改为“流程拆分有效，强模型不必要”；
- 如果 A4 cascade 的成本/效果优于 A3，优先推荐 cascade（实现更简单）；
- 每臂至少跑 2 次（TB/SWE 方差大），报告区间。

---

## 6. 风险与开放问题

| # | 风险/问题 | 说明 | 缓解 |
|---|---|---|---|
| 1 | 收益归因混淆 | P0 harness 修复可能吃掉大部分收益 | 强制先有 A1 新基线；加 A5 对照 |
| 2 | 弱模型不遵从 guidance | Flash 可能忽略注入的建议（与 R1 同类） | guidance 用祈使句和具体动作；T5/T6 以硬拦截（拒绝 stop）兜底；统计“建议后的下一步是否执行” |
| 3 | 强模型验收假阳性/假阴性 | inferential judge 不可靠 | computational 优先（先跑测试）；Accept 带只读工具；记录与真实测试对照的混淆矩阵 |
| 4 | 成本/延迟放大 | 强模型价格约 10x；同步调用会增加延迟 | `max_calls`、`budget_cny`、防抖；短任务 profile 关闭干预 |
| 5 | Gateway 单 provider 限制 | 跨 provider 需改 `prepareRequest` | Phase 1 用同 provider 的强模型；P4 再做路由 |
| 6 | 密钥安全 | 不能让 CLI 直连上游 | 只经 SimpleBase gateway；配置不含 key；`base_url` 只允许 SimpleBase endpoint |
| 7 | Benchmark 泄漏/过拟合 | 触发规则可能对 8 题 TB 过拟合 | 扩大 TB/SWE 样本；规则保持通用，不针对题目 |
| 8 | eino 中间件行为 | `BeforeModelRewriteState` 注入消息与 compaction、Session 持久化的交互 **未实测** | P2 先写单测验证消息顺序和持久化 |
| 9 | 上游缓存命中 | SiliconFlow 对 GLM-5.3 是否自动 prompt cache，以及计费口径 **未核实** | 评测里单独记录 cache tokens（若网关返回） |
| 10 | 推荐强模型选择 | GLM-5.3 / DeepSeek-V4-Pro / Kimi-K2.7-Code 在 gocloudagent 任务上的强弱 **未测** | A2 阶段做小规模 strong-only 比较，选性价比最高者 |
| 11 | 多 worker 并行 | 本方案不做 | 依据 2/2b：coding 并行性弱、工具密集任务有开销；如需要另立方案 |
| 12 | 外部数据的时效 | Anthropic/OpenAI 数据基于其自家模型，迁移到 GLM 系列不一定成立 | 以本地 A/B 为准 |

---

## 附录 A：链接核实记录（2026-10-18）

- curl 返回 HTTP 200：表中除 openai.com 两条外的所有链接。arXiv 条目已核对标题与摘要（2305.05176、2406.18665、2310.12963、2407.21787、2312.09390、2510.04371、2410.10934、2503.09572、2506.11578、2507.23370、2407.01489、2405.15793、2501.14723、2502.14382、2512.08296、2305.20050、2408.03314）。
- openai.com/index/harness-engineering、openai.com/index/unrolling-the-codex-agent-loop：curl 403（站点屏蔽），WebSearch 返回了对应内容和日期（2026-02-11 / 2026-01-23）。
- docs.cursor.com/agent/planning、cognition.ai/blog/devin-2、developers.openai.com/codex/subagents：可访问，正文未细读，细节标为未核实。
- DeepSWE（arXiv 2607.07946）：标题已核实。“+23pt”数据转引自 Anthropic 文档，未在论文正文中核对，记为“转引”。

## 附录 B：已读的仓库文件

`internal/gocloudagent/cli/{runner,config,repl,repeat_detector,compaction,client,prompt}.go`、`internal/gocloudagent/agentcore/{model,types,stream}.go`、`internal/gocloudagent/eino-readme.md`、`internal/llmgateway/{service,fallback_resolver,model_resolution_test}.go`、`plan/planv6.1/{gocloudagent-agent-benchmark-plan,gocloudagent-optimization-plan,gocloudagent-smoke-report}.md`。
