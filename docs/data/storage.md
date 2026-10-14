---
title: 对象存储概览
order: 20
group: 对象存储
---

# 对象存储概览

每个项目都有独立的对象命名空间：SimpleBase 将项目 ID 拼入物理 S3 Key，API 返回的是去掉项目前缀的对象名。控制台「对象存储」可浏览、上传、删除和获取下载链接。

## 何时使用

使用对象存储保留文件、图片或导出的数据；结构化查询用[数据库](/docs/data)，短期状态用[Key-Value](/docs/data/kv)。对象不会作为 SQL 表自动出现。

## 最小示例

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/s3/objects" \
  -H "Authorization: Bearer $SB_KEY" \
  -F 'key=reports/hello.txt' -F 'file=@hello.txt'
curl -sS "$BASE/v1/projects/$PROJECT/s3/objects?prefix=reports/" \
  -H "Authorization: Bearer $SB_KEY"
```

上传需要 `database:write`；列举与预签名下载需要 `database:read`。设置 `BASE` / `PROJECT` / `SB_KEY` 参见[上手指南](/docs/getting-started/quickstart)。本示例要求先创建本地 `hello.txt` 且已配置可用的 S3（本地开发也可使用兼容服务）。

## 相关

完整请求参数见[对象存储 HTTP API](/docs/data/storage-api)，SDK 调用见 [JavaScript](/docs/sdk/storage) 与 [Go](/docs/sdk/go-storage)。生产环境请使用私有 bucket、TLS 和服务端加密；实际部署约束见[部署指南](/docs/ops/deployment)。

## 在控制台预览与下载

控制台「对象存储」表格的第一列是**文件**（对象名）。点击行内「打开」会弹出预览窗口，而不是直接跳转到新标签页：

| 类型 | 扩展名 | 预览方式 |
|---|---|---|
| 图片 | `jpg` `jpeg` `png` `webp` `svg`（另支持 `gif` `bmp` `ico` `avif`） | 直接显示图片；SVG 通过 `<img>` 加载，不会执行其中的脚本 |
| Markdown | `md` `markdown` | 默认显示渲染效果，可切换到「源码」 |
| 文本 | `txt` `log` `logs`（另支持 `json` `csv` `yaml` `yml` `xml` `ini` `conf` `sql`） | 等宽字体显示原文，可「复制内容」 |
| 其它 | 如 `zip` `pdf` | 提示暂不支持预览，仅提供下载 |

窗口底部的「下载」按钮可把文件保存到本地。类型按扩展名判断，不读取文件内容；文本类文件超过 1 MB 时不加载正文，请直接下载。预览用的临时链接约 15 分钟有效，每次打开窗口都会重新获取。

> **注意** 对象存储使用真实 S3 时，浏览器要读取文本需要 bucket 对控制台所在域名放行 `GET` 的 CORS。图片预览不受影响。若文本预览失败或下载被浏览器拦截，请参考[部署指南](/docs/ops/deployment)配置 CORS；下载失败时控制台会退回到新窗口打开临时链接。

## 使用场景

以下示例沿用[上手指南](/docs/getting-started/quickstart)中的 `BASE`、`PROJECT`、`SB_KEY`。

### 场景一：上传头像并分享临时链接

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/s3/objects" \
  -H "Authorization: Bearer $SB_KEY" \
  -F 'key=avatars/u-42.png' -F 'file=@avatar.png'

curl -sS "$BASE/v1/projects/$PROJECT/s3/presign?key=avatars%2Fu-42.png" \
  -H "Authorization: Bearer $SB_KEY"
```

返回 `{ "url": "…" }`，链接约 15 分钟有效，适合发给下游服务或前端临时展示。链接过期后重新调用即可，不要把它保存为长期地址。

### 场景二：按前缀组织目录

`key` 中的 `/` 只是命名约定，没有真实目录。用统一前缀区分业务，再用 `prefix` 过滤：

```bash
curl -sS "$BASE/v1/projects/$PROJECT/s3/objects?prefix=reports/2026-10/" \
  -H "Authorization: Bearer $SB_KEY"
```

控制台顶部的前缀输入框等价于这个参数，按回车刷新。`key` 不能以 `/` 开头，不能含 `\`、`.`、`..` 路径段，长度不超过 1024 字节。

### 场景三：保存导出报表并在控制台查看

由云函数、定时任务或外部脚本把导出的 CSV / Markdown 上传到 `reports/` 下，运维人员在控制台打开即可直接预览，不必下载：

```bash
printf 'date,requests\n2026-10-10,1200\n' > daily.csv
curl -sS -X POST "$BASE/v1/projects/$PROJECT/s3/objects" \
  -H "Authorization: Bearer $SB_KEY" \
  -F 'key=reports/daily.csv' -F 'file=@daily.csv'
```

### 场景四：清理过期文件

先按前缀列出，再逐个删除。删除需要 `database:write`：

```bash
curl -sS -X DELETE "$BASE/v1/projects/$PROJECT/s3/objects?key=reports%2Fdaily.csv" \
  -H "Authorization: Bearer $SB_KEY"
```

控制台里同样可以逐行删除，删除前会二次确认。

### 场景五：用 JS SDK 上传

```ts
import { createClient } from '@simplebase/sdk'

const sb = createClient({
  url: process.env.SIMPLEBASE_URL!,
  apiKey: process.env.SIMPLEBASE_API_KEY!,
  projectId: process.env.SIMPLEBASE_PROJECT_ID!,
})

await sb.storage.upload('notes/hello.md', '# Hello', { contentType: 'text/markdown', filename: 'hello.md' })
const { url } = await sb.storage.presign('notes/hello.md')
console.log(url)
```

## 限制

| 项 | 说明 |
|---|---|
| 单次上传 | 控制台限制 50 MB；服务端另有请求体上限，以部署配置为准 |
| 权限 | 列表与预签名需要 `database:read`，上传与删除需要 `database:write` |
| 只读实例 | 上传与删除会返回 503 `writer_unavailable` |
| 项目隔离 | 对象名按项目隔离，API 返回的对象名不含项目前缀 |
