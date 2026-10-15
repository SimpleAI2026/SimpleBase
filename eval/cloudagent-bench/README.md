# cloudagent 冒烟

只覆盖计划里的冒烟层：编译两个二进制、探测模型出口、SimpleBase 自定义题 L0–L3、HumanEval 前 10 题。不跑 Terminal-Bench 和 SWE-bench。

```bash
python3 eval/cloudagent-bench/run_smoke.py --self-check
python3 eval/cloudagent-bench/run_smoke.py
```

供应商密钥从进程环境读取（`SIMPLEBASE_LLM_PROVIDER_API_KEY` 等）。脚本只检查是否非空，不打印、不写入仓库。结果写到 `plan/planv6.1/cloudagent-smoke-report.md`。

带 `--yolo` 的 `cloudagent` 跑在 user namespace 里：根文件系统只读，只有题目目录、会话目录和 `/tmp` 可写。运行目录是 `/home/ubuntu/cloudagent-bench`（不在 `/tmp` 下，否则盖住 `/tmp` 时二进制和题目会一起消失）。L3 的哨兵在只读挂载上。进程没有真正启动时，哨兵仍在也不算通过。
