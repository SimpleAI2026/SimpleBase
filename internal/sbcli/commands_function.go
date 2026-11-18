// 文件说明：internal/sbcli function 命令实现（原 commands_resources.go 拆出）
package sbcli

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func cmdFunctionList(ctx context.Context, c *gosdk.Client, _ loadedConfig, _ []string, _ io.Reader) (any, error) {
	raw, err := c.ListFunctions(ctx)
	return decodeRaw(raw), err
}

func cmdFunctionGet(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	name, err := one(args, "name")
	if err != nil {
		return nil, err
	}
	raw, err := c.GetFunction(ctx, name)
	return decodeRaw(raw), err
}

func cmdFunctionCreate(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, stdin io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "file", "description", "source-b64", "activate")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	path, err := need(f, "file")
	if err != nil {
		return nil, err
	}
	activate := true
	if s := strings.TrimSpace(*f["activate"]); s != "" {
		activate, err = strconv.ParseBool(s)
		if err != nil {
			return nil, usagef("--activate must be true or false")
		}
	}
	src, err := readSource(path, *f["source-b64"], stdin, cfg.Agent)
	if err != nil {
		return nil, err
	}
	raw, err := c.CreateFunctionWith(ctx, name, string(src), *f["description"], activate)
	return decodeRaw(raw), err
}

func cmdFunctionUpdate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "description")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	raw, err := c.UpdateFunction(ctx, name, *f["description"])
	return decodeRaw(raw), err
}

func cmdFunctionDelete(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	name, err := one(args, "name")
	if err != nil {
		return nil, err
	}
	if err := c.DeleteFunction(ctx, name); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdFunctionVersions(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, stdin io.Reader) (any, error) {
	// function versions list|get|create  --name --version --file
	if len(args) == 0 {
		return nil, usagef("usage: function versions <list|get|create>")
	}
	sub := args[0]
	f, _, err := mustFlags(args[1:], "name", "version", "file", "note", "source-b64")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	switch sub {
	case "list":
		raw, err := c.ListFunctionVersions(ctx, name)
		return decodeRaw(raw), err
	case "get":
		ver, err := need(f, "version")
		if err != nil {
			return nil, err
		}
		raw, err := c.GetFunctionVersion(ctx, name, ver)
		return decodeRaw(raw), err
	case "create":
		path, err := need(f, "file")
		if err != nil {
			return nil, err
		}
		var b64 string
		if f["source-b64"] != nil {
			b64 = *f["source-b64"]
		}
		src, err := readSource(path, b64, stdin, cfg.Agent)
		if err != nil {
			return nil, err
		}
		raw, err := c.CreateFunctionVersion(ctx, name, string(src), *f["note"], false)
		return decodeRaw(raw), err
	default:
		return nil, usagef("unknown function versions action")
	}
}

func cmdFunctionActivate(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "version")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	ver, err := need(f, "version")
	if err != nil {
		return nil, err
	}
	raw, err := c.ActivateFunctionVersion(ctx, name, ver)
	return decodeRaw(raw), err
}

func cmdFunctionTest(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "version", "export", "input")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	ver, err := need(f, "version")
	if err != nil {
		return nil, err
	}
	export, err := need(f, "export")
	if err != nil {
		return nil, err
	}
	body := json.RawMessage(*f["input"])
	raw, err := c.TestFunction(ctx, name, ver, export, body)
	return decodeRaw(raw), err
}

func cmdFunctionInvoke(ctx context.Context, c *gosdk.Client, _ loadedConfig, args []string, _ io.Reader) (any, error) {
	f, _, err := mustFlags(args, "name", "export", "input")
	if err != nil {
		return nil, err
	}
	name, err := need(f, "name")
	if err != nil {
		return nil, err
	}
	export, err := need(f, "export")
	if err != nil {
		return nil, err
	}
	body := json.RawMessage(*f["input"])
	raw, err := c.InvokeFunction(ctx, name, export, body)
	return decodeRaw(raw), err
}
