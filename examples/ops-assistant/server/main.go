package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	ex "github.com/linkxzhou/SimpleBase/examples/lib/goexample"
	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func main() {
	r, err := ex.New()
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	registerTicketRoutes(mux, r)
	registerPlatformRoutes(mux, r)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) { servePage(w, "examples/ops-assistant/web/index.html") })
	port := os.Getenv("EXAMPLE_PORT")
	if port == "" {
		port = "8097"
	}
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, mux))
}

func registerTicketRoutes(mux *http.ServeMux, r *ex.Runtime) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { reply(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /api/tickets", func(w http.ResponseWriter, req *http.Request) {
		result, err := r.Client.Query(req.Context(), "SELECT id, title, priority, status, due_at FROM tickets ORDER BY due_at LIMIT 100", nil, 100)
		if err != nil {
			reply(w, 500, map[string]string{"error": "tickets unavailable"})
			return
		}
		rows, err := ex.Rows(result)
		if err != nil {
			reply(w, 500, map[string]string{"error": "invalid rows"})
			return
		}
		reply(w, 200, rows)
	})
	mux.HandleFunc("POST /api/tickets", func(w http.ResponseWriter, req *http.Request) {
		var input struct {
			Title    string `json:"title"`
			Priority string `json:"priority"`
		}
		req.Body = http.MaxBytesReader(w, req.Body, 8192)
		if json.NewDecoder(req.Body).Decode(&input) != nil || strings.TrimSpace(input.Title) == "" {
			reply(w, 400, map[string]string{"error": "invalid ticket"})
			return
		}
		id := uuid.NewString()
		now := time.Now().UTC()
		batch, err := r.Client.Batch(req.Context(), []gosdk.SQLStatement{
			{SQL: "INSERT INTO tickets (id, title, priority, status, due_at, version) VALUES (?, ?, ?, ?, ?, ?)", Args: []any{id, input.Title, input.Priority, "open", now.Add(24 * time.Hour).Format(time.RFC3339), 1}},
			{SQL: "INSERT INTO ticket_events (id, ticket_id, kind, created_at) VALUES (?, ?, ?, ?)", Args: []any{uuid.NewString(), id, "created", now.Format(time.RFC3339)}},
		}, true)
		if ex.CheckBatch(batch, err) != nil {
			reply(w, 500, map[string]string{"error": "ticket transaction failed"})
			return
		}
		// 工单事件写进集合，驱动 record_change 触发器；SQL 直写不产生文档事件。
		_, _ = r.Client.InsertDocument(req.Context(), "ops_events", gosdk.Document{"id": uuid.NewString(), "kind": "ticket", "action": "create", "detail": input.Title, "at": now.Format(time.RFC3339)})
		reply(w, 201, map[string]string{"id": id})
	})
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, req *http.Request) {
		result := map[string]any{}
		for name, path := range map[string]string{"logs": "/logs", "metrics": "/metrics/summary", "trend": "/metrics/trend", "audit": "/audit", "quota": "/quota"} {
			var data any
			if err := r.Request(req.Context(), http.MethodGet, r.ProjectPath(path), nil, &data); err != nil {
				reply(w, 502, map[string]string{"error": name + " unavailable"})
				return
			}
			result[name] = data
		}
		reply(w, 200, result)
	})
}

