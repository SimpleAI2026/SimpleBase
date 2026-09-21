# Config unification: `config.yaml` as the single control plane

> **Status**: plan only (this file). **Do not implement in this PR.**  
> **Date**: 2026-09-21  
> **Scope**: Go backend (`internal/config`, `internal/app`, `cmd/simplebased`, observability/logging, S3, DuckLake, auth, LLM gateway, cloud agent, cron, gofunction). Frontend `VITE_*` and JS SDK client env are noted then excluded.  
> **Verified against**: `internal/config/config.go` loader + tests, `config.example.yaml`, `.env.example`, `build.sh`, `internal/app/app.go`, call sites under `internal/` and `gofunction/`.  
> **Out of scope for this PR**: any application-code change. Deliverable is this markdown only.

---

## 0. One-sentence goal

Make **`config.yaml` the documented, loadable source of truth** for every SimpleBase server knob (including logging), keep **environment variables only for bootstrap path + secrets injection**, and delete the current split-brain between env-first loading, a stale “YAML is documentation only” example, and a pile of hardcoded / unconsumed fields.

---

## 1. Inventory — configuration sources today

### 1.1 How loading actually works (loader code, not comments)

`internal/config.Load()` is the only backend loader. Effective algorithm:

1. **Always** build a `Config` from environment (`loadFromEnv()`), applying Go defaults when `SIMPLEBASE_*` is unset.
2. Resolve a YAML path:
   - `SIMPLEBASE_CONFIG_PATH` if set (even if the file is missing);
   - else `./config.yaml` **only if that file exists in the process cwd**;
   - else skip YAML.
3. If the file can be read **and** `yaml.Unmarshal` succeeds, `applyYAML` copies non-zero YAML fields onto the env-built struct **only when the corresponding env var is unset**.
4. Invalid YAML / missing file is **silent**: Load falls back to env+defaults. No error, no log.
5. `Validate()` then fails the process if required fields are empty.

Comments in `Load()` say “YAML first, then env overlay”. The implementation is the inverse order of assignment but the **same precedence**: **explicit env wins over YAML wins over code defaults**.

There is a **second, unused struct layer**: `Config` carries `mapstructure:"..."` tags (Plan 1 / Viper leftover). Runtime parse uses a parallel `yamlConfig` with `yaml:"..."` tags. `mapstructure` is never invoked.

`config.yaml` is gitignored (`.gitignore`). Operators are expected to copy `config.example.yaml`.

### 1.2 YAML keys — `config.example.yaml` vs `yamlConfig`

`config.example.yaml` header is **stale**. It still says:

> `internal/config` 实际只从环境变量加载… 不解析 YAML。本文件仅作为字段清单参考

That is false. Tests in `config_more_test.go` (`TestLoadFromEnvAndYAMLOverride`, `TestLoadYAMLDoesNotOverrideExplicitEnv`) prove YAML is parsed.

| YAML path | In `yamlConfig`? | In example file? | Notes |
| --- | --- | --- | --- |
| `http.address` | yes | yes | |
| `http.read_timeout` | yes | yes | Go `time.Duration` (e.g. `15s`) |
| `http.write_timeout` | yes | yes | |
| `http.idle_timeout` | yes | yes | |
| `instance.id` | yes | yes | required |
| `instance.writable` | yes | yes | **zero-value trap** (see §3) |
| `database.engine` | yes | yes | must be `ducklake` |
| `database.cache_dir` | yes | yes | required |
| `database.idle_timeout` | yes | yes | |
| `database.max_open` | yes | yes | registry concurrent DBs, **not** sql.DB pool |
| `database.max_idle` | yes | yes | **loaded, never consumed** |
| `database.cache_max_bytes` | **no** | **no** | documented in `docs/ops/deployment.md`; on `DatabaseConfig` struct only |
| `database.cache_max_databases` | **no** | **no** | same |
| `database.ducklake.memory_limit` | yes | yes | |
| `database.ducklake.threads` | yes | yes | YAML `> 0` only; cannot set 0 via YAML |
| `database.ducklake.extension_dir` | yes | yes | |
| `database.ducklake.data_inlining_row_limit` | yes | yes | |
| `database.ducklake.parquet_compression` | yes | yes | |
| `database.ducklake.target_file_size` | yes | yes | |
| `database.ducklake.require_commit_message` | yes | **no** | env + yaml loader; missing from example |
| `database.ducklake.catalog_sync.mode` | yes | yes | `debounce` \| `sync_on_commit` |
| `database.ducklake.catalog_sync.debounce_ms` | yes | yes | YAML is **milliseconds int**; env is `*_DEBOUNCE_MS` |
| `database.ducklake.catalog_sync.keep_versions` | yes | yes | |
| `database.ducklake.maintenance.checkpoint_interval` | yes | yes | duration |
| `database.ducklake.maintenance.expire_older_than` | yes | yes | YAML **string** (`7d`); env duration |
| `database.ducklake.maintenance.delete_older_than` | yes | yes | same |
| `database.ducklake.maintenance.rewrite_delete_threshold` | yes | yes | |
| `s3.endpoint` | yes | yes | |
| `s3.region` | yes | yes | |
| `s3.bucket` | yes | yes | |
| `s3.prefix` | yes | yes | default `simplebase` |
| `s3.access_key` | yes | yes | secret |
| `s3.secret_key` | yes | yes | secret |
| `s3.kms_key_id` | **no** | **no** | env-only `SIMPLEBASE_S3_KMS_KEY_ID` |
| `s3.force_path_style` | yes | yes | **zero-value trap** |
| `system_database.name` | yes | yes | |
| `system_database.hide_from_list` | yes (`*bool`) | yes | **loaded, never consumed** |
| `system_database.metrics_flush_interval` | yes | yes | **loaded, never consumed** |
| `system_database.log_flush_interval` | yes | yes | **loaded, never consumed** |
| `system_database.log_keep_days` | yes | yes | **loaded, never consumed** (retention lives in `sys_log_retention`) |
| `auth.api_key_hash_secret` | yes | yes | secret, required |
| `llm` (any nested keys) | **no** | commented, non-structural | env-only providers |
| `llm.enabled` | **no** | **no** | struct field never set by Load |
| `limits.*` (6 fields) | yes | yes | |
| `observability.log_level` | yes | yes | |
| `observability.log_format` | yes | yes | |
| `observability.metrics_path` | yes | yes | |
| `dev_mode` | yes | **no** | loader + tests; missing from example file |

Example file also uses **non-loadable comment shapes** for LLM (`# provider.openai.api_key`), not a real YAML map.

### 1.3 Environment variables

