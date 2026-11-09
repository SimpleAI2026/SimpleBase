# 存储本地优先 + 路径扁平化 + 去 DEV_MODE

> **仓库**：SimpleBase
> **日期**：2026-10-11
> **核对基线**：`main` @ `5a03d47` + 工作区未提交改动（含今日 `build.sh` 健康等待 150s、`seedOnce` 分阶段日志）
> **约束**：[`internal/AGENTS.md`](../../internal/AGENTS.md)；单写单进程（副本数固定 1）不变
> **触发**：开发启动时系统库 seed 阶段静默 46s。原因是 COS 远端 DATA_PATH 上积累了 1638 个小文件，每次首查都要逐个拉取（读放大）。现在可以直接挂载磁盘，DB 不再需要依赖对象存储

## 0. 需求与结论

| # | 需求 | 现状核对 | 结论 |
| --- | --- | --- | --- |
| 1 | `internal/database` 要能用环境变量配置本地目录和缓存目录 | 只有 `database.cache_dir` / `SIMPLEBASE_DB_CACHE_DIR` 一个根目录。系统库目录 `{cache}/system/dbs`、开发用户库目录 `{cache}/dev/dbs`、开发文件目录 `{cache}/dev/files` 都写死在 `app.go` 里。**没有「持久数据目录」这个概念** | 新增 `storage.data_dir`（`SIMPLEBASE_DATA_DIR`），`cache_dir` 只放可丢弃的内容 |
| 2 | 路径改成 `{项目ID}/files`、`{项目ID}/databases/xxx` | 现在的 key 是 `{bucket}/{s3.prefix}/{instance.id}/tenants/{tenantUUID}/databases/{dbUUID}/...`，文件是 `{s3.prefix}/{instance.id}/files/{projectID}/{key}` | 去掉 `prefix`、`instance.id`、`tenants/{uuid}` 三层（bucket 是 S3 本身的概念，无法去掉；`prefix` 保留为可选项，默认空）。本地盘和 S3 用同一套相对布局 |
| 3 | DB 默认用本地盘，S3/COS 同步默认关闭 | 现在 `dev_mode=false` 且配置了 bucket，就会**强制**走远端 DATA_PATH 加 catalog 同步 | 新增 `storage.database: local \| s3`，默认 `local`；`catalog_sync` / `lease` / CAS 探针只在 `s3` 模式下启用 |
| 4 | `config.yaml` 加详细注释，并写明对应的代码功能 | 现有注释只说明了分组 | 重写 `config.example.yaml`（`config.yaml` 由它生成），每个键注明：作用、默认值、环境变量、使用它的代码位置、生效条件 |
| 5 | UI 增加 DB 存储模式设置，仅管理员可改；清理 DEV_MODE | `dev_mode` 涉及 21 个 Go 文件、`build.sh`、5 份文档，UI 写死了 `DEFAULT_PROJECT_ID='dev-shop'` | 设置弹窗新增「存储」页签：`superadminl1` 可修改（写回配置文件），`admin` 只读，`user` 看不到。**`dev_mode` 彻底删除，不留兼容代码** |

**已确认的决策（2026-10-11 11:12）**：

1. `dev_mode` / `SIMPLEBASE_DEV_MODE` **彻底删除**：不保留告警，也不做兼容分支，代码、脚本、文档、测试里都不再出现。
2. **切换存储模式时不迁移数据，直接执行 `./build.sh reset` 清空**，然后重新启动并重新种子。COS 上现有的旧布局数据（系统库和用户库）**不再迁移，直接废弃**，原阶段 C（`storage migrate`）取消。

**P0 风险（必须和需求 3 一起修，不能拆开做）**：`cache.Manager.Evict` 会按 LRU **直接 `removeAll` 缓存根目录下 UUID 命名的子目录**（`internal/database/cache/evict.go:116`）。远端模式下这些目录可以从 S3 重建，所以删掉没关系。但改成本地模式后，它们就是**唯一副本**。如果数据目录仍然放在 `cache_dir` 下，淘汰一次就会丢数据。所以数据目录和缓存目录**必须物理分开**，`cache.Manager` 的根目录绝不能指向 `data_dir`。

