# guzzle/guzzle（`src/Utils.php`）：simplify-code 分析报告

- **仓库**：https://github.com/guzzle/guzzle
- **Commit**：`93939470950a9b11e2e84204166ef5e048c55fe4`
- **范围**：`src/Utils.php`（297 行，`final class Utils`，全文）
- **约束**：`php: ^7.4 || ^8.0`（不能用 `match`、构造器提升、nullsafe `?->`）；`declare(strict_types=1)`；PHP-CS-Fixer
- **技能参考**：`languages-tier1.md#PHP`、`rules.md` §5/§6/§20、`behavior-traps.md`

## Summary
`chooseHandler` 这一组私有方法近期新增了连接上限/传输共享逻辑，出现了**同一校验函数被调用 3 次**的重复。找到 1 个 P2、2 个 P3。box 无 PHP，未运行测试。

## 代码在做什么
`chooseHandler()` 根据扩展可用性、`allow_url_fopen`、传输共享模式与连接上限选项挑选 HTTP handler（cURL multi / cURL / stream / 代理组合），不满足要求时抛出带明确文案的异常（文案为公共契约）。`headersFromLines`、`normalizeProtocols` 等为解析工具。

## 发现（按优先级）

### P2-1 `connectionCapOptions($handlerOptions)` 计算并校验 3 次 — `Utils.php:77(→135)`、`111`、`156`
- **规则**：§6 同一知识/计算重复；§3 `hasConnectionCapOptions` 是只包一层的浅模块
- `connectionCapOptions()` 会校验并在非法时抛 `InvalidArgumentException`，第一次调用（经 `hasConnectionCapOptions`）就已抛出，后两次纯属重复计算。
- **After（示意）**
  ```php
  $sharingMode = CurlShareHandleState::normalizeMode(...);          // 顺序不变：先校验共享模式
  $sharingRequired = self::isTransportSharingRequired($sharingMode);
  $capOptions = self::connectionCapOptions($handlerOptions);         // 只算一次
  $connectionCapsRequired = $capOptions !== [];
  ...
  $handler = self::createCurlHandler($sharingMode, $handlerOptions, $capOptions);
  ...
  return self::addStreamHandler($handler, $sharingMode, $capOptions);
  ```
  并删除 `hasConnectionCapOptions()`。
- **行为**：异常类型/文案/抛出时机（第一次计算处）不变；只改私有方法签名。
- **风险**：低；**行为保持**：是（推理，未运行）。

### P3-1 重复的组合条件 — `Utils.php:81`、`85`
`$connectionCapsRequired && !$sharedPoolCapsSupported` 出现两次，可命名为 `$capsUnsupportedOnSharedPool`，再分别与 `PERSISTENT_REQUIRE` / `PERSISTENT_PREFER` 比较（examples A34 提取谓词）。

### P3-2 `trim` 显式字符表等于默认值 — `Utils.php:37`
`" \n\r\t\0\x0B"` 正是 PHP `trim()` 的默认字符集（`\x0B` 即 `\v`），因此 `\trim($parts[0])` 等价。不过显式字符表也起到"文档化"作用，维护者可能有意保留——列为可选。

## 刻意保留
- `chooseHandler` 中多个 `throw` 分支：每个对应不同环境组合与不同错误文案，合并会损失诊断信息。
- `addStreamHandler` 中 `$handler ? … : …`（241）：`?callable` 上的真值判断与 `!== null` 对所有合法 callable 等价，但改动无收益。
- `normalizeProtocols`：已是校验 + 去重的最小形态（`array_keys` 保序去重）。
- `isset($parts[1]) ? … : null`（37）：PHP 7.4 可用 `??`，但这里需要对存在的值再 `trim`，`??` 不能直接表达，保持。

## 验证
- 仅阅读与推理；box 无 PHP/Composer，未运行 PHPUnit。

## 置信度
中高。
