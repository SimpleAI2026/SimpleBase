// Package sbcli is the SimpleBase CLI. cmd/simplebase is a thin main.
// The assistant host may call RunIO in-process; operators run the binary.
package sbcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

// Run is the process entrypoint.
func Run(args []string) int {
	return RunIO(context.Background(), args, os.Environ(), os.Stdout, os.Stderr, os.Stdin)
}

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
	if cfg.Agent {
		data = stripAgentSecrets(data)
	}
	writeOK(stdout, data)
	return exitOK
}

type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

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
		return loadedConfig{}, nil, exitConfig, errors.New("endpoint, token, and project are required")
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
