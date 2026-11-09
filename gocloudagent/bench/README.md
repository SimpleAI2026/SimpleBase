# gocloudagent 评测（bench）

gocloudagent 的 agent 压测 harness。放在 `gocloudagent/bench/` 下，与它评测的代码在同一个模块目录里；与 Go 代码解耦，只用 Python 标准库，不进 `go.mod`。

冒烟层是 L0–L3 和 HumanEval 前 10 题。已跑过的核心层是 HumanEval 全量 164 题，以及 Terminal-Bench v4.0.0 里固定的 8 题。MBPP sanitized 全量在 `run_mbpp.py`。SWE-bench Verified 的固定 50 题在 `run_swebench.py`：推理在宿主机检出目录，评分用单独 venv 里的 `swebench==5.0.2`（见 `requirements-swe.txt`），不把 gocloudagent 装进评测镜像。

## 目录

| 文件 | 作用 |
| --- | --- |
| `run_smoke.py` | 冒烟层：编译、探测、L0–L3、HumanEval 前 10；其它脚本都 `import run_smoke as smoke` 复用它的隔离与评分 |
| `run_core.py` | 核心层：HumanEval 全量 164 |
| `run_mbpp.py` | MBPP sanitized 全量 |
| `run_swebench.py` | SWE-bench Verified 固定 50 题 |
| `run_terminal_bench.py` + `harbor_agent.py` | Terminal-Bench（Harbor 0.24.0 驱动） |
| `tasks/{smoke,core}/` | 题面与 instance_id 清单 |

## 运行

在仓库根执行（`run_smoke.py` 会自己向上定位仓库根，用于构建两个二进制与读 `config.example.yaml`）：

```bash
python3 gocloudagent/bench/run_smoke.py --self-check
python3 gocloudagent/bench/run_smoke.py
python3 gocloudagent/bench/run_core.py --self-check
python3 gocloudagent/bench/run_core.py
python3 gocloudagent/bench/run_mbpp.py --self-check
python3 gocloudagent/bench/run_swebench.py --self-check
python3 gocloudagent/bench/run_swebench.py --gold-one
```

`--self-check` 只校验夹具与依赖，不需要密钥；其余模式需要真实模型与 Docker/USERNS 环境。

运行目录默认是评测机上的 `/home/ubuntu/gocloudagent-bench`。在别的机器上跑（例如 macOS 本地只想做自检）时用 `GOCLOUDAGENT_BENCH_RUN_ROOT` 指过去：

```bash
GOCLOUDAGENT_BENCH_RUN_ROOT=/tmp/sbbench python3 gocloudagent/bench/run_smoke.py --self-check
```

## 构建口径

`gocloudagent` 是嵌套模块（`gocloudagent/go.mod`），`simplebase` 在根模块，所以 `compile_bins()` 分别用各自的模块目录作 `cwd`：

- `simplebase`：`cwd=<repo>`，`go build ./cmd/simplebase`
- `gocloudagent`：`cwd=<repo>/gocloudagent`，`go build ./cmd/gocloudagent`

仓库根用「含 `go.mod` 且模块名为 `github.com/linkxzhou/SimpleBase`，同时有 `config.example.yaml`」的目录定位，不依赖 `parents[N]`，这样目录再移动也不用改索引。

HumanEval 全量读 `tasks/core/humaneval.jsonl`（没有标准答案）。Terminal-Bench 用 Harbor 0.24.0，见 `requirements-core.txt` 和 `run_terminal_bench.py`。任务镜像若低于 glibc 2.38，adapter 会带上宿主机的 `ld-linux` 和 `libm` 再启动已编译的 `gocloudagent`，不在容器里编译。

供应商密钥从进程环境读取（`SIMPLEBASE_LLM_PROVIDER_API_KEY` 等）。脚本只检查是否非空，不打印、不写入仓库。结果写到 `plan/planv6.1/gocloudagent-smoke-report.md`。

带 `--yolo` 的 `gocloudagent` 跑在 user namespace 里：根文件系统只读，只有题目目录、会话目录和 `/tmp` 可写。运行目录是 `/home/ubuntu/gocloudagent-bench`（不在 `/tmp` 下，否则盖住 `/tmp` 时二进制和题目会一起消失）。L3 的哨兵在只读挂载上。进程没有真正启动时，哨兵仍在也不算通过。
