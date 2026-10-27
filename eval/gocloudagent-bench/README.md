# gocloudagent 评测

冒烟层是 L0–L3 和 HumanEval 前 10 题。已跑过的核心层是 HumanEval 全量 164 题，以及 Terminal-Bench v4.0.0 里固定的 8 题。MBPP sanitized 全量在 `run_mbpp.py`。SWE-bench Verified 的固定 50 题在 `run_swebench.py`：推理在宿主机检出目录，评分用单独 venv 里的 `swebench==5.0.2`（见 `requirements-swe.txt`），不把 gocloudagent 装进评测镜像。

```bash
python3 eval/gocloudagent-bench/run_smoke.py --self-check
python3 eval/gocloudagent-bench/run_smoke.py
python3 eval/gocloudagent-bench/run_core.py --self-check
python3 eval/gocloudagent-bench/run_core.py
python3 eval/gocloudagent-bench/run_mbpp.py --self-check
python3 eval/gocloudagent-bench/run_swebench.py --self-check
python3 eval/gocloudagent-bench/run_swebench.py --gold-one
```

HumanEval 全量读 `tasks/core/humaneval.jsonl`（没有标准答案）。Terminal-Bench 用 Harbor 0.24.0，见 `requirements-core.txt` 和 `run_terminal_bench.py`。任务镜像若低于 glibc 2.38，adapter 会带上宿主机的 `ld-linux` 和 `libm` 再启动已编译的 `gocloudagent`，不在容器里编译。

供应商密钥从进程环境读取（`SIMPLEBASE_LLM_PROVIDER_API_KEY` 等）。脚本只检查是否非空，不打印、不写入仓库。结果写到 `plan/planv6.1/gocloudagent-smoke-report.md`。

带 `--yolo` 的 `gocloudagent` 跑在 user namespace 里：根文件系统只读，只有题目目录、会话目录和 `/tmp` 可写。运行目录是 `/home/ubuntu/gocloudagent-bench`（不在 `/tmp` 下，否则盖住 `/tmp` 时二进制和题目会一起消失）。L3 的哨兵在只读挂载上。进程没有真正启动时，哨兵仍在也不算通过。
