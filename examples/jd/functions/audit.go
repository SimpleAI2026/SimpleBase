//go:build ignore

package main

import (
	"fmt"
	"net/http"
	"encoding/json"
	"bytes"
	"time"
)

// 事件触发器统一的入参：record_change / object_event / http 三种 kind 共用。
type Event struct {
	Kind     string         `json:"kind"`
	Target   string         `json:"target"`
	Action   string         `json:"action"`
	RecordID string         `json:"record_id"`
	Key      string         `json:"key"`
	Changed  []string       `json:"changed"`
	Data     map[string]any `json:"data"`
}

type EventResult struct {
	Accepted bool   `json:"accepted"`
	Forwarded bool  `json:"forwarded"`
	Error    string `json:"error,omitempty"`
}

// BFF 的审计回调地址。示例固定本机端口，端口被占用时用 EXAMPLE_PORT 覆盖服务端口后需同步改这里。
const auditHook = "http://127.0.0.1:8093/api/hooks/audit"

type hookBody struct {
	Source   string `json:"source"`
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Action   string `json:"action"`
	RecordID string `json:"record_id"`
	Key      string `json:"key"`
	Detail   string `json:"detail"`
}

// forward 把事件转发给 BFF，由 BFF 落到 audit_log 表；返回是否成功。
func forward(source string, event Event) EventResult {
	result := EventResult{Accepted: true}
	detail := ""
	if event.RecordID != "" {
		detail = "record " + event.RecordID
	} else if event.Key != "" {
		detail = "object " + event.Key
	} else if len(event.Data) > 0 {
		detail = "payload fields: " + fmt.Sprint(len(event.Data))
	}
	body, err := json.Marshal(hookBody{
		Source:   source,
		Kind:     event.Kind,
		Target:   event.Target,
		Action:   event.Action,
		RecordID: event.RecordID,
		Key:      event.Key,
		Detail:   detail,
	})
	if err != nil {
		result.Error = err.Error()
		return result
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(auditHook, "application/json", bytes.NewReader(body))
	if err != nil {
		result.Error = err.Error()
		return result
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("hook status %d", resp.StatusCode)
		return result
	}
	result.Forwarded = true
	return result
}

// OnRecord 处理集合文档变更（record_change）。
func OnRecord(event Event) EventResult {
	return forward("record_change", event)
}

// OnObject 处理对象上传/删除（object_event）。
func OnObject(event Event) EventResult {
	return forward("object_event", event)
}

// OnHttp 处理手动 invoke 的 HTTP 触发器。
func OnHttp(event Event) EventResult {
	return forward("http", event)
}