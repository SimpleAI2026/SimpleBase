// 文件说明：handler_validation_test.go — internal/api 的单元测试（TestDatabaseHandler_MissingContextAndValidation、TestSQLHandler_MissingContextAndSystem 等）
//
// 用法
//
//	go test ./internal/api -run 'TestDatabaseHandler_MissingContextAndValidation|TestSQLHandler_MissingContextAndSystem|TestDataHandler_Branches|TestProjectsHandler_MoreBranches'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/handler_validation_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	setupBareEcho(injectPrincipal, injectProject bool, register func(*echo.Echo)) *echo.Echo
//	TestDatabaseHandler_MissingContextAndValidation(t *testing.T)
//	TestSQLHandler_MissingContextAndSystem(t *testing.T)
//	TestDataHandler_Branches(t *testing.T)
//	TestProjectsHandler_MoreBranches(t *testing.T)
//	wrapInvalid(msg string) error
//	TestS3Handler_MissingProjectAndIndex(t *testing.T)
//	TestSQLTypesZeroValues(t *testing.T)

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

func setupBareEcho(injectPrincipal, injectProject bool, register func(*echo.Echo)) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			if injectPrincipal {
				ctx = WithPrincipal(ctx, auth.Principal{
					APIKeyID: "key-test", TenantID: "tenant-1",
					ProjectIDs:  map[string]struct{}{"proj-1": {}},
					Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}, auth.DatabaseWrite: {}, auth.DatabaseAdmin: {}},
				})
			}
			if injectProject {
				ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			}
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	register(e)
	return e
}

func TestProjectsHandler_MoreBranches(t *testing.T) {
	e := setupProjectsRouter(t, &fakeProjectCatalog{})
	if rec := doRequest(e, http.MethodGet, "/v1/projects", nil); rec.Code != http.StatusOK {
		t.Fatalf("list empty catalog %d %s", rec.Code, rec.Body.String())
	}
	// missing principal
	e2 := echo.New()
	h := NewProjectsHandler(&fakeProjectCatalog{}, nil)
	e2.GET("/v1/projects", h.ListProjects)
	e2.POST("/v1/projects", h.CreateProject)
	rec := doRequest(e2, http.MethodGet, "/v1/projects", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("list no prin %d", rec.Code)
	}
	rec = doRequest(e2, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "x"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("create no prin %d", rec.Code)
	}

	cat := &fakeProjectCatalog{err: catalog.ErrNotFound}
	e = setupProjectsRouter(t, cat)
	rec = doRequest(e, http.MethodGet, "/v1/projects", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("list err %d", rec.Code)
	}
	cat.err = catalog.ErrInvalidName
	rec = doRequest(e, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "x"})
	if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("invalid_project_name")) {
		t.Fatalf("invalid name %d %s", rec.Code, rec.Body.String())
	}
	cat.err = errors.New("catalog: invalid name: id must be 8 chars [A-Za-z0-9-]")
	// errors.Is won't match unless wrapped
	cat.err = wrapInvalid("id must be 8 chars [A-Za-z0-9-]")
	rec = doRequest(e, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "x", ID: "not-valid!"})
	if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("invalid_project_id")) {
		t.Fatalf("invalid id %d %s", rec.Code, rec.Body.String())
	}
	cat.err = wrapInvalid("reserved project id")
	rec = doRequest(e, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "x"})
	if !bytes.Contains(rec.Body.Bytes(), []byte("invalid_project_id")) {
		t.Fatalf("reserved %s", rec.Body.String())
	}
	cat.err = errors.New("other")
	rec = doRequest(e, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "x"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("other %d", rec.Code)
	}
}

func wrapInvalid(msg string) error {
	return errors.Join(catalog.ErrInvalidName, errors.New(msg))
}

func TestSQLTypesZeroValues(t *testing.T) {
	var q QueryRequest
	var e ExecuteRequest
	var b BatchRequest
	var qr QueryResponse
	var er ExecuteResponse
	var br BatchResponse
	_ = q
	_ = e
	_ = b
	if qr.RowCount != 0 || er.RowsAffected != 0 || br.Error != nil {
		t.Fatal("zero values")
	}
	item := BatchResultItem{Index: 1, ErrorCode: "x"}
	if item.Index != 1 {
		t.Fatal(item)
	}
	if _, err := json.Marshal(BatchError{FailedIndex: 0, Code: "c", Message: "m"}); err != nil {
		t.Fatal(err)
	}
}