#### Server (`internal/config`)

Prefix `SIMPLEBASE_`. Helpers: `envStr`, `envBool`, `envInt`, `envInt64`, `envDuration` (Go duration **or** `Nd` days), `envFloat64`, `envList` (comma-separated). Invalid parse → **silently use default** (no error).

| Env var | Default | Secret? |
| --- | --- | --- |
| `SIMPLEBASE_CONFIG_PATH` | empty → optional `./config.yaml` if present | no (path) |
| `SIMPLEBASE_DEV_MODE` | `false` in loader; **`true` in `build.sh dev`** | no |
| `SIMPLEBASE_HTTP_ADDRESS` | `:8080` | no |
| `SIMPLEBASE_HTTP_READ_TIMEOUT` | `15s` | no |
| `SIMPLEBASE_HTTP_WRITE_TIMEOUT` | `30s` | no |
| `SIMPLEBASE_HTTP_IDLE_TIMEOUT` | `60s` | no |
| `SIMPLEBASE_INSTANCE_ID` | `""` (required) | no |
| `SIMPLEBASE_INSTANCE_WRITABLE` | `true` | no |
| `SIMPLEBASE_DB_ENGINE` | `ducklake` | no |
| `SIMPLEBASE_DB_CACHE_DIR` | `""` (required) | no |
| `SIMPLEBASE_DB_IDLE_TIMEOUT` | `5m` | no |
| `SIMPLEBASE_DB_MAX_OPEN` | `8` | no |
| `SIMPLEBASE_DB_MAX_IDLE` | `2` | no |
| `SIMPLEBASE_DUCKLAKE_MEMORY_LIMIT` | `512MB` | no |
| `SIMPLEBASE_DUCKLAKE_THREADS` | `2` | no |
| `SIMPLEBASE_DUCKLAKE_EXTENSION_DIR` | `""` | no |
| `SIMPLEBASE_DUCKLAKE_DATA_INLINING_ROW_LIMIT` | `100` | no |
| `SIMPLEBASE_DUCKLAKE_PARQUET_COMPRESSION` | `zstd` | no |
| `SIMPLEBASE_DUCKLAKE_TARGET_FILE_SIZE` | `64MB` | no |
| `SIMPLEBASE_DUCKLAKE_REQUIRE_COMMIT_MESSAGE` | `false` | no |
| `SIMPLEBASE_DUCKLAKE_SYNC_MODE` | `debounce` | no |
| `SIMPLEBASE_DUCKLAKE_SYNC_DEBOUNCE_MS` | `200` | no |
| `SIMPLEBASE_DUCKLAKE_SYNC_KEEP_VERSIONS` | `10` | no |
| `SIMPLEBASE_DUCKLAKE_MAINT_CHECKPOINT_INTERVAL` | `1h` | no |
| `SIMPLEBASE_DUCKLAKE_MAINT_EXPIRE_OLDER_THAN` | `7d` | no |
| `SIMPLEBASE_DUCKLAKE_MAINT_DELETE_OLDER_THAN` | `24h` | no |
| `SIMPLEBASE_DUCKLAKE_MAINT_REWRITE_DELETE_THRESHOLD` | `0.95` | no |
| `SIMPLEBASE_S3_ENDPOINT` | `""` | no |
| `SIMPLEBASE_S3_REGION` | `""` | no (required if writable && !dev) |
| `SIMPLEBASE_S3_BUCKET` | `""` | no (required if writable && !dev) |
| `SIMPLEBASE_S3_PREFIX` | `simplebase` | no |
| `SIMPLEBASE_S3_ACCESS_KEY` | `""` | **yes** |
| `SIMPLEBASE_S3_SECRET_KEY` | `""` | **yes** |
| `SIMPLEBASE_S3_KMS_KEY_ID` | `""` | low (key id, not secret material) |
| `SIMPLEBASE_S3_FORCE_PATH_STYLE` | `false` | no |
| `SIMPLEBASE_AUTH_APIKEY_SECRET` | `""` (required) | **yes** |
| `SIMPLEBASE_LLM_PROVIDERS` | `""` (CSV names) | no |
| `SIMPLEBASE_LLM_PROVIDER_<NAME>_API_KEY` | `""` | **yes** |
| `SIMPLEBASE_LLM_PROVIDER_<NAME>_BASE_URL` | `""` | no |
| `SIMPLEBASE_LLM_PROVIDER_<NAME>_DEFAULT_MODEL` | `""` | no |
| `SIMPLEBASE_LLM_PROVIDER_<NAME>_ALLOWED_MODELS` | empty list | no |
| `SIMPLEBASE_LLM_PROVIDER_<NAME>_TIMEOUT` | `60s` | no |
| `SIMPLEBASE_LIMITS_MAX_REQUEST_BYTES` | `1048576` | no |
| `SIMPLEBASE_LIMITS_MAX_QUERY_ROWS` | `1000` | no |
| `SIMPLEBASE_LIMITS_QUERY_TIMEOUT` | `30s` | no |
| `SIMPLEBASE_LIMITS_MAX_CONCURRENT_QUERIES` | `64` | no |
| `SIMPLEBASE_LIMITS_MAX_BATCH_STATEMENTS` | `100` | no |
| `SIMPLEBASE_LIMITS_MAX_SQL_BYTES` | `65536` | no |
| `SIMPLEBASE_LOG_LEVEL` | `info` | no |
| `SIMPLEBASE_LOG_FORMAT` | `json` | no |
| `SIMPLEBASE_METRICS_PATH` | `/metrics` | no |
| `SIMPLEBASE_SYSTEM_DB_NAME` | `simplebase-system` | no |
| `SIMPLEBASE_SYSTEM_DB_HIDE_FROM_LIST` | `true` | no |
| `SIMPLEBASE_METRICS_FLUSH_INTERVAL` | `2s` | no |
| `SIMPLEBASE_LOG_FLUSH_INTERVAL` | `2s` | no |
| `SIMPLEBASE_LOG_KEEP_DAYS` | `14` | no |

`.env.example` documents most of the above but **omits**: `SIMPLEBASE_CONFIG_PATH`, `SIMPLEBASE_S3_KMS_KEY_ID`, `SIMPLEBASE_DUCKLAKE_REQUIRE_COMMIT_MESSAGE`, DuckLake maintenance envs, `SIMPLEBASE_LLM_PROVIDER_*_ALLOWED_MODELS`, `SIMPLEBASE_LLM_PROVIDER_*_TIMEOUT`. It **does** include COS-shaped S3 endpoint/bucket/region sample values.

