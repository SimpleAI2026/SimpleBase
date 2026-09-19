package api

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func TestGoFunctionErrorHelpers(t *testing.T) {
	if mapGoFunctionValidationError(nil, "r") != nil {
		t.Fatal("nil")
	}
	if ae, ok := mapGoFunctionValidationError(errors.New("函数不符合约定"), "r").(*APIError); !ok || ae.Body.Error.Code != "gofunction_signature_invalid" {
		t.Fatalf("%v", mapGoFunctionValidationError(errors.New("函数不符合约定"), "r"))
	}
	if ae, ok := mapGoFunctionValidationError(errors.New("至少导出一个 HTTP 函数"), "r").(*APIError); !ok || ae.Body.Error.Code != "gofunction_signature_invalid" {
		t.Fatal("export")
	}
	if ae, ok := mapGoFunctionValidationError(errors.New("syntax error"), "r").(*APIError); !ok || ae.Body.Error.Code != "gofunction_compile_error" {
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
		ae := mapGoFunctionRunError(tc.err, "r").(*APIError)
		if ae.Body.Error.Code != tc.code {
			t.Fatalf("%v → %s want %s", tc.err, ae.Body.Error.Code, tc.code)
		}
	}

	if truncateMessage("abc", 10) != "abc" {
		t.Fatal("short")
	}
	long := truncateMessage("abcdefghij", 3)
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
	sd := toAgentScheduleDTO(systemdb.AgentSchedule{
		ID: "s", AgentID: "a", Prompt: "p", CronExpr: "* * * * *", Enabled: true,
		LastRunAt: now, NextRunAt: now, CreatedAt: now, UpdatedAt: now,
	}, "Agent")
	if sd.LastRunAt == nil || sd.NextRunAt == nil || sd.AgentName != "Agent" {
		t.Fatalf("%+v", sd)
	}
	rd := toAgentScheduleRunDTO(systemdb.AgentScheduleRun{
		ID: "r", StartedAt: now, FinishedAt: now, CreatedAt: now,
	})
	if rd.StartedAt == nil || rd.FinishedAt == nil {
		t.Fatalf("%+v", rd)
	}
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
	_ = fmt.Sprintf("%s", sd.ID)
}
