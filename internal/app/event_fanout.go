// 文件说明：event_fanout.go — 领域事件的进程内同步 fan-out 与触发器 ACL（app 私有）
//
// 用法
//
//	trig := newTriggerEventPublisher(a.triggerDispatcher)                  // 自动化上下文的入站防腐层；dispatcher 为 nil 返回 nil
//	events := newEventFanout([]recordEventPublisher{realtime, trig}, []objectEventPublisher{trig})
//	(events *eventFanout) PublishRecord(event collection.RecordEvent)     // 实现 DataHandler 的发布端口
//	(events *eventFanout) PublishObject(event objectstore.ObjectEvent)    // 实现 S3Handler 的发布端口（api.Dependencies.TriggerEvents）
//
// 调用架构
//
//	[上游] internal/app（NewWithRegistry 装配：先建触发器，再建数据 / 文件写 handler）
//	         │ 同包调用
//	         ▼
//	[本文件] internal/app/event_fanout.go
//	         │ 依赖
//	         ▼
//	[下游] internal/collection, internal/objectstore（发布语言）, internal/trigger（消费方）
//	[同包] app.go, assemble_api.go, assemble_handlers.go
//
// 主要符号
//
//	type recordEventPublisher interface {…}
//	type objectEventPublisher interface {…}
//	type eventFanout struct {…}
//	newEventFanout(record []recordEventPublisher, object []objectEventPublisher) *eventFanout
//	type triggerEventPublisher struct {…}
//	newTriggerEventPublisher(d *trigger.Dispatcher) *triggerEventPublisher
//	... 另有 4 个顶层符号
//
// 归属（internal-ddd-hexagonal-implementation-plan §4、决策 D5）：事件类型归发布方领域包
// （collection.RecordEvent / objectstore.ObjectEvent）；发布端口留在发布调用点所在包，这里的接口与之同形；
// 多订阅方的 fan-out 与「发布语言 → trigger.Event」的翻译只是组合根里的接线，不构成新包，也不引入事件总线。
// 分发是进程内同步的：按构造顺序逐个调用，不持久化、不承诺 exactly-once。

package app

import (
	"github.com/linkxzhou/SimpleBase/internal/collection"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/trigger"
)

// recordEventPublisher 与 data_handler 的发布端口同形（结构化匹配，不 import api）。
type recordEventPublisher interface {
	PublishRecord(event collection.RecordEvent)
}

// objectEventPublisher 与 s3_handler 的发布端口同形。
type objectEventPublisher interface {
	PublishObject(event objectstore.ObjectEvent)
}

// eventFanout 按构造顺序同步发布领域事件。
type eventFanout struct {
	record []recordEventPublisher
	object []objectEventPublisher
}

// newEventFanout 构造 fan-out；nil 订阅方被丢弃。
func newEventFanout(record []recordEventPublisher, object []objectEventPublisher) *eventFanout {
	f := &eventFanout{}
	for _, p := range record {
		if p != nil {
			f.record = append(f.record, p)
		}
	}
	for _, p := range object {
		if p != nil {
			f.object = append(f.object, p)
		}
	}
	return f
}

func (f *eventFanout) PublishRecord(event collection.RecordEvent) {
	for _, p := range f.record {
		p.PublishRecord(event)
	}
}

func (f *eventFanout) PublishObject(event objectstore.ObjectEvent) {
	for _, p := range f.object {
		p.PublishObject(event)
	}
}

// triggerEventPublisher 是自动化上下文的入站防腐层：把发布语言翻译成触发器的内部事件形状。
// 翻译规则与迁移前一致：记录事件 target = 集合名，对象事件 target = key 且附带 Key 字段。
type triggerEventPublisher struct{ dispatcher *trigger.Dispatcher }

// newTriggerEventPublisher 构造 ACL；dispatcher 为 nil（无系统库的实例）时返回 nil。
func newTriggerEventPublisher(d *trigger.Dispatcher) *triggerEventPublisher {
	if d == nil {
		return nil
	}
	return &triggerEventPublisher{dispatcher: d}
}

func (p *triggerEventPublisher) PublishRecord(event collection.RecordEvent) {
	p.dispatcher.PublishRecord(trigger.Event{Kind: "record_change", ProjectID: event.ProjectID, Target: event.Collection, Action: event.Action, RecordID: event.RecordID, Changed: event.Changed, Data: event.Data})
}

func (p *triggerEventPublisher) PublishObject(event objectstore.ObjectEvent) {
	p.dispatcher.PublishObject(trigger.Event{Kind: "object_event", ProjectID: event.ProjectID, Target: event.Key, Action: event.Action, Key: event.Key})
}
