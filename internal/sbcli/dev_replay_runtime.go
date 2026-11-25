// 文件说明：dev_replay_runtime.go — 重放器的云函数与定时任务步骤
//
// 用法
//
//	同包内由 dev.go 的 runDev 调用；与 skills/simplebase/scripts/sbdev.py 的对应步骤语义一致。
//
// 调用架构
//
//	[上游] dev.go（devCommand）
//	         ▼
//	[本文件] internal/sbcli/dev_replay_runtime.go
//	         ▼
//	[同包] dev.go（devError / DevAction / devTarget）、dev_workspace.go（devWorkspace）
//
// 主要符号
//
//	(*devReplayer).functions / cron / devCronDiff

package sbcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"

	"github.com/linkxzhou/SimpleBase/godatabase/hashx"
)

type devRemoteFunc struct {
	Description   string `json:"description"`
	ActiveVersion int64  `json:"active_version"`
	LatestVersion int64  `json:"latest_version"`
	Source        string `json:"source"`
}

func (r *devReplayer) testFunction(ctx context.Context, f devFunction, version int64) error {
	if f.test == nil {
		return nil
	}
	raw, err := r.c.TestFunction(ctx, f.name, strconv.FormatInt(version, 10), f.test.export, json.RawMessage(f.test.body))
	if err != nil {
		return err
	}
	var res struct {
		OK    bool            `json:"ok"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &res); err != nil || !res.OK {
		detail := strings.Trim(string(res.Error), `"`)
		if detail == "" || detail == "null" {
			detail = string(raw)
		}
		return &devError{code: "function_test_failed", exit: exitAPI,
			msg: fmt.Sprintf("函数 %s v%d 试跑失败：%s", f.name, version, detail)}
	}
	return nil
}

