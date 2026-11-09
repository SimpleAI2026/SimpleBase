package main

import (
	"encoding/json"
	"fmt"
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
	registerPostRoutes(mux, r)
	registerPlatformRoutes(mux, r)
	port := os.Getenv("EXAMPLE_PORT")
	if port == "" {
		port = "8096"
	}
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, mux))
}

func registerPostRoutes(mux *http.ServeMux, r *ex.Runtime) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { respond(w, 200, map[string]bool{"ok": true}) })

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		page, err := os.ReadFile("examples/community/web/index.html")
		if err != nil {
			page, err = os.ReadFile("../web/index.html")
		}
		if err != nil {
			respond(w, 404, map[string]string{"error": "page not found; run from repo root"})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(page)
	})

	// 热榜直接读 KV zset（rank:hot），与点赞接口写入的分数对应。
	mux.HandleFunc("GET /api/hot", func(w http.ResponseWriter, req *http.Request) {
		reply, err := r.KV(req.Context(), "ZRANGE", "rank:hot", "0", "20", "REV", "WITHSCORES")
		if err != nil {
			respond(w, 200, []any{})
			return
		}
		// WITHSCORES 的回复是扁平数组 [member, score, member, score, ...]；
		// 部分实现会把 score 编码成数字，统一转字符串输出。
		list, _ := reply.([]any)
		items := make([]map[string]any, 0, len(list)/2)
		for index := 0; index+1 < len(list); index += 2 {
			items = append(items, map[string]any{"id": fmt.Sprint(list[index]), "score": fmt.Sprint(list[index+1])})
		}
		respond(w, 200, items)
	})
	mux.HandleFunc("GET /api/posts", func(w http.ResponseWriter, req *http.Request) {
		posts, err := r.Client.ListDocuments(req.Context(), "posts")
		if err != nil {
			respond(w, 502, map[string]string{"error": "posts unavailable"})
			return
		}
		respond(w, 200, posts.Rows)
	})
	mux.HandleFunc("POST /api/posts", func(w http.ResponseWriter, req *http.Request) {
		var input struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		req.Body = http.MaxBytesReader(w, req.Body, 8192)
		if json.NewDecoder(req.Body).Decode(&input) != nil || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Body) == "" {
			respond(w, 400, map[string]string{"error": "invalid post"})
			return
		}
		var checked struct {
			Allowed bool `json:"allowed"`
		}
		if err := r.Request(req.Context(), http.MethodPost, "/go/"+r.Project+"/moderation/Check", input, &checked); err != nil || !checked.Allowed {
			respond(w, 400, map[string]string{"error": "post rejected"})
			return
		}
		id := uuid.NewString()
		post, err := r.Client.InsertDocument(req.Context(), "posts", gosdk.Document{"id": id, "title": input.Title, "body": input.Body, "author": "demo-user"})
		if err != nil {
			respond(w, 502, map[string]string{"error": "post write failed"})
			return
		}
		if _, err := r.Client.Execute(req.Context(), "INSERT INTO post_stats (post_id, likes, score) VALUES (?, 0, 0)", []any{id}); err != nil {
			_ = r.Client.DeleteDocument(req.Context(), "posts", id)
			respond(w, 502, map[string]string{"error": "post compensation performed"})
			return
		}
		respond(w, 201, post)
	})
	mux.HandleFunc("POST /api/posts/{id}/likes", func(w http.ResponseWriter, req *http.Request) {
		id := req.PathValue("id")
		if len(id) > 80 {
			respond(w, 400, map[string]string{"error": "invalid post"})
			return
		}
		docs, err := r.Client.ListDocuments(req.Context(), "posts")
		if err != nil {
			respond(w, 502, map[string]string{"error": "posts unavailable"})
			return
		}
		found := false
		for _, doc := range docs.Rows {
			if doc["id"] == id {
				found = true
				break
			}
		}
		if !found {
			respond(w, 404, map[string]string{"error": "post not found"})
			return
		}
		like, err := r.Client.Query(req.Context(), "SELECT post_id FROM likes WHERE post_id = ? AND user_id = ?", []any{id, "local-demo"}, 1)
		if err != nil {
			respond(w, 502, map[string]string{"error": "likes unavailable"})
			return
		}
		if len(like.Rows) > 0 {
			respond(w, 200, map[string]bool{"liked": true})
			return
		}
		batch, err := r.Client.Batch(req.Context(), []gosdk.SQLStatement{
			{SQL: "INSERT INTO likes (post_id, user_id, created_at) SELECT ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM likes WHERE post_id = ? AND user_id = ?)", Args: []any{id, "local-demo", time.Now().UTC().Format(time.RFC3339), id, "local-demo"}},
			{SQL: "UPDATE post_stats SET likes = likes + 1, score = score + 10 WHERE post_id = ?", Args: []any{id}},
		}, true)
		if ex.CheckBatch(batch, err) != nil {
			respond(w, 502, map[string]string{"error": "like transaction failed"})
			return
		}
		_, _ = r.KV(req.Context(), "SADD", "liked:local-demo", id)
		_, _ = r.KV(req.Context(), "ZINCRBY", "rank:hot", "10", id)
		respond(w, 201, map[string]bool{"liked": true})
	})
	mux.HandleFunc("DELETE /api/posts/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := req.PathValue("id")
		docs, err := r.Client.ListDocuments(req.Context(), "posts")
		if err != nil {
			respond(w, 502, map[string]string{"error": "posts unavailable"})
			return
		}
		for _, doc := range docs.Rows {
			if doc["id"] == id {
				if key, ok := doc["attachment_key"].(string); ok && key != "" {
					if _, err := r.Client.DeleteObject(req.Context(), key); err != nil {
						respond(w, 502, map[string]string{"error": "attachment delete failed"})
						return
					}
				}
				if err := r.Client.DeleteDocument(req.Context(), "posts", id); err != nil {
					respond(w, 502, map[string]string{"error": "post delete failed"})
					return
				}
				_, _ = r.KV(req.Context(), "DEL", "post:"+id)
				respond(w, 200, map[string]bool{"deleted": true})
				return
			}
		}
		respond(w, 404, map[string]string{"error": "post not found"})
	})
	mux.HandleFunc("POST /api/posts/{id}/summary", func(w http.ResponseWriter, req *http.Request) {
		id := req.PathValue("id")
		docs, err := r.Client.ListDocuments(req.Context(), "posts")
		if err != nil {
			respond(w, 502, map[string]string{"error": "posts unavailable"})
			return
		}
		for _, doc := range docs.Rows {
			if doc["id"] != id {
				continue
			}
			// OpenAI 兼容出口：路径里的两段 /v1 都要保留（见 internal/AGENTS.md）。
			var models struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if err := r.Request(req.Context(), http.MethodGet, r.ProjectPath("/v1/models"), nil, &models); err != nil || len(models.Data) == 0 {
				respond(w, 501, map[string]string{"error": "LLM provider unavailable"})
				return
			}
			payload := map[string]any{"messages": []map[string]string{{"role": "user", "content": "请用一句话概括：" + ex.Text(doc["title"]) + " " + ex.Text(doc["body"])}}}
			var completion struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			}
			if err := r.Request(req.Context(), http.MethodPost, r.ProjectPath("/v1/chat/completions"), payload, &completion); err != nil || len(completion.Choices) == 0 {
				respond(w, 502, map[string]string{"error": "summary failed"})
				return
			}
			summary := completion.Choices[0].Message.Content
			doc["summary"] = summary
			if _, err := r.Client.UpdateDocument(req.Context(), "posts", id, doc); err != nil {
				respond(w, 502, map[string]string{"error": "summary save failed"})
				return
			}
			respond(w, 200, map[string]string{"summary": summary})
			return
		}
		respond(w, 404, map[string]string{"error": "post not found"})
	})
}

