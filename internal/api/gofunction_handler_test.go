package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

const testGoFunctionSrc = `package main

type Request struct {
	Name string ` + "`json:\"name\"`" + `
}

type Response struct {
	Message string ` + "`json:\"message\"`" + `
}

func Hello(req Request) Response {
	return Response{Message: "hello, " + req.Name}
}
`

// setupGoFunctionTestRouter 构造带 GoFunctionHandler 的测试路由。
// writable=false 可测 503；adminProject 模拟系统项目。
func setupGoFunctionTestRouter(t *testing.T, writable bool, projectID string) (*echo.Echo, *systemdb.Store) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := systemdb.ApplySystemMigrations(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := systemdb.NewStoreForTest(db)

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = errorHandler(Dependencies{Logger: nil})
	h := NewGoFunctionHandler(store, writable, nil)

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := auth.Principal{
				APIKeyID:   "key-test",
				TenantID:   "tenant-1",
				ProjectIDs: map[string]struct{}{projectID: {}},
				Permissions: map[auth.Permission]struct{}{
					auth.DatabaseRead:  {},
					auth.DatabaseWrite: {},
				},
			}
			ctx := WithPrincipal(c.Request().Context(), p)
			ctx = WithProject(ctx, ProjectContext{ID: projectID, TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	v1 := e.Group("/v1")
	p := v1.Group("/projects/:projectID")
	p.GET("/gofunctions", h.List)
	p.POST("/gofunctions", h.Create)
	p.GET("/gofunctions/:name", h.Get)
	p.PUT("/gofunctions/:name", h.Update)
	p.DELETE("/gofunctions/:name", h.Delete)

	goGrp := e.Group("/go/:projectID")
	goGrp.POST("/:name/:functionName", h.Invoke)
	goGrp.GET("/:name/:functionName", h.MethodNotAllowed)

	return e, store
}

func postJSON(t *testing.T, e *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestGoFunction_CreateListGetUpdateDelete(t *testing.T) {
	e, _ := setupGoFunctionTestRouter(t, true, "proj-1")

	// 创建
	body, _ := json.Marshal(map[string]string{"name": "hello", "source": testGoFunctionSrc})
	rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/gofunctions", string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created goFunctionDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.File != "hello.go" || len(created.Exports) != 1 || created.Exports[0] != "Hello" {
		t.Fatalf("unexpected dto: %+v", created)
	}

	// 列表（不含 source）
	rec = postJSON(t, e, http.MethodGet, "/v1/projects/proj-1/gofunctions", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var listResp struct {
		Functions []goFunctionDTO `json:"functions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Functions) != 1 || listResp.Functions[0].Source != "" {
		t.Fatalf("list should omit source: %+v", listResp.Functions)
	}

	// 详情（含 source）
	rec = postJSON(t, e, http.MethodGet, "/v1/projects/proj-1/gofunctions/hello", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}
	var detail goFunctionDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail.Source, "func Hello") {
		t.Fatalf("detail should include source: %+v", detail)
	}

	// 更新（改函数名）
	updated := strings.Replace(testGoFunctionSrc, "Hello", "Ping", -1)
	body, _ = json.Marshal(map[string]string{"source": updated})
	rec = postJSON(t, e, http.MethodPut, "/v1/projects/proj-1/gofunctions/hello", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	var updDTO goFunctionDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &updDTO); err != nil {
		t.Fatal(err)
	}
	if updDTO.Exports[0] != "Ping" {
		t.Fatalf("exports should update: %+v", updDTO)
	}

	// 删除 → 204
	rec = postJSON(t, e, http.MethodDelete, "/v1/projects/proj-1/gofunctions/hello", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	// 再删 → 404
	rec = postJSON(t, e, http.MethodDelete, "/v1/projects/proj-1/gofunctions/hello", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d", rec.Code)
	}
}

func TestGoFunction_CreateInvalidSignature(t *testing.T) {
	e, _ := setupGoFunctionTestRouter(t, true, "proj-1")

	// 多参函数 → 400 signature_invalid，message 指明函数名
	body, _ := json.Marshal(map[string]string{"name": "bad", "source": "package main\nfunc Bad(a, b int) int { return a + b }"})
	rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/gofunctions", string(body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var errResp APIErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatal(err)
	}
	if errResp.Error.Code != "gofunction_signature_invalid" {
		t.Fatalf("want gofunction_signature_invalid, got %s", errResp.Error.Code)
	}
	if !strings.Contains(errResp.Error.Message, "Bad") {
		t.Fatalf("message should mention Bad: %s", errResp.Error.Message)
	}

	// 仅未导出函数 → 400
	body, _ = json.Marshal(map[string]string{"name": "lower", "source": "package main\nfunc helper(x int) int { return x }"})
	rec = postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/gofunctions", string(body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for unexported-only, got %d", rec.Code)
	}

	// 非法 name → invalid_gofunction_name
	body, _ = json.Marshal(map[string]string{"name": "1bad", "source": testGoFunctionSrc})
	rec = postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/gofunctions", string(body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bad name, got %d", rec.Code)
	}
	json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "invalid_gofunction_name" {
		t.Fatalf("want invalid_gofunction_name, got %s", errResp.Error.Code)
	}

	// 编译错误 → gofunction_compile_error
	body, _ = json.Marshal(map[string]string{"name": "broken", "source": "package main\nfunc Broken( {"})
	rec = postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/gofunctions", string(body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for compile error, got %d", rec.Code)
	}
	json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "gofunction_compile_error" {
		t.Fatalf("want gofunction_compile_error, got %s", errResp.Error.Code)
	}
}

func TestGoFunction_CreateDuplicate409(t *testing.T) {
	e, _ := setupGoFunctionTestRouter(t, true, "proj-1")
	body, _ := json.Marshal(map[string]string{"name": "hello", "source": testGoFunctionSrc})
	if rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/gofunctions", string(body)); rec.Code != http.StatusCreated {
		t.Fatalf("first create: %d", rec.Code)
	}
	rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/gofunctions", string(body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGoFunction_ReadOnly503(t *testing.T) {
	e, _ := setupGoFunctionTestRouter(t, false, "proj-1")
	body, _ := json.Marshal(map[string]string{"name": "hello", "source": testGoFunctionSrc})
	rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/gofunctions", string(body))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 on read-only, got %d", rec.Code)
	}
	var errResp APIErrorBody
	json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "writer_unavailable" {
		t.Fatalf("want writer_unavailable, got %s", errResp.Error.Code)
	}
}

func TestGoFunction_AdminProjectProtected(t *testing.T) {
	adminID := "00000000-0000-0000-0000-000000000099"
	e, _ := setupGoFunctionTestRouter(t, true, adminID)

	// 读返回空列表
	rec := postJSON(t, e, http.MethodGet, "/v1/projects/"+adminID+"/gofunctions", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var listResp struct {
		Functions []goFunctionDTO `json:"functions"`
	}
	json.Unmarshal(rec.Body.Bytes(), &listResp)
	if len(listResp.Functions) != 0 {
		t.Fatalf("admin project should return empty list, got %d", len(listResp.Functions))
	}

	// 写被拒
	body, _ := json.Marshal(map[string]string{"name": "hello", "source": testGoFunctionSrc})
	rec = postJSON(t, e, http.MethodPost, "/v1/projects/"+adminID+"/gofunctions", string(body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 for admin write, got %d", rec.Code)
	}
}

func TestGoFunction_Invoke(t *testing.T) {
	e, _ := setupGoFunctionTestRouter(t, true, "proj-1")

	// 先创建
	body, _ := json.Marshal(map[string]string{"name": "hello", "source": testGoFunctionSrc})
	if rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/gofunctions", string(body)); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	// invoke 成功：响应体即返回值 JSON（无 envelope）
	rec := postJSON(t, e, http.MethodPost, "/go/proj-1/hello/Hello", `{"name":"a"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("invoke: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["message"] != "hello, a" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}

	// GET → 405 JSON（G11）
	req := httptest.NewRequest(http.MethodGet, "/go/proj-1/hello/Hello", nil)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405 for GET, got %d", rec.Code)
	}
	var errResp APIErrorBody
	json.Unmarshal(rec2.Body.Bytes(), &errResp)
	if errResp.Error.Code != "method_not_allowed" {
		t.Fatalf("want method_not_allowed, got %s", errResp.Error.Code)
	}

	// 函数不存在 → 404 function_not_found
	rec = postJSON(t, e, http.MethodPost, "/go/proj-1/hello/Nope", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "function_not_found" {
		t.Fatalf("want function_not_found, got %s", errResp.Error.Code)
	}

	// 云函数不存在 → 404 gofunction_not_found
	rec = postJSON(t, e, http.MethodPost, "/go/proj-1/missing/Foo", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "gofunction_not_found" {
		t.Fatalf("want gofunction_not_found, got %s", errResp.Error.Code)
	}

	// 未导出函数名（小写）→ 404（不暴露存在性）
	rec = postJSON(t, e, http.MethodPost, "/go/proj-1/hello/helper", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unexported, got %d", rec.Code)
	}

	// body 无法绑定 → 400 invalid_request
	rec = postJSON(t, e, http.MethodPost, "/go/proj-1/hello/Hello", `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bad body, got %d: %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "invalid_request" {
		t.Fatalf("want invalid_request, got %s", errResp.Error.Code)
	}
}

func TestGoFunction_InvokeMetrics(t *testing.T) {
	e, store := setupGoFunctionTestRouter(t, true, "proj-1")
	body, _ := json.Marshal(map[string]string{"name": "hello", "source": testGoFunctionSrc})
	if rec := postJSON(t, e, http.MethodPost, "/v1/projects/proj-1/gofunctions", string(body)); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}
	rec := postJSON(t, e, http.MethodPost, "/go/proj-1/hello/Hello", `{"name":"a"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("invoke: %d", rec.Code)
	}
	// RecordMetric 为缓冲写入（64 条阈值），手动 flush 后断言落库
	if err := store.FlushMetrics(context.Background()); err != nil {
		t.Fatal(err)
	}

	// sys_metric_samples 应包含三个 invoke 指标（G12）
	var invokes, duration, errors int
	ctx := context.Background()
	rows, err := store.DB().QueryContext(ctx,
		`SELECT name, COUNT(*) FROM sys_metric_samples WHERE project_id='proj-1' AND name LIKE 'gofunction%' GROUP BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]int{}
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			t.Fatal(err)
		}
		seen[name] = n
	}
	invokes = seen["gofunction_invokes"]
	duration = seen["gofunction_invoke_duration_ms"]
	errors = seen["gofunction_invoke_errors"]
	if invokes == 0 || duration == 0 {
		t.Fatalf("missing invoke metrics: %+v", seen)
	}
	if errors != 0 {
		t.Fatalf("unexpected error metric: %+v", seen)
	}
	if seen["gofunction_compile_ms"] == 0 {
		t.Fatalf("missing compile metric: %+v", seen)
	}
}

// TestGoFunction_InvokeMetricsOnFailure 失败 invoke（404/400）也计 invokes 与 errors（§10.1）
func TestGoFunction_InvokeMetricsOnFailure(t *testing.T) {
	e, store := setupGoFunctionTestRouter(t, true, "proj-1")

	// 云函数不存在 → 404；路径函数名非法 → 404，都应计数
	postJSON(t, e, http.MethodPost, "/go/proj-1/missing/Foo", `{}`)
	postJSON(t, e, http.MethodPost, "/go/proj-1/hello/helper", `{}`)

	if err := store.FlushMetrics(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rows, err := store.DB().QueryContext(ctx,
		`SELECT name, SUM(value_double) FROM sys_metric_samples WHERE project_id='proj-1' AND name IN ('gofunction_invokes','gofunction_invoke_errors') GROUP BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]float64{}
	for rows.Next() {
		var name string
		var v float64
		if err := rows.Scan(&name, &v); err != nil {
			t.Fatal(err)
		}
		seen[name] = v
	}
	if seen["gofunction_invokes"] != 2 {
		t.Fatalf("want 2 invokes, got %+v", seen)
	}
	if seen["gofunction_invoke_errors"] != 2 {
		t.Fatalf("want 2 invoke errors, got %+v", seen)
	}
}
