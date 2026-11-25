# internal/utils

SimpleBase 宿主私有的通用纯函数。数据库运行库与宿主共用的能力已下沉到 `godatabase` 公共包，本目录只保留不属于数据库公共 API 的宿主工具。

## 入库标准

代码放入 `internal/utils/<pkg>` 必须同时满足：

1. **零业务依赖**：只 import 标准库，不得 import `catalog`、`auth`、`systemdb`、`api` 或 `godatabase`。
2. **有复用事实**：至少两处调用，或已存在多份同义实现。
3. **语义单一**：一个包只做一件事，不用参数开关混合不同语义。
4. **属于宿主私有边界**：若 `godatabase` 与宿主都需要，应直接复用 `godatabase` 公共包，而不是在这里新增副本。

## 现有包

| 包 | 职责 |
| --- | --- |
| `crontab` | 受限 5 字段 cron 解析与下次触发计算（UTC、分钟粒度） |
| `strutil` | 空白/大小写规范化、字节/rune 截断与 CSV 列表切分 |
| `mask` | 敏感字段名判定与密钥掩码 |
| `keyedlock` | 按字符串键串行的互斥锁：解锁即回收键，等待可被 ctx 取消；`systemdb` 与 `api` 共用 |

## 已下沉的公共能力

| 包 | 职责 |
| --- | --- |
| `godatabase/atomicfile` | 同目录临时文件 + rename 原子写 |
| `godatabase/hashx` | SHA-256 十六进制摘要 |
| `godatabase/limit` | 分页参数归一化 |
| `godatabase/singleflight` | 泛型按 key 单飞，不缓存结果 |
| `godatabase/sqlval` | database/sql 参数值归一化 |
| `godatabase/sqlx` | SQL 标识符与字面量转义 |

这些包可被其他平台直接调用。测试记录实现位于 `godatabase/internal/testutil`，不属于公共生产 API。沙盒共用的环境校验与 UTF-8 输出截断位于 `gosandbox/vm`，同样不保留宿主副本。

## 不要放这里

- 测试基础设施、带 `httptest` 或具体驱动依赖的代码。
- 适配层、业务白名单、HTTP 协议或 `go:embed` 装配。
- 带生命周期的 `lease`、`registry`、`cache`。
- 带 TTL、容量或负缓存语义的缓存；`singleflight` 只做在途调用合并。
- HMAC、多字段凭据指纹等带领域规则的摘要。

`gocloudagent`、`gosandbox`、`gofunction` 是嵌套独立模块，不能 import 根模块 `internal/...`。不得为了复用本目录而制造反向依赖；是否让其依赖 `godatabase` 必须遵守各自模块的依赖约束。
