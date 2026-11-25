// 文件说明：arch_gate_test.go — internal/app 的单元测试（TestArchitectureGate、TestArchitectureGateRules 等）
//
// 用法
//
//	go test ./internal/app -run 'TestArchitectureGate|TestArchitectureGateRules|TestArchitectureGateBaselinePresent'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/arch_gate_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_shutdown_test.go, app.go, app_alerts.go, app_lease.go, app_test.go 等 41 个
//
// 主要符号
//
//	const archRootModule / archRepoRoot / archBaselinePath
//	var archUpdate
//	type archPackage struct{…}
//	var archRestrictedMethods
//	var archScanDirs
//	var archNestedModules
//	archScan(t *testing.T) map[string]*archPackage
//	restrictedCalls(f *ast.File) map[string][]string
//	... 另有 11 个顶层符号

package app

// 架构门禁（internal-ddd-hexagonal-implementation-plan §1.3 / §8.3）：
// 用标准库 go/parser 扫描生产代码的 import，强制 internal/ 的依赖方向，并把
//「直接使用 systemdb 的文件数」按包记入基线、只减不增。随 `go test ./internal/app` 执行；
// `go test ./internal/app -run TestArchitectureGate -update` 重新生成基线。

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	archRootModule   = "github.com/linkxzhou/SimpleBase"
	archRepoRoot     = "../.." // 测试工作目录是 internal/app
	archBaselinePath = "testdata/arch-baseline.txt"
)

var archUpdate = flag.Bool("update", false, "rewrite "+archBaselinePath)

// archPackage 是一个目录（Go 包）的生产代码快照；不含 _test.go。
type archPackage struct {
	Rel      string // 相对仓库根，如 internal/api/agenthttp
	Files    int
	Imports  map[string]bool // 生产代码 import 的完整路径
	SysDB    int             // import internal/systemdb 的生产文件数
	StoreLit int             // 直接声明 *systemdb.Store 的生产文件数（按 AST，注释不计）
	// Restricted 是文件里出现的受限仓储方法调用：方法名 → 文件（相对仓库根）。
	Restricted map[string][]string
}

// archRestrictedMethods 列出「只能从唯一出口发出」的仓储写方法（聚合不变量的调用点收口，plan §3.7）：
// 方法名 → 允许调用它的包（"" 前缀匹配任意层级）或文件。
var archRestrictedMethods = map[string][]string{
	"UpdateAgentRunStatus":  {"internal/agentrun"},                // run 状态迁移：agentrun.transitionRun 唯一出口
	"UpdateAgentTurnStatus": {"internal/agentrun/group_store.go"}, // 轮次状态迁移：只经群引擎的仓储适配器
}

// archScanDirs 是参与扫描的目录：宿主 internal/ 与四个嵌套独立模块。
var archScanDirs = []string{"internal", "gocloudagent", "godatabase", "gosandbox", "gofunction"}

// archNestedModules 规定嵌套独立模块只允许 import 的本仓路径前缀（AGENTS.md：不得 import 根模块任何包）。
var archNestedModules = map[string][]string{
	"gocloudagent": {archRootModule + "/gocloudagent", archRootModule + "/gosandbox"},
	"godatabase":   {archRootModule + "/godatabase"},
	"gosandbox":    {archRootModule + "/gosandbox"},
	"gofunction":   {archRootModule + "/gofunction"},
}

func archScan(t *testing.T) map[string]*archPackage {
	t.Helper()
	pkgs := map[string]*archPackage{}
	fset := token.NewFileSet()
	for _, root := range archScanDirs {
		err := filepath.WalkDir(filepath.Join(archRepoRoot, root), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch name := d.Name(); {
				case name == "testdata" || name == "node_modules" || strings.HasPrefix(name, "."):
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(archRepoRoot, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			dir := filepath.ToSlash(filepath.Dir(rel))
			p := pkgs[dir]
			if p == nil {
				p = &archPackage{Rel: dir, Imports: map[string]bool{}, Restricted: map[string][]string{}}
				pkgs[dir] = p
			}
			p.Files++
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			usesSysDB := false
			for _, imp := range f.Imports {
				ip, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					return fmt.Errorf("%s: %w", rel, err)
				}
				p.Imports[ip] = true
				if strings.HasPrefix(ip, archRootModule+"/internal/systemdb") {
					usesSysDB = true
				}
			}
			if usesSysDB {
				p.SysDB++
			}
			if usesStorePointer(f) {
				p.StoreLit++
			}
			for method := range restrictedCalls(f) {
				p.Restricted[method] = append(p.Restricted[method], rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("arch scan: %v", err)
		}
	}
	return pkgs
}

// restrictedCalls 找出文件内对受限仓储方法的调用（任意接收者），返回被调用的方法名集合。
func restrictedCalls(f *ast.File) map[string][]string {
	out := map[string][]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if _, restricted := archRestrictedMethods[sel.Sel.Name]; restricted {
			out[sel.Sel.Name] = nil
		}
		return true
	})
	return out
}

// usesStorePointer 按 AST 判断文件是否直接声明 `*systemdb.Store`（参数、字段、断言等）；
// 注释里提到的不算，避免文档性文字干扰计数。
func usesStorePointer(f *ast.File) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		star, ok := n.(*ast.StarExpr)
		if !ok || found {
			return !found
		}
		sel, ok := star.X.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "systemdb" && sel.Sel.Name == "Store" {
			found = true
			return false
		}
		return true
	})
	return found
}

