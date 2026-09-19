package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/internal/cronjob"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

// setupCronJobTestRouter 构造带 CronJobHandler 的测试路由（含一个可用云函数）。
func setupCronJobTestRouter(t *testing.T, writable bool, projectID string) (*echo.Echo, *systemdb.Store) {
	t.Helper()
	e, store := setupGoFunctionTestRouter(t, writable, projectID)

	// 种子云函数：hello.go 导出 Hello
	if _, err := store.CreateGoFunction(t.Context(), systemdb.GoFunction{
		ProjectID: projectID, Name: "hello",
		Source: "package main\ntype Request struct {\n\tName string `json:\"name\"`\n}\ntype Response struct {\n\tMessage string `json:\"message\"`\n}\nfunc Hello(req Request) Response {\n\treturn Response{Message: \"hello, \" + req.Name}\n}",
		Exports: []string{"Hello"},
	}); err != nil {
		t.Fatal(err)
	}

	h := NewCronJobHandler(store, writable, nil, nil)
	v1 := e.Group("/v1")
	p := v1.Group("/projects/:projectID")
	p.GET("/cron-jobs", h.List)
	p.POST("/cron-jobs", h.Create)
	p.GET("/cron-jobs/:jobID", h.Get)
	p.PATCH("/cron-jobs/:jobID", h.Update)
	p.DELETE("/cron-jobs/:jobID", h.Delete)
	p.GET("/cron-jobs/:jobID/runs", h.ListRuns)
	p.POST("/cron-jobs/:jobID/trigger", h.Trigger)
	return e, store
}

