// 文件说明：event_fanout.go — internal/api 中 RecordEventFanout、Add 等符号的实现
//
// 用法
//
//	NewTriggerEventPublisher(dispatcher *trigger.Dispatcher) *TriggerEventPublisher
//	(f *RecordEventFanout) Add(publisher recordEventPublisher)
//	(f *RecordEventFanout) PublishRecord(event RecordEvent)
//	同包调用：Add() → agent_handler.go, auth_handler.go 等；PublishRecord() → data_handler.go, data_handler.go 等；objectEventPublisher → router.go, s3_handler.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/event_fanout.go
//	         │ 依赖
//	         ▼
//	[下游] internal/trigger
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] Add() ← agent_handler.go, auth_handler.go, cronjob_handler.go
//
// 主要符号
//
//	type RecordEventFanout struct {…}
//	(f *RecordEventFanout) Add(publisher recordEventPublisher)
//	(f *RecordEventFanout) PublishRecord(event RecordEvent)
//	type ObjectEvent struct {…}
//	type TriggerEventPublisher struct {…}
//	NewTriggerEventPublisher(dispatcher *trigger.Dispatcher) *TriggerEventPublisher
//	... 另有 3 个顶层符号

package api

import (
	"github.com/linkxzhou/SimpleBase/internal/trigger"
)

type RecordEventFanout struct {
	publishers []recordEventPublisher
}

func (f *RecordEventFanout) Add(publisher recordEventPublisher) {
	if publisher != nil {
		f.publishers = append(f.publishers, publisher)
	}
}

func (f *RecordEventFanout) PublishRecord(event RecordEvent) {
	for _, publisher := range f.publishers {
		publisher.PublishRecord(event)
	}
}

type ObjectEvent struct {
	ProjectID string
	Key       string
	Action    string
}

type objectEventPublisher interface {
	PublishObject(event ObjectEvent)
}

type TriggerEventPublisher struct {
	dispatcher *trigger.Dispatcher
}

func NewTriggerEventPublisher(dispatcher *trigger.Dispatcher) *TriggerEventPublisher {
	return &TriggerEventPublisher{dispatcher: dispatcher}
}

func (p *TriggerEventPublisher) PublishRecord(event RecordEvent) {
	p.dispatcher.PublishRecord(trigger.Event{Kind: "record_change", ProjectID: event.ProjectID, Target: event.Collection, Action: event.Action, RecordID: event.RecordID, Changed: event.Changed, Data: event.Data})
}

func (p *TriggerEventPublisher) PublishObject(event ObjectEvent) {
	p.dispatcher.PublishObject(trigger.Event{Kind: "object_event", ProjectID: event.ProjectID, Target: event.Key, Action: event.Action, Key: event.Key})
}