// registerPlatformRoutes 暴露 KV、对象、云函数、定时任务、触发器与审计轨迹，
// 以及附件上传 / 事件写入 / HTTP 触发器 invoke 等演示动作。
func registerPlatformRoutes(mux *http.ServeMux, r *ex.Runtime) {
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
		_, err := r.Client.Execute(req.Context(),
			"INSERT INTO audit_log (id, source, kind, target, action, detail, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			[]any{uuid.NewString(), body.Source, body.Kind, body.Target, body.Action, truncate(body.Detail, 200), time.Now().UTC().Format(time.RFC3339Nano)})
		if err != nil {
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
		if objects, err := r.Client.ListObjects(req.Context(), "posts/", true); err == nil {
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
		// KV：SCAN 返回 [cursor, keys]，逐个取类型与内容。
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
				row["value"] = ex.Text(func() any { v, _ := r.KV(req.Context(), "GET", key); return v }())
			case "hash", "list", "set", "zset":
				var value any
				switch ex.Text(kind) {
				case "hash":
					value, _ = r.KV(req.Context(), "HGETALL", key)
				case "list":
					value, _ = r.KV(req.Context(), "LRANGE", key, "0", "5")
				case "set":
					value, _ = r.KV(req.Context(), "SMEMBERS", key)
				default:
					value, _ = r.KV(req.Context(), "ZRANGE", key, "0", "5", "WITHSCORES")
				}
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
			} else {
				out["cron_jobs"] = []any{}
			}
		} else {
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
		respond(w, 200, out)
	})

	mux.HandleFunc("POST /api/platform/emit-event", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096)).Decode(&body)
		if strings.TrimSpace(body.Detail) == "" {
			body.Detail = "平台页手动写入的演示事件"
		}
		doc := gosdk.Document{"id": uuid.NewString(), "title": "平台演示事件", "body": body.Detail, "author": "platform-page", "at": time.Now().UTC().Format(time.RFC3339)}
		if _, err := r.Client.InsertDocument(req.Context(), "platform_events", doc); err != nil {
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
		if json.NewDecoder(req.Body).Decode(&body) != nil || !strings.HasPrefix(body.Key, "posts/") {
			respond(w, 400, map[string]string{"error": "key must start with posts/"})
			return
		}
		if body.Content == "" {
			body.Content = fmt.Sprintf("SimpleBase 示例附件 %s", time.Now().UTC().Format(time.RFC3339))
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
			if ex.Text(job["name"]) == "community_hot" {
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

// readTriggersSnapshot 读初始化脚本写下的触发器快照（管理面需要 project:admin，
// 运行时 Key 读不到，所以由 examples/init.sh 落一份只含非敏感字段的快照）。
func readTriggersSnapshot() (triggerSnapshot, error) {
	var snapshot triggerSnapshot
	// 工作目录不确定（`go run ./examples/community/server` 时是仓库根），按候选路径依次尝试。
	candidates := []string{
		os.Getenv("EXAMPLE_TRIGGERS"),
		"examples/community/.simplebase/triggers.json",
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

// mapRows 把 SDK 的位置行（columns + [][]any）转成对象数组。
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

func respond(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
