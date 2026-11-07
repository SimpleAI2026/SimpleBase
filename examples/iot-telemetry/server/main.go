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

// 物联网遥测的轻量 BFF：页面要看的设备、读数、KV、导出对象、定时任务与触发器审计都从这里出。
// 数据面仍由 CLI / 脚本（scripts/curl.sh、simulate.mjs、worker.mjs）驱动。
func main() {
	r, err := ex.New()
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { respond(w, 200, map[string]bool{"ok": true}) })

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		page, err := os.ReadFile("examples/iot-telemetry/web/index.html")
		if err != nil {
			page, err = os.ReadFile("../web/index.html")
		}
		if err != nil {
			page, err = os.ReadFile("web/index.html")
		}
		if err != nil {
			respond(w, 404, map[string]string{"error": "page not found; run from repo root"})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(page)
	})

	mux.HandleFunc("GET /api/devices", func(w http.ResponseWriter, req *http.Request) {
		result, err := r.Client.Query(req.Context(), "SELECT d.id, d.name, d.model, (SELECT COUNT(*) FROM readings r WHERE r.device_id = d.id) AS readings FROM devices d ORDER BY d.id", nil, 50)
		if err != nil {
			respond(w, 502, map[string]string{"error": "devices unavailable"})
			return
		}
		respond(w, 200, mapRows(result))
	})

	mux.HandleFunc("GET /api/readings", func(w http.ResponseWriter, req *http.Request) {
		device := req.URL.Query().Get("device")
		sql := "SELECT device_id, ts, metric, value FROM readings"
		args := []any{}
		if device != "" {
			sql += " WHERE device_id = ?"
			args = append(args, device)
		}
		sql += " ORDER BY ts DESC LIMIT 30"
		result, err := r.Client.Query(req.Context(), sql, args, 30)
		if err != nil {
			respond(w, 502, map[string]string{"error": "readings unavailable"})
			return
		}
		respond(w, 200, mapRows(result))
	})

	// 写一条读数：SQL 写入不产生文档事件，所以同时写一条 device_events 文档驱动触发器。
	mux.HandleFunc("POST /api/readings", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			DeviceID string  `json:"device_id"`
			Metric   string  `json:"metric"`
			Value    float64 `json:"value"`
		}
		req.Body = http.MaxBytesReader(w, req.Body, 4096)
		if json.NewDecoder(req.Body).Decode(&body) != nil || strings.TrimSpace(body.DeviceID) == "" || strings.TrimSpace(body.Metric) == "" {
			respond(w, 400, map[string]string{"error": "invalid reading"})
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := r.Client.Execute(req.Context(), "INSERT INTO readings (device_id, ts, metric, value) VALUES (?, ?, ?, ?)", []any{body.DeviceID, now, body.Metric, body.Value}); err != nil {
			respond(w, 502, map[string]string{"error": "reading write failed"})
			return
		}
		_, _ = r.KV(req.Context(), "INCR", "counter:readings")
		_, _ = r.KV(req.Context(), "SET", "online:"+body.DeviceID, "1", "EX", "120")
		_, _ = r.KV(req.Context(), "LPUSH", "events:"+body.DeviceID, fmtReading(body.Metric, body.Value))
		_, _ = r.KV(req.Context(), "LTRIM", "events:"+body.DeviceID, "0", "99")
		if _, err := r.Client.InsertDocument(req.Context(), "device_events", gosdk.Document{
			"id": uuid.NewString(), "device_id": body.DeviceID, "kind": "reading",
			"detail": fmtReading(body.Metric, body.Value), "at": now,
		}); err != nil {
			respond(w, 502, map[string]string{"error": "event write failed"})
			return
		}
		respond(w, 201, map[string]any{"ok": true, "device_id": body.DeviceID, "ts": now})
	})

	mux.HandleFunc("POST /api/hooks/audit", func(w http.ResponseWriter, req *http.Request) {
		if !isLoopback(req) {
			respond(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		var body struct {
			Source, Kind, Target, Action, RecordID, Key, Detail string
		}
		req.Body = http.MaxBytesReader(w, req.Body, 8192)
		if json.NewDecoder(req.Body).Decode(&body) != nil {
			respond(w, 400, map[string]string{"error": "invalid audit payload"})
			return
		}
		if body.Detail == "" {
			body.Detail = strings.TrimSpace(body.RecordID + " " + body.Key)
		}
		if _, err := r.Client.Execute(req.Context(),
			"INSERT INTO audit_log (id, source, kind, target, action, detail, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			[]any{uuid.NewString(), body.Source, body.Kind, body.Target, body.Action, truncate(body.Detail, 200), time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
			respond(w, 502, map[string]string{"error": "audit write failed"})
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
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
		if objects, err := r.Client.ListObjects(req.Context(), "exports/", true); err == nil {
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
		keys := []string{}
		cursor := "0"
		for page := 0; page < 5 && len(keys) < 30; page++ {
			reply, err := r.KV(req.Context(), "SCAN", cursor, "MATCH", "*", "COUNT", "50")
			if err != nil {
				break
			}
			pair, ok := reply.([]any)
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
				if raw, err := json.Marshal(value); err == nil {
					row["value"] = truncate(string(raw), 160)
				}
			case "hash":
				value, _ := r.KV(req.Context(), "HGETALL", key)
				if raw, err := json.Marshal(value); err == nil {
					row["value"] = truncate(string(raw), 160)
				}
			case "set":
				value, _ := r.KV(req.Context(), "SMEMBERS", key)
				if raw, err := json.Marshal(value); err == nil {
					row["value"] = truncate(string(raw), 160)
				}
			case "zset":
				value, _ := r.KV(req.Context(), "ZRANGE", key, "0", "5", "WITHSCORES")
				if raw, err := json.Marshal(value); err == nil {
					row["value"] = truncate(string(raw), 160)
				}
			}
			rows = append(rows, row)
		}
		out["kv"] = rows
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
		if result, err := r.Client.Query(req.Context(), "SELECT device_id, bucket, metric, avg, max, count FROM rollup_5m ORDER BY bucket DESC LIMIT 12", nil, 12); err == nil {
			out["rollups"] = mapRows(result)
		} else {
			out["rollups"] = []any{}
		}
		respond(w, 200, out)
	})

	// 手动聚合：把最近读数按窗口求平均，写进 rollup_5m（演示 SQL + 云函数配合）。
	mux.HandleFunc("POST /api/platform/rollup", func(w http.ResponseWriter, req *http.Request) {
		result, err := r.Client.Query(req.Context(), "SELECT device_id, metric, value FROM readings ORDER BY ts DESC LIMIT 80", nil, 80)
		if err != nil {
			respond(w, 502, map[string]string{"error": "readings unavailable"})
			return
		}
		rows := mapRows(result)
		points := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			points = append(points, map[string]any{"value": row["value"]})
		}
		var aggregated struct {
			Avg   float64 `json:"avg"`
			Max   float64 `json:"max"`
			Count int     `json:"count"`
		}
		if err := r.Request(req.Context(), http.MethodPost, "/go/"+r.Project+"/rollup/Aggregate", map[string]any{"points": points}, &aggregated); err != nil {
			respond(w, 502, map[string]string{"error": err.Error()})
			return
		}
		bucket := time.Now().UTC().Truncate(5 * time.Minute).Format(time.RFC3339)
		if len(rows) > 0 {
			if _, err := r.Client.Execute(req.Context(),
				"INSERT INTO rollup_5m (device_id, bucket, metric, avg, max, count, source_run_id) VALUES (?, ?, ?, ?, ?, ?, ?)",
				[]any{ex.Text(rows[0]["device_id"]), bucket, ex.Text(rows[0]["metric"]), aggregated.Avg, aggregated.Max, aggregated.Count, "manual"}); err != nil {
				respond(w, 502, map[string]string{"error": "rollup write failed"})
				return
			}
		}
		respond(w, 201, map[string]any{"ok": true, "bucket": bucket, "avg": aggregated.Avg, "max": aggregated.Max, "count": aggregated.Count})
	})

	mux.HandleFunc("POST /api/platform/emit-event", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096)).Decode(&body)
		if strings.TrimSpace(body.Detail) == "" {
			body.Detail = "平台页手动写入的演示事件"
		}
		doc := gosdk.Document{"id": uuid.NewString(), "device_id": "device-01", "kind": "manual", "detail": body.Detail, "at": time.Now().UTC().Format(time.RFC3339)}
		if _, err := r.Client.InsertDocument(req.Context(), "device_events", doc); err != nil {
			respond(w, 502, map[string]string{"error": "emit failed"})
			return
		}
		respond(w, 201, map[string]string{"id": ex.Text(doc["id"])})
	})

	mux.HandleFunc("POST /api/platform/upload", func(w http.ResponseWriter, req *http.Request) {
		req.Body = http.MaxBytesReader(w, req.Body, 512*1024)
		var body struct {
			Key     string `json:"key"`
			Content string `json:"content"`
		}
		if json.NewDecoder(req.Body).Decode(&body) != nil || !strings.HasPrefix(body.Key, "exports/") {
			respond(w, 400, map[string]string{"error": "key must start with exports/"})
			return
		}
		if body.Content == "" {
			body.Content = "ts,device_id,metric,value\n" + time.Now().UTC().Format(time.RFC3339) + ",device-01,temperature,22.5\n"
		}
		if _, err := r.Client.UploadObject(req.Context(), body.Key, strings.NewReader(body.Content), body.Key[strings.LastIndex(body.Key, "/")+1:], "text/plain"); err != nil {
			respond(w, 502, map[string]string{"error": "upload failed"})
			return
		}
		respond(w, 201, map[string]string{"key": body.Key})
	})

	mux.HandleFunc("POST /api/platform/invoke-trigger", func(w http.ResponseWriter, req *http.Request) {
		snapshot, err := readTriggersSnapshot()
		if err != nil {
			respond(w, 503, map[string]string{"error": "trigger snapshot missing; rerun examples/init.sh"})
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
			respond(w, 404, map[string]string{"error": "no enabled http trigger"})
			return
		}
		if err := r.Client.InvokeFunctionTrigger(req.Context(), target, map[string]any{"source": "http", "detail": "平台页手动 invoke"}); err != nil {
			respond(w, 502, map[string]string{"error": err.Error()})
			return
		}
		respond(w, 202, map[string]any{"ok": true, "queued": true})
	})

	mux.HandleFunc("POST /api/platform/run-cron", func(w http.ResponseWriter, req *http.Request) {
		raw, err := r.Client.ListCronJobs(req.Context())
		if err != nil {
			respond(w, 503, map[string]string{"error": "cron jobs unavailable"})
			return
		}
		var payload struct {
			Jobs []map[string]any `json:"jobs"`
		}
		if json.Unmarshal(raw, &payload) != nil || len(payload.Jobs) == 0 {
			respond(w, 503, map[string]string{"error": "cron jobs unavailable"})
			return
		}
		var jobID, jobName string
		for _, job := range payload.Jobs {
			if ex.Text(job["name"]) == "iot_rollup" {
				jobID, jobName = ex.Text(job["id"]), ex.Text(job["name"])
				break
			}
		}
		if jobID == "" {
			jobID, jobName = ex.Text(payload.Jobs[0]["id"]), ex.Text(payload.Jobs[0]["name"])
		}
		if _, err := r.Client.TriggerCronJob(req.Context(), jobID); err != nil {
			respond(w, 502, map[string]string{"error": err.Error()})
			return
		}
		respond(w, 200, map[string]string{"ok": "true", "name": jobName})
	})

	port := os.Getenv("EXAMPLE_PORT")
	if port == "" {
		port = "8098"
	}
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, mux))
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

// readTriggersSnapshot 读初始化脚本写下的触发器快照（管理面需要 project:admin）。
func readTriggersSnapshot() (triggerSnapshot, error) {
	var snapshot triggerSnapshot
	candidates := []string{
		os.Getenv("EXAMPLE_TRIGGERS"),
		"examples/iot-telemetry/.simplebase/triggers.json",
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

func fmtReading(metric string, value float64) string {
	return metric + "=" + trimFloat(value)
}

// trimFloat 把 22.50 这类多余小数位去掉：先按 json 编码（最短往返表示），再去尾零。
func trimFloat(value float64) string {
	raw, _ := json.Marshal(value)
	text := string(raw)
	if !strings.Contains(text, ".") {
		return text
	}
	text = strings.TrimRight(text, "0")
	return strings.TrimRight(text, ".")
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

func respond(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
