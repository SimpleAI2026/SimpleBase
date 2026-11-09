# 预期结果

| 步骤 | 通过条件 |
|---|---|
| 前置探测 | `sandbox capabilities` 返回 `available=false` → 整体 SKIP，退出码 0，jsonl 记录 `ok=true` + `detail=skipped: ...` |
| capabilities | `available=true` 且 `backend` 非空（仅在隔离能力存在时执行） |
| create | `sandboxID` 非空，`state` 或 `x_simplebase.status` 非空 |
| get | `sandboxID` 与创建一致 |
| list | 列表里恰好 1 个该 id |
| pause | `ok=true` |
| delete | `ok=true`，随后列表为 0 个 |

> 本机 macOS（DSH 沙箱内）`sandbox-exec: Operation not permitted`，所以预期结果是
> **SKIP**（不计 FAIL）。在 Linux bubblewrap 主机上应走到最后一行 PASS。