## 1. 目标形态

### 1.1 目录与 key 布局（本地盘和 S3 共用）

```
{root}/                                   # 本地：storage.data_dir；S3：s3://{bucket}/{s3.prefix?}
  _system/                                # 平台级对象（项目 ID 固定 8 位 [A-Za-z0-9-]，不会和下划线开头的名字冲突）
    instance-format.json                  # 原 {base}/catalog/instance-format.json
    reset.lock                            # 原 {base}/catalog/reset.lock
    cas-probe/...                         # 原 CatalogPrefix() 下的 CAS 探针
    identity.json                         # 原 {cache}/system/instance-identity.json，改为记录存储身份
  {projectID}/
    databases/{databaseUUID}/
      descriptor.json
      catalog/catalog.ducklake(.wal)      # 本地模式：活文件；S3 模式：snapshots/ + manifest/ + lease/
      data/...parquet                     # DuckLake DATA_PATH
      orphans/{date}/{uuid}.json          # 仅 S3 模式
    files/{userKey}                       # 用户文件（原 files/{projectID}/{key}）
```

- 系统库属于 `sb-admin` 项目，所以路径是 `sb-admin/databases/{sysDBUUID}`。
- 路径里不再有 `tenantID`：现在只有一个保留租户 `00000000-...-0001`，留着它只会多一层目录。`catalog.Database.TenantID` 字段本身保留，只是不再参与拼路径。
- `{cache_dir}` 只放可丢弃的内容：每个库的 `tmp/`（DuckDB `temp_directory`）、`sync-staging/`（S3 模式）、远端 catalog 的本地工作副本（S3 模式）、扩展缓存。

### 1.2 配置形态

```yaml
storage:
  database: local          # local | s3     SIMPLEBASE_STORAGE_DATABASE
  files: local             # local | s3     SIMPLEBASE_STORAGE_FILES
  data_dir: "./data"       # 本地持久根     SIMPLEBASE_DATA_DIR
database:
  cache_dir: ".cache"      # 可丢弃缓存     SIMPLEBASE_DB_CACHE_DIR（已有）
s3:
  prefix: ""               # 可选；默认空，即 bucket 根目录
```

- 删除 `dev_mode` / `SIMPLEBASE_DEV_MODE`，不留任何读取或告警代码。`unmarshalYAML` 用的是非严格解析，旧 `config.yaml` 里残留的 `dev_mode:` 行会被直接忽略，不会导致启动失败；本地的 `config.yaml` 在阶段 F 按新模板重新生成，这一行随之消失。
- `instance.id` 只用于日志、指标、租约 owner，不再参与拼路径。

### 1.3 切换存储模式的流程

```
UI「设置 → 存储」改模式（或手工改 config.yaml / .env）
  → Ctrl+C 停止服务
  → ./build.sh reset        # 清空 data_dir + cache_dir（加 --remote 时同时清空 S3 新布局）
  → ./build.sh dev          # 用新模式建空库并重新种子
```

- 数据目录 `{data_dir}/_system/identity.json` 记录当前的 `{storage.database, storage.files, bucket, prefix}`。启动时如果发现和配置不一致，**拒绝启动**，并提示「存储模式已变更，请先执行 `./build.sh reset`」，防止新模式在旧数据上运行。
- 本地数据目录为空、且远端没有 `_system/identity.json` 时，视为全新实例，直接写入身份文件。

## 2. 分阶段实施

### 阶段 A：配置模型与目录分离（需求 1、3 的基础）