func archShort(ip string) string { return strings.TrimPrefix(ip, archRootModule+"/") }

func archHasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// archCheckRules 是纯函数：输入包快照，输出依赖方向违例（已排序、去重）。
func archCheckRules(pkgs map[string]*archPackage) []string {
	seen := map[string]bool{}
	var out []string
	viol := func(format string, a ...any) {
		s := fmt.Sprintf(format, a...)
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	api := archRootModule + "/internal/api"
	httpkit := api + "/httpkit"
	appPkg := archRootModule + "/internal/app"
	agentrun := archRootModule + "/internal/agentrun"
	for rel, p := range pkgs {
		// perfbench 是压测入口的既有例外（分层计划 §3.1 规则 4），不按业务域子包约束。
		isAPISub := strings.HasPrefix(rel, "internal/api/") && rel != "internal/api/httpkit" && rel != "internal/api/perfbench"
		for ip := range p.Imports {
			inRepo := strings.HasPrefix(ip, archRootModule+"/")
			switch {
			// R1：只有 app 与既有例外 perfbench 可以 import api。
			case ip == api && rel != "internal/app" && rel != "internal/api/perfbench" && !strings.HasPrefix(rel, "internal/api/"):
				viol("R1 %s → %s：只有 internal/app / internal/api/perfbench 可以 import internal/api", rel, archShort(ip))
			// R2：httpkit 是叶子包。
			case rel == "internal/api/httpkit" && (strings.HasPrefix(ip, api) || strings.HasPrefix(ip, appPkg)):
				viol("R2 internal/api/httpkit → %s：协议内核叶子包不得 import api / 子包 / app", archShort(ip))
			// R3：业务域子包只可依赖 httpkit、agentrun 与领域包。
			case isAPISub && !strings.HasPrefix(ip, httpkit) && (strings.HasPrefix(ip, api) || strings.HasPrefix(ip, appPkg)):
				viol("R3 %s → %s：业务域子包不得 import api、app 或同级子包", rel, archShort(ip))
			// R4：入口内核不依赖组合根。
			case rel == "internal/api" && strings.HasPrefix(ip, appPkg):
				viol("R4 internal/api → %s：入口内核不得 import app", archShort(ip))
			// R5：应用服务不认识入口。
			case rel == "internal/agentrun" && (strings.HasPrefix(ip, api) || strings.HasPrefix(ip, appPkg) || strings.HasPrefix(ip, "github.com/labstack/echo")):
				viol("R5 internal/agentrun → %s：应用服务不得 import api / app / Echo", archShort(ip))
			// R6：agentrun 实现 agentbridge 的端口，方向不可反。
			case (rel == "internal/agentbridge" || rel == "internal/cronjob") && strings.HasPrefix(ip, agentrun):
				viol("R6 %s → %s：agentbridge / cronjob 不得 import agentrun（由 agentrun 实现其端口）", rel, archShort(ip))
			// R7：litellm 是独立客户端库。
			case strings.HasPrefix(rel, "internal/litellm") && strings.HasPrefix(ip, archRootModule+"/internal/") && !strings.HasPrefix(ip, archRootModule+"/internal/litellm"):
				viol("R7 %s → %s：internal/litellm 不得 import 其它 internal 包", rel, archShort(ip))
			// R8：utils 只依赖标准库。
			case strings.HasPrefix(rel, "internal/utils") && inRepo:
				viol("R8 %s → %s：internal/utils 只放零业务依赖的工具", rel, archShort(ip))
			}
			// R9：嵌套独立模块不得 import 根模块或其它模块。
			for module, allow := range archNestedModules {
				if strings.HasPrefix(rel, module+"/") && inRepo && !archHasAnyPrefix(ip, allow...) {
					viol("R9 %s → %s：独立模块只允许 import %s", rel, archShort(ip), strings.Join(archShorts(allow), " / "))
				}
			}
		}
	}
	// R10：受限仓储写方法只允许从登记的唯一出口调用。
	for rel, p := range pkgs {
		if strings.HasSuffix(rel, "_test") {
			continue
		}
		for method, files := range p.Restricted {
			for _, file := range files {
				if !archAllowedCaller(archRestrictedMethods[method], file) {
					viol("R10 %s 调用 %s：run / 轮次状态迁移只允许从 %s 发出", file, method, strings.Join(archRestrictedMethods[method], " / "))
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// archAllowedCaller 判断调用方文件是否在白名单内：条目以 .go 结尾按文件精确匹配，否则按包前缀匹配。
func archAllowedCaller(allow []string, file string) bool {
	for _, entry := range allow {
		if strings.HasSuffix(entry, ".go") {
			if file == entry {
				return true
			}
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(file))
		if dir == entry {
			return true
		}
	}
	return false
}

func archShorts(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = archShort(s)
	}
	return out
}

// archBaseline 读取基线：rel → (SysDB, StoreLit)。缺失或无法解析时返回空表。
func archBaseline(t *testing.T) map[string][2]int {
	t.Helper()
	out := map[string][2]int{}
	f, err := os.Open(archBaselinePath)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("%s: 行格式应为 rel<TAB>sysdb<TAB>store：%q", archBaselinePath, line)
		}
		sysdb, err1 := strconv.Atoi(fields[1])
		store, err2 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil {
			t.Fatalf("%s: 计数不是整数：%q", archBaselinePath, line)
		}
		out[fields[0]] = [2]int{sysdb, store}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("%s: %v", archBaselinePath, err)
	}
	return out
}

func archWriteBaseline(t *testing.T, pkgs map[string]*archPackage) {
	t.Helper()
	var rels []string
	for rel, p := range pkgs {
		if p.SysDB > 0 || p.StoreLit > 0 {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	var buf bytes.Buffer
	buf.WriteString("# internal/app/testdata/arch-baseline.txt — 直接使用 systemdb 的生产文件数（按包）。\n")
	buf.WriteString("# 由 `go test ./internal/app -run TestArchitectureGate -update` 生成；两列分别为\n")
	buf.WriteString("# import internal/systemdb 的文件数、直接声明 *systemdb.Store 的文件数（按 AST）。规则：每包只减不增，新包基线为 0。\n")
	buf.WriteString("# 包<TAB>import_systemdb_files<TAB>store_literal_files\n")
	for _, rel := range rels {
		p := pkgs[rel]
		fmt.Fprintf(&buf, "%s\t%d\t%d\n", rel, p.SysDB, p.StoreLit)
	}
	if err := os.MkdirAll(filepath.Dir(archBaselinePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(archBaselinePath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
}

// TestArchitectureGate 规则 + 基线；违例时输出 importer → imported，评审只需看基线 diff 是否只降不升。
func TestArchitectureGate(t *testing.T) {
	pkgs := archScan(t)
	if *archUpdate {
		archWriteBaseline(t, pkgs)
	}
	if viols := archCheckRules(pkgs); len(viols) > 0 {
		t.Fatalf("架构依赖违例 %d 条：\n%s", len(viols), strings.Join(viols, "\n"))
	}
	if *archUpdate {
		return
	}
	base := archBaseline(t)
	var worse []string
	for rel, p := range pkgs {
		want, ok := base[rel]
		if !ok && (p.SysDB > 0 || p.StoreLit > 0) {
			worse = append(worse, fmt.Sprintf("%s：基线缺席，现 %d/%d（新包不得直接使用 systemdb）", rel, p.SysDB, p.StoreLit))
			continue
		}
		if p.SysDB > want[0] || p.StoreLit > want[1] {
			worse = append(worse, fmt.Sprintf("%s：基线 %d/%d，现 %d/%d（只减不增）", rel, want[0], want[1], p.SysDB, p.StoreLit))
		}
	}
	sort.Strings(worse)
	if len(worse) > 0 {
		t.Fatalf("systemdb 使用面扩大 %d 处：\n%s\n如确属等价搬迁，用 -update 重新生成基线并在 PR 说明。", len(worse), strings.Join(worse, "\n"))
	}
}

// TestArchitectureGateRules 用构造的包快照验证门禁本身能判好坏（含模拟违例）。
func TestArchitectureGateRules(t *testing.T) {
	pkgs := func(pkg string, imports ...string) map[string]*archPackage {
		p := &archPackage{Rel: pkg, Imports: map[string]bool{}}
		for _, ip := range imports {
			p.Imports[ip] = true
		}
		return map[string]*archPackage{pkg: p}
	}
	pkg2 := func(file string, methods ...string) map[string]*archPackage {
		p := &archPackage{Rel: filepath.ToSlash(filepath.Dir(file)), Imports: map[string]bool{}, Restricted: map[string][]string{}}
		for _, m := range methods {
			p.Restricted[m] = []string{file}
		}
		return map[string]*archPackage{p.Rel: p}
	}
	cases := []struct {
		name    string
		pkgs    map[string]*archPackage
		wantSub string // 期望包含的违例片段；空表示必须零违例
	}{
		{name: "app 可 import api", pkgs: pkgs("internal/app", "github.com/linkxzhou/SimpleBase/internal/api", "github.com/linkxzhou/SimpleBase/internal/agentrun")},
		{name: "perfbench 既有例外", pkgs: pkgs("internal/api/perfbench", "github.com/linkxzhou/SimpleBase/internal/api", "github.com/linkxzhou/SimpleBase/internal/app")},
		{name: "业务域子包依赖 httpkit / agentrun / 领域包", pkgs: pkgs("internal/api/agenthttp", "github.com/linkxzhou/SimpleBase/internal/api/httpkit", "github.com/linkxzhou/SimpleBase/internal/agentrun", "github.com/linkxzhou/SimpleBase/internal/auth", "github.com/labstack/echo/v4")},
		{name: "模拟违例：agentrun import Echo", pkgs: pkgs("internal/agentrun", "github.com/labstack/echo/v4"), wantSub: "R5 internal/agentrun → github.com/labstack/echo/v4"},
		{name: "模拟违例：子包 import api", pkgs: pkgs("internal/api/agenthttp", "github.com/linkxzhou/SimpleBase/internal/api"), wantSub: "R3 internal/api/agenthttp → internal/api"},
		{name: "模拟违例：同级子包互 import", pkgs: pkgs("internal/api/agenthttp", "github.com/linkxzhou/SimpleBase/internal/api/datahttp"), wantSub: "R3 internal/api/agenthttp → internal/api/datahttp"},
		{name: "模拟违例：httpkit import 子包", pkgs: pkgs("internal/api/httpkit", "github.com/linkxzhou/SimpleBase/internal/api/agenthttp"), wantSub: "R2 internal/api/httpkit → internal/api/agenthttp"},
		{name: "模拟违例：领域包 import api", pkgs: pkgs("internal/catalog", "github.com/linkxzhou/SimpleBase/internal/api"), wantSub: "R1 internal/catalog → internal/api"},
		{name: "模拟违例：agentbridge import agentrun", pkgs: pkgs("internal/agentbridge", "github.com/linkxzhou/SimpleBase/internal/agentrun"), wantSub: "R6 internal/agentbridge → internal/agentrun"},
		{name: "模拟违例：独立模块 import 宿主", pkgs: pkgs("gocloudagent/group", "github.com/linkxzhou/SimpleBase/internal/auth"), wantSub: "R9 gocloudagent/group → internal/auth"},
		{name: "gocloudagent 可依赖 gosandbox/host", pkgs: pkgs("gocloudagent/tools", "github.com/linkxzhou/SimpleBase/gosandbox/host", "github.com/linkxzhou/SimpleBase/gocloudagent/agentcore")},
		{name: "模拟违例：litellm import catalog", pkgs: pkgs("internal/litellm", "github.com/linkxzhou/SimpleBase/internal/catalog"), wantSub: "R7 internal/litellm → internal/catalog"},
		{name: "模拟违例：utils import catalog", pkgs: pkgs("internal/utils/mask", "github.com/linkxzhou/SimpleBase/internal/catalog"), wantSub: "R8 internal/utils/mask → internal/catalog"},
		{name: "模拟违例：api 直调 UpdateAgentRunStatus", pkgs: pkg2("internal/api/agent_run_handler.go", "UpdateAgentRunStatus"), wantSub: "R10 internal/api/agent_run_handler.go 调用 UpdateAgentRunStatus"},
		{name: "agentrun 内的 run 状态迁移与群仓储适配器的轮次迁移合法", pkgs: pkg2("internal/agentrun/group_store.go", "UpdateAgentTurnStatus", "UpdateAgentRunStatus")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := archCheckRules(tc.pkgs)
			if tc.wantSub == "" {
				if len(got) != 0 {
					t.Fatalf("期望零违例，得到：%s", strings.Join(got, "\n"))
				}
				return
			}
			hit := false
			for _, v := range got {
				if strings.Contains(v, tc.wantSub) {
					hit = true
				}
			}
			if !hit {
				t.Fatalf("期望包含 %q，得到：%v", tc.wantSub, got)
			}
		})
	}
}

// TestArchitectureGateBaselinePresent 保证基线文件入库且被扫描覆盖，避免门禁因缺文件而空转。
func TestArchitectureGateBaselinePresent(t *testing.T) {
	if _, err := os.Stat(archBaselinePath); err != nil {
		t.Fatalf("基线文件缺失：%v（先运行 go test ./internal/app -run TestArchitectureGate -update）", err)
	}
	for _, root := range archScanDirs {
		if _, err := os.Stat(filepath.Join(archRepoRoot, root)); err != nil {
			t.Fatalf("扫描目录缺失 %s：%v", root, err)
		}
	}
}
