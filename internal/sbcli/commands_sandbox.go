// 文件说明：internal/sbcli sandbox 命令实现（原 commands_resources.go 拆出）
package sbcli

import (
	"context"
	"io"
	"strconv"
	"strings"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func cmdSandboxCaps(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	return c.SandboxCapabilities(ctx)
}

func cmdSandboxList(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "state", "limit")
	if err != nil {
		return nil, err
	}
	limit, _ := strconv.Atoi(*f["limit"])
	return c.ListSandboxes(ctx, *f["state"], limit)
}

func cmdSandboxGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	return c.GetSandbox(ctx, id)
}

func cmdSandboxCreate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "template", "name", "timeout")
	if err != nil {
		return nil, err
	}
	in := gosdk.CreateSandboxInput{TemplateID: *f["template"], Name: *f["name"]}
	if s := strings.TrimSpace(*f["timeout"]); s != "" {
		n, perr := strconv.ParseInt(s, 10, 64)
		if perr != nil {
			return nil, usagef("--timeout must be an integer")
		}
		in.TimeoutS = n
	}
	return c.CreateSandbox(ctx, in)
}

func cmdSandboxDelete(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	if err := c.DeleteSandbox(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdSandboxConnect(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "id", "timeout")
	if err != nil {
		return nil, err
	}
	id, err := need(f, "id")
	if err != nil {
		return nil, err
	}
	timeout := int64(0)
	if s := strings.TrimSpace(*f["timeout"]); s != "" {
		n, perr := strconv.ParseInt(s, 10, 64)
		if perr != nil {
			return nil, usagef("--timeout must be an integer")
		}
		timeout = n
	}
	return c.ConnectSandbox(ctx, id, timeout)
}

func cmdSandboxPause(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	id, err := one(args, "id")
	if err != nil {
		return nil, err
	}
	if err := c.PauseSandbox(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}