**Retired (no-op):** `SIMPLEBASE_CATALOG_DATABASE_ID` / `catalog.database_id` — already removed from loader (`cleanup-refactor-plan.md` L10). Docs still mention the retirement.

#### Dev-script / frontend-bridge (not in `internal/config`)

| Env var | Where | Default | Purpose |
| --- | --- | --- | --- |
| `SIMPLEBASE_DEV_UI_HOST` | `build.sh` | `127.0.0.1` | Vite bind/open host |
| `SIMPLEBASE_DEV_UI_PORT` | `build.sh` | `5173` | Vite port |
| `SIMPLEBASE_DEV_API_HOST` | `build.sh` | `127.0.0.1` | backend host for URLs |
| `SIMPLEBASE_DEV_API_PORT` | `build.sh` | `8080` | backend port |
| `SIMPLEBASE_DEV_API_PROXY` | `build.sh` → `ui/vite.config.ts` | `http://127.0.0.1:8080` | Vite proxy target |

`build.sh dev` **unconditionally exports** `SIMPLEBASE_HTTP_ADDRESS=":${DEV_API_PORT}"` after sourcing `.env`, so YAML `http.address` and `.env` `SIMPLEBASE_HTTP_ADDRESS` both lose to the script flag/`--api-port`.

#### Client / SDK (out of server unification)

`SIMPLEBASE_URL`, `SIMPLEBASE_API_KEY`, `SIMPLEBASE_PROJECT_ID`, `SIMPLEBASE_DATABASE_ID` — JS SDK / examples only.

#### UI Vite (out of server unification)

`VITE_USE_MOCK`, `VITE_API_BASE_URL` in `ui/.env.*`.

### 1.4 CLI flags

**None in the Go server.** `flag` / `pflag` / `cobra` are unused.

`build.sh` is the only CLI:

- build: `--skip-ui`, `--ui-only`
- dev: `--no-open`, `--host`, `--port`, `--api-port`

`cmd/simplebased` is referenced by `README.md`, `Dockerfile`, `build.sh`, `internal/README.md`, and Plan 1, but **the directory is not present on `origin/main`** (never committed as `cmd/simplebased`; historic `cmd/http` / `cmd/server` were LessDB-era). The intended entrypoint is `config.Load()` → `app.New` → `app.RunWithSignal`. Restoring that binary is a prerequisite of any config migration that wants a runnable server; it is **not** implemented in this plan PR.

### 1.5 Hardcoded defaults in code (not in YAML/env)

| Knob | Value | Where | Consumed? |
| --- | --- | --- | --- |
| DuckLake `DefaultOptions()` | 512MB / 2 threads / zstd / 64MB / debounce 200ms / keep 10 / maint 1h/7d/1d/0.95 | `internal/database/ducklake/options.go` | yes, `normalized()` fills blanks |
| DuckLake lake alias | `"lake"` | same | yes; **not configurable** (by design — user SQL cannot ATTACH) |
| DuckDB pool | `SetMaxOpenConns(1)`, `SetMaxIdleConns(1)` | `ducklake/factory.go` | yes; **ignores** `database.max_idle` |
| Cache LRU | 1 GiB, 256 DBs | `database/cache/manager.go` when `MaxBytes`/`MaxDatabases` ≤ 0 | yes; config fields never loaded so **always** these defaults |
| Registry idle | 5m if `IdleTimeout <= 0` | `registry.New` | yes (usually overridden by config) |
| Registry evictor interval | 1m | `registry/evictor.go` | **Evictor never started in `app`** |
| System DB name fallback | `simplebase-system` | `systemdb.DefaultName` | yes if config name empty |
| Log/metrics buffer flush | 64 records, not time | `systemdb/logs.go`, `metrics.go` | yes; **ignores** flush-interval config |
| Store close timeout | 15s | `systemdb/store.go` | yes |
| Log retention default | 14 days | `systemdb/logs.go` `GetRetention` | yes; **ignores** `LogKeepDays` config |
| Cloud agent scheduler | tick 30s, concurrency 4, history 40 | `cloudagent/scheduler.go` | yes |
| Cron scheduler | tick 30s, concurrency 4, response 4KiB | `cronjob/scheduler.go` | yes |
| Cron search horizon | 2 years of minutes | `crontab/crontab.go` | yes |
| Go function source cap | 256 KiB, 100 per project | `api/gofunction_handler.go` | yes |
| Go function interpreter | `defaultTimeout = 10s` (wait goroutines); executor timeout `-1` (unlimited) unless ctx | `gofunction/frame.go`, `executor.go` | yes |
| Go function importer cache | 5m / size 1000 | `gofunction/importer/registry.go` | yes |
| Go function pool | idle 10 / active 100 / age 30m | `gofunction/executor.go` | yes, if pool used |
| Usage quota period fallback | 1h | `usage/service.go` | yes |
| Shutdown timeout | passed into `RunWithSignal`; docs say 30s | `app.go` / `docs/ops/deployment.md` | **no config field** — only the missing `main` would pick a value |
| Logger writer | `os.Stderr` | `observability.NewLogger` | yes |
| Logger encoder | JSON only (`zap.NewProductionConfig`); `log_format=console` is a **no-op** | `internal/log/log.go`, `observability/logger.go` | yes |
| Unknown log level | treated as `info` | `parseLevel` | yes |
| Dev seed API key | `sb_live_dev_key_12345` | `systemdb/seed.go` | yes if `DevMode` |
| Echo banner/port | hidden | `api/router.go` | n/a |
| AWS SDK default credential chain | when access/secret empty | `objectstore/client.go` | yes (IAM roles) |

Duplicate default tables: `config.loadFromEnv` **and** `ducklake.DefaultOptions` **and** `cache.NewManager` each encode overlapping product defaults.

### 1.6 Logging-related settings

| Concern | Current | Notes |
| --- | --- | --- |
| Level | `observability.log_level` / `SIMPLEBASE_LOG_LEVEL` / default `info` | `debug\|info\|warn\|error`; other → info |
| Format | `observability.log_format` / `SIMPLEBASE_LOG_FORMAT` / default `json` | `"console"` documented but `newConsoleLogger` still JSON-encodes |
| Output | hardcoded stderr | no file path, no rotation (removed from `internal/log`) |
| Access log | Echo middleware → zap | not configurable |
| Prometheus path | `observability.metrics_path` | mounted in `api.NewRouter` |
| In-DB run logs | `sys_log_events` buffer, flush at 64 rows or Close | `log_flush_interval` unused |
| In-DB metrics | `sys_metric_samples` buffer, flush at 64 rows | `metrics_flush_interval` unused |
| Retention | API `PUT .../logs/retention` + table default 14 | `log_keep_days` unused |
| Redaction | `observability` field-name denylist | not a config knob (must stay code) |
| Startup dump | `cfg.Redacted()` logged at info | secrets as `has_*` flags only |
| `build.sh` log level | **`SIMPLEBASE_LOG_LEVEL="${SIMPLEBASE_LOG_LEVEL:-debug}"`** on `./build.sh dev` | landed in `9c062e1` (`fix(dev): show backend debug logs`). Production binary default remains `info`. Env overlay means yaml `observability.log_level` **never applies** under `build.sh dev` unless the script stops exporting it |
| Dev process logs | stdout/stderr via `tee` to a temp file | script-only sink, not zap `log_output` |

