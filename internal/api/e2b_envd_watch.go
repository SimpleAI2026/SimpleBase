// 文件说明：e2b_envd_watch.go — envd 的目录监听 handler
//
// 四个端点都走 envd 的 Connect 形状（POST + JSON）：
//
//	WatchDir          建订阅并立刻返回第一批事件
//	CreateWatcher     建订阅，返回 watcherID
//	GetWatcherEvents  按 id 拉一批事件（不阻塞：没有事件时返回空数组）
//	RemoveWatcher     取消订阅
//
// watcher 存在进程内（一个沙盒一个进程内表），不落库：监听是会话级资源，
// 服务重启后订阅自然失效，SDK 会重建。
//
// 调用架构
//
//	[上游] internal/api/router.go（路由注册）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/e2b_envd_watch.go
//	         │
//	         ▼
//	[下游] internal/sandbox（Watch）、gosandbox/vm
//	[同包] e2b_envd_handler.go, sandbox_handler.go
//
// 主要符号
//
//	(h *SandboxHandler) FsWatchDir(c echo.Context) error
//	(h *SandboxHandler) FsCreateWatcher(c echo.Context) error
//	(h *SandboxHandler) FsGetWatcherEvents(c echo.Context) error
//	(h *SandboxHandler) FsRemoveWatcher(c echo.Context) error

package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/e2bproto"
)

// watcherEntry 是一个进行中的订阅。
type watcherEntry struct {
	id     string
	watch  vm.FileWatch
	cancel func()
	// events 是 vm 事件转成 envd 事件后的队列；满了丢最老的。
	mu     sync.Mutex
	queue  []e2bproto.WatchEvent
	closed atomic.Bool
}

// watcherTable 是进程内的订阅表，按「项目/沙盒/watcherID」索引。
//
// 服务重启后表为空：这是有意的——监听是会话级资源，重启即失效。
type watcherTable struct {
	mu   sync.Mutex
	next atomic.Uint64
	by   map[string]*watcherEntry
}

var envdWatchers = &watcherTable{by: map[string]*watcherEntry{}}

// watcherKey 拼出表键。
func watcherKey(projectID, sandboxID, watcherID string) string {
	return projectID + "/" + sandboxID + "/" + watcherID
}

// add 登记一个订阅。
func (t *watcherTable) add(projectID, sandboxID string, w vm.FileWatch, cancel func()) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	id := "w" + itoa64(t.next.Add(1))
	e := &watcherEntry{id: id, watch: w, cancel: cancel}
	t.by[watcherKey(projectID, sandboxID, id)] = e
	// 事件泵：把 vm 事件搬进队列，GetWatcherEvents 只读队列。
	go func() {
		for ev := range w.Events() {
			e.push(ev)
		}
	}()
	return id
}

// get 取一个订阅。
func (t *watcherTable) get(projectID, sandboxID, watcherID string) *watcherEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.by[watcherKey(projectID, sandboxID, watcherID)]
}

// remove 取消并删除订阅；不存在时返回 false。
func (t *watcherTable) remove(projectID, sandboxID, watcherID string) bool {
	t.mu.Lock()
	e := t.by[watcherKey(projectID, sandboxID, watcherID)]
	delete(t.by, watcherKey(projectID, sandboxID, watcherID))
	t.mu.Unlock()
	if e == nil {
		return false
	}
	e.close()
	return true
}