| 文件 | 改动 |
| --- | --- |
| `internal/config/config.go` | 新增 `StorageConfig{Database, Files, DataDir}`，默认 `local/local/"./data"`。`Validate`：模式只能是 `local`/`s3`；**只有** `storage.database=s3` 或 `storage.files=s3` 时才要求 `s3.bucket/region`（原来的条件是 `Writable && !DevMode`）；`data_dir` 和 `cache_dir` 不能相同，也不能互为父子目录（防止 P0 误删）。`Redacted()` 输出 storage |
| `internal/config/env.go` | 新增 `SIMPLEBASE_STORAGE_DATABASE`、`SIMPLEBASE_STORAGE_FILES`、`SIMPLEBASE_DATA_DIR`；删除 `SIMPLEBASE_DEV_MODE` 的映射 |
| `internal/config/yaml.go` | `yamlConfig` 新增 `storage` 块；删除 `DevMode` 字段及 `setBool(&cfg.DevMode, …)`。`rejectYAMLSecrets(devMode,…)` 去掉参数，改为**无条件拒绝** YAML 里的 S3 AK/SK 和 `auth.api_key_hash_secret`，这些只能来自环境变量或 `.env`（见阶段 D 的 build.sh） |
| `internal/app/app.go` `assembleDeps` | 所有 `cfg.DevMode` 分支改为按 `cfg.Storage.Database == "s3"`（用 `dbRemote` 表示）或 `cfg.Storage.Files == "s3"`（用 `filesRemote` 表示）判断：objectstore client、CAS 探针、lease、catalog 引擎远端校验只在 `dbRemote` 时启用；S3 FileStore 只在 `filesRemote` 时启用，否则用 `NewLocalFileStore(data_dir)` |
| `internal/app/app.go` 工厂 | 新增 `ducklake.Factory.DataRoot`（持久根）。本地模式下 `catalog/` 和 `data/` 放在 `DataRoot` 下，`tmp/` 放在 `CacheDir` 下；S3 模式下 catalog 工作副本仍在 `CacheDir`（可从远端重建）。系统库和用户库共用同一个 `DataRoot`，按 `{projectID}/databases/{id}` 区分，不再需要 `{cache}/system/dbs` 和 `{cache}/dev/dbs` 两套目录 |
| `internal/database/cache` | `Manager.Root` 固定为 `cache_dir`，只统计和淘汰 `tmp/` 与 S3 模式的工作副本。本地模式下 `Evict` 跳过所有含活 catalog 的目录，加断言：`Root` 不能落在 `data_dir` 内 |
| `internal/app/identity.go` | 身份文件改为 `{data_dir}/_system/identity.json`，记录 `{storage.database, storage.files, bucket, prefix}`，取代原来的 `{instance.id, s3.prefix}`。存储身份变化（例如 s3→local，或换了 bucket）时拒绝启动，错误信息里直接给出 `./build.sh reset` 命令（见 §1.3） |

**验收**：在全新目录下，零配置执行 `go run ./cmd/simplebased`，可以启动（不需要 S3），`./data/sb-admin/databases/{id}/catalog` 生成；`.cache` 删掉后重启，数据不丢；修改 `storage.database` 后不执行 reset 直接启动会被拒绝，并提示 reset 命令；单测覆盖 `data_dir` 和 `cache_dir` 重叠时拒绝启动。

### 阶段 B：路径扁平化（需求 2）

