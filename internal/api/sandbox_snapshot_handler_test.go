// 文件说明：sandbox_snapshot_handler_test.go — internal/api 的单元测试（TestSnapshotCreate、TestSnapshotList 等）
//
// 用法
//
//	go test ./internal/api -run 'TestSnapshotCreate|TestSnapshotList|TestSnapshotRemove|TestFork'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sandbox_snapshot_handler_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestSnapshotCreate(t *testing.T)
//	TestSnapshotList(t *testing.T)
//	TestSnapshotRemove(t *testing.T)
//	TestFork(t *testing.T)
//	TestSnapshotFork(t *testing.T)
//	TestSnapshotForkUnknown(t *testing.T)
//	TestSnapshotUnsupported(t *testing.T)

package api

// sandbox_snapshot_handler_test.go：快照与 fork 端点的形状。
//
//   1. 拍快照 201，响应里 processes=false 且 names 含沙盒名；
//   2. 列表只返回同一个沙盒名的快照；
//   3. 删快照 204；未知 id 404；
//   4. fork 一次完成「拍快照 + 建新沙盒」，新沙盒名字带后缀且不启动；
//   5. 指定快照 fork 走 /snapshots/{id}/fork；
//   6. 后端没有能力时 501；
//   7. 只读 Key 只能列，不能拍。

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

// 拍快照：201 + processes=false。
func TestSnapshotCreate(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)

	rec := call(e, http.MethodPost, "/e2b/sandboxes/"+id+"/snapshots", key, "", map[string]string{"E2b-Sandbox-Id": id})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got e2bSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if got.SnapshotID == "" {
		t.Fatalf("no snapshotID: %s", rec.Body.String())
	}
	if got.Processes {
		t.Fatal("processes must be false: snapshots only carry the filesystem")
	}
	if len(got.Names) != 1 || got.Names[0] != "logs" {
		t.Fatalf("names = %v", got.Names)
	}
	if got.SizeBytes <= 0 || got.Digest == "" {
		t.Fatalf("size/digest missing: %+v", got)
	}
}

// 列表：只含同一个沙盒名的快照。
func TestSnapshotList(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)

	// 拍两个。
	for i := 0; i < 2; i++ {
		if rec := call(e, http.MethodPost, "/e2b/sandboxes/"+id+"/snapshots", key, "",
			map[string]string{"E2b-Sandbox-Id": id}); rec.Code != http.StatusCreated {
			t.Fatalf("create #%d: %d", i, rec.Code)
		}
	}
	// 另一个沙盒的快照不该出现在这里。
	other, err := sb.Create(t.Context(), "proj-1", sandbox.CreateInput{Name: "other"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if rec := call(e, http.MethodPost, "/e2b/sandboxes/"+other.ID+"/snapshots", key, "",
		map[string]string{"E2b-Sandbox-Id": other.ID}); rec.Code != http.StatusCreated {
		t.Fatalf("other create: %d", rec.Code)
	}

	rec := call(e, http.MethodGet, "/e2b/sandboxes/"+id+"/snapshots", key, "", map[string]string{"E2b-Sandbox-Id": id})
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var list []e2bSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if len(list) != 2 {
		t.Fatalf("len = %d: %s", len(list), rec.Body.String())
	}
	for _, s := range list {
		if len(s.Names) != 1 || s.Names[0] != "logs" {
			t.Fatalf("leaked another sandbox's snapshot: %v", s.Names)
		}
	}
}

// 删快照：204；再删 404。
func TestSnapshotRemove(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)
	rec := call(e, http.MethodPost, "/e2b/sandboxes/"+id+"/snapshots", key, "", map[string]string{"E2b-Sandbox-Id": id})
	snap := watcherIDOf(t, rec.Body.Bytes()) // 同一形状的 {"snapshotID":...} 提取
	if snap == "" {
		// watcherIDOf 找的是 watcherID；这里直接解 e2bSnapshot。
		var s e2bSnapshot
		if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
			t.Fatalf("decode: %v", err)
		}
		snap = s.SnapshotID
	}

	if r := call(e, http.MethodDelete, "/e2b/snapshots/"+snap, key, "", nil); r.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", r.Code, r.Body.String())
	}
	if r := call(e, http.MethodDelete, "/e2b/snapshots/"+snap, key, "", nil); r.Code != http.StatusNotFound {
		t.Fatalf("second delete: %d", r.Code)
	}
}

// fork：一次完成拍快照 + 建新沙盒。
func TestFork(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)

	rec := call(e, http.MethodPost, "/e2b/sandboxes/"+id+"/fork", key, "", map[string]string{"E2b-Sandbox-Id": id})
	if rec.Code != http.StatusCreated {
		t.Fatalf("fork: %d %s", rec.Code, rec.Body.String())
	}
	var got e2bDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	// 名字在 metadata 的保留键 name 里。
	if name := got.Metadata["name"]; name != "logs-fork" {
		t.Fatalf("name = %q (metadata %v)", name, got.Metadata)
	}
	// fork 不启动沙盒：检查传给 Manager 的 Start 为 false。
	// （对外 state 只有 running/paused，pending 会被映射成 running，看不出来。）
	last := sb.createdIn[len(sb.createdIn)-1]
	if last.Start {
		t.Fatal("fork must not start the sandbox")
	}
	if got.SandboxID == id {
		t.Fatal("fork must create a new sandbox")
	}
}

// 指定快照 fork。
func TestSnapshotFork(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)
	rec := call(e, http.MethodPost, "/e2b/sandboxes/"+id+"/snapshots", key, "", map[string]string{"E2b-Sandbox-Id": id})
	var snap e2bSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}

	rec = call(e, http.MethodPost, "/e2b/snapshots/"+snap.SnapshotID+"/fork", key, "", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("snapshot fork: %d %s", rec.Code, rec.Body.String())
	}
	var got e2bDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SandboxID == "" || got.SandboxID == id {
		t.Fatalf("sandbox id = %q", got.SandboxID)
	}
}

// 未知快照 fork：404。
func TestSnapshotForkUnknown(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	rec := call(e, http.MethodPost, "/e2b/snapshots/nope/fork", key, "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

// 后端没有能力：三个端点都 501。
func TestSnapshotUnsupported(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	sb.noSnapshot = true
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/e2b/sandboxes/" + id + "/snapshots"},
		{http.MethodGet, "/e2b/sandboxes/" + id + "/snapshots"},
		{http.MethodPost, "/e2b/sandboxes/" + id + "/fork"},
	} {
		rec := call(e, tc.method, tc.path, key, "", map[string]string{"E2b-Sandbox-Id": id})
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s %s: status = %d", tc.method, tc.path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "unsupported_feature") {
			t.Errorf("%s %s: body = %s", tc.method, tc.path, rec.Body.String())
		}
	}
}