func decodeCronJobs(t *testing.T, rec *httptest.ResponseRecorder) []cronJobDTO {
	t.Helper()
	var body struct {
		Jobs []cronJobDTO `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	return body.Jobs
}

func TestCronJob_CRUDLifecycle(t *testing.T) {
	e, _ := setupCronJobTestRouter(t, true, "proj-1")

	// 创建：cron 模式
	rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs",
		`{"name":"nightly","description":"每晚","schedule_kind":"cron","cron_expr":"0 2 * * *","func_file":"hello","func_export":"Hello","input_json":"{\"name\":\"cron\"}"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var job cronJobDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if !job.Enabled || job.NextRunAt == nil || job.TargetMissing {
		t.Fatalf("unexpected created job: %+v", job)
	}

	// 列表
	rec = postJSON(t, e, http.MethodGet, "/v1/projects/proj-1/cron-jobs", "")
	if rec.Code != http.StatusOK || len(decodeCronJobs(t, rec)) != 1 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}

	// 详情
	rec = postJSON(t, e, http.MethodGet, "/v1/projects/proj-1/cron-jobs/"+job.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}

	// 更新：切 interval 模式 + 禁用
	rec = postJSON(t, e, http.MethodPatch, "/v1/projects/proj-1/cron-jobs/"+job.ID,
		`{"description":"改间隔","schedule_kind":"interval","interval_seconds":600,"func_file":"hello","func_export":"Hello","enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	var updated cronJobDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || updated.CronExpr != "" || updated.IntervalSeconds == nil || *updated.IntervalSeconds != 600 {
		t.Fatalf("unexpected updated job: %+v", updated)
	}
	// 禁用时 next_run_at 保留（重新启用无需重算；调度器按 enabled=1 过滤天然跳过）。

	// 运行记录为空
	rec = postJSON(t, e, http.MethodGet, "/v1/projects/proj-1/cron-jobs/"+job.ID+"/runs", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"runs":[]`) {
		t.Fatalf("runs: %d %s", rec.Code, rec.Body.String())
	}

	// 删除
	rec = postJSON(t, e, http.MethodDelete, "/v1/projects/proj-1/cron-jobs/"+job.ID, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(t, e, http.MethodGet, "/v1/projects/proj-1/cron-jobs", "")
	if len(decodeCronJobs(t, rec)) != 0 {
		t.Fatal("job should be archived")
	}
	// 删除后同名可复建
	rec = postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs",
		`{"name":"nightly","schedule_kind":"cron","cron_expr":"0 2 * * *","func_file":"hello","func_export":"Hello"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("recreate: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCronJob_ValidationErrors(t *testing.T) {
	e, _ := setupCronJobTestRouter(t, true, "proj-1")

	cases := []struct {
		name string
		body string
		code int
		errc string
	}{
		{"bad name", `{"name":"1bad","schedule_kind":"cron","cron_expr":"0 2 * * *","func_file":"hello","func_export":"Hello"}`, 400, "invalid_cron_job_name"},
		{"bad cron", `{"name":"ok","schedule_kind":"cron","cron_expr":"99 * * * *","func_file":"hello","func_export":"Hello"}`, 400, "invalid_cron_expression"},
		{"missing cron", `{"name":"ok","schedule_kind":"cron","func_file":"hello","func_export":"Hello"}`, 400, "invalid_cron_expression"},
		{"bad interval", `{"name":"ok","schedule_kind":"interval","interval_seconds":10,"func_file":"hello","func_export":"Hello"}`, 400, "invalid_interval"},
		{"bad kind", `{"name":"ok","schedule_kind":"weekly","func_file":"hello","func_export":"Hello"}`, 400, "invalid_request"},
		{"missing target", `{"name":"ok","schedule_kind":"interval","interval_seconds":600,"func_file":"hello","func_export":""}`, 400, "invalid_request"},
		{"bad input json", `{"name":"ok","schedule_kind":"interval","interval_seconds":600,"func_file":"hello","func_export":"Hello","input_json":"{bad"}`, 400, "invalid_request"},
		{"target file missing", `{"name":"ok","schedule_kind":"interval","interval_seconds":600,"func_file":"nope","func_export":"Hello"}`, 400, "gofunction_not_found"},
		{"target not exported", `{"name":"ok","schedule_kind":"interval","interval_seconds":600,"func_file":"hello","func_export":"helper"}`, 400, "function_not_exported"},
	}
	for _, tc := range cases {
		rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs", tc.body)
		if rec.Code != tc.code {
			t.Errorf("%s: want %d, got %d %s", tc.name, tc.code, rec.Code, rec.Body.String())
			continue
		}
		if !strings.Contains(rec.Body.String(), tc.errc) {
			t.Errorf("%s: want error code %s, got %s", tc.name, tc.errc, rec.Body.String())
		}
	}
}

func TestCronJob_DuplicateConflict(t *testing.T) {
	e, _ := setupCronJobTestRouter(t, true, "proj-1")
	body := `{"name":"dup","schedule_kind":"cron","cron_expr":"0 2 * * *","func_file":"hello","func_export":"Hello"}`
	if rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs", body); rec.Code != http.StatusCreated {
		t.Fatalf("first create: %d", rec.Code)
	}
	rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs", body)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "cron_job_exists") {
		t.Fatalf("dup: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCronJob_ReadOnlyMode(t *testing.T) {
	e, _ := setupCronJobTestRouter(t, false, "proj-1")
	rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs",
		`{"name":"ro","schedule_kind":"cron","cron_expr":"0 2 * * *","func_file":"hello","func_export":"Hello"}`)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "readonly_mode") {
		t.Fatalf("readonly create: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCronJob_NotFound(t *testing.T) {
	e, _ := setupCronJobTestRouter(t, true, "proj-1")
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/projects/proj-1/cron-jobs/missing"},
		{http.MethodPatch, "/v1/projects/proj-1/cron-jobs/missing"},
		{http.MethodDelete, "/v1/projects/proj-1/cron-jobs/missing"},
		{http.MethodPost, "/v1/projects/proj-1/cron-jobs/missing/trigger"},
	} {
		rec := postJSON(t, e, tc.method, tc.path, `{}`)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "cron_job_not_found") {
			t.Errorf("%s %s: want 404 cron_job_not_found, got %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestCronJob_TargetMissingFlag(t *testing.T) {
	e, store := setupCronJobTestRouter(t, true, "proj-1")
	rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs",
		`{"name":"tm","schedule_kind":"cron","cron_expr":"0 2 * * *","func_file":"hello","func_export":"Hello"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	// 归档目标云函数 → target_missing=true
	if err := store.ArchiveGoFunction(t.Context(), "proj-1", "hello"); err != nil {
		t.Fatal(err)
	}
	rec = postJSON(t, e, http.MethodGet, "/v1/projects/proj-1/cron-jobs", "")
	jobs := decodeCronJobs(t, rec)
	if len(jobs) != 1 || !jobs[0].TargetMissing {
		t.Fatalf("want target_missing=true, got %+v", jobs)
	}
}

func TestCronJob_TriggerNoScheduler(t *testing.T) {
	e, _ := setupCronJobTestRouter(t, true, "proj-1")
	rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs",
		`{"name":"tg","schedule_kind":"cron","cron_expr":"0 2 * * *","func_file":"hello","func_export":"Hello"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}
	var job cronJobDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &job)

	// handler 构造时 scheduler=nil → trigger 503
	rec = postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs/"+job.ID+"/trigger", "")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "cron_scheduler_unavailable") {
		t.Fatalf("trigger without scheduler: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCronJob_NextRunComputed(t *testing.T) {
	e, _ := setupCronJobTestRouter(t, true, "proj-1")
	rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs",
		`{"name":"nx","schedule_kind":"cron","cron_expr":"30 3 * * *","func_file":"hello","func_export":"Hello"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var job cronJobDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &job)
	if job.NextRunAt == nil {
		t.Fatal("next_run_at missing")
	}
	next := job.NextRunAt.UTC()
	if next.Minute() != 30 || next.Hour() != 3 || next.Second() != 0 {
		t.Fatalf("next not on 03:30 boundary: %v", next)
	}
	if next.Before(time.Now().UTC()) {
		t.Fatalf("next in the past: %v", next)
	}
}

// TestCronJob_TriggerWithScheduler 端到端：真实调度器 + systemDBRunner 路径由 app 层
// 装配；此处用 cronjob.Scheduler + 真实 RunJSON 内核构造（app 包外不可导出 runner，
// 故通过 systemdb store 直接驱动）。改为验证 scheduler 非 nil 时 trigger 走通 202。
func TestCronJob_TriggerWithScheduler(t *testing.T) {
	e, store := setupCronJobTestRouter(t, true, "proj-1")

	// 直接构造调度器注入 handler 路由（覆盖 setup 中 scheduler=nil 的默认）
	sched := cronjob.NewScheduler(store, &cronJobTestRunner{store: store})
	h := NewCronJobHandler(store, true, sched, nil)
	v1 := e.Group("/v1")
	p := v1.Group("/projects/:projectID")
	p.POST("/cron-jobs/:jobID/trigger2", h.Trigger)

	rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs",
		`{"name":"trg","schedule_kind":"cron","cron_expr":"0 2 * * *","func_file":"hello","func_export":"Hello"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var job cronJobDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &job)

	rec = postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/cron-jobs/"+job.ID+"/trigger2", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("trigger: %d %s", rec.Code, rec.Body.String())
	}
	sched.Stop()

	// 等 in-flight 异步执行落库后确认 manual run 出现
	deadline := time.Now().Add(5 * time.Second)
	for {
		runs, err := store.ListCronJobRuns(t.Context(), "proj-1", job.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) > 0 && runs[0].Status != systemdb.CronJobRunRunning {
			if runs[0].Trigger != "manual" || runs[0].Status != systemdb.CronJobRunCompleted {
				t.Fatalf("unexpected run: %+v", runs[0])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("run not completed in time: %+v", runs)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// cronJobTestRunner 真实执行 hello.Hello（复用 gofunction 内核）。
type cronJobTestRunner struct{ store *systemdb.Store }

func (r *cronJobTestRunner) RunFunction(ctx context.Context, projectID, file, export string, input json.RawMessage) ([]byte, error) {
	g, err := r.store.GetGoFunction(ctx, projectID, file)
	if err != nil {
		return nil, err
	}
	return gofunction.RunJSON(ctx, "cron-test", file, g.Source, export, input)
}