| 文件 | 改动 |
| --- | --- |
| `internal/objectstore/keys.go` | `KeyBuilder{RootPrefix}`，删掉 `Environment` 字段。所有方法的签名从 `(tenantID, databaseID)` 改为 `(projectID, databaseID)`，用 `ValidateProjectID` + `validateID` 校验。新增 `FilesPrefix(projectID)`、`SystemKey(name)`。`CatalogPrefix`/`InstanceFormatKey`/`ResetLockKey` 改到 `_system/`。删除 `TenantsPrefix`，reset 改为按项目枚举（见下） |
| `internal/database/ducklake/factory.go` | `layoutForEngine(root, projectID, dbID, engine)`；`dataPathFor` 在本地模式下返回 `{DataRoot}/{project}/databases/{id}/data/`，S3 模式下返回 `s3://{bucket}/{prefix?}/{project}/databases/{id}/data/` |
| `internal/database/ducklake/{remote,catalog_syncer,manifest,orphan_ledger,local_state,instance_format}.go` | 去掉 `Environment` 与 `tenantID` 入参，改用 `meta.ProjectID`。`RemoteStorage.keyBuilder()` 不再默认补 `"simplebase"` 或 `"dev"` |
| `internal/systemdb/bootstrap.go` | `DataPrefix(loc.TenantID,…)` 改为 `DatabasePrefix(ReservedSystemProjectID, …)` |
| `internal/api/s3_handler.go` 与 `objectstore/filestore.go`、`localfilestore.go` | 物理 key 从 `{base}/files/{project}/{key}` 改为 `{prefix?}/{project}/files/{key}`。由 handler 拼出 `project + "/files/" + key`，FileStore 的 base prefix 只用 `s3.prefix`；本地 FileStore 的根目录是 `data_dir`。`ValidateFileKey` 和防穿越校验保持不变 |
| `internal/app/reset.go` | 只认新布局。远端清理范围：`_system/` 加上系统库中登记的每个 `{project}/`（不能再用整个前缀一次删掉，因为 bucket 根目录可能和其他业务共用）。本地清理范围：`data_dir` + `cache_dir`。`--dry-run` 列出全部待删对象。旧布局（`{prefix}/{instance.id}/tenants/...`）不再处理，由人工在 COS 控制台删除（见阶段 C） |
| 全部 `*_test.go` | 断言的期望 key 全部换成新布局（`keys_test.go`、`client_test.go`、`manifest*_test.go`、`catalog_syncer_test.go` 等） |

**验收**：`grep -rn 'tenants/' internal --include='*.go'` 无结果；`KeyBuilder` 单测覆盖项目 ID 非法、路径穿越、`_system` 不能作为项目 ID。

### 阶段 C：`./build.sh reset` 作为唯一的切换手段（替代原数据迁移）

**不做数据迁移**。切换存储模式、首次升级到新布局，统一用 reset 清空后重建。

`build.sh` `cmd_reset` 的改动：

| 项 | 现状 | 改为 |
| --- | --- | --- |
| 清理范围 | 只删 `database.cache_dir` 整目录 | 删除 `storage.data_dir` + `database.cache_dir`。两个路径都按 `环境变量 > config.yaml > 默认值` 解析（新增 `resolve_data_dir`，和 `resolve_cache_dir` 同构）；两个目录都要经过 `assert_safe_cache_dir` 同级的安全检查（不能是 `/`、`$HOME`、仓库根目录，也不能包含 `config.yaml` / `.env`） |
| 远端 | 不处理，提示改用 `./simplebased reset --confirm=<instance_id>` | 新增 `--remote` 选项：只有在 `storage.database=s3` 或 `storage.files=s3` 时才允许使用，内部执行 `go run ./cmd/simplebased reset --confirm=<instance.id>`（按新布局清理，见阶段 B 的 `reset.go`）；不加 `--remote` 时只清本地，并打印提示「远端数据保留」 |
| 占用检查 | 只检查 `cache_dir` | 两个目录都用 `lsof +D` 检查，有进程占用时拒绝执行 |
| 确认 | 输入 yes / `-y` | 不变；确认前列出两个目录的路径、大小、当前存储模式，加 `--remote` 时再列出 bucket/prefix |
| 帮助文本 | 说明只删除缓存 | 更新为「切换存储模式或升级布局时使用；会清空全部数据」 |

`cmd/simplebased reset`（`internal/app/reset.go`）的改动：
- 删除 `dev_mode=false` 时必须设置 `SIMPLEBASE_ALLOW_RESET=1` 的门槛。改为：清理远端时要求设置 `SIMPLEBASE_ALLOW_RESET=1` 并传入 `--confirm=<instance_id>`；纯本地清理只要求 `--confirm`。
- `--local-only` 保留，语义为「只清 `data_dir` + `cache_dir`」。

**当前实例的一次性操作**（按本计划上线后执行）：

