// 文件说明：event_fanout_test.go — internal/app 的单元测试（TestEventFanoutOrderingAndRouting、TestTriggerEventPublisherTranslation 等）
//
// 用法
//
//	go test ./internal/app -run 'TestEventFanoutOrderingAndRouting|TestTriggerEventPublisherTranslation'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/event_fanout_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_shutdown_test.go, app.go, app_alerts.go, app_lease.go, app_test.go 等 41 个
//
// 主要符号
//
//	type recordingSubscriber struct{…}
//	(r *recordingSubscriber) record(tag string)
//	(r *recordingSubscriber) snapshot() []string
//	type realtimeRecorder struct{…}
//	(r realtimeRecorder) PublishRecord(collection.RecordEvent)
//	type objectRecorder struct{…}
//	(o objectRecorder) PublishObject(objectstore.ObjectEvent)
//	TestEventFanoutOrderingAndRouting(t *testing.T)
//	... 另有 8 个顶层符号

package app

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/collection"
	"github.com/linkxzhou/SimpleBase/internal/functionrun"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/trigger"
)

// recordingSubscriber 记录收到的事件与顺序，用来验证 fan-out 的订阅顺序与事件种类路由。
type recordingSubscriber struct {
	mu     sync.Mutex
	events []string
}

func (r *recordingSubscriber) record(tag string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, tag)
}

func (r *recordingSubscriber) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

type realtimeRecorder struct{ rec *recordingSubscriber }

func (r realtimeRecorder) PublishRecord(collection.RecordEvent) { r.rec.record("realtime") }

type objectRecorder struct{ rec *recordingSubscriber }

func (o objectRecorder) PublishObject(objectstore.ObjectEvent) { o.rec.record("object") }

// fan-out 按构造顺序同步分发：记录事件先实时后触发器，对象事件只走对象订阅方；nil 订阅方被丢弃。
func TestEventFanoutOrderingAndRouting(t *testing.T) {
	rec := &recordingSubscriber{}
	f := newEventFanout(
		[]recordEventPublisher{realtimeRecorder{rec}, nil, recordFunc(func(collection.RecordEvent) { rec.record("trigger") })},
		[]objectEventPublisher{nil, objectRecorder{rec}},
	)
	f.PublishRecord(collection.RecordEvent{ProjectID: "p1", Collection: "tasks", RecordID: "r1", Action: "create"})
	f.PublishObject(objectstore.ObjectEvent{ProjectID: "p1", Key: "a.txt", Action: "upload"})
	got := rec.snapshot()
	want := []string{"realtime", "trigger", "object"}
	if len(got) != len(want) {
		t.Fatalf("order = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if newEventFanout(nil, nil) == nil || newTriggerEventPublisher(nil) != nil {
		t.Fatal("empty fan-out must still be usable; nil dispatcher must yield nil ACL")
	}
}

type recordFunc func(collection.RecordEvent)

func (f recordFunc) PublishRecord(e collection.RecordEvent) { f(e) }

// 触发器 ACL 的翻译：发布语言的字段逐一落到 trigger.Event（target / key / changed / data），经真实 Dispatcher 到达执行器。
func TestTriggerEventPublisherTranslation(t *testing.T) {
	store := fanoutStore{triggers: []systemdb.FunctionTrigger{
		{ID: "t-rec", ProjectID: "p1", Kind: "record_change", Target: "tasks", Events: []string{"update"}, Enabled: true},
		{ID: "t-obj", ProjectID: "p1", Kind: "object_event", Target: "*", Enabled: true},
	}}
	runner := &capturingRunner{got: make(chan functionrun.Request, 4)}
	dispatcher := trigger.NewDispatcher(store, runner, 8, 1)
	defer dispatcher.Close()

	trig := newTriggerEventPublisher(dispatcher)
	trig.PublishRecord(collection.RecordEvent{ProjectID: "p1", Collection: "tasks", RecordID: "r9", Action: "update", Changed: []string{"status"}, Data: map[string]any{"status": "open"}})
	trig.PublishObject(objectstore.ObjectEvent{ProjectID: "p1", Key: "docs/a.txt", Action: "upload"})

	first := waitRequest(t, runner.got)
	second := waitRequest(t, runner.got)
	if first.RequestID != "t-rec" || second.RequestID != "t-obj" || first.ProjectID != "p1" {
		t.Fatalf("trigger ids = %q, %q project=%q", first.RequestID, second.RequestID, first.ProjectID)
	}
	var rec trigger.Event
	if err := json.Unmarshal(first.Input, &rec); err != nil {
		t.Fatal(err)
	}
	// ProjectID 带 json:"-"：不进事件正文（函数经 Request.ProjectID 取项目）。
	if rec.Kind != "record_change" || rec.ProjectID != "" || rec.Target != "tasks" || rec.Action != "update" || rec.RecordID != "r9" || len(rec.Changed) != 1 || rec.Changed[0] != "status" || rec.Data["status"] != "open" {
		t.Fatalf("record event = %+v", rec)
	}
	var obj trigger.Event
	if err := json.Unmarshal(second.Input, &obj); err != nil {
		t.Fatal(err)
	}
	if obj.Kind != "object_event" || obj.Target != "docs/a.txt" || obj.Key != "docs/a.txt" || obj.Action != "upload" {
		t.Fatalf("object event = %+v", obj)
	}
}

type fanoutStore struct{ triggers []systemdb.FunctionTrigger }

func (s fanoutStore) ListFunctionTriggers(context.Context, string) ([]systemdb.FunctionTrigger, error) {
	return s.triggers, nil
}

type capturingRunner struct{ got chan functionrun.Request }

func (r *capturingRunner) Run(_ context.Context, req functionrun.Request) (functionrun.Result, error) {
	r.got <- req
	return functionrun.Result{}, nil
}

func waitRequest(t *testing.T, ch <-chan functionrun.Request) functionrun.Request {
	t.Helper()
	select {
	case req := <-ch:
		return req
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for trigger invocation")
		return functionrun.Request{}
	}
}
