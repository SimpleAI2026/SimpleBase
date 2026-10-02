---
title: Go · 安装
order: 10
---

# 安装 Go SDK

Go SDK 位于仓库根模块 `github.com/linkxzhou/SimpleBase` 的 `packages/go-sdk`，当前没有独立 `go.mod`、版本标签或单独发布的包。推荐在同一仓库内使用 Go 1.25+：

```go
import gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
```

如在其他项目试用，可在该项目 `go.mod` 中引用根模块对应版本；本地开发可用指向 SimpleBase 仓库的 `replace`。该包本身只使用 Go 标准库，无须额外 SDK 依赖。运行测试：

```bash
go test ./packages/go-sdk
```

准备服务地址（HTTP origin）、项目 ID、具备目标权限的 API Key；SQL 和文档操作还需要用户数据库 ID。生产环境请通过环境变量或可信密钥管理服务注入 API Key。下一步：[快速开始](/docs/sdk/go-quickstart)。