1. 停掉服务，执行 `./build.sh reset -y`，清空本地 `.cache`（原来的系统库本地副本和用户库缓存）。
2. 用新模板重新生成 `config.yaml`（`storage.database: local`，`s3` 段保留，供文件存储以后切回 s3 时使用）。
3. 执行 `./build.sh dev`，重新种子出超管 `simplebase2026`。项目、Key、云助手需要重新创建。
4. COS 上的旧布局 `simplebase-1256235179/simplebase/simplebase-dev-1/` 整个前缀由人工在控制台删除（约 1638 个数据文件、18 个快照），代码不负责处理。

**验收**：`./build.sh reset -y` 后 `data_dir` 和 `cache_dir` 都不存在；随后 `./build.sh dev` 能在 30s 内就绪（本地模式），`system database ready` 到 `http server listening` 的间隔 < 3s（原来是 46s）；修改 `storage.database` 后不 reset 直接启动会被身份检查拒绝。

### 阶段 D：清理 DEV_MODE（需求 5 的后端部分）

`dev_mode` 的引用分布（`_test` 除外）：`config` 23 处、`app.go` 13 处、`seed.go` 6 处、`perfbench/harness.go` 6 处、`reset.go` 4 处、`catalog_engine.go` 2 处，另有若干处只在注释里出现。各处处理方式：

| 位置 | 原语义 | 处理 |
| --- | --- | --- |
| `app.go` S3 / lease / 探针分支 | 开发模式绕过 S3 | 改为按 `storage.database` 判断（阶段 A） |
| `app.go:227` `sandbox.backend=fake && DevMode` | fake 沙盒只在开发模式可用 | 改为 `backend=fake` 需要 `SIMPLEBASE_SANDBOX_ALLOW_FAKE=1`（只给 e2e 和单测用）；`validateSandbox` 同步修改 |
| `systemdb/seed.go` DevMode 种子 | 种子项目 `dev-shop`、**固定明文 Key `sb_live_dev_key_12345`**、KV 库、配额、内置助手 | **删除固定明文 Key**（安全隐患）。`dev-shop` 种子改为可选配置 `bootstrap.demo_project: false`，默认不创建；内置助手种子改为在项目创建时触发（现有 `SeedDefaultCloudAgentsWithSandbox` 已经支持按项目播种） |
| `catalog/reserved.go` `DevProjectID`/`DevAPIKeyID` | 种子 ID | `DevAPIKeyID` 删除；`DevProjectID` 只在开启 demo 时使用，注释同步修改 |
| `api/perfbench/harness.go` | 依赖种子 Key | 压测启动后改为通过超管登录调 API 创建项目和 Key（走真实链路），不再依赖种子 |
| `reset.go` | `dev_mode=false` 时必须设置 `SIMPLEBASE_ALLOW_RESET=1` | 见阶段 C：只有清理远端时才要求设置该环境变量 |
| `config` 包（`config.go`/`env.go`/`yaml.go`）| `DevMode` 字段、`dev_mode` yaml tag、`SIMPLEBASE_DEV_MODE`、`Redacted()` 中的 `dev_mode`、`validateSandbox(…, devMode)` 参数 | 全部删除；相关测试（`config_more_test.go`、`sandbox_test.go`）中的 DevMode 用例删除或改为对应的 storage 用例 |
| `cmd/simplebased/main_test.go`、`internal/app/*_test.go`、`systemdb_test.go` | 测试里设置 `cfg.DevMode = true` 来走本地盘 | 改为 `cfg.Storage.Database = "local"`（新的默认值，大多数情况下直接删掉这一行即可）+ 临时 `DataDir` |
| `objectstore`/`ducklake`/`sandbox` 注释 | 提到 DevMode | 改为「本地模式」 |
| `build.sh` | 默认导出 `SIMPLEBASE_DEV_MODE=true`（`cmd_dev` 第 357-364 行）；`ensure_dev_config` 用 awk 把 `dev_mode` 改成 true；把 `api_key_hash_secret` 写进 YAML；启动横幅打印 `SIMPLEBASE_DEV_MODE=` | 全部删除；`api_key_hash_secret` 改为写入 `.env`（`SIMPLEBASE_AUTH_APIKEY_SECRET`）；横幅改为打印 `storage.database/files`；健康等待按存储模式调整（本地 30s，s3 150s）。`reset` 改动见阶段 C |
| `ui/src/stores/project.ts` `DEFAULT_PROJECT_ID='dev-shop'` | 假定种子项目存在 | 改为登录后从项目列表里选择（优先上次选中的，其次第一个），没有项目时进入新建项目引导 |
| `ui/src/services/http.ts`、`ui/src/stores/auth.ts`、`ui/src/locales/*` | 注释或文案里提到 DevMode | 删除 |
| 文档：`README.md`、`AGENTS.md`、`docs/getting-started/*`、`docs/ops/deployment.md`、`docs/automation/sandbox*.md`、`docs/sdk/quickstart.md`、`config.example.yaml` | 介绍 dev_mode 用法 | 改为介绍「本地存储模式 / S3 模式」以及 §1.3 的 reset 切换流程 |