### 1.7 `build.sh` overrides

On `./build.sh dev`:

1. `source` repo-root `.env` if present (`set -a`).
2. `export SIMPLEBASE_DEV_MODE="${SIMPLEBASE_DEV_MODE:-true}"` — default true **only for this script**, unlike loader default false.
3. `export SIMPLEBASE_LOG_LEVEL="${SIMPLEBASE_LOG_LEVEL:-debug}"` — **always if unset**, so yaml `observability.log_level: info` is ignored in `dev`.
4. `export SIMPLEBASE_HTTP_ADDRESS=":${DEV_API_PORT}"` — **always**, clobbering yaml/env.
5. `export SIMPLEBASE_DEV_API_PROXY=http://${DEV_API_HOST}:${DEV_API_PORT}` — Vite only.
6. Warns if `config.yaml` missing; does not copy `config.example.yaml`.
7. Does **not** set instance id, cache dir, or auth secret (those still come from yaml / `.env`).
8. Backend stdout/stderr is `tee`'d to the terminal and a temp log file.

Production `./build.sh` (build mode) compiles only; no env injection.

---

## 2. Current mapping

Columns: **name** | **current source** | **where read** | **purpose** | **sensitive?**

Legend for source: `yaml` = parsed today; `env` = `SIMPLEBASE_*`; `flag` = `build.sh` CLI; `hardcoded` = const/default in a package; `docs-only` = written in docs/example but not loaded; `dead` = loaded into `Config` but no consumer.

| Name | Current source | Where read | Purpose | Sensitive? |
| --- | --- | --- | --- | --- |
| `SIMPLEBASE_CONFIG_PATH` | env | `config.configFilePath` | locate YAML | no |
| `http.address` | yaml + env + **build.sh always** in `dev` | `app` HTTP server | listen addr | no |
| `http.read_timeout` | yaml + env / default 15s | `http.Server` | request read deadline | no |
| `http.write_timeout` | yaml + env / default 30s | `http.Server` | response write deadline | no |
| `http.idle_timeout` | yaml + env / default 60s | `http.Server` | keep-alive idle | no |
| `instance.id` | yaml + env (**required**) | S3 key prefix `Environment`, logs | instance identity / object prefix | no |
| `instance.writable` | yaml + env / default true | `app`, `api` handlers, registry | enable writes | no |
| `database.engine` | yaml + env / default ducklake | `Validate` only | reject non-ducklake | no |
| `database.cache_dir` | yaml + env (**required**) | factory, cache manager, system locator | local working set | no |
| `database.idle_timeout` | yaml + env / default 5m | `registry.Options` | close idle DB handles | no |
| `database.max_open` | yaml + env / default 8 | `registry.Options.MaxOpen` | max concurrently open DBs | no |
| `database.max_idle` | yaml + env / default 2 | **dead** (only Validate/Redacted) | intended sql pool idle | no |
| `database.cache_max_bytes` | **hardcoded 1GiB** (struct zero) | `cache.NewManager` | LRU byte cap | no |
| `database.cache_max_databases` | **hardcoded 256** | `cache.NewManager` | LRU db-count cap | no |
| `database.ducklake.*` | yaml + env + `DefaultOptions` | `app.duckLakeOptions` → factory | DuckDB/DuckLake engine | no |
| `s3.endpoint/region/bucket/prefix` | yaml + env | objectstore, ducklake remote | persistence | no |
| `s3.access_key` / `s3.secret_key` | yaml **or** env | objectstore static creds | S3 identity | **yes** |
| `s3.kms_key_id` | **env only** | objectstore Put SSE-KMS | CMK id | low |
| `s3.force_path_style` | yaml + env | AWS SDK | COS/MinIO compatibility | no |
| `auth.api_key_hash_secret` | yaml + env (**required**) | `auth.NewService` HMAC | API key hashing | **yes** |
| `llm.enabled` | **hardcoded false** | `app.assembleDeps` | construct gateway | no |
| `llm.providers.*` | **env only** | `loadLLMProviders` | instance-level providers | API key **yes** |
| `limits.*` | yaml + env | SQL handler + Echo BodyLimit | request/query caps | no |
| `observability.log_level` | yaml + env / default `info`; **`build.sh dev` forces `debug` if unset** | `observability.NewLogger` | zap level | no |
| `observability.log_format` | yaml + env | NewLogger (console broken) | encoder | no |
| `observability.metrics_path` | yaml + env | router GET | Prometheus scrape path | no |
| `system_database.name` | yaml + env | bootstrap | system DuckLake name | no |
| `system_database.hide_from_list` | yaml + env | **dead** | intended list filter | no |
| `system_database.metrics_flush_interval` | yaml + env | **dead** | intended ticker | no |
| `system_database.log_flush_interval` | yaml + env | **dead** | intended ticker | no |
| `system_database.log_keep_days` | yaml + env | **dead** | intended default retention | no |
| `dev_mode` | yaml + env + **build.sh default true** | assembleDeps, seed, S3 bypass | local disk / seed key | no |
| logger output | hardcoded stderr | `NewLogger` | sink | no |
| graceful shutdown deadline | hardcoded caller / docs 30s | `RunWithSignal` | SIGTERM budget | no |
| agent/cron scheduler tick+concurrency | hardcoded 30s / 4 | schedulers | background jobs | no |
| gofunction source/count/timeout | hardcoded | handler + interpreter | sandbox limits | no |
| Dev seed key | hardcoded | `systemdb.Seed` | local auth | **yes** (dev-only) |
| `SIMPLEBASE_DEV_*` | env + flags | `build.sh`, Vite | local UX | no |
| `SIMPLEBASE_URL` / `API_KEY` / … | client env | JS SDK | **not server config** | API key **yes** |

Docs that **disagree with the loader** (stale names, not implemented):

