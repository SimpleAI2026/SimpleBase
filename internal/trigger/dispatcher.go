// 文件说明：dispatcher.go — internal/trigger 中 ErrQueueFull、ErrClosed 等符号的实现
//
// 用法
//
//	NewDispatcher(store Store, runner FunctionRunner, capacity, workers int) *Dispatcher
//	(d *Dispatcher) PublishRecord(event Event)
//	(d *Dispatcher) PublishObject(event Event)
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/trigger/dispatcher.go
//	         │ 依赖
//	         ▼
//	[下游] internal/functionrun, internal/systemdb
//
// 主要符号
//
//	var ErrQueueFull
//	var ErrClosed
//	type Store interface {…}
//	type FunctionRunner interface {…}
//	type Event struct {…}
//	type Dispatcher struct {…}
//	... 另有 15 个顶层符号

package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/functionrun"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

var (
	ErrQueueFull = errors.New("trigger_queue_full")
	ErrClosed    = errors.New("trigger_dispatcher_closed")
)

type Store interface {
	ListFunctionTriggers(ctx context.Context, projectID string) ([]systemdb.FunctionTrigger, error)
}

type FunctionRunner interface {
	Run(ctx context.Context, request functionrun.Request) (functionrun.Result, error)
}

type Event struct {
	Kind      string         `json:"kind"`
	ProjectID string         `json:"-"`
	Target    string         `json:"target"`
	Action    string         `json:"action"`
	RecordID  string         `json:"record_id,omitempty"`
	Key       string         `json:"key,omitempty"`
	Changed   []string       `json:"changed,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	At        time.Time      `json:"at"`
}

type task struct {
	trigger systemdb.FunctionTrigger
	event   Event
}

type Dispatcher struct {
	store     Store
	runner    FunctionRunner
	queue     chan task
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	dropped   atomic.Uint64
	retryBase time.Duration
}

func NewDispatcher(store Store, runner FunctionRunner, capacity, workers int) *Dispatcher {
	if capacity <= 0 {
		capacity = 1024
	}
	if workers <= 0 {
		workers = 4
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &Dispatcher{store: store, runner: runner, queue: make(chan task, capacity), ctx: ctx, cancel: cancel, retryBase: 100 * time.Millisecond}
	for i := 0; i < workers; i++ {
		d.wg.Add(1)
		go d.worker()
	}
	return d
}

func (d *Dispatcher) PublishRecord(event Event) {
	d.publish(systemdb.FunctionTrigger{Kind: "record_change"}, event)
}

func (d *Dispatcher) PublishObject(event Event) {
	d.publish(systemdb.FunctionTrigger{Kind: "object_event"}, event)
}

func (d *Dispatcher) publish(_ systemdb.FunctionTrigger, event Event) {
	if d == nil || d.store == nil || event.ProjectID == "" {
		return
	}
	triggers, err := d.store.ListFunctionTriggers(d.ctx, event.ProjectID)
	if err != nil {
		return
	}
	for _, trigger := range triggers {
		targetMatches := trigger.Target == "*" || trigger.Target == event.Target
		if event.Kind == "object_event" && trigger.Target != "*" {
			targetMatches = strings.HasPrefix(event.Target, trigger.Target)
		}
		if trigger.Kind != event.Kind || !trigger.Enabled || !targetMatches || !eventEnabled(trigger.Events, event.Action) || !matchesFilter(trigger.Filter, event.Data) {
			continue
		}
		if err := d.enqueue(trigger, event); err != nil {
			d.dropped.Add(1)
		}
	}
}

func (d *Dispatcher) Invoke(trigger systemdb.FunctionTrigger, data map[string]any) error {
	return d.enqueue(trigger, Event{Kind: "http", ProjectID: trigger.ProjectID, Target: trigger.ID, Action: "http", Data: data, At: time.Now().UTC()})
}

func (d *Dispatcher) enqueue(trigger systemdb.FunctionTrigger, event Event) error {
	if d == nil {
		return ErrClosed
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	select {
	case <-d.ctx.Done():
		return ErrClosed
	default:
	}
	select {
	case d.queue <- task{trigger: trigger, event: event}:
		return nil
	default:
		return ErrQueueFull
	}
}

func (d *Dispatcher) worker() {
	defer d.wg.Done()
	for {
		select {
		case <-d.ctx.Done():
			return
		case item := <-d.queue:
			d.execute(item)
		}
	}
}

func (d *Dispatcher) execute(item task) {
	payload, _ := json.Marshal(item.event)
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(d.retryBase * time.Duration(1<<(attempt-1)))
			select {
			case <-d.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		_, err := d.runner.Run(d.ctx, functionrun.Request{ProjectID: item.trigger.ProjectID, FuncFile: item.trigger.FuncFile, FuncExport: item.trigger.FuncExport, Input: payload, Channel: "trigger", RequestID: item.trigger.ID, Actor: "trigger"})
		if err == nil {
			return
		}
	}
}

func (d *Dispatcher) Dropped() uint64 {
	if d == nil {
		return 0
	}
	return d.dropped.Load()
}

func (d *Dispatcher) Close() {
	if d == nil {
		return
	}
	d.cancel()
	d.wg.Wait()
}

func eventEnabled(events []string, action string) bool {
	if len(events) == 0 {
		return true
	}
	for _, event := range events {
		if event == "*" || event == action {
			return true
		}
	}
	return false
}

func ValidateFilter(filter string) bool {
	if filter == "" {
		return true
	}
	name, raw, ok := strings.Cut(filter, "=")
	if !ok || strings.TrimSpace(name) == "" || strings.ContainsAny(name, " .\t\r\n") {
		return false
	}
	var value any
	return json.Unmarshal([]byte(strings.TrimSpace(raw)), &value) == nil
}

func matchesFilter(filter string, data map[string]any) bool {
	if filter == "" {
		return true
	}
	name, raw, _ := strings.Cut(filter, "=")
	var expected any
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &expected) != nil {
		return false
	}
	actual, ok := data[strings.TrimSpace(name)]
	if !ok {
		return false
	}
	a, _ := json.Marshal(actual)
	b, _ := json.Marshal(expected)
	return string(a) == string(b) || actual != nil && expected != nil && numericString(actual) == numericString(expected)
}

func numericString(value any) string {
	switch typed := value.(type) {
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case string:
		return typed
	default:
		return ""
	}
}