**验收**：`grep -rniE 'dev_?mode' internal cmd ui/src build.sh docs README.md AGENTS.md config.example.yaml`（排除 `node_modules`、`internal/web/dist`）**无任何结果**；全新启动后系统里没有明文种子 Key。

### 阶段 E：UI 存储设置（需求 5 的前端部分）

**语义**：存储模式以配置文件为准，UI 只是一个修改配置的入口。修改后需要按 §1.3 **停服 → `./build.sh reset` → 重启**，不支持运行时热切换，也不迁移数据。

后端：
- `GET /v1/system/storage`（`superadminl1` 和 `admin` 可调）返回 `{database, files, data_dir, cache_dir, bucket, prefix, effective, pending, source: {database: "yaml"|"env"|"default", …}, config_path, writable_config, data_present: {local, s3}}`。
- `PUT /v1/system/storage`（只有 `superadminl1` 可调，在 `router.go` 里用 `Role.CanManageUsers()` 同级的角色守卫，不复用 `ProjectAdmin` 权限）：
  - 用 `yaml.Node` 原地修改 `storage.*` 几个键，**保留注释和顺序**，写临时文件后 rename，原文件备份为 `config.yaml.bak`。
  - 如果某个字段被环境变量覆盖（`source=env`），返回 409，并提示去改 `.env`。
  - 请求体必须带 `confirm_reset: true`（UI 二次确认「切换后需要执行 reset，将清空全部数据」），否则返回 400。
  - 写入审计日志（`audit.Service`）。
  - 返回 `{restart_required: true, reset_required: true, commands: ["./build.sh reset", "./build.sh dev"]}`。

前端（`ui/src/components/SettingsModal.vue` + 新增 `settings/StoragePanel.vue`）：
- `TAB_KEYS` 新增 `storage` 页签，只有 `auth.isSuper || auth.isAdminRole` 时显示；`admin` 角色看到的是只读视图（表单禁用，并提示「仅超级管理员可修改」）。
- 页面内容：DB 存储模式和文件存储模式（单选：本地盘 / S3 兼容对象存储）、数据目录、缓存目录（只读，显示配置来源）、S3 bucket 和 prefix（只读，S3 模式时显示）、「待 reset」标记、保存成功后展示 §1.3 的三步命令（可复制）。
- 保存前弹出 `ConfirmAction` 危险确认：「切换存储模式后需要执行 `./build.sh reset`，会清空全部数据，且不迁移」，要求输入 `reset` 才能保存。
- 文案写入 `locales/zh-CN.ts`、`locales/en-US.ts`。
- 单测：三种角色的可见性；`source=env` 时字段禁用；PUT 返回 409 时显示提示。

**验收**：超管把 DB 存储从 local 改成 s3，`config.yaml` 只改动这一个键，注释原样保留；不 reset 直接重启会被身份检查拒绝，reset 后能以 s3 模式启动。`admin` 调 PUT 返回 403，`user` 看不到这个页签。

### 阶段 F：config 注释重写（需求 4）

