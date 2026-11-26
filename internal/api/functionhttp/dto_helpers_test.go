// 文件说明：dto_helpers_test.go — internal/api 的单元测试（TestGoFunctionErrorHelpers、TestScheduleAndCronDTOs 等）
//
// 用法
//
//	go test ./internal/api -run 'TestGoFunctionErrorHelpers|TestScheduleAndCronDTOs'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/dto_helpers_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestGoFunctionErrorHelpers(t *testing.T)
//	TestScheduleAndCronDTOs(t *testing.T)

package functionhttp

import (
	"errors"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

func TestGoFunctionErrorHelpers(t *testing.T) {
	if mapGoFunctionValidationError(nil, "r") != nil {
		t.Fatal("nil")
	}
	if ae, ok := mapGoFunctionValidationError(errors.New("函数不符合约定"), "r").(*httpkit.APIError); !ok || ae.Body.Error.Code != "gofunction_signature_invalid" {
		t.Fatalf("%v", mapGoFunctionValidationError(errors.New("函数不符合约定"), "r"))
	}
	if ae, ok := mapGoFunctionValidationError(errors.New("至少导出一个 HTTP 函数"), "r").(*httpkit.APIError); !ok || ae.Body.Error.Code != "gofunction_signature_invalid" {
		t.Fatal("export")
	}
	if ae, ok := mapGoFunctionValidationError(errors.New("syntax error"), "r").(*httpkit.APIError); !ok || ae.Body.Error.Code != "gofunction_compile_error" {
		t.Fatal("compile")
	}

	if mapGoFunctionRunError(nil, "r") != nil {
		t.Fatal("run nil")
	}
	cases := []struct {
		err  error
		code string
	}{
		{gofunction.ErrFunctionNotFound, "function_not_found"},
		{gofunction.ErrTimeout, "request_timeout"},
		{gofunction.ErrBind, "invalid_request"},
		{gofunction.ErrCompile, "gofunction_compile_error"},
		{errors.New("boom"), "gofunction_runtime_error"},
	}
	for _, tc := range cases {
		ae := mapGoFunctionRunError(tc.err, "r").(*httpkit.APIError)
		if ae.Body.Error.Code != tc.code {
			t.Fatalf("%v → %s want %s", tc.err, ae.Body.Error.Code, tc.code)
		}
	}

	if strutil.TruncateEllipsis("abc", 10) != "abc" {
		t.Fatal("short")
	}
	long := strutil.TruncateEllipsis("abcdefghij", 3)
	if long != "abc…" {
		t.Fatalf("trunc=%q", long)
	}
	if err := validateGoFunctionSource(""); err == nil {
		t.Fatal("empty source")
	}
	if err := validateGoFunctionSource(string(make([]byte, gofunctionMaxSourceBytes+1))); err == nil {
		t.Fatal("too large")
	}
	if err := validateGoFunctionSource("package main"); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleAndCronDTOs(t *testing.T) {
	now := time.Now().UTC()
	cd := toCronJobDTO(systemdb.CronJob{
		ID: "j", Name: "n", IntervalSeconds: 5, LastRunAt: now, NextRunAt: now,
	}, true)
	if cd.IntervalSeconds == nil || *cd.IntervalSeconds != 5 || !cd.TargetMissing {
		t.Fatalf("%+v", cd)
	}
	crd := toCronJobRunDTO(systemdb.CronJobRun{ID: "r", StartedAt: now, FinishedAt: now})
	if crd.StartedAt == nil || crd.FinishedAt == nil {
		t.Fatalf("%+v", crd)
	}
}