| Docs name | File | Reality |
| --- | --- | --- |
| `s3.access_key_id` / `secret_access_key` | `docs/ops/deployment.md` | fields are `access_key` / `secret_key` |
| `auth.server_secret` | same | field is `api_key_hash_secret` |
| `auth.default_plan` | same | **does not exist** |
| `database.cache_max_bytes` = 10GiB | same | not loaded; runtime default 1GiB |
| “YAML is not parsed” | `config.example.yaml` header | it is parsed |
| `docs/deployment.md` / `docs/migration-guide.md` | `README.md` links | actual paths are `docs/ops/deployment.md`, `docs/ops/migration.md` |

---

## 3. Gaps / duplication

### 3.1 Env vs YAML conflicts

1. **Documented precedence vs example header.** README / `internal/README.md` / `internal/AGENTS.md` say yaml + `SIMPLEBASE_` overlay. `config.example.yaml` says YAML is not loadable. Operators following the example file will ignore YAML entirely.
2. **DevMode three defaults.** Loader `false`; `build.sh dev` `true`; missing `dev_mode` in `config.example.yaml`. A checked-in yaml with `dev_mode: false` is still overridden by the script’s `export SIMPLEBASE_DEV_MODE=true` unless `.env` already set it.
3. **Log level two defaults.** Loader/`config.example.yaml` `info`; `build.sh dev` `debug`. Yaml `observability.log_level` loses under `dev` because the script exports the env var.
4. **HTTP address in dev.** `build.sh` always sets `SIMPLEBASE_HTTP_ADDRESS`, so yaml `http.address` never applies under `./build.sh dev`.
5. **Boolean YAML overlay bugs.** `applyYAML` assigns `yc.Instance.Writable`, `yc.S3.ForcePathStyle`, `yc.DevMode`, `yc.DuckLake.RequireCommitMessage` whenever the env var is unset — **including when the key is omitted** (Go `false`). A yaml that only sets `http`/`database` can silently force `writable=false` and `dev_mode=false`. `hide_from_list` avoided this with `*bool`; others did not.
6. **Cannot express zero via YAML** for ints/durations (`max_idle: 0` skipped by `!= 0`; `threads: 0` skipped by `> 0`). Env can set them.
7. **Duration type split.** Most durations unmarshal as `time.Duration`. DuckLake `expire_older_than` / `delete_older_than` are YAML **strings** parsed by `parseDuration`. Easy to break if someone writes `7d` on a `time.Duration` field (works) vs a field that expects string.
8. **Silent YAML failure.** Typo in `config.yaml` → env defaults, production looks “configured” and isn’t.
9. **Invalid env parse is silent.** `SIMPLEBASE_HTTP_READ_TIMEOUT=nope` → 15s default, process starts.

### 3.2 Undocumented / env-only knobs

- `SIMPLEBASE_CONFIG_PATH`
- `SIMPLEBASE_S3_KMS_KEY_ID` (no yaml)
- Entire LLM provider map (no yaml struct)
- `SIMPLEBASE_DUCKLAKE_REQUIRE_COMMIT_MESSAGE` (yaml loader exists; example omits)
- DuckLake maintenance env vars (in loader + example yaml; **missing from `.env.example`**)
- `llm.enabled` (not even an env var)

### 3.3 Env-only settings that belong in YAML

Everything non-secret in §1.3 should live in yaml. Highest pain: DuckLake engine, limits, observability, system_database, instance, http.

Secrets **may** stay env-only (see §4). Today they are **also accepted from yaml** (`s3.access_key`, `s3.secret_key`, `auth.api_key_hash_secret`). That contradicts the comment at the top of `Config` (“敏感字段只能来自环境或密钥服务；禁止写入仓库或日志”) and README (“生产凭据必须从环境变量或 IAM 角色注入”).

### 3.4 YAML/struct fields that are dead

| Field | Loaded? | Consumer |
| --- | --- | --- |
| `Database.CacheMaxBytes` / `CacheMaxDatabases` | no | cache manager zeros → hardcoded 1GiB/256; docs show 10GiB |
| `Database.MaxIdle` | yes | none (`SetMaxIdleConns(1)` hardcoded) |
| `LLM.Enabled` | no | if false, **LLM gateway + cloud agent LLM are never constructed** even when providers env is set |
| `LLM.Providers` | env only | **unused by `llmgateway`**. Runtime keys come from catalog `CredentialRef`. Instance env providers are a leftover of Plan 8 “global providers” and never wired into `NewCatalogResolver` (`credential` is passed `nil` in `app.go`) |
| `SystemDatabase.HideFromList` | yes | listing uses admin project + `kind=system`, not this flag |
| `SystemDatabase.MetricsFlushInterval` / `LogFlushInterval` | yes | flush is count-based (64) |
| `SystemDatabase.LogKeepDays` | yes | retention table default 14, API-managed |
| `Observability.LogFormat=console` | yes | still JSON |
| `registry.Evictor` | n/a | not started; idle close only on Acquire path / tests |

### 3.5 `build.sh`-only overrides

- DevMode default true
- Forced HTTP address from `--api-port`
- Vite proxy URL
- Backend stdout `tee`'d to the terminal (plus a temp file) — not a zap `log_output` setting
- **`SIMPLEBASE_LOG_LEVEL=debug` by default** on `./build.sh dev` (already shipped in `9c062e1`). This is the right local UX but it **shadows** yaml `observability.log_level`. Later change: only export when yaml/env did not set a level, or drop the export and put `log_level: debug` in the generated local `config.yaml`.

### 3.6 Dual default tables

Changing “product default threads = 4” requires edits in `loadFromEnv`, `config.example.yaml`, `.env.example`, `ducklake.DefaultOptions`, and possibly tests. Unification should pick **one** canonical default table (config package) and have ducklake/cache `normalized()` call it or accept already-filled values.

### 3.7 Missing process entrypoint

Without `cmd/simplebased`, `config.Load()` is only reached from tests. Docker `ENTRYPOINT ["/simplebased"]` and `go build ./cmd/simplebased` cannot work on current `main`. Config unification Phase 1 should restore a flag-free `main` that: `Load` → `New` → `RunWithSignal(cfg.HTTP.ShutdownTimeout)` (new field). That restore is listed as a **later implementation** step, not this PR.

### 3.8 LLM credential story is split three ways

1. Instance env `SIMPLEBASE_LLM_PROVIDER_*` (loaded, not used at runtime).
2. Catalog `llm_provider_configs.credential_ref` + `CredentialResolver` (designed path; resolver is nil in app).
3. UI/localStorage historically (plan says retired).

Unification must decide: yaml/env is **instance default catalog for bootstrap**, or **forbidden** (catalog-only). Plan recommendation in §4.4.

