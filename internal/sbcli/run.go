// 文件说明：run.go — is the SimpleBase resource CLI used by cmd/simplebase
//
// 用法
//
//	RunIO(ctx context.Context, args, environ []string, stdout, stderr io.Writer, stdin ...  // RunIO executes one CLI invocation. environ is explicit so tests stay isolated.
//	DispatchKnown(resource, action string) bool  // DispatchKnown 报告 resource+action 是否为可分发的 CLI 命令
//	同包调用：usagef() → collection_transfer.go, commands.go 等；rawOutput → collection_transfer.go；Error() → commands.go
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/agentbridge
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/sbcli/run.go
//	         │ 依赖
//	         ▼
//	[下游] packages/go-sdk
//	[同包] collection_transfer.go, commands.go, config.go, migrations.go, output.go 等 6 个
//	[关键引用] usagef() ← collection_transfer.go, commands.go, migrations.go
//
// 主要符号
//
//	RunIO(ctx context.Context, args, environ []string, stdout, stderr io.Writer, stdin ...
//	DispatchKnown(resource, action string) bool
//	... 另有 5 个顶层符号

package sbcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

// RunIO executes one CLI invocation. environ is explicit so tests stay isolated.
func RunIO(ctx context.Context, args, environ []string, stdout, stderr io.Writer, stdin io.Reader) int {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, rest, code, err := parseArgs(args, environ)
	if err != nil {
		if code == exitUsage {
			writeFail(stderr, "usage", err.Error(), 0, "")
		} else {
			c := "config_missing"
			if errors.Is(err, errInsecureConfig) {
				c = "config_insecure"
			}
			if err.Error() == "project_mismatch" {
				c = "project_mismatch"
			}
			writeFail(stderr, c, err.Error(), 0, "")
		}
		return code
	}
	if len(rest) < 2 {
		writeFail(stderr, "usage", "usage: simplebase <resource> <action> [--flags]", 0, "")
		return exitUsage
	}
	client, err := gosdk.NewClient(gosdk.Options{URL: cfg.Endpoint, Token: cfg.Token, ProjectID: cfg.ProjectID})
	if err != nil {
		writeFail(stderr, "config_missing", err.Error(), 0, "")
		return exitConfig
	}
	fn := dispatch(rest[0], rest[1])
	if fn == nil {
		writeFail(stderr, "usage", "unknown command "+rest[0]+" "+rest[1], 0, "")
		return exitUsage
	}
	data, err := fn(ctx, client, cfg, rest[2:], stdin)
	if err != nil {
		var api *gosdk.APIError
		if errors.As(err, &api) {
			writeFail(stderr, api.Code, api.Message, api.Status, api.RequestID)
			return exitAPI
		}
		var ue *usageError
		if errors.As(err, &ue) {
			writeFail(stderr, "usage", ue.Error(), 0, "")
			return exitUsage
		}
		writeFail(stderr, "usage", err.Error(), 0, "")
		return exitUsage
	}
	if raw, ok := data.(rawOutput); ok {
		if cfg.Agent {
			writeFail(stderr, "agent_raw_export_disabled", "raw collection export is unavailable in agent mode", 0, "")
			return exitUsage
		}
		if _, err := stdout.Write(raw.data); err != nil {
			writeFail(stderr, "output_error", err.Error(), 0, "")
			return exitUsage
		}
		return exitOK
	}
	if cfg.Agent {
		data = stripAgentSecrets(data)
	}
	writeOK(stdout, data)
	return exitOK
}

type rawOutput struct{ data []byte }

type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// DispatchKnown 报告 resource+action 是否为可分发的 CLI 命令
// （planv6.0 §3.6：供 skill 文本对账测试使用）。
func DispatchKnown(resource, action string) bool {
	return dispatch(resource, action) != nil
}

func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

func parseArgs(args, environ []string) (loadedConfig, []string, int, error) {
	fileCfg, err := loadFileConfig(environ)
	if err != nil {
		return loadedConfig{}, nil, exitConfig, err
	}
	cfg := applyEnv(fileCfg, environ)
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "--") {
			rest = append(rest, args[i:]...)
			break
		}
		name, val := a, ""
		if eq := strings.IndexByte(a, '='); eq >= 0 {
			name, val = a[:eq], a[eq+1:]
		} else {
			if i+1 >= len(args) {
				return loadedConfig{}, nil, exitUsage, usagef("flag %s needs a value", name)
			}
			i++
			val = args[i]
		}
		switch name {
		case "--endpoint":
			cfg.Endpoint = val
		case "--token":
			cfg.Token = val
		case "--project":
			cfg.ProjectID = val
		case "--output":
			cfg.Output = val
		default:
			return loadedConfig{}, nil, exitUsage, usagef("unknown flag %s", name)
		}
	}
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.Token) == "" || strings.TrimSpace(cfg.ProjectID) == "" {
		return loadedConfig{}, nil, exitConfig, errors.New("endpoint, token, and project are required " +
			"(set SIMPLEBASE_URL / SIMPLEBASE_TOKEN / SIMPLEBASE_PROJECT_ID or the simplebase config file; " +
			"shells started by the local gocloudagent do not inherit SIMPLEBASE_* variables, use the config file there)")
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return loadedConfig{}, nil, exitConfig, errors.New("endpoint must be an http(s) origin")
	}
	if bound := delegationProject(cfg.Token); bound != "" && bound != cfg.ProjectID {
		return loadedConfig{}, nil, exitConfig, errors.New("project_mismatch")
	}
	if cfg.Output == "" {
		cfg.Output = "json"
	}
	return cfg, rest, 0, nil
}
