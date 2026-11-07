//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// 事件触发器统一入参（record_change / object_event / http 共用）。
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
	Accepted  bool   `json:"accepted"`
	Forwarded bool   `json:"forwarded"`
	Error     string `json:"error,omitempty"`
}

// 物联网 BFF 的审计回调（默认 8098）。
const auditHook = "http://127.0.0.1:8098/api/hooks/audit"

type hookBody struct {
	Source   string `json:"source"`
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Action   string `json:"action"`
	RecordID string `json:"record_id"`
	Key      string `json:"key"`
	Detail   string `json:"detail"`
}

func forward(source string, event Event) EventResult {
	result := EventResult{Accepted: true}
	detail := ""
	if event.RecordID != "" {
		detail = "record " + event.RecordID
	} else if event.Key != "" {
		detail = "object " + event.Key
	} else if len(event.Data) > 0 {
		detail = fmt.Sprintf("payload fields: %d", len(event.Data))
	}
	body, err := json.Marshal(hookBody{Source: source, Kind: event.Kind, Target: event.Target, Action: event.Action, RecordID: event.RecordID, Key: event.Key, Detail: detail})
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

// OnRecord 处理遥测集合写入（设备读数）。
func OnRecord(event Event) EventResult { return forward("record_change", event) }

// OnObject 处理导出文件对象事件。
func OnObject(event Event) EventResult { return forward("object_event", event) }

// OnHttp 处理手动 invoke 的 HTTP 触发器。
func OnHttp(event Event) EventResult { return forward("http", event) }