package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/functionrun"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/trigger"

	_ "github.com/uglyer/go-sqlite3"
)

func TestFunctionTriggerHTTPAndRecordEndToEnd(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	if err := systemdb.EnsureSystemSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := systemdb.NewStoreForTest(db)
	function, err := store.CreateGoFunc(ctx, systemdb.GoFunc{ProjectID: "p1", Name: "handler"})
	if err != nil {
		t.Fatal(err)
	}
	version, err := store.AppendGoFuncVersion(ctx, systemdb.GoFuncVersion{FuncID: function.ID, ProjectID: "p1", Name: "handler", Source: "package main\nfunc Handle(event map[string]interface{}) string { return \"ok\" }\n", Exports: []string{"Handle"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateGoFuncVersion(ctx, "p1", "handler", version.Version); err != nil {
		t.Fatal(err)
	}
	runner := functionrun.NewRunner(store, 2)
	dispatcher := trigger.NewDispatcher(store, runner, 8, 2)
	defer dispatcher.Close()
	handler := NewFunctionTriggerHandler(store, dispatcher, true)
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			request := c.Request().WithContext(WithProject(WithPrincipal(c.Request().Context(), auth.Principal{UserID: "u1"}), ProjectContext{ID: "p1"}))
			c.SetRequest(request)
			return next(c)
		}
	})
	e.POST("/function-triggers", handler.Create)
	e.POST("/function-triggers/:id/invoke", handler.Invoke)
	publisher := NewTriggerEventPublisher(dispatcher)

	rec := postJSON(t, e, http.MethodPost, "/function-triggers", `{"kind":"http","func_file":"handler","func_export":"Handle","enabled":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create trigger: %d %s", rec.Code, rec.Body.String())
	}
	var httpTrigger systemdb.FunctionTrigger
	if err := json.Unmarshal(rec.Body.Bytes(), &httpTrigger); err != nil {
		t.Fatal(err)
	}
	rec = postJSON(t, e, http.MethodPost, "/function-triggers/"+httpTrigger.ID+"/invoke", `{"message":"hello"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("invoke: %d %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(t, e, http.MethodPost, "/function-triggers", `{"kind":"record_change","target":"tasks","events":["create"],"func_file":"handler","func_export":"Handle","filter":"status=\"open\"","enabled":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create record trigger: %d %s", rec.Code, rec.Body.String())
	}
	publisher.PublishRecord(RecordEvent{ProjectID: "p1", Collection: "tasks", RecordID: "r1", Action: "create", Data: map[string]any{"status": "open"}})
	publisher.PublishRecord(RecordEvent{ProjectID: "p1", Collection: "tasks", RecordID: "r2", Action: "create", Data: map[string]any{"status": "closed"}})

	deadline := time.Now().Add(30 * time.Second)
	var count int
	for time.Now().Before(deadline) {
		if err := db.QueryRow(`SELECT COUNT(*) FROM sys_go_func_invokes WHERE project_id = 'p1' AND channel = 'trigger'`).Scan(&count); err == nil && count == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	rows, queryErr := db.Query(`SELECT status_code, func_name, function_name FROM sys_go_func_invokes WHERE project_id = 'p1'`)
	if queryErr != nil {
		t.Fatalf("trigger invocations count=%d query error=%v", count, queryErr)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var status int
		var name, export string
		_ = rows.Scan(&status, &name, &export)
		details = append(details, fmt.Sprintf("%s.%s=%d", name, export, status))
	}
	t.Fatalf("trigger invocations count=%d details=%v", count, details)
}
