// 文件说明：internal/sbcli kv 命令与通用 flag 辅助（原 commands_resources.go 拆出）
package sbcli

import (
	"context"
	"flag"
	"io"
	"strings"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func databaseFlag(args []string) (string, error) {
	f, _, err := mustFlags(args, "database")
	if err != nil {
		return "", err
	}
	return need(f, "database")
}

func cmdKV(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	set := flag.NewFlagSet("kv", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	command := set.String("command", "", "")
	var argv []string
	set.Func("arg", "", func(s string) error {
		argv = append(argv, s)
		return nil
	})
	if err := set.Parse(args); err != nil {
		return nil, usagef("%s", err.Error())
	}
	if strings.TrimSpace(*command) == "" {
		return nil, usagef("--command is required")
	}
	raw, err := c.KVExec(ctx, append([]string{*command}, argv...))
	if err != nil {
		return nil, err
	}
	return decodeRaw(raw), nil
}