重写 `config.example.yaml`（`build.sh` 从它生成 `config.yaml`；当前本地的 `config.yaml` 由人工按新模板合并）。每个键用统一格式注释：

```yaml
# <作用一句话>
# 默认：<值>   环境变量：SIMPLEBASE_XXX   生效条件：<如 storage.database=s3>
# 代码：<包/文件:符号>
```

需要逐键写清楚的对应关系（实施时以代码为准逐一核对）：

| 键 | 代码位置 | 说明要点 |
| --- | --- | --- |
| `http.*` | `internal/app/app.go` `http.Server` | 有沙盒时 `write_timeout` 会被自动抬高到 `exec_timeout_max+1m` |
| `instance.id` | 日志、指标、租约 owner、`reset --confirm` | 不再参与拼路径 |
| `instance.writable` | `assembleDeps`；只读实例不挂 SQL/KV 路由 | |
| `instance.lease.*` | `internal/database/lease` | 只在 `storage.database=s3` 时生效 |
| `storage.*` | 阶段 A | 本地模式下 data_dir 就是唯一副本，需要自行备份 |
| `database.cache_dir` / `cache_max_bytes` / `cache_max_databases` | `internal/database/cache` | 只用于可丢弃的缓存，淘汰时不会碰 data_dir |
| `database.idle_timeout` / `max_open` | `internal/database/registry` | 空闲多久关闭库句柄、同时打开的库数上限 |
| `database.ducklake.catalog_engine` | `ducklake.NormalizeCatalogEngine`、`checkCatalogEngine` | 切换前必须先 reset |
| `ducklake.memory_limit` / `threads` / `extension_dir` | `buildBootSQL` | 每个库的 DuckDB 实例各自独立计算 |
| `ducklake.data_inlining_row_limit` | `bootAttachSQL` / `optionSQL` | 小批写入内联到 catalog；S3 模式下同步前会刷成 parquet |
| `ducklake.parquet_compression` / `target_file_size` / `require_commit_message` | `optionSQL` | |
| `ducklake.catalog_sync.*` | `ducklake.CatalogSyncer` | 只在 s3 模式生效；本地模式忽略 |
| `ducklake.maintenance.*` | `systemdb.MaintenanceRunner` | merge / expire / cleanup 的周期；cleanup 默认 dry-run，需要设置 `SIMPLEBASE_SYSTEMDB_CLEANUP_DELETE=1` 才真正删除 |
| `s3.*` | `objectstore.NewClient` / `NewS3FileStore` | AK/SK 只能放在 `.env` |
| `system_database.*` | `systemdb.Store.StartPeriodicFlush`、`SeedGlobalRetention` | |
| `auth.api_key_hash_secret` | `auth.NewService` | 只能通过环境变量设置；更换后所有已发放的 Key 失效 |
| `llm.*` / `agent.*` | `llmgateway`、`cloudAgentRuntime` | 模型连接以「设置 → 模型」页为准，YAML 只作兜底 |
| `sandbox.*` | `internal/sandbox` | fake 后端只在 `SIMPLEBASE_SANDBOX_ALLOW_FAKE=1` 时可用 |
| `limits.*` | `api.SQLLimits`、gofunction 并发上限 | |
| `observability.*` | `observability.NewLogger`、`/metrics`、`perf_stage` | |

另外新增 `TestConfigExampleCoversAllKeys`：用反射遍历 `yamlConfig` 的全部 yaml tag，断言 `config.example.yaml` 里每个键都出现过，并且上方都有 `# 代码：` 注释行，防止以后新增配置时漏写注释。

## 3. 顺序与依赖

```
A（配置与目录分离，含 P0 淘汰保护）
 ├─> B（路径扁平化）──> C（build.sh reset 覆盖 data_dir + 新布局远端）
 ├─> D（彻底删除 DEV_MODE，依赖 storage 判定）──> E（UI，依赖 /v1/system/storage）
 └─> F（注释，最后按最终键集合写）
```