---

## 4. Target design

### 4.1 Principles

1. **One file shape.** `config.example.yaml` is a valid, loadable document. Copy to `config.yaml`. No second “this is just comments” disclaimer.
2. **YAML owns non-secrets.** Engine, limits, listen address, log level, DuckLake, system DB, feature flags.
3. **Env owns injection.** Kubernetes Secret / IAM / `.env` for material that must not be in git: S3 keys, HMAC secret, optional LLM keys. Also `SIMPLEBASE_CONFIG_PATH` as bootstrap.
4. **Fail closed.** Missing yaml when `SIMPLEBASE_CONFIG_PATH` is set → error. Unreadable/invalid yaml → error. Unknown required fields → existing `Validate`. Unknown yaml keys: log warning in Phase 2, reject in Phase 3 (optional).
5. **No CLI flags on `simplebased`.** Keep `build.sh` as a dev orchestrator only.
6. **Do not log secrets.** Keep `Redacted()`; never print yaml raw at info if it may contain keys. Prefer refusing to unmarshal secrets from yaml in production (`dev_mode: false`).
7. **Wire or delete dead fields.** No “loaded but ignored” knobs after the migration.

### 4.2 Precedence (target)

```text
code defaults
    → config.yaml (or SIMPLEBASE_CONFIG_PATH)
        → env overlay
```

Env overlay rules:

| Class | Overlay? | Examples |
| --- | --- | --- |
| Bootstrap | env only | `SIMPLEBASE_CONFIG_PATH` |
| Secrets | env **wins**; yaml **forbidden in non-dev** | `s3.access_key`, `s3.secret_key`, `auth.api_key_hash_secret`, `llm.providers.*.api_key` |
| Non-secrets | env wins **if set**, for one release (compat); then deprecate | `SIMPLEBASE_LOG_LEVEL`, `SIMPLEBASE_HTTP_ADDRESS`, … |
| IAM / empty keys | neither | AWS default credential chain when access/secret empty |

Compat period: keep every current `SIMPLEBASE_*` name as an alias (`SIMPLEBASE_HTTP_ADDRESS` → `http.address`) so existing `.env` and k8s manifests do not break. Document “prefer yaml; env is override”. After one documented release, drop non-secret env from `.env.example` (keep in loader until a later breaking change if needed).

**Not recommended:** “env overrides secrets only, yaml always wins for the rest” in the first implementation — `build.sh` and k8s already depend on env for `HTTP_ADDRESS` / `DEV_MODE`. Phase A = env-still-wins; Phase B = warn when non-secret env is set; Phase C (optional) = ignore non-secret env.

### 4.3 Proposed `config.yaml` schema

```yaml
# config.example.yaml (target). Copy to config.yaml.
# Secrets: leave empty here; inject via SIMPLEBASE_S3_* / SIMPLEBASE_AUTH_APIKEY_SECRET
# or (dev only) fill locally — never commit.

http:
  address: ":8080"
  read_timeout: 15s
  write_timeout: 30s
  idle_timeout: 60s
  shutdown_timeout: 30s          # NEW — RunWithSignal

instance:
  id: "simplebase-dev-1"         # required
  writable: true

dev_mode: false                  # true: local DATA_PATH, bypass S3, seed key

database:
  engine: ducklake               # only legal value
  cache_dir: ".cache"            # required
  cache_max_bytes: 1073741824    # 1GiB; docs/ops 10GiB is a prod suggestion
  cache_max_databases: 256
  idle_timeout: 5m
  max_open: 8                    # registry: concurrent open DBs
  # max_idle: removed (unused; DuckDB conn is always 1)
  ducklake:
    memory_limit: 512MB
    threads: 2
    extension_dir: ""
    data_inlining_row_limit: 100
    parquet_compression: zstd
    target_file_size: 64MB
    require_commit_message: false
    catalog_sync:
      mode: debounce             # debounce | sync_on_commit
      debounce: 200ms            # unify away debounce_ms
      keep_versions: 10
    maintenance:
      checkpoint_interval: 1h
      expire_older_than: 7d
      delete_older_than: 1d
      rewrite_delete_threshold: 0.95

s3:
  endpoint: ""
  region: ""
  bucket: ""
  prefix: "simplebase"
  force_path_style: false
  kms_key_id: ""
  # access_key / secret_key: omit in committed files

system_database:
  name: "simplebase-system"
  hide_from_list: true           # WIRE or delete in same change
  metrics_flush_interval: 2s     # WIRE ticker (in addition to size-64)
  log_flush_interval: 2s
  log_keep_days: 14              # seed sys_log_retention if row missing

auth:
  api_key_hash_secret: ""        # required; env SIMPLEBASE_AUTH_APIKEY_SECRET

llm:
  enabled: true                  # NEW — default true if any provider or catalog exists
  # Instance bootstrap providers (optional). Runtime still catalog-first.
  providers:
    openai:
      api_key: ""                # env SIMPLEBASE_LLM_PROVIDER_OPENAI_API_KEY
      base_url: ""
      default_model: "gpt-4o-mini"
      allowed_models: ["gpt-4o-mini", "gpt-4o"]
      timeout: 60s

limits:
  max_request_bytes: 1048576
  max_query_rows: 1000
  query_timeout: 30s
  max_concurrent_queries: 64
  max_batch_statements: 100
  max_sql_bytes: 65536

observability:
  log_level: info                # debug | info | warn | error
  log_format: json               # json | console (console must actually console-encode)
  log_output: stderr             # NEW: stderr | stdout (no files in v1)
  metrics_path: /metrics

# Optional sections — include only if we choose to promote hardcoded knobs.
# Recommendation: Phase 2, not Phase 1.

schedulers:
  agent:
    tick: 30s
    concurrency: 4
  cron:
    tick: 30s
    concurrency: 4

gofunction:
  max_source_bytes: 262144
  max_per_project: 100
  execution_timeout: 10s
```

**Stay out of yaml (by design):** DuckLake ATTACH alias `lake`; HMAC algorithm; redaction denylist; Echo middleware order; reserved tenant/project UUIDs; Dev seed key string (code constant; only enabled by `dev_mode`).

**Stay env-only even after unification:**

| Env | Reason |
| --- | --- |
| `SIMPLEBASE_CONFIG_PATH` | bootstrap (cannot be inside the file it names) |
| `SIMPLEBASE_S3_ACCESS_KEY` / `SIMPLEBASE_S3_SECRET_KEY` | k8s secret / IAM companion |
| `SIMPLEBASE_AUTH_APIKEY_SECRET` | k8s secret |
| `SIMPLEBASE_LLM_PROVIDER_*_API_KEY` | k8s secret (if instance-level keys remain) |
| AWS standard vars (`AWS_ROLE_ARN`, `AWS_REGION`, …) | SDK chain; do not re-prefix |

