package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestReproExact(t *testing.T) {
	e, _ := setupCronJobTestRouter(t, true, "proj-1")

	rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs",
		`{"name":"nightly","description":"每晚","schedule_kind":"cron","cron_expr":"0 2 * * *","func_file":"hello","func_export":"Hello","input_json":"{\"name\":\"cron\"}"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var job cronJobDTO
	json.Unmarshal(rec.Body.Bytes(), &job)

	// 列表
	rec = postJSON(t, e, http.MethodGet, "/v1/projects/proj-1/cron-jobs", "")
	if rec.Code != http.StatusOK || len(decodeCronJobs(t, rec)) != 1 {
		t.Fatalf("list: %d", rec.Code)
	}
	// 详情
	rec = postJSON(t, e, http.MethodGet, "/v1/projects/proj-1/cron-jobs/"+job.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}
	// 更新禁用（与原测试完全相同）
	rec = postJSON(t, e, http.MethodPatch, "/v1/projects/proj-1/cron-jobs/"+job.ID,
		`{"description":"改间隔","schedule_kind":"interval","interval_seconds":600,"func_file":"hello","func_export":"Hello","enabled":false}`)
	t.Logf("patch disable: %d %s", rec.Code, rec.Body.String())
	if !strings.Contains(rec.Body.String(), `"next_run_at"`) {
		t.Log("OK: next_run_at absent")
	} else {
		t.Log("BUG PRESENT: next_run_at present")
	}
}