A、B、C 必须在同一个版本里发布。上线后，当前实例按阶段 C 的「一次性操作」执行一次 reset 即可，**现有数据全部废弃**。

## 4. 测试与验收总表

| # | 验收项 | 方法 | 状态 |
| --- | --- | --- | --- |
| 1 | 零配置启动走本地盘，不访问 S3 | 不设置 S3 环境变量启动；`config_test` 覆盖默认值 | ✅ |
| 2 | data_dir 和 cache_dir 重叠时拒绝启动；Evict 不碰 data_dir | `config` 单测 + `cache` 单测（`TestNewManagerRejectsDataDirOverlap`） | ✅ |
| 3 | 新布局 key / 路径正确 | `keys_test`、`factory_test`、`s3_handler_test` | ✅ |
| 4 | s3 模式全链路回归（同步、租约、manifest、split-brain） | 现有 `catalog_syncer`/`manifest_chain`/`lease` 测试改用新 key 后全部通过 | ✅ |
| 5 | reset 清空本地两个目录（加 `--remote` 时也清空新布局远端）；模式变更不 reset 就被拒绝启动 | `build.sh reset` 手工验收 + `identity` 单测 + `reset` 单测（dry-run 列出的范围） | ✅ |
| 6 | DEV_MODE 彻底清零、无明文种子 Key | grep 结果为空（2026-10-12 复查为零）+ `seed` 单测 | ✅ |
| 7 | UI 角色权限、配置写回保留注释 | 后端 `storage_settings_handler_test.go`（GET/PUT/confirm/角色/YAML 补丁/备份/env 冲突）+ `StoragePanel` 前端单测 9 例 | ✅ |
| 8 | 配置注释覆盖全部键 | `TestConfigExampleCoversAllKeys` + `TestConfigExampleNoDevMode` | ✅ |
| 9 | 全量回归 | `go build ./... && go vet ./... && go test ./...`、`gofmt -l` 为空、`cd ui && npm test`（106 文件 / 673 例全过；函数覆盖 94.88% 为历史欠账，新增文件 100%） | ✅ |
| 10 | 本机 reset + dev 冒烟 | `./build.sh reset -y` 清空后 `./build.sh dev` 正常就绪 | ✅ |

## 5. 风险

| 风险 | 缓解 |
| --- | --- |
| 本地模式下 data_dir 是唯一副本，磁盘损坏就会丢数据 | 文档和 UI 明确提示需要自行备份（例如挂载盘做快照）；`/health/ready` 暴露 `storage.database=local` |
| 切换模式必须 reset，误操作会清空全部数据 | UI 保存前要求输入 `reset` 确认；`build.sh reset` 交互确认时列出两个目录和存储模式；身份检查保证不 reset 就无法用新模式启动，避免数据错位 |
| 现有 COS 数据直接废弃 | 已确认接受；旧前缀由人工删除，代码不会去删 bucket 里的未知前缀 |
| `build.sh reset` 误删范围扩大（新增了 data_dir） | `resolve_data_dir` 和 `assert_safe_*` 拒绝 `/`、`$HOME`、仓库根目录，以及包含 `config.yaml` / `.env` 的目录；Go 侧的 `Validate` 同时禁止 data_dir 和 cache_dir 重叠 |
| UI 写配置文件失败（只读挂载、容器镜像内的配置） | `writable_config=false` 时 UI 只读，并提示改用环境变量 |
| 删除 `dev-shop` 种子后，现有前端 e2e 和 perfbench 依赖它 | 阶段 D 里同步改造，测试改为通过 API 创建项目 |
| s3 模式下扁平布局可能和 bucket 内其他业务冲突 | `s3.prefix` 保留为可选项；reset 只按 `_system/` 和已知项目枚举删除，不做整个前缀的删除 |

## 6. 不在本期范围

- 存储模式之间的数据迁移（已确认：切换时直接 reset）。
- 本地模式的自动定期备份到 S3。
- 多实例 / 多副本（仍然固定 1 副本）。
- 运行时热切换存储模式。
- 清理 COS 上的旧布局数据（人工操作）。
