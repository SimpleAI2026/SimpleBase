# 预期结果

| 步骤 | 通过条件 |
|---|---|
| validate | `errors` 为空；databases=1、migrations=1、collections=0、functions=1、cron_jobs=1 |
| skill-validate（可选） | `sbdev.py validate -C <work>` 退出码 0；脚本不存在时打印 SKIP |
| plan | `mode=plan`、`seed=true`，动作种类覆盖 database/migration/function/cron |
| apply | `mode=apply`、`changes>=1` |
| apply-idempotent | `changes=0`、`skips == 动作数`、所有动作 `verb=skip` |
| status | `current` 非空，`keys`/`targets` 至少各 1 项 |
| resource-db | 数据库列表里 `devmain-<suffix>` 恰好 1 个 |
| resource-function | 函数列表里 `devfn<suffix>` 恰好 1 个 |
| resource-cron | 定时任务列表里 `devcron-<suffix>` 恰好 1 个 |
| delete-db / delete-function / delete-cron | 三类资源删除成功 |
| 工作区 | 临时副本被删除，仓库内不产生 `.simplebase/` |

> 同一项目重复跑本用例也应为 PASS：每次随机后缀生成新名字；旧 `devmain-*` 残留会被先清掉。
> 远端实例上第一次 `dev apply` 额外要求 `--yes`（本用例已带）；不带时退出码为 3。