// registerPlatformRoutes 暴露 SLA 云函数、KV、对象、定时任务、触发器与审计轨迹。
func registerPlatformRoutes(mux *http.ServeMux, r *ex.Runtime) {
	mux.HandleFunc("POST /api/hooks/audit", func(w http.ResponseWriter, req *http.Request) {
		if !isLoopback(req) {
			reply(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		var body struct {
			Source, Kind, Target, Action, RecordID, Key, Detail string
		}
		req.Body = http.MaxBytesReader(w, req.Body, 8192)
		if json.NewDecoder(req.Body).Decode(&body) != nil {
			reply(w, 400, map[string]string{"error": "invalid audit payload"})
			return
		}
		if body.Detail == "" {
			body.Detail = strings.TrimSpace(body.RecordID + " " + body.Key)
		}
		_, err := r.Client.Execute(req.Context(),
			"INSERT INTO audit_log (id, source, kind, target, action, detail, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			[]any{uuid.NewString(), body.Source, body.Kind, body.Target, body.Action, truncate(body.Detail, 200), time.Now().UTC().Format(time.RFC3339Nano)})
		if err != nil {
			reply(w, 502, map[string]string{"error": "audit write failed"})
			return
		}
		reply(w, 200, map[string]bool{"ok": true})
	})

	// SLA 云函数：把库里的未关闭工单交给 sla.Evaluate 判定。
	mux.HandleFunc("POST /api/sla/evaluate", func(w http.ResponseWriter, req *http.Request) {
		result, err := r.Client.Query(req.Context(), "SELECT id, due_at, priority, status FROM tickets LIMIT 100", nil, 100)
		if err != nil {
			reply(w, 502, map[string]string{"error": "tickets unavailable"})
			return
		}
		tickets := mapRows(result)
		var evaluated struct {
			Levels  []string `json:"levels"`
			Overdue []string `json:"overdue"`
			Version int      `json:"version"`
		}
		if err := r.Request(req.Context(), http.MethodPost, "/go/"+r.Project+"/sla/Evaluate", map[string]any{"tickets": tickets}, &evaluated); err != nil {
			reply(w, 502, map[string]string{"error": err.Error()})
			return
		}
		var summary struct {
			Text  string `json:"text"`
			Level string `json:"level"`
		}
		open := 0
		for _, ticket := range tickets {
			if ex.Text(ticket["status"]) != "closed" {
				open++
			}
		}
		if err := r.Request(req.Context(), http.MethodPost, "/go/"+r.Project+"/sla/Summary", map[string]any{
			"open": open, "overdue": len(evaluated.Overdue), "total": len(tickets),
		}, &summary); err != nil {
			reply(w, 502, map[string]string{"error": err.Error()})
			return
		}
		// 标记落到 sla_marks，便于控制台核对。
		for _, id := range evaluated.Overdue {
			_, _ = r.Client.Execute(req.Context(), "INSERT INTO sla_marks (ticket_id, level, run_id) VALUES (?, ?, ?)", []any{id, "overdue", "manual"})
		}
		reply(w, 200, map[string]any{"levels": evaluated.Levels, "overdue": evaluated.Overdue, "version": evaluated.Version, "summary": summary.Text, "summary_level": summary.Level})
	})

	mux.HandleFunc("GET /api/platform", func(w http.ResponseWriter, req *http.Request) {
		out := map[string]any{}
		if result, err := r.Client.Query(req.Context(), "SELECT source, kind, action, detail, created_at FROM audit_log ORDER BY created_at DESC LIMIT 20", nil, 20); err == nil {
			out["audit"] = mapRows(result)
		} else {
			out["audit"] = []any{}
		}
		if total, err := r.Client.Query(req.Context(), "SELECT COUNT(*) AS c FROM audit_log", nil, 1); err == nil {
			if list := mapRows(total); len(list) > 0 {
				out["audit_total"] = list[0]["c"]
			}
		}
		if bySource, err := r.Client.Query(req.Context(), "SELECT source, COUNT(*) AS c FROM audit_log GROUP BY source", nil, 10); err == nil {
			out["audit_calls"] = mapRows(bySource)
		}
		if objects, err := r.Client.ListObjects(req.Context(), "ops/attachments/", true); err == nil {
			out["objects"] = objects
		} else {
			out["objects"] = []any{}
		}
		if snapshot, err := readTriggersSnapshot(); err == nil {
			out["triggers"] = snapshot.Triggers
			out["dropped"] = snapshot.Dropped
		} else {
			out["triggers"] = []any{}
		}
		out["kv"] = scanKV(req, r)
		if raw, err := r.Client.ListCronJobs(req.Context()); err == nil {
			var payload struct {
				Jobs []map[string]any `json:"jobs"`
			}
			if json.Unmarshal(raw, &payload) == nil {
				out["cron_jobs"] = payload.Jobs
			}
		}
		if _, ok := out["cron_jobs"]; !ok {
			out["cron_jobs"] = []any{}
		}
		if raw, err := r.Client.ListFunctions(req.Context()); err == nil {
			var payload struct {
				Functions []map[string]any `json:"functions"`
			}
			if json.Unmarshal(raw, &payload) == nil {
				out["functions"] = payload.Functions
			}
		}
		if _, ok := out["functions"]; !ok {
			out["functions"] = []any{}
		}
		if marks, err := r.Client.Query(req.Context(), "SELECT ticket_id, level, run_id FROM sla_marks ORDER BY level LIMIT 20", nil, 20); err == nil {
			out["sla_marks"] = mapRows(marks)
		} else {
			out["sla_marks"] = []any{}
		}
		reply(w, 200, out)
	})

	mux.HandleFunc("POST /api/platform/emit-event", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096)).Decode(&body)
		if strings.TrimSpace(body.Detail) == "" {
			body.Detail = "平台页手动写入的演示事件"
		}
		doc := gosdk.Document{"id": uuid.NewString(), "kind": "manual", "action": "create", "detail": body.Detail, "at": time.Now().UTC().Format(time.RFC3339)}
		if _, err := r.Client.InsertDocument(req.Context(), "ops_events", doc); err != nil {
			reply(w, 502, map[string]string{"error": "emit failed"})
			return
		}
		reply(w, 201, map[string]string{"id": ex.Text(doc["id"])})
	})

	mux.HandleFunc("POST /api/platform/upload", func(w http.ResponseWriter, req *http.Request) {
		req.Body = http.MaxBytesReader(w, req.Body, 512*1024)
		var body struct {
			Key     string `json:"key"`
			Content string `json:"content"`
		}
		if json.NewDecoder(req.Body).Decode(&body) != nil || !strings.HasPrefix(body.Key, "ops/attachments/") {
			reply(w, 400, map[string]string{"error": "key must start with ops/attachments/"})
			return
		}
		if body.Content == "" {
			body.Content = "运维示例附件 " + time.Now().UTC().Format(time.RFC3339)
		}
		if _, err := r.Client.UploadObject(req.Context(), body.Key, strings.NewReader(body.Content), body.Key[strings.LastIndex(body.Key, "/")+1:], "text/plain"); err != nil {
			reply(w, 502, map[string]string{"error": "upload failed"})
			return
		}
		_, _ = r.Client.Execute(req.Context(), "INSERT INTO runbook_assets (key, ticket_id, size, mime, created_at) VALUES (?, ?, ?, ?, ?)",
			[]any{body.Key, "ticket-demo", int64(len(body.Content)), "text/plain", time.Now().UTC().Format(time.RFC3339)})
		reply(w, 201, map[string]string{"key": body.Key})
	})

	mux.HandleFunc("POST /api/platform/invoke-trigger", func(w http.ResponseWriter, req *http.Request) {
		snapshot, err := readTriggersSnapshot()
		if err != nil {
			reply(w, 503, map[string]string{"error": "trigger snapshot missing; rerun examples/init.sh"})
			return
		}
		var target string
		for _, item := range snapshot.Triggers {
			if item.Kind == "http" && item.Enabled {
				target = item.ID
				break
			}
		}
		if target == "" {
			reply(w, 404, map[string]string{"error": "no enabled http trigger"})
			return
		}
		if err := r.Client.InvokeFunctionTrigger(req.Context(), target, map[string]any{"source": "http", "detail": "平台页手动 invoke"}); err != nil {
			reply(w, 502, map[string]string{"error": err.Error()})
			return
		}
		reply(w, 202, map[string]any{"ok": true, "queued": true})
	})

	mux.HandleFunc("POST /api/platform/run-cron", func(w http.ResponseWriter, req *http.Request) {
		raw, err := r.Client.ListCronJobs(req.Context())
		if err != nil {
			reply(w, 503, map[string]string{"error": "cron jobs unavailable"})
			return
		}
		var payload struct {
			Jobs []map[string]any `json:"jobs"`
		}
		if json.Unmarshal(raw, &payload) != nil || len(payload.Jobs) == 0 {
			reply(w, 503, map[string]string{"error": "cron jobs unavailable"})
			return
		}
		var jobID, jobName string
		for _, job := range payload.Jobs {
			if ex.Text(job["name"]) == "ops_sla_scan" {
				jobID, jobName = ex.Text(job["id"]), ex.Text(job["name"])
				break
			}
		}
		if jobID == "" {
			jobID, jobName = ex.Text(payload.Jobs[0]["id"]), ex.Text(payload.Jobs[0]["name"])
		}
		if _, err := r.Client.TriggerCronJob(req.Context(), jobID); err != nil {
			reply(w, 502, map[string]string{"error": err.Error()})
			return
		}
		reply(w, 200, map[string]string{"ok": "true", "name": jobName})
	})
}

