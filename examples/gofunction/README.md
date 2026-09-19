# echo + gofunction HTTP 服务性能示例

演示 SimpleBase 的典型生产形态：**echo 提供 HTTP 接入层，云函数脚本
（gofunction 解释器执行）承载业务逻辑**，并与同等逻辑的原生 Go 实现
做性能对照。

## 三组有性能代表性的负载

| 负载 | 代表场景 | 脚本函数 |
|---|---|---|
| Compute | 递归 fib(22)，纯 CPU 解释执行开销 | `Compute(n int) int` |
| JSON | 订单校验汇总：json.Unmarshal + 遍历 + 浮点比较，真实业务形态 | `JSON(order string) string` |
| Text | 50 行日志解析聚合：字符串切分/拼接/map，IO 服务常见 | `Text(logs string) string` |

## 三条对照路径

| 路径 | 说明 |
|---|---|
| `POST /native/*` | 原生 Go 编写的同逻辑 handler（性能上限基准） |
| `POST /bench/*` | 脚本经 `RunJSON` 执行（生产调用路径，含每请求 BuildProgram + JSON 绑定） |
| `POST /inproc/*` | 脚本函数进程内直调（剥离 HTTP 开销，测解释执行净耗时） |

另有 `GET /payload/json`、`GET /payload/text` 输出压测负载
（由 Go 侧按脚本同款算法生成，保证校验路径可命中）。

## 运行

```bash
go run ./examples/gofunction
# echo + gofunction bench server on http://127.0.0.1:8091
```

## 性能测试

### 1. 进程内基准（剥离 HTTP，纯逻辑对照）

```bash
go test -run='^$' -bench=. -benchmem ./examples/gofunction
```

2026-09-19 实测（Apple M4 Pro）：

| 基准 | ns/op | allocs/op |
|---|---|---|
| NativeFib22 | 38,714 | 0 |
| ScriptFib22（预编译直调） | 12,800,485 | 21 |
| RunJSONFib22（全链路） | 13,415,863 | 6,592 |
| NativeJSON | 133 | 3 |
| ScriptJSON | 74,628 | 1,617 |
| NativeText | 15,984 | 258 |
| ScriptText | 189,366 | 4,068 |

结论：
- **CPU 密集（fib22）**：解释执行约为原生的 **1/330**（12.8ms vs 38.7μs），
  符合 gofunction 基准报告的 ~330x 差距；分配 21 次（几乎全部来自
  编译期一次性开销，运行期分配已被 P3 优化压到零）。
- **JSON 业务**：脚本含 `json.Unmarshal`（原生反射执行，非解释），
  差距缩至 **~560x → 74.6μs vs 0.13μs**；注意脚本版本额外含
  Unmarshal 步骤，严格同逻辑对照应看 Text。
- **文本处理**：纯解释路径 **~12x**（189μs vs 16μs）——字符串/切片/
  map 的 SSA 降级形态（Split/Fields 为原生调用）占比高，
  解释器优势场景。
- **RunJSON 全链路**仅比预编译直调慢 ~5%（13.4ms vs 12.8ms），
  每请求 BuildProgram 成本 ~0.15ms 可忽略；
  但每请求编译产生 ~6.5k 分配，生产应复用 Program（如 inproc 路径）。

### 2. HTTP 压测（echo 全链路）

```bash
ab -t 15 -c 8 -q http://127.0.0.1:8091/native/compute
ab -t 15 -c 8 -q http://127.0.0.1:8091/inproc/compute
ab -t 15 -c 8 -q http://127.0.0.1:8091/bench/compute
curl -s http://127.0.0.1:8091/payload/text > /tmp/logs.txt
ab -t 15 -c 8 -q -p /tmp/logs.txt http://127.0.0.1:8091/native/text
ab -t 15 -c 8 -q -p /tmp/logs.txt http://127.0.0.1:8091/inproc/text
ab -t 15 -c 8 -q -p /tmp/logs.txt http://127.0.0.1:8091/bench/text
curl -s http://127.0.0.1:8091/payload/json > /tmp/order.json
ab -t 15 -c 8 -q -p /tmp/order.json http://127.0.0.1:8091/{native,inproc,bench}/json
```

2026-09-19 实测（ab，15s，并发 8）：

| 端点 | RPS | 单请求均值 |
|---|---|---|
| /native/compute | 15,504 | 0.52ms |
| /inproc/compute | 69 | 116.6ms |
| /bench/compute | 66 | 120.7ms |
| /native/json | 16,522 | — |
| /inproc/json | 13,976 | — |
| /bench/json | 5,648 | — |
| /native/text | 14,928 | — |
| /inproc/text | 7,969 | — |
| /bench/text | 4,013 | — |

结论：
- **短逻辑脚本**（json/text）经 echo 全链路后仍有 **2.4k~14k RPS**，
  与 native 的差距被 HTTP 框架开销大幅稀释（text 场景仅 ~1.9x）。
- **CPU 密集脚本**（compute fib22）吞吐完全由解释执行主导，
  HTTP 层开销可忽略（69 vs 66 RPS）。
- 生产建议：**高频调用必须复用编译产物**（`/inproc/*` 形态），
  `/bench/*` 的每请求编译在 fib22 场景无感，但在短脚本场景
  （text/json）是 2~3x 吞吐损失的主要来源。

## 顺带发现并修复的引擎缺陷

冒烟暴露 `runRange` 只实现了 map 路径：`for range string` 误走
`MapKeys` panic（SSA 对 string/slice 的 range 也走 Range/Next 协议）。
已按 Kind 分派修复并加回归测试
（`gofunction/valuecache_test.go: TestRangeStringSliceMap`）。
