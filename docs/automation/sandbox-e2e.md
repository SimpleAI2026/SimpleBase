---
title: 云沙盒 e2e 与 CI
order: 32
group: 云沙盒
---

# 在 e2e / CI 中运行代码

首选一次性 `POST /sandboxes/run`：按需创建沙盒，依次写入 `/workspace` 文件、同步执行、返回结果，再清理。失败后仍触发清理。真实代码执行需要 `backend=local`（本机进程隔离，命令用的是宿主机上的工具链，见 [云沙盒概览](/docs/automation/sandbox)）；`backend=fake`（须 `SIMPLEBASE_SANDBOX_ALLOW_FAKE=1`）只用于测试控制面和简单命令。命令里用相对路径（如 `python3 test.py`），命令看不到字面量 `/workspace`。

## curl

```bash
curl -sS -X POST "$BASE_URL/v1/projects/$PROJECT_ID/sandboxes/run" \
  -H "Authorization: Bearer $API_KEY" -H 'Content-Type: application/json' \
  -d '{"files":[{"path":"/workspace/test.py","content":"assert 1 + 1 == 2\nprint(\"ok\")"}],"command":"python3 test.py","timeout_s":60}'
```

成功返回 `{ "exit_code": 0, "stdout": "ok\n", ... }`。测试断言应同时检查 HTTP 2xx、`exit_code === 0` 和 `timed_out === false`。命令失败不是 HTTP 错误；只有平台、权限或输入错误才返回非 2xx。

## JS SDK

```ts
import { createClient } from '@simplebase/sdk'
const client = createClient({ url: process.env.BASE_URL!, apiKey: process.env.API_KEY!, projectId: process.env.PROJECT_ID! })
const result = await client.sandboxes.run({
  files: [{ path: '/workspace/test.py', content: 'assert 1+1==2\nprint("ok")' }],
  command: 'python3 test.py'
})
if (result.timed_out || result.exit_code !== 0) throw new Error(result.stderr || `exit ${result.exit_code}`)
```

## Go SDK

```go
client, err := gosdk.NewClient(gosdk.Options{URL: baseURL, APIKey: apiKey, ProjectID: projectID})
if err != nil { return err }
result, err := client.RunSandbox(ctx, gosdk.SandboxRunInput{
    Files: []gosdk.SandboxRunFile{{Path: "/workspace/test.py", Content: "print('ok')\n"}},
    SandboxExecInput: gosdk.SandboxExecInput{Command: "python3 test.py", TimeoutS: 60},
})
if err != nil { return err }
if result.TimedOut || result.ExitCode != 0 { return fmt.Errorf("sandbox failed: %s", result.Stderr) }
```

调试失败任务可传 `keep:true`，响应包含 `sandbox_id`，在控制台「云沙盒」中查看文件；记得手工删除。创建持久资源时 `Idempotency-Key` 只由单进程内存缓存，重启后可能重新创建，请在 e2e 结束时显式删除资源。
