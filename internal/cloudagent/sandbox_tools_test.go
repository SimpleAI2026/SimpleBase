package cloudagent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/cloudwego/eino/components/tool"
)

// fakeSandbox 是记录调用参数的假沙盒后端（cloud-agent-sandbox-plan §10）。
type fakeSandbox struct {
	available bool
	lastExec  struct {
		projectID, threadID, cmd string
		args                     []string
	}
	lastShell struct {
		projectID, threadID, command string
	}
	lastFile struct {
		projectID, threadID, path, content string
		read, write                        bool
	}
	released struct {
		projectID, threadID string
	}
	execOut SandboxOutput
	err     error
}

func (f *fakeSandbox) Available() bool { return f.available }

func (f *fakeSandbox) Exec(ctx context.Context, projectID, threadID, cmd string, args []string) (SandboxOutput, error) {
	f.lastExec.projectID, f.lastExec.threadID, f.lastExec.cmd, f.lastExec.args = projectID, threadID, cmd, args
	if f.err != nil {
		return SandboxOutput{}, f.err
	}
	return f.execOut, nil
}

func (f *fakeSandbox) Shell(ctx context.Context, projectID, threadID, command string) (SandboxOutput, error) {
	f.lastShell.projectID, f.lastShell.threadID, f.lastShell.command = projectID, threadID, command
	if f.err != nil {
		return SandboxOutput{}, f.err
	}
	return f.execOut, nil
}

func (f *fakeSandbox) ReadFile(ctx context.Context, projectID, threadID, path string) (string, error) {
	f.lastFile.projectID, f.lastFile.threadID, f.lastFile.path, f.lastFile.read = projectID, threadID, path, true
	if f.err != nil {
		return "", f.err
	}
	return "file-body", nil
}

func (f *fakeSandbox) WriteFile(ctx context.Context, projectID, threadID, path, content string) error {
	f.lastFile.projectID, f.lastFile.threadID, f.lastFile.path, f.lastFile.content, f.lastFile.write = projectID, threadID, path, content, true
	return f.err
}

func (f *fakeSandbox) ReleaseThread(ctx context.Context, projectID, threadID string) error {
	f.released.projectID, f.released.threadID = projectID, threadID
	return nil
}

func sandboxRunCtx(write bool) context.Context {
	perms := map[auth.Permission]struct{}{auth.DatabaseRead: {}}
	if write {
		perms[auth.DatabaseWrite] = struct{}{}
	}
	return withRunContext(context.Background(), RunContext{
		ProjectID: "proj-1",
		ThreadID:  "thread-1",
		Principal: auth.Principal{Permissions: perms},
	})
}

func sandboxToolByName(t *testing.T, tools []tool.BaseTool, name string) tool.InvokableTool {
	t.Helper()
	for _, tl := range tools {
		if it, ok := tl.(tool.InvokableTool); ok {
			info, err := it.Info(context.Background())
			if err == nil && info != nil && info.Name == name {
				return it
			}
		}
	}
	t.Fatalf("tool %s not found", name)
	return nil
}