type triggerSnapshot struct {
	Triggers []struct {
		ID         string `json:"id"`
		Kind       string `json:"kind"`
		Target     string `json:"target"`
		FuncFile   string `json:"func_file"`
		FuncExport string `json:"func_export"`
		Enabled    bool   `json:"enabled"`
	} `json:"triggers"`
	Dropped int `json:"dropped"`
}

func readTriggersSnapshot() (triggerSnapshot, error) {
	var snapshot triggerSnapshot
	candidates := []string{
		os.Getenv("EXAMPLE_TRIGGERS"),
		"examples/ops-assistant/.simplebase/triggers.json",
		"../.simplebase/triggers.json",
		".simplebase/triggers.json",
	}
	var err error
	for _, path := range candidates {
		if path == "" {
			continue
		}
		var raw []byte
		if raw, err = os.ReadFile(path); err == nil {
			if err = json.Unmarshal(raw, &snapshot); err == nil {
				return snapshot, nil
			}
		}
	}
	return snapshot, err
}

// scanKV 用 SCAN 游标遍历项目 KV，逐个取类型、TTL 与内容。
func scanKV(req *http.Request, r *ex.Runtime) []map[string]any {
	keys := []string{}
	cursor := "0"
	for page := 0; page < 5 && len(keys) < 30; page++ {
		replyValue, err := r.KV(req.Context(), "SCAN", cursor, "MATCH", "*", "COUNT", "50")
		if err != nil {
			break
		}
		pair, ok := replyValue.([]any)
		if !ok || len(pair) != 2 {
			break
		}
		cursor, _ = pair[0].(string)
		if list, ok := pair[1].([]any); ok {
			for _, item := range list {
				if key, ok := item.(string); ok {
					keys = append(keys, key)
				}
			}
		}
		if cursor == "0" {
			break
		}
	}
	rows := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		kind, _ := r.KV(req.Context(), "TYPE", key)
		ttl, _ := r.KV(req.Context(), "TTL", key)
		row := map[string]any{"key": key, "type": ex.Text(kind), "ttl": ex.Text(ttl)}
		switch ex.Text(kind) {
		case "string":
			value, _ := r.KV(req.Context(), "GET", key)
			row["value"] = ex.Text(value)
		case "list":
			value, _ := r.KV(req.Context(), "LRANGE", key, "0", "5")
			row["value"] = marshalTruncated(value)
		case "hash":
			value, _ := r.KV(req.Context(), "HGETALL", key)
			row["value"] = marshalTruncated(value)
		case "set":
			value, _ := r.KV(req.Context(), "SMEMBERS", key)
			row["value"] = marshalTruncated(value)
		case "zset":
			value, _ := r.KV(req.Context(), "ZRANGE", key, "0", "5", "WITHSCORES")
			row["value"] = marshalTruncated(value)
		}
		rows = append(rows, row)
	}
	return rows
}

func marshalTruncated(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return truncate(string(raw), 160)
}

func mapRows(result *gosdk.QueryResult) []map[string]any {
	out := make([]map[string]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		item := make(map[string]any, len(result.Columns))
		for index, column := range result.Columns {
			if index < len(row) {
				item[column] = row[index]
			}
		}
		out = append(out, item)
	}
	return out
}

func isLoopback(req *http.Request) bool {
	host := req.RemoteAddr
	if index := strings.LastIndex(host, ":"); index > 0 {
		host = host[:index]
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "::1"
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}

func servePage(w http.ResponseWriter, path string) {
	page, err := os.ReadFile(path)
	if err != nil {
		page, err = os.ReadFile("../web/index.html")
	}
	if err != nil {
		reply(w, 404, map[string]string{"error": "page not found; run from repo root"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}

func reply(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