**Stay script-only (not server yaml):** `SIMPLEBASE_DEV_UI_HOST/PORT`, `SIMPLEBASE_DEV_API_HOST/PORT`, `SIMPLEBASE_DEV_API_PROXY`. Optionally later a `dev:` yaml section read by `build.sh` with `yq` — not required.

**Stay client-only:** `SIMPLEBASE_URL`, `SIMPLEBASE_API_KEY`, `SIMPLEBASE_PROJECT_ID`, `SIMPLEBASE_DATABASE_ID`, `VITE_*`.

### 4.4 LLM / cloud agent

Recommended product rule (matches `llmgateway` package comment *and* current catalog design):

- **Runtime keys** come from catalog `CredentialRef` (settings UI / sys tables).
- `llm.providers` in yaml/env is an **optional instance bootstrap**: if catalog has no provider for a project, gateway may fall back to instance providers. Today neither path works (`Enabled` false, `credential` nil).
- Phase 1 of the later implementation PR: `llm.enabled` default **true**; construct `llmgateway.NewService` always when catalog exists; keep env providers as fallback resolver when `CredentialResolver` is nil.
- Do not auto-discover `OPENAI_API_KEY` unprefixed env (package forbids it).

### 4.5 Logging target

| Field | Target |
| --- | --- |
| `log_level` | yaml; `build.sh dev` may set env override to `debug` if unset |
| `log_format` | yaml; implement real console encoder (zap console) |
| `log_output` | stderr/stdout only for now; no rotation (deleted on purpose) |
| metrics HTTP path | yaml as today |
| system log flush/retention | yaml **wired** to ticker + default retention row |

### 4.6 Loader rewrite sketch (later PR, not now)

Replace dual `yamlConfig` + hand-written `applyYAML` with a single struct tagged `yaml:"..."`:

1. `defaults()` fills code defaults (today’s `loadFromEnv` defaults, without reading env).
2. Unmarshal YAML onto a copy (use `yaml.Node` or a pointer-field overlay so `false` is distinguishable — **use pointers or `json.RawMessage`/`IsSet` for bools**).
3. Walk a generated or explicit env-binding table for overrides (`SIMPLEBASE_HTTP_ADDRESS` → `&cfg.HTTP.Address`).
4. Reject secrets in yaml when `!DevMode`.
5. `Validate()`.

Keep `parseDuration` (`7d`). Unify `catalog_sync.debounce` as duration in yaml; still accept `SIMPLEBASE_DUCKLAKE_SYNC_DEBOUNCE_MS` as compat alias.

Drop unused `mapstructure` tags unless a decoder is actually added.

### 4.7 What `build.sh` should do after unification

- Load `.env` for secrets only (or stop sourcing it if `config.yaml` exists and document `set -a; source .env` for humans).
- Default `dev_mode` via **writing/expecting** `dev_mode: true` in a gitignored `config.yaml` generated from example, **or** export `SIMPLEBASE_DEV_MODE` only when yaml omits it.
- Stop blindly exporting `SIMPLEBASE_HTTP_ADDRESS` if yaml already has `http.address`; `--api-port` remains an explicit override (env overlay).
- Keep defaulting log level to debug for `dev`, but prefer writing it into generated `config.yaml` instead of exporting env (so yaml remains visible and overridable).
- Copy `config.example.yaml` → `config.yaml` on first `dev` if missing (prompt/warn).

---

## 5. Change list (later implementation; not this PR)

### 5.1 Files / packages

| Area | Files | Change |
| --- | --- | --- |
| Loader | `internal/config/config.go` | YAML-primary load; pointer bools; env bind table; fail on bad yaml; add fields in §4.3 |
| Tests | `internal/config/config_test.go`, `config_more_test.go` | yaml round-trip; env overlay; secret rejection in !dev; unknown-key policy |
| Assembly | `internal/app/app.go` | use `CacheMax*`; `LLM.Enabled`; `ShutdownTimeout`; start log/metrics flush tickers; optional schedulers from cfg |
| Logging | `internal/observability/logger.go`, `internal/log/log.go` | real console encoder; `log_output` |
| Cache | `internal/database/cache/manager.go` | keep fallback defaults but prefer filled config |
| DuckLake | `internal/database/ducklake/options.go` | treat config as already complete; `DefaultOptions` used only by tests |
| System DB | `internal/systemdb/{store,logs,metrics}.go` | honor flush intervals + `LogKeepDays` seed |
| Catalog list | `internal/catalog/service.go` | honor `hide_from_list` **or delete the field** |
| Auth | `internal/auth/service.go` | no API change; secret still from cfg |
| LLM | `internal/llmgateway/catalog_resolver.go`, `service.go` | instance-provider fallback; do not read raw `os.Getenv` |
| Cloud agent / cron | `internal/cloudagent/scheduler.go`, `internal/cronjob/scheduler.go` | optional cfg tick/concurrency |
| Go function | `internal/api/gofunction_handler.go`, `gofunction/frame.go` | optional cfg caps/timeout |
| Entrypoint | **restore** `cmd/simplebased/main.go` | `Load` → `New` → `RunWithSignal` |
| Docker | `Dockerfile` | already points at that binary |
| Examples | `config.example.yaml` | become loadable; delete “YAML not parsed”; add missing keys; no secrets |
| Env template | `.env.example` | shrink to secrets + `CONFIG_PATH` + optional overlays |
| Dev script | `build.sh` | respect yaml; optional debug log level; generate yaml if missing |
| Docs | `docs/ops/deployment.md`, `docs/ops/index.md`, `README.md`, `internal/README.md`, `internal/AGENTS.md` | one schema; fix stale `server_secret` / `access_key_id` / `default_plan`; fix README links |
| Workflows | `.github/workflows/build-image.yml` | still builds `cmd/http` (LessDB leftover) — out of config schema but blocks Docker; note only |
| Gitignore | `.gitignore` | keep ignoring `config.yaml` |

### 5.2 Phased steps (implementation PR sequence)

**Phase 0 — this PR:** land this plan. No code.

**Phase 1 — loader truth + docs**