func TestBuildToolsSandboxNilRejected(t *testing.T) {
	_, err := buildTools(SandboxToolIDs(), toolDeps{})
	if err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

func TestBuildToolsSandboxUnavailableRejected(t *testing.T) {
	f := &fakeSandbox{available: false}
	_, err := buildTools(SandboxToolIDs(), toolDeps{Sandbox: f})
	if err == nil {
		t.Fatal("unavailable sandbox must be rejected")
	}
}

func TestSandboxExecTool(t *testing.T) {
	f := &fakeSandbox{available: true, execOut: SandboxOutput{Stdout: "hello\n", Stderr: "", ExitCode: 0}}
	tools, err := buildTools([]string{ToolSandboxExec}, toolDeps{Sandbox: f})
	if err != nil {
		t.Fatal(err)
	}
	it := sandboxToolByName(t, tools, ToolSandboxExec)
	out, err := it.InvokableRun(sandboxRunCtx(true), `{"cmd":"python","args":["-c","print(1)"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if f.lastExec.cmd != "python" || f.lastExec.threadID != "thread-1" || f.lastExec.projectID != "proj-1" {
		t.Fatalf("exec args not forwarded: %+v", f.lastExec)
	}
	if !strings.Contains(out, `"stdout":"hello\n"`) || !strings.Contains(out, `"exit_code":0`) {
		t.Fatalf("unexpected output json: %s", out)
	}
}

func TestSandboxExecRequiresWrite(t *testing.T) {
	f := &fakeSandbox{available: true}
	tools, err := buildTools([]string{ToolSandboxExec}, toolDeps{Sandbox: f})
	if err != nil {
		t.Fatal(err)
	}
	it := sandboxToolByName(t, tools, ToolSandboxExec)
	out, err := it.InvokableRun(sandboxRunCtx(false), `{"cmd":"ls"}`)
	// BUG-02：工具错误转为 is_error 文本回给模型，不再以 error 终止流。
	if err != nil {
		t.Fatalf("readonly principal must surface as error text, got %v", err)
	}
	if !strings.Contains(out, "database:write") || !strings.Contains(out, `"is_error":true`) {
		t.Fatalf("readonly principal must be rejected, got %s", out)
	}
}

func TestSandboxShellLengthLimit(t *testing.T) {
	f := &fakeSandbox{available: true}
	tools, err := buildTools([]string{ToolSandboxShell}, toolDeps{Sandbox: f})
	if err != nil {
		t.Fatal(err)
	}
	it := sandboxToolByName(t, tools, ToolSandboxShell)
	long := strings.Repeat("a", maxShellCommandBytes+1)
	out, err := it.InvokableRun(sandboxRunCtx(true), `{"command":"`+long+`"}`)
	if err != nil {
		t.Fatalf("oversized command must surface as error text, got %v", err)
	}
	if !strings.Contains(out, "exceeds") || !strings.Contains(out, `"is_error":true`) {
		t.Fatalf("oversized command must be rejected, got %s", out)
	}
}

func TestSandboxReadFileTool(t *testing.T) {
	f := &fakeSandbox{available: true}
	tools, err := buildTools([]string{ToolSandboxReadFile}, toolDeps{Sandbox: f})
	if err != nil {
		t.Fatal(err)
	}
	it := sandboxToolByName(t, tools, ToolSandboxReadFile)
	out, err := it.InvokableRun(sandboxRunCtx(true), `{"path":"/workspace/a.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "file-body") {
		t.Fatalf("file body missing: %s", out)
	}
	if f.lastFile.path != "/workspace/a.txt" {
		t.Fatalf("path not forwarded: %s", f.lastFile.path)
	}
}

func TestSandboxWriteFileTool(t *testing.T) {
	f := &fakeSandbox{available: true}
	tools, err := buildTools([]string{ToolSandboxWriteFile}, toolDeps{Sandbox: f})
	if err != nil {
		t.Fatal(err)
	}
	it := sandboxToolByName(t, tools, ToolSandboxWriteFile)
	_, err = it.InvokableRun(sandboxRunCtx(true), `{"path":"/workspace/a.txt","content":"data"}`)
	if err != nil {
		t.Fatal(err)
	}
	if f.lastFile.content != "data" || f.lastFile.path != "/workspace/a.txt" {
		t.Fatalf("write not forwarded: %+v", f.lastFile)
	}
}

func TestSandboxToolErrorSurface(t *testing.T) {
	f := &fakeSandbox{available: true, err: errors.New("boom")}
	tools, err := buildTools([]string{ToolSandboxExec}, toolDeps{Sandbox: f})
	if err != nil {
		t.Fatal(err)
	}
	it := sandboxToolByName(t, tools, ToolSandboxExec)
	out, err := it.InvokableRun(sandboxRunCtx(true), `{"cmd":"ls"}`)
	if err != nil {
		t.Fatalf("transport error must surface as error text, got %v", err)
	}
	if !strings.Contains(out, "boom") || !strings.Contains(out, `"is_error":true`) {
		t.Fatalf("transport error must surface, got %s", out)
	}
}

func TestSandboxThreadIDRequired(t *testing.T) {
	f := &fakeSandbox{available: true}
	tools, err := buildTools([]string{ToolSandboxExec}, toolDeps{Sandbox: f})
	if err != nil {
		t.Fatal(err)
	}
	it := sandboxToolByName(t, tools, ToolSandboxExec)
	ctx := withRunContext(context.Background(), RunContext{
		ProjectID: "proj-1",
		Principal: auth.Principal{Permissions: map[auth.Permission]struct{}{auth.DatabaseWrite: {}}},
	})
	out, err := it.InvokableRun(ctx, `{"cmd":"ls"}`)
	if err != nil {
		t.Fatalf("missing thread id must surface as error text, got %v", err)
	}
	if !strings.Contains(out, "thread") || !strings.Contains(out, `"is_error":true`) {
		t.Fatalf("missing thread id must be rejected, got %s", out)
	}
}

func TestSandboxModuleTemplate(t *testing.T) {
	tpl := ModuleTemplate(ModuleSandbox)
	if !strings.Contains(tpl, "/workspace") || !strings.Contains(tpl, "sandbox_exec") {
		t.Fatalf("sandbox template incomplete: %s", tpl)
	}
}
