// 文件说明：dev.go — `simplebase dev plan|apply|status`：把本地工作区幂等重放到任意 SimpleBase 实例（planv6.4 P0b）
//
// 用法
//
//	simplebase dev plan   [--dir DIR] [--manifest FILE] [--seed]          只读：列出将要执行的变更
//	simplebase dev apply  [--dir DIR] [--manifest FILE] [--seed] [--yes]  幂等重放；远端目标不带 --yes 只打印计划并以退出码 3 退出
//	simplebase dev status [--dir DIR]                   查看 .simplebase/lock.json 里各环境的记录
//
// 与 skills/simplebase/scripts/sbdev.py 行为一一对应（同一份清单、同一组 fixture）：
// 固定顺序 库 → 迁移 →（种子）→ 集合 → 函数 → 定时任务；每步先查后写；失败即停，
// 已完成的步骤下次运行自然变为 skip。不删除远端资源，不触发 cron。
//
// 连接信息只取自已加载的 cfg（flag / 环境变量 / config.json），不读工作区 .env；
// 凭据不进 lock、env 提示文件和输出。
//
// 主要符号
//
//	devCommand(name, apply) / cmdDevStatus（commandFunc）
//	type devError（RunIO 识别，映射退出码 3 等）
//	type DevAction

package sbcli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"

	"github.com/linkxzhou/SimpleBase/godatabase/atomicfile"
)

// exitConfirm 表示远端 apply 缺少 --yes：计划已输出，未做任何写入。
const exitConfirm = 3

const devToolVersion = "simplebase-dev/1"

// DevAction 是一步重放动作，plan 与 apply 共用。
type DevAction struct {
	Verb   string `json:"verb"` // create | update | apply | skip
	Kind   string `json:"kind"` // database | migration | collection | document | function | cron
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
}

// devResult 是 plan/apply/status 的 data 载荷。
type devResult struct {
	Mode     string      `json:"mode"` // plan | apply
	Target   devTarget   `json:"target"`
	Seed     bool        `json:"seed"`
	Actions  []DevAction `json:"actions"`
	Changes  int         `json:"changes"`
	Skips    int         `json:"skips"`
	Warnings []string    `json:"warnings,omitempty"`
	EnvHint  string      `json:"env_hint,omitempty"`
}

type devTarget struct {
	Label   string `json:"label"`
	URL     string `json:"url"`
	Project string `json:"project"`
}

// devError 携带稳定错误码与退出码；data 非空时 RunIO 先把它写到 stdout。
type devError struct {
	code string
	msg  string
	exit int
	data any
}

func (e *devError) Error() string { return e.msg }

var devLocalHosts = map[string]bool{"127.0.0.1": true, "localhost": true, "::1": true, "0.0.0.0": true}

var devLabelStrip = regexp.MustCompile(`[^a-z0-9]+`)

func devTargetOf(cfg loadedConfig) devTarget {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	host := ""
	if u, err := url.Parse(endpoint); err == nil {
		host = u.Hostname()
	}
	label := "local"
	if !devLocalHosts[host] {
		label = strings.Trim(devLabelStrip.ReplaceAllString(strings.ToLower(host), "-"), "-")
		if label == "" {
			label = "remote"
		}
	}
	return devTarget{Label: label, URL: endpoint, Project: cfg.ProjectID}
}

func (t devTarget) isLocal() bool { return t.Label == "local" }

// key 是 lock 的分区键。本地目标不写端口：每人本地端口不同，写进 lock 会
// 在提交后散成多份分区；库 ID 命中失败会回退到按名称匹配，共用分区是安全的。
func (t devTarget) key() string {
	if t.isLocal() {
		return "local#" + t.Project
	}
	return t.URL + "#" + t.Project
}

