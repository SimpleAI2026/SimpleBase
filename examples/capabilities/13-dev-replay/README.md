# 本地→云端重放：`simplebase dev` 全流程

把一份**最小可重放工作区**（1 个迁移 + 1 个集合种子 + 1 个云函数 + 1 个定时任务）重放到当前实例，
验证 plan/apply 的幂等性：

1. `dev validate`：离线校验清单、迁移、函数（不访问远端）；
2. `dev plan --seed`：只读列出将要执行的变更，不写任何东西；
3. `dev apply --seed --yes`：创建数据库、跑迁移、建函数（先试跑再激活）、建定时任务；
4. 再次 `dev apply --seed --yes`：**全部 skip、changes=0**；
5. `dev status`：lock 里记录当前目标；
6. CLI 读回数据库 / 函数 / 定时任务，确认资源存在；
7. 删除本次资源，清理临时工作区。

工作区取自本目录 `workspace/`，复制到 `mktemp -d` 后替换占位符（`__PROJECT_ID__` / `__SUFFIX__`）。
`.simplebase/lock.json` 与环境提示文件只落在临时副本里，跑完删除，不污染仓库。

## 前置

- 服务端已启动，`SIMPLEBASE_BIN` / `SIMPLEBASE_URL` / `SIMPLEBASE_TOKEN` / `SIMPLEBASE_PROJECT_ID` 已导出。
- 目标项目可以还没有用户数据库（dev apply 会创建 `devmain-<suffix>`）；项目已有 `devmain-*`
  残留库时，用例先清掉它们（每个项目只允许一个用户数据库）。
- 目标项目已有其他名字的用户库时 dev apply 会报 `database_already_exists`；这是项目单文件
  模型的限制，跑本用例前应确保前面的用例已清理自己的库。
- `skills/simplebase/scripts/sbdev.py` 存在时，用例会额外跑一次 `sbdev.py validate` 做口径对账
  （同一份清单、同一组 fixture，计划里说明两端校验结论应一致）。

## 运行

```bash
export CASE=13-dev-replay
./examples/capabilities/13-dev-replay/run.sh
```

## 命令

```bash
# 离线
simplebase dev validate --dir "$WORK"
python3 skills/simplebase/scripts/sbdev.py validate -C "$WORK"   # 可选对账

# 只读计划 / 幂等重放
simplebase dev plan   --dir "$WORK" --seed
simplebase dev apply  --dir "$WORK" --seed --yes
simplebase dev apply  --dir "$WORK" --seed --yes   # 第二次：全部 skip
simplebase dev status --dir "$WORK"

# 读回与清理
simplebase database list
simplebase function list
simplebase cron list
simplebase database delete --id "$DBID"
simplebase function delete --name "$FNAME"
simplebase cron delete --id "$CRONID"
```

`dev apply` 在远端目标（URL 主机不是 loopback）且不带 `--yes` 时只打印计划并以退出码 3 结束；
本用例显式带 `--yes`。连接信息只从 flag / 环境变量 / `simplebase config` 读取，不读工作区 `.env`。

## 断言点

- `dev validate`：`errors` 为空；计数为 databases=1、migrations=1、collections=1、functions=1、cron_jobs=1。
- `dev plan`：`mode=plan`，动作种类覆盖 database/migration/collection/document/function/cron，`seed=true`。
- 第一次 `dev apply`：`mode=apply`、`changes>=1`。
- 第二次 `dev apply`：`changes=0`、`skips` 等于动作数、所有动作 `verb=skip`。
- `dev status`：`current` 非空，lock 至少有一个 target。
- 资源读回：`devmain-<suffix>` 库、`devfn<suffix>` 函数、`devcron-<suffix>` 定时任务各存在 1 个。
- 清理后三类资源均删除成功。

## 已知边界

- 清单带一个 collection seed（`notes/welcome`）：服务端建文档时忽略清单里的 `id`、总是生成 UUID，
  所以 dev 重放不按文档 id 查重，而是在 `_migrations` 记一行 `seed-doc:<集合>/<id>`；第二次
  `--seed` 因此全 skip。
- `_migrations` 记账表属于用户库：删库时随库一起清空，所以同一项目重跑本用例不会被旧标记影响。
  断言仍只要求「第二次 changes=0 全 skip」。
- `dev apply --seed` 会写工作区 `.simplebase/lock.json`、`.simplebase/env.<label>`；本用例在临时
  副本里执行并整体删除，仓库目录保持干净。
- 本用例结束时删除用户数据库：只清空表、视图与集合，项目级 KV 与备份保留，不影响后续用例。
