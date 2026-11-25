// 文件说明：tools_sandbox.go — 云沙盒内置工具（sandbox_*）

package agentbridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

type sandboxExecInput struct {
	Cmd  string   `json:"cmd" jsonschema:"description=Executable name, passed literally without a shell"`
	Args []string `json:"args,omitempty" jsonschema:"description=Optional argv arguments"`
}

type sandboxShellInput struct {
	Command string `json:"command" jsonschema:"description=Shell command run via /bin/sh -c (max 4096 bytes)"`
}

type sandboxReadFileInput struct {
	Path string `json:"path" jsonschema:"description=Absolute file path under /workspace"`
}

type sandboxWriteFileInput struct {
	Path    string `json:"path" jsonschema:"description=Absolute file path under /workspace"`
	Content string `json:"content" jsonschema:"description=File content to write"`
}

// maxShellCommandBytes 限制 sandbox_shell 命令长度（cloud-agent-sandbox-plan §7.1）。
const maxShellCommandBytes = 4096

// buildSandboxTools 装配四个云沙盒工具（cloud-agent-sandbox-plan §7）。
// Sandbox 为 nil 或不可用时返回明确错误，工具仍注册但调用即失败。
func buildSandboxTools(out *[]tool.BaseTool, want map[string]bool, deps toolDeps) error {
	anyWanted := false
	for _, id := range sandboxToolIDs() {
		if want[id] {
			anyWanted = true
			break
		}
	}
	if !anyWanted {
		return nil
	}
	if deps.Sandbox == nil || !deps.Sandbox.Available() {
		return fmt.Errorf("sandbox is not enabled; sandbox tools are unavailable")
	}
	add := func(t tool.InvokableTool, err error) error {
		if err != nil {
			return err
		}
		*out = append(*out, t)
		return nil
	}
	if want[ToolSandboxExec] {
		if err := add(utils.InferTool(ToolSandboxExec, "Run one command (argv, no shell) in this thread's sandbox. The current directory is the sandbox workspace; use relative paths.",
			func(ctx context.Context, in sandboxExecInput) (string, error) {
				rc, err := requireSandboxRun(ctx)
				if err != nil {
					return "", err
				}
				if strings.TrimSpace(in.Cmd) == "" {
					return "", fmt.Errorf("cmd is required")
				}
				out, err := deps.Sandbox.Exec(ctx, rc.ProjectID, rc.ThreadID, in.Cmd, in.Args)
				if err != nil {
					return "", err
				}
				return agentcore.MarshalToolJSON(sandboxOutputJSON(out))
			})); err != nil {
			return err
		}
	}
	if want[ToolSandboxShell] {
		if err := add(utils.InferTool(ToolSandboxShell, "Run a shell command (/bin/sh -c) in this thread's sandbox. The current directory is the sandbox workspace; use relative paths.",
			func(ctx context.Context, in sandboxShellInput) (string, error) {
				rc, err := requireSandboxRun(ctx)
				if err != nil {
					return "", err
				}
				if strings.TrimSpace(in.Command) == "" {
					return "", fmt.Errorf("command is required")
				}
				if len(in.Command) > maxShellCommandBytes {
					return "", fmt.Errorf("command exceeds %d bytes", maxShellCommandBytes)
				}
				out, err := deps.Sandbox.Shell(ctx, rc.ProjectID, rc.ThreadID, in.Command)
				if err != nil {
					return "", err
				}
				return agentcore.MarshalToolJSON(sandboxOutputJSON(out))
			})); err != nil {
			return err
		}
	}
	if want[ToolSandboxReadFile] {
		if err := add(utils.InferTool(ToolSandboxReadFile, "Read one file from this thread's sandbox. Path must be absolute under /workspace.",
			func(ctx context.Context, in sandboxReadFileInput) (string, error) {
				rc, err := requireSandboxRun(ctx)
				if err != nil {
					return "", err
				}
				text, err := deps.Sandbox.ReadFile(ctx, rc.ProjectID, rc.ThreadID, in.Path)
				if err != nil {
					return "", err
				}
				return agentcore.MarshalToolJSON(map[string]any{
					"stdout":    text,
					"stderr":    "",
					"exit_code": 0,
				})
			})); err != nil {
			return err
		}
	}
	if want[ToolSandboxWriteFile] {
		if err := add(utils.InferTool(ToolSandboxWriteFile, "Write one file in this thread's sandbox. Path must be absolute under /workspace.",
			func(ctx context.Context, in sandboxWriteFileInput) (string, error) {
				rc, err := requireSandboxRun(ctx)
				if err != nil {
					return "", err
				}
				if err := deps.Sandbox.WriteFile(ctx, rc.ProjectID, rc.ThreadID, in.Path, in.Content); err != nil {
					return "", err
				}
				return agentcore.MarshalToolJSON(map[string]any{
					"stdout":    "",
					"stderr":    "",
					"exit_code": 0,
				})
			})); err != nil {
			return err
		}
	}
	return nil
}

// sandboxOutputJSON 是工具返回体（cloud-agent-sandbox-plan §7.1）。
func sandboxOutputJSON(o SandboxOutput) map[string]any {
	return map[string]any{
		"stdout":    o.Stdout,
		"stderr":    o.Stderr,
		"exit_code": o.ExitCode,
	}
}

// requireSandboxRun 校验运行上下文与写权限（cloud-agent-sandbox-plan §7.2）：
// 只持有 database:read 的 principal 被拒绝；write/admin 放行。
func requireSandboxRun(ctx context.Context) (RunContext, error) {
	rc, err := requireRun(ctx)
	if err != nil {
		return RunContext{}, err
	}
	if !rc.Principal.HasPermission(auth.DatabaseWrite) && !rc.Principal.HasPermission(auth.DatabaseAdmin) {
		return RunContext{}, fmt.Errorf("sandbox execution requires database:write permission")
	}
	if rc.ThreadID == "" {
		return RunContext{}, fmt.Errorf("agent run context missing thread id")
	}
	return rc, nil
}
