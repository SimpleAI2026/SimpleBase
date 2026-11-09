// 文件说明：cli.go — internal/agentbridge 中 InProcessCLI 等符号的实现
//
// 用法
//
//	type InProcessCLI（导出类型，同包与上层均可使用）
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/cli.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, internal/sbcli, internal/utils/strutil
//	[同包] command.go, cron_agent.go, delegation.go, errors.go, principal.go 等 6 个
//
// 主要符号
//
//	type InProcessCLI struct {…}
//	... 另有 3 个顶层符号

package agentbridge

import (
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/internal/sbcli"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// InProcessCLI 在进程内调用 sbcli.RunIO，实现 gocloudagent.CLIRunner。
//
// 为什么放在宿主：InProcessCLI 直接链接 internal/sbcli（进而是整个资源 CLI
// 与 DuckDB），放进独立模块会让 gocloudagent 拖进宿主服务依赖。它只用于测试与
// 单进程装配。
type InProcessCLI struct{}

// Run 执行一次 simplebase 命令并返回截断后的输出。
func (InProcessCLI) Run(ctx context.Context, argv []string, env []string, stdin string) (string, string, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var out, errb bytes.Buffer
	var r io.Reader = strings.NewReader("")
	if stdin != "" {
		r = strings.NewReader(stdin)
	}
	code := sbcli.RunIO(ctx, argv, env, &out, &errb, r)
	return limitOut(out.String()), limitOut(errb.String()), code, nil
}

const cliOutputLimit = 64 << 10

func limitOut(s string) string {
	return strutil.Truncate(s, cliOutputLimit)
}

// 编译期断言：适配器满足模块的 CLI 运行接口。
var _ gocloudagent.CLIRunner = InProcessCLI{}