// push 把一条事件放进队列（上限 1024，满了丢最老的）。
func (e *watcherEntry) push(ev vm.FileEvent) {
	if e.closed.Load() {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.queue = append(e.queue, e2bproto.WatchEvent{Path: ev.Path, Type: eventType(ev.Kind), IsDir: ev.Kind == "directory"})
	if len(e.queue) > 1024 {
		e.queue = e.queue[len(e.queue)-1024:]
	}
}

// drain 取走队列里的至多 limit 条。
func (e *watcherEntry) drain(limit int) []e2bproto.WatchEvent {
	if limit <= 0 || limit > 1024 {
		limit = 256
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	n := len(e.queue)
	if n > limit {
		n = limit
	}
	out := make([]e2bproto.WatchEvent, n)
	copy(out, e.queue[:n])
	e.queue = e.queue[n:]
	return out
}

// close 关闭订阅。
func (e *watcherEntry) close() {
	if e.closed.Swap(true) {
		return
	}
	if e.cancel != nil {
		e.cancel()
	}
	_ = e.watch.Close()
}

// eventType 把 vm 的 kind 映射成 envd 的事件类型。
//
// vm 只给「变化后的类型」，没有区分 create/write：这里按目录/文件粗分，
// 调用方靠 path 自行判断（envd 的真实实现也是这个粒度）。
func eventType(kind string) string {
	if kind == "directory" {
		return "create"
	}
	return "write"
}

// itoa64 是 strconv.FormatUint 的短名。
func itoa64(v uint64) string {
	var buf [20]byte
	i := len(buf)
	if v == 0 {
		return "0"
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// FsWatchDir 对应 envd POST /filesystem.Filesystem/WatchDir。
//
// 建订阅并等一小会儿收第一批事件，然后返回 watcherID 与这批事件。
// 不等的话调用方拿到空数组会以为监听没生效。
func (h *SandboxHandler) FsWatchDir(c echo.Context) error {
	var req e2bproto.WatchRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, false)
	if err != nil {
		return writeConnectErr(c, err)
	}
	if !h.svc.CanWatch() {
		return writeConnectErr(c, &connectErr{e2bproto.CodeUnimplemented, "directory watching is not supported"})
	}
	entry, werr := h.startWatch(c, p, id, req)
	if werr != nil {
		return writeConnectErr(c, werr)
	}
	// 等第一批事件：最多 300ms，够 inotify 把已发生的变化报上来。
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if evs := entry.drain(256); len(evs) > 0 {
			return c.JSON(http.StatusOK, e2bproto.WatchEventsResponse{Events: evs})
		}
		time.Sleep(20 * time.Millisecond)
	}
	return c.JSON(http.StatusOK, e2bproto.WatchEventsResponse{})
}

// FsCreateWatcher 对应 envd POST /filesystem.Filesystem/CreateWatcher。
func (h *SandboxHandler) FsCreateWatcher(c echo.Context) error {
	var req e2bproto.WatchRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, false)
	if err != nil {
		return writeConnectErr(c, err)
	}
	if !h.svc.CanWatch() {
		return writeConnectErr(c, &connectErr{e2bproto.CodeUnimplemented, "directory watching is not supported"})
	}
	entry, werr := h.startWatch(c, p, id, req)
	if werr != nil {
		return writeConnectErr(c, werr)
	}
	h.record(c, p, id, "watch.create", req.Path)
	return c.JSON(http.StatusOK, e2bproto.WatchCreateResponse{WatcherID: entry.id})
}

// FsGetWatcherEvents 对应 envd POST /filesystem.Filesystem/GetWatcherEvents。
//
// 不阻塞：没有事件时返回空数组。阻塞式语义（等有新事件才返回）会让
// SDK 的轮询线程卡住，而它本来就在轮询。
func (h *SandboxHandler) FsGetWatcherEvents(c echo.Context) error {
	var req e2bproto.WatchEventsRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, false)
	if err != nil {
		return writeConnectErr(c, err)
	}
	entry := envdWatchers.get(p, id, req.WatcherID)
	if entry == nil {
		return writeConnectErr(c, &connectErr{e2bproto.CodeNotFound, "watcher not found"})
	}
	return c.JSON(http.StatusOK, e2bproto.WatchEventsResponse{Events: entry.drain(int(req.Limit))})
}

// FsRemoveWatcher 对应 envd POST /filesystem.Filesystem/RemoveWatcher。
func (h *SandboxHandler) FsRemoveWatcher(c echo.Context) error {
	var req e2bproto.WatchRemoveRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, false)
	if err != nil {
		return writeConnectErr(c, err)
	}
	// 幂等：不存在也返回成功，SDK 的清理路径不必处理 404。
	envdWatchers.remove(p, id, req.WatcherID)
	h.record(c, p, id, "watch.remove", req.WatcherID)
	return c.JSON(http.StatusOK, e2bproto.WatchRemoveResponse{})
}

// startWatch 建一个订阅；失败返回 connectErr。
func (h *SandboxHandler) startWatch(c echo.Context, p, id string, req e2bproto.WatchRequest) (*watcherEntry, *connectErr) {
	if req.Path == "" {
		return nil, &connectErr{e2bproto.CodeInvalidArgument, "path is required"}
	}
	ctx, cancel := context.WithCancel(c.Request().Context())
	w, err := h.svc.Watch(ctx, p, id, req.Path, req.Recursive)
	if err != nil {
		// 失败路径必须释放 ctx，否则父 ctx 上会留一个永不取消的子节点。
		cancel()
		switch {
		case errors.Is(err, vm.ErrNotSupported):
			return nil, &connectErr{e2bproto.CodeUnimplemented, "directory watching is not supported"}
		case errors.Is(err, vm.ErrUnavailable), errors.Is(err, vm.ErrBackend):
			return nil, &connectErr{e2bproto.CodeInternal, "cannot start the watcher"}
		}
		return nil, &connectErr{e2bproto.CodeNotFound, "sandbox not found"}
	}
	wid := envdWatchers.add(p, id, w, cancel)
	entry := envdWatchers.get(p, id, wid)
	if entry == nil {
		// 极窄的竞态：登记后立刻被删。按失败处理，不让调用方拿到悬空 id。
		cancel()
		_ = w.Close()
		return nil, &connectErr{e2bproto.CodeInternal, "watcher registration failed"}
	}
	return entry, nil
}
