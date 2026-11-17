package api

// e2b_envd_watch_test.go：envd 目录监听的四个端点。
//
//   1. CreateWatcher 返回 watcherID；GetWatcherEvents 按 id 取到推入的事件；
//   2. WatchDir 立刻返回第一批事件（等一小会儿）；
//   3. RemoveWatcher 幂等；未知 id 的 GetWatcherEvents 404；
//   4. 后端没有 watch 能力时 501；
//   5. path 缺失时 400。

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// 建 watcher → 推事件 → 取事件。
func TestWatchCreateAndGet(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)
	path := "/workspace"

	rec := call(e, http.MethodPost, "/filesystem.Filesystem/CreateWatcher", key,
		`{"path":"`+path+`"}`, map[string]string{"E2b-Sandbox-Id": id})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	wid := watcherIDOf(t, rec.Body.Bytes())
	if wid == "" {
		t.Fatalf("no watcherID: %s", rec.Body.String())
	}

	// 推一条事件进去。
	if !fakeWatches.push(path, vm.FileEvent{Path: path + "/a.txt", Kind: "file"}) {
		t.Fatal("no fake watch registered for the path")
	}
	// 事件泵是异步的，等它搬到队列。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec = call(e, http.MethodPost, "/filesystem.Filesystem/GetWatcherEvents", key,
			`{"watcherID":"`+wid+`"}`, map[string]string{"E2b-Sandbox-Id": id})
		if rec.Code != http.StatusOK {
			t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if strings.Contains(body, "a.txt") {
			return
		}
		// 还没搬过来：空数组表示「暂时没有事件」，继续等。
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("event never appeared")
}

// WatchDir 立刻返回第一批事件。
func TestWatchDirReturnsEvents(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)
	path := "/workspace/wd"

	// 先建一个订阅并推事件，再调 WatchDir：它的语义是「建订阅 + 回一批」。
	_ = call(e, http.MethodPost, "/filesystem.Filesystem/CreateWatcher", key,
		`{"path":"`+path+`"}`, map[string]string{"E2b-Sandbox-Id": id})
	fakeWatches.push(path, vm.FileEvent{Path: path + "/pre.txt", Kind: "file"})
	time.Sleep(50 * time.Millisecond)

	rec := call(e, http.MethodPost, "/filesystem.Filesystem/WatchDir", key,
		`{"path":"`+path+`"}`, map[string]string{"E2b-Sandbox-Id": id})
	if rec.Code != http.StatusOK {
		t.Fatalf("watchdir: %d %s", rec.Code, rec.Body.String())
	}
	// 新订阅没有事件也能成功（返回空数组）。
	if !strings.Contains(rec.Body.String(), "events") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

// 未知 watcherID：404。
func TestWatchGetUnknown(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)
	rec := call(e, http.MethodPost, "/filesystem.Filesystem/GetWatcherEvents", key,
		`{"watcherID":"nope"}`, map[string]string{"E2b-Sandbox-Id": id})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

// RemoveWatcher 幂等。
func TestWatchRemoveIdempotent(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)
	for i := 0; i < 2; i++ {
		rec := call(e, http.MethodPost, "/filesystem.Filesystem/RemoveWatcher", key,
			`{"watcherID":"whatever"}`, map[string]string{"E2b-Sandbox-Id": id})
		if rec.Code != http.StatusOK {
			t.Fatalf("remove #%d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
}

// 缺少 path：400。
func TestWatchNeedsPath(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)
	rec := call(e, http.MethodPost, "/filesystem.Filesystem/CreateWatcher", key,
		`{}`, map[string]string{"E2b-Sandbox-Id": id})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

// 后端没有 watch 能力：501。
func TestWatchUnsupported(t *testing.T) {
	sb := newFakeRouterSandbox(true)
	sb.noWatch = true
	e, key, _ := routerWith(t, sb)
	id := rowFor(t, sb)
	rec := call(e, http.MethodPost, "/filesystem.Filesystem/CreateWatcher", key,
		`{"path":"/workspace"}`, map[string]string{"E2b-Sandbox-Id": id})
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

// watcherIDOf 从 CreateWatcher 的响应里取 watcherID。
func watcherIDOf(t *testing.T, body []byte) string {
	t.Helper()
	var resp struct {
		WatcherID string `json:"watcherID"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	return resp.WatcherID
}