func (r *devReplayer) latestVersion(ctx context.Context, name string, created json.RawMessage) (int64, error) {
	var c struct {
		LatestVersion int64 `json:"latest_version"`
	}
	if json.Unmarshal(created, &c) == nil && c.LatestVersion > 0 {
		return c.LatestVersion, nil
	}
	raw, err := r.c.ListFunctionVersions(ctx, name)
	if err != nil {
		return 0, err
	}
	var listed struct {
		Versions []struct {
			Version int64 `json:"version"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		return 0, err
	}
	var max int64
	for _, v := range listed.Versions {
		if v.Version > max {
			max = v.Version
		}
	}
	return max, nil
}

func (r *devReplayer) functions(ctx context.Context) error {
	if len(r.w.functions) > 0 && r.entry.Functions == nil {
		r.entry.Functions = map[string]devLockFunc{}
	}
	for _, f := range r.w.functions {
		r.step = "function " + f.name
		remote, err := r.remoteFunction(ctx, f.name)
		if err != nil {
			return err
		}
		if remote == nil {
			if err := r.createReplayedFunction(ctx, f); err != nil {
				return err
			}
			continue
		}
		if err := r.replayExistingFunction(ctx, f, remote); err != nil {
			return err
		}
	}
	return nil
}

func (r *devReplayer) remoteFunction(ctx context.Context, name string) (*devRemoteFunc, error) {
	raw, err := r.c.GetFunction(ctx, name)
	if err == nil {
		remote := &devRemoteFunc{}
		if err := json.Unmarshal(raw, remote); err != nil {
			return nil, err
		}
		return remote, nil
	}
	var apiErr *gosdk.APIError
	if !errors.As(err, &apiErr) || (apiErr.Code != "not_found" && apiErr.Code != "gofunction_not_found") {
		return nil, err
	}
	return nil, nil
}

func (r *devReplayer) createReplayedFunction(ctx context.Context, f devFunction) error {
	r.act("create", "function", f.name, "")
	if !r.apply {
		return nil
	}
	desc := ""
	if f.description != nil {
		desc = *f.description
	}
	// 先建未生效版本，试跑通过后再 activate。
	created, err := r.c.CreateFunctionWith(ctx, f.name, f.source, desc, false)
	if err != nil {
		return err
	}
	ver, err := r.latestVersion(ctx, f.name, created)
	if err != nil {
		return err
	}
	return r.testActivateAndLock(ctx, f, ver)
}

func (r *devReplayer) replayExistingFunction(ctx context.Context, f devFunction, remote *devRemoteFunc) error {
	if remote.ActiveVersion > 0 && hashx.SumString(remote.Source) == f.sha {
		r.act("skip", "function", f.name, fmt.Sprintf("v%d", remote.ActiveVersion))
		if r.apply {
			r.entry.Functions[f.name] = devLockFunc{SHA: f.sha, Version: remote.ActiveVersion}
		}
	} else if err := r.updateReplayedFunction(ctx, f, remote); err != nil {
		return err
	}
	return r.updateReplayedDescription(ctx, f, remote.Description)
}

func (r *devReplayer) updateReplayedFunction(ctx context.Context, f devFunction, remote *devRemoteFunc) error {
	reuse, err := r.reusableFunctionVersion(ctx, f, remote)
	if err != nil {
		return err
	}
	detail := fmt.Sprintf("v%d → 新版本", remote.ActiveVersion)
	if reuse > 0 {
		detail = fmt.Sprintf("v%d → 复用 v%d", remote.ActiveVersion, reuse)
	}
	r.act("update", "function", f.name, detail)
	if !r.apply {
		return nil
	}
	ver := reuse
	if ver == 0 {
		created, err := r.c.CreateFunctionVersion(ctx, f.name, f.source, "dev "+f.sha[:12], false)
		if err != nil {
			return err
		}
		if ver, err = r.latestVersion(ctx, f.name, created); err != nil {
			return err
		}
	}
	return r.testActivateAndLock(ctx, f, ver)
}

func (r *devReplayer) reusableFunctionVersion(ctx context.Context, f devFunction, remote *devRemoteFunc) (int64, error) {
	if remote.LatestVersion <= 0 || remote.LatestVersion == remote.ActiveVersion {
		return 0, nil
	}
	raw, err := r.c.GetFunctionVersion(ctx, f.name, strconv.FormatInt(remote.LatestVersion, 10))
	if err != nil {
		return 0, err
	}
	var v struct {
		Source string `json:"source"`
	}
	if json.Unmarshal(raw, &v) == nil && hashx.SumString(v.Source) == f.sha {
		return remote.LatestVersion, nil
	}
	return 0, nil
}

func (r *devReplayer) testActivateAndLock(ctx context.Context, f devFunction, version int64) error {
	if err := r.testFunction(ctx, f, version); err != nil {
		return err
	}
	if _, err := r.c.ActivateFunctionVersion(ctx, f.name, strconv.FormatInt(version, 10)); err != nil {
		return err
	}
	r.entry.Functions[f.name] = devLockFunc{SHA: f.sha, Version: version}
	return nil
}

func (r *devReplayer) updateReplayedDescription(ctx context.Context, f devFunction, remoteDescription string) error {
	if f.description == nil || remoteDescription == *f.description {
		return nil
	}
	r.act("update", "function", f.name, "description")
	if r.apply {
		if _, err := r.c.UpdateFunction(ctx, f.name, *f.description); err != nil {
			return err
		}
	}
	return nil
}

type devRemoteCron struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	ScheduleKind    string `json:"schedule_kind"`
	CronExpr        string `json:"cron_expr"`
	IntervalSeconds int64  `json:"interval_seconds"`
	RunAt           string `json:"run_at"`
	TargetKind      string `json:"target_kind"`
	FuncFile        string `json:"func_file"`
	FuncExport      string `json:"func_export"`
	InputJSON       string `json:"input_json"`
	Enabled         bool   `json:"enabled"`
}

func devCronInput(w devCron) gosdk.CronJobInput {
	enabled := w.enabled
	desc := ""
	if w.description != nil {
		desc = *w.description
	}
	in := gosdk.CronJobInput{
		Name: w.name, Description: desc, ScheduleKind: w.kind,
		FuncFile: w.funcFile, FuncExport: w.funcExport, InputJSON: w.inputJSON, Enabled: &enabled,
	}
	switch w.kind {
	case "cron":
		in.CronExpr = w.cronExpr
	case "interval":
		in.IntervalSeconds = w.interval
	default:
		in.RunAt = w.runAt
	}
	return in
}

func devJSONEqual(a, b string) bool {
	if a == "" {
		a = "{}"
	}
	var va, vb any
	if json.Unmarshal([]byte(a), &va) != nil || json.Unmarshal([]byte(b), &vb) != nil {
		return a == b
	}
	return reflect.DeepEqual(va, vb)
}

func devCronDiff(old devRemoteCron, w devCron) []string {
	var diff []string
	switch {
	case old.ScheduleKind != w.kind:
		diff = append(diff, "schedule_kind")
	case w.kind == "cron" && old.CronExpr != w.cronExpr:
		diff = append(diff, "cron_expr")
	case w.kind == "interval" && old.IntervalSeconds != w.interval:
		diff = append(diff, "interval_seconds")
	case w.kind == "once":
		ot, e1 := time.Parse(time.RFC3339, old.RunAt)
		wt, e2 := time.Parse(time.RFC3339, w.runAt)
		if e1 != nil || e2 != nil || !ot.Equal(wt) {
			diff = append(diff, "run_at")
		}
	}
	if old.FuncFile != w.funcFile {
		diff = append(diff, "func_file")
	}
	if old.FuncExport != w.funcExport {
		diff = append(diff, "func_export")
	}
	if !devJSONEqual(old.InputJSON, w.inputJSON) {
		diff = append(diff, "input_json")
	}
	if old.Enabled != w.enabled {
		diff = append(diff, "enabled")
	}
	// nil 表示清单没写 description：保留远端值。写了（含空串）才比较，这样能把描述清空。
	if w.description != nil && old.Description != *w.description {
		diff = append(diff, "description")
	}
	return diff
}

// devHintProjectMissing 把「目标实例上还没有这个项目」的 404 换成可执行的提示：
// 项目要由用户在控制台用同一 ID 建（dev 不建项目）。
func devHintProjectMissing(err error, t devTarget) error {
	var api *gosdk.APIError
	if !errors.As(err, &api) {
		return err
	}
	if api.Status != 404 || (api.Code != "database_not_found" && api.Code != "not_found") {
		return err
	}
	return &devError{code: "project_not_found", exit: exitAPI,
		msg: fmt.Sprintf("目标 %s 上没有项目 %s：先在控制台「新建项目」里用同一个项目 ID 建项目并签发 Key", t.Label, t.Project)}
}

func (r *devReplayer) cron(ctx context.Context) error {
	if len(r.w.cronJobs) == 0 {
		return nil
	}
	r.step = "cron list"
	raw, err := r.c.ListCronJobs(ctx)
	if err != nil {
		return err
	}
	var listed struct {
		Jobs []devRemoteCron `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		return err
	}
	if r.entry.Cron == nil {
		r.entry.Cron = map[string]devLockCron{}
	}
	for _, w := range r.w.cronJobs {
		r.step = "cron " + w.name
		var old *devRemoteCron
		for i := range listed.Jobs {
			if listed.Jobs[i].Name == w.name {
				old = &listed.Jobs[i]
				break
			}
		}
		if old == nil && w.kind == "once" {
			if at, err := time.Parse(time.RFC3339, w.runAt); err == nil && !at.After(r.now()) {
				r.warn("cron %s: run_at 已过期，跳过创建", w.name)
				r.act("skip", "cron", w.name, "run_at 已过期")
				continue
			}
		}
		if old == nil {
			r.act("create", "cron", w.name, w.kind)
			if r.apply {
				created, err := r.c.CreateCronJob(ctx, devCronInput(w))
				if err != nil {
					return err
				}
				var j struct {
					ID string `json:"id"`
				}
				_ = json.Unmarshal(created, &j)
				r.entry.Cron[w.name] = devLockCron{ID: j.ID}
			}
			continue
		}
		if old.TargetKind != "" && old.TargetKind != "function" {
			return &devError{code: "cron_target_mismatch", exit: exitAPI,
				msg: fmt.Sprintf("远端 cron %s 的目标不是云函数（%s），换个任务名", w.name, old.TargetKind)}
		}
		if diff := devCronDiff(*old, w); len(diff) > 0 {
			r.act("update", "cron", w.name, strings.Join(diff, ","))
			if r.apply {
				// update 是全量替换：清单没写 description 时用远端现值，避免被清空。
				if w.description == nil {
					w.description = &old.Description
				}
				if _, err := r.c.UpdateCronJob(ctx, old.ID, devCronInput(w)); err != nil {
					return err
				}
			}
		} else {
			r.act("skip", "cron", w.name, "")
		}
		if r.apply {
			r.entry.Cron[w.name] = devLockCron{ID: old.ID}
		}
	}
	return nil
}

// writeDevEnvHint 生成不含密钥的环境提示文件，业务侧复制需要的行。
