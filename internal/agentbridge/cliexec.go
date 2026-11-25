// 文件说明：cliexec.go — CLIRunner 端口与 SimpleBase CLI 子进程适配
//
// 子进程执行与输出限幅的机制在 gocloudagent/cloudagent.ProcessRunner；
// 这里只放 SimpleBase 的默认值（二进制名、临时目录前缀、每路 64KiB 上限）。

package agentbridge

import (
	"context"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
)

// cliOutputLimit 是 simplebase CLI 每路输出的字节上限。
const cliOutputLimit = cloudagent.DefaultProcessOutputLimit

// CLIRunner executes one simplebase invocation.
// stdin 由宿主内联传入（planv6.0 D3：助手模式不读宿主机文件）。
type CLIRunner interface {
	Run(ctx context.Context, argv []string, env []string, stdin string) (stdout, stderr string, exitCode int, err error)
}

// SubprocessCLI execs the simplebase binary. The environment is replaced, not inherited.
type SubprocessCLI struct {
	Path string
}

func (s SubprocessCLI) Run(ctx context.Context, argv []string, env []string, stdin string) (string, string, int, error) {
	path := strings.TrimSpace(s.Path)
	if path == "" {
		path = "simplebase"
	}
	return cloudagent.ProcessRunner{Path: path, OutputLimit: cliOutputLimit, TempDirPrefix: "sbcli-run-"}.Run(ctx, argv, env, stdin)
}