- Rewrite `config.example.yaml` to match loader (even before loader rewrite).
- Fix example header; add `dev_mode`, `require_commit_message`, `kms_key_id`, LLM map.
- Make `Load` error on invalid YAML when a path was selected.
- Fix boolean omit/false (`*bool` or overlay flags).
- Add yaml for `cache_max_*`, `llm.enabled`, `s3.kms_key_id`, `http.shutdown_timeout`.
- Restore `cmd/simplebased/main.go` (flag-free) so Load is production-reachable.
- Tests: yaml-only happy path (dev_mode, no s3); env overlay; bad yaml fails.

**Phase 2 — consume dead fields**

- Pass `CacheMaxBytes` / `CacheMaxDatabases` (already plumbed in `app` — they are just always zero).
- Default `LLM.Enabled=true` or `true` when providers/catalog exist; construct gateway.
- Wire flush intervals **or delete** them from yaml.
- Wire `hide_from_list` **or delete**.
- Seed retention from `log_keep_days`.
- Implement console log format.
- Remove `database.max_idle` from schema (or document it is unused and delete).

**Phase 3 — secrets policy + build.sh**

- Reject secret yaml keys when `dev_mode=false`.
- Shrink `.env.example` to secrets.
- `build.sh dev`: generate yaml, set debug log if unset, only override HTTP addr when `--api-port` passed.
- Align `docs/ops/deployment.md` k8s snippet: ConfigMap for yaml + Secret for keys.

**Phase 4 (optional) — promote hardcoded knobs**

- `schedulers.*`, `gofunction.*` as in §4.3.
- Start `registry.Evictor` with `database.evict_interval`.
- Deprecate non-secret `SIMPLEBASE_*` (warn at boot listing which env vars overrode yaml).

### 5.3 Risks

| Risk | Mitigation |
| --- | --- |
| Existing deployments are env-only and have no `config.yaml` | Keep env overlay; cwd yaml is optional if all required env set; only fail when `CONFIG_PATH` points at a bad file |
| Boolean overlay already flipping `writable` in the wild | Phase 1 tests + changelog; use pointers |
| YAML containing secrets committed | gitignore `config.yaml`; Validate rejects secrets in !dev; Redacted() audits `has_*` |
| Breaking `debounce_ms` rename | accept both keys one release |
| LLM.Enabled=true suddenly constructs gateway | gateway already no-ops without providers; add test |
| Duplicate DuckLake defaults drift during transition | single `defaults()` in config package |
| Missing `cmd/simplebased` | restore in Phase 1 before claiming unification is testable end-to-end |
| `build.sh` HTTP clobber surprises yaml users | change only when `--api-port` is explicit |
| COS sample values in `.env.example` | keep as comments, not as yaml defaults, to avoid leaking tenant ids into new deploys |

### 5.4 Acceptance criteria (later implementation PR)

1. A process started with **only** a filled `config.yaml` (dev_mode, no env except maybe `CONFIG_PATH`) passes `Validate` and listens.
2. The same file with secrets omitted + k8s env for `SIMPLEBASE_S3_*` and `SIMPLEBASE_AUTH_APIKEY_SECRET` passes in writable non-dev.
3. `config.example.yaml` unmarshals with zero unknown-key surprises and is the unique field catalog.
4. `.env.example` lists secrets + overlay aliases only; no duplicate “source of truth” for `log_level` etc.
5. Setting `observability.log_level: debug` without env produces debug zap logs. `./build.sh dev` keeps a debug default but documents that it is an **env overlay**, not a second source of truth; yaml `log_level` applies when the script does not export the var.
6. `log_format: console` emits non-JSON lines.
7. `database.cache_max_bytes` in yaml is the value `cache.Manager` uses (not 1GiB unless that is the yaml value).
8. Invalid YAML exits non-zero with a parse error (no silent env fallback when a file was requested).
9. `cfg.Redacted()` / startup log still contain no secret material.
10. `go test ./internal/config ./internal/app ./cmd/simplebased` (once restored) covers yaml, env overlay, and secret policy.
11. Docs (`README`, `docs/ops/deployment.md`, `internal/AGENTS.md`) describe one precedence story.
12. Dead fields are either wired with tests or deleted from struct + examples.

---

## 6. Out of scope

- **This PR:** application code, loader changes, example rewrites, `build.sh` behavior, restoring `cmd/simplebased`. Only this plan file.
- Frontend `VITE_*` and JS SDK `SIMPLEBASE_URL` / `API_KEY` / `PROJECT_ID` / `DATABASE_ID`.
- Re-introducing log file rotation / syslog.
- Distributed config (etcd, feature flags).
- Changing DuckLake ATTACH alias or SQL guard rules.
- IAM/IRSA implementation beyond “empty keys → SDK default chain” (already exists).
- Fixing `.github/workflows/build-image.yml` still building `cmd/http` (related hygiene, separate cleanup).
- Migrating catalog LLM credentials into yaml (catalog remains per-project source of truth).

---

## Appendix A — Loader vs comments (quote)

`Load` comment (`internal/config/config.go`):

> 1. 若 SIMPLEBASE_CONFIG_PATH 指定的 YAML 文件（默认 config.yaml）存在，先解析为底；
> 2. 再用 SIMPLEBASE_ 前缀环境变量覆盖（环境变量优先级更高）。

Implementation: `cfg := loadFromEnv()` then `applyYAML` if file reads.

`config.example.yaml` header still claims YAML is not parsed — **treat as a documentation bug to fix in Phase 1**.

## Appendix B — Suggested env alias table (compat)

Keep names stable; bind them in one place in the later loader:

```
SIMPLEBASE_HTTP_ADDRESS            → http.address
SIMPLEBASE_HTTP_READ_TIMEOUT       → http.read_timeout
…
SIMPLEBASE_LOG_LEVEL               → observability.log_level
SIMPLEBASE_AUTH_APIKEY_SECRET      → auth.api_key_hash_secret
SIMPLEBASE_LLM_PROVIDER_<N>_API_KEY → llm.providers.<n>.api_key
```

No new env names. Do not add `SIMPLEBASE_CACHE_MAX_BYTES`; yaml-only for newly wired fields is acceptable if documented, but adding env aliases is cheap and matches “env overlay still wins”.

## Appendix C — Investigation notes

- Ripgrep: `os.Getenv` / `envStr` / `SIMPLEBASE_` only in `internal/config` for the server process (plus `build.sh` / docs / UI vite proxy). `gofunction/jsonrun_test.go` uses `os.Getenv` inside a **user script fixture**, not server config.
- `cmd/` on `origin/main`: **absent**. Historic LessDB `cmd/http`, `cmd/server`.
- `config.yaml` gitignored; not in the tree.
- `internal/AGENTS.md` already states the target (“配置只从 config.yaml / SIMPLEBASE_ 加载”) — the gap is implementation and docs drift, not a missing policy sentence.
