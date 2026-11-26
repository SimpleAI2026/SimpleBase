package api

// kernel_residue_test.go：锁定 internal/api 顶层（入口内核）的生产文件清单（internal-ddd-hexagonal-implementation-plan §6.3 / P6）。
// 新增顶层生产文件必须同时更新本清单与 plan §6.2 的「api（内核残留）」行；业务 handler 一律放进 api/<context>http 子包。

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// kernelResidueFiles 是入口内核 + 仍在内核侧的内置 MCP 派发 + 路由依赖装配面。
var kernelResidueFiles = []string{
	// 路由树与守卫
	"router.go", "routes_v1.go", "routes_agent.go", "routes_function.go", "routes_guards.go",
	"route_meta.go", "policy_gate.go", "project_context.go",
	// 中间件
	"middleware_accesslog.go", "middleware_bodylimit.go", "middleware_http.go", "middleware_ratelimit.go",
	// 健康 / 观测 / Echo 错误兜底 / 分段计时（认证子阶段）
	"health.go", "observability_routes.go", "error.go", "stage_timing.go",
	// 内置 MCP：经 subRequest 回到完整路由树，必须持有 Echo 实例（plan §10 既有例外）
	"mcp_builtin.go", "mcp_builtin_admin.go", "mcp_builtin_session.go", "mcp_builtin_simplebase.go",
	"mcp_dispatch.go", "mcp_handler.go", "mcp_test_connect.go",
	// 路由依赖装配面（不含任何业务 handler 实现）
	"backup_handler.go", "projects_handler.go",
}

func TestKernelResidueFiles(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		got = append(got, name)
	}
	sort.Strings(got)
	want := append([]string(nil), kernelResidueFiles...)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("internal/api 顶层生产文件清单已变化：\n--- got ---\n%s\n--- want ---\n%s\n"+
			"新增业务 handler 请放到 api/<context>http；确属内核的文件再更新 kernelResidueFiles 与 plan §6.2。", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// 子包只能提供 handler，不得自带路由注册（Mount）。
	for _, sub := range []string{"agenthttp", "backuphttp", "cataloghttp", "datahttp", "filehttp", "functionhttp", "iamhttp", "llmhttp", "opshttp", "sandboxhttp"} {
		matches, _ := filepath.Glob(filepath.Join(sub, "*.go"))
		if len(matches) == 0 {
			t.Fatalf("业务域子包 %s 不存在或为空", sub)
		}
	}
}