func writeDevEnvHint(w *devWorkspace, t devTarget, order []string, ids map[string]string) (string, error) {
	lines := []string{
		"# simplebase dev 生成，不含密钥。SIMPLEBASE_API_KEY 请在该环境的控制台签发后自行填写。",
		"SIMPLEBASE_URL=" + t.URL,
		"SIMPLEBASE_PROJECT_ID=" + t.Project,
	}
	nonAlnum := regexp.MustCompile(`[^A-Za-z0-9]`)
	var known []string
	for _, name := range order {
		if ids[name] != "" {
			known = append(known, name)
		}
	}
	// 不带后缀的 SIMPLEBASE_DATABASE_ID 只在单库时写出；多库时依赖清单顺序容易用错。
	if len(known) == 1 {
		lines = append(lines, "SIMPLEBASE_DATABASE_ID="+ids[known[0]])
	}
	for _, name := range known {
		lines = append(lines, "SIMPLEBASE_DATABASE_ID_"+strings.ToUpper(nonAlnum.ReplaceAllString(name, "_"))+"="+ids[name])
	}
	rel := filepath.ToSlash(filepath.Join(devStateDir, "env."+t.Label))
	err := atomicfile.WriteEnsureDir(filepath.Join(w.root, devStateDir, "env."+t.Label), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	return rel, err
}

// ── 命令 ─────────────────────────────────────────────────

type devFlags struct {
	dir      string
	manifest string
	seed     bool
	yes      bool
}

func parseDevFlags(name string, args []string, allowSeed, allowYes bool) (devFlags, error) {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	f := devFlags{dir: "."}
	set.StringVar(&f.dir, "dir", ".", "")
	set.StringVar(&f.dir, "C", ".", "")
	if allowSeed { // plan / apply
		set.StringVar(&f.manifest, "manifest", "", "")
	}
	if allowSeed {
		set.BoolVar(&f.seed, "seed", false, "")
	}
	if allowYes {
		set.BoolVar(&f.yes, "yes", false, "")
	}
	if err := set.Parse(args); err != nil {
		return f, usagef("%s", err.Error())
	}
	if set.NArg() != 0 {
		return f, usagef("dev %s takes no positional arguments", name)
	}
	return f, nil
}

func devRejectAgent(cfg loadedConfig) error {
	if cfg.Agent {
		return &devError{code: "file_not_allowed", exit: exitUsage,
			msg: "dev commands read the host workspace and are unavailable in agent mode"}
	}
	return nil
}

// devCommand 是 `dev plan`（apply=false）与 `dev apply`（apply=true）共用的入口：
// agent 模式下一律拒绝；flag 解析与执行只在 apply 这一个布尔上不同。
func devCommand(name string, apply bool) commandFunc {
	return func(ctx context.Context, c *gosdk.Client, cfg loadedConfig, args []string, _ io.Reader) (any, error) {
		if err := devRejectAgent(cfg); err != nil {
			return nil, err
		}
		f, err := parseDevFlags(name, args, true, apply)
		if err != nil {
			return nil, err
		}
		return runDev(ctx, c, cfg, f, apply)
	}
}

func runDev(ctx context.Context, c *gosdk.Client, cfg loadedConfig, f devFlags, apply bool) (any, error) {
	w, err := loadDevWorkspaceManifest(f.dir, f.manifest)
	if err != nil {
		return nil, err
	}
	if len(w.errors) > 0 {
		return nil, &devError{code: "workspace_invalid", exit: exitUsage,
			data: map[string]any{"errors": w.errors, "warnings": w.warnings},
			msg:  fmt.Sprintf("工作区校验失败（%d 个错误）：%s", len(w.errors), strings.Join(w.errors, "；"))}
	}
	target := devTargetOf(cfg)
	if w.projectID != "" && w.projectID != target.Project {
		w.warnf("目标 project %s 与清单 project.id %s 不同：业务侧将不能只换 URL", target.Project, w.projectID)
	}
	lock, err := loadDevLock(w)
	if err != nil {
		return nil, err
	}
	result := func(mode string, r *devReplayer) *devResult {
		warns := append(append([]string(nil), w.warnings...), r.warns...)
		return &devResult{Mode: mode, Target: target, Seed: f.seed, Actions: nonNilActions(r.actions),
			Changes: r.changes, Skips: r.skips, Warnings: warns}
	}
	if apply && !target.isLocal() && !f.yes {
		preview := newDevReplayer(w, c, target, lock, false, f.seed)
		if err := preview.run(ctx); err != nil {
			return nil, err
		}
		res := result("plan", preview)
		if preview.changes == 0 {
			return res, nil
		}
		return nil, &devError{code: "confirmation_required", exit: exitConfirm, data: res,
			msg: fmt.Sprintf("目标是远端 %s（%s），共 %d 项变更；确认后加 --yes 重跑 apply", target.Label, target.key(), preview.changes)}
	}
	r := newDevReplayer(w, c, target, lock, apply, f.seed)
	r.checkEngine(ctx, lock)
	runErr := r.run(ctx)
	if apply {
		// 失败路径也保存已完成步骤的记录，下次重跑自然变为 skip。
		if saveErr := saveDevLock(w, lock); saveErr != nil && runErr == nil {
			runErr = &devError{code: "dev_failed", msg: "写入 lock 失败：" + saveErr.Error(), exit: exitUsage}
		}
	}
	if runErr != nil {
		var de *devError
		if errors.As(runErr, &de) && de.data == nil {
			de.data = result("apply", r)
		}
		return nil, runErr
	}
	mode := "plan"
	if apply {
		mode = "apply"
	}
	res := result(mode, r)
	if apply {
		hint, err := writeDevEnvHint(w, target, r.dbOrder, r.dbIDs)
		if err != nil {
			return nil, &devError{code: "dev_failed", msg: "写入环境提示文件失败：" + err.Error(), exit: exitUsage}
		}
		res.EnvHint = hint
	}
	return res, nil
}

func nonNilActions(a []DevAction) []DevAction {
	if a == nil {
		return []DevAction{}
	}
	return a
}

// cmdDevValidate 只读工作区做离线校验，不访问远端，也不需要连接配置。
func cmdDevValidate(_ context.Context, _ *gosdk.Client, cfg loadedConfig, args []string, _ io.Reader) (any, error) {
	if err := devRejectAgent(cfg); err != nil {
		return nil, err
	}
	f, err := parseDevFlags("validate", args, true, false)
	if err != nil {
		return nil, err
	}
	w, err := loadDevWorkspaceManifest(f.dir, f.manifest)
	if err != nil {
		return nil, err
	}
	nMig := 0
	for _, d := range w.databases {
		nMig += len(d.migrations)
	}
	res := map[string]any{
		"errors":      nonNilStrings(w.errors),
		"warnings":    nonNilStrings(w.warnings),
		"databases":   len(w.databases),
		"migrations":  nMig,
		"collections": len(w.colls),
		"functions":   len(w.functions),
		"cron_jobs":   len(w.cronJobs),
	}
	if len(w.errors) > 0 {
		return nil, &devError{code: "workspace_invalid", exit: exitUsage, data: res,
			msg: fmt.Sprintf("工作区校验失败（%d 个错误）：%s", len(w.errors), strings.Join(w.errors, "；"))}
	}
	return res, nil
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func cmdDevStatus(_ context.Context, _ *gosdk.Client, cfg loadedConfig, args []string, _ io.Reader) (any, error) {
	if err := devRejectAgent(cfg); err != nil {
		return nil, err
	}
	f, err := parseDevFlags("status", args, false, false)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(f.dir)
	if err != nil {
		return nil, usagef("%v", err)
	}
	lock, err := loadDevLock(&devWorkspace{root: root})
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(lock.Targets))
	for k := range lock.Targets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	current := ""
	if cfg.Endpoint != "" { // status 不要求连接配置，缺配置时不给 current
		current = devTargetOf(cfg).key()
	}
	return map[string]any{
		"current": current,
		"keys":    keys,
		"targets": lock.Targets,
	}, nil
}
