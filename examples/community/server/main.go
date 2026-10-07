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
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { respond(w, 200, map[string]bool{"ok": true}) })
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
			var providers struct {
				Providers []string `json:"providers"`
			}
			if err := r.Request(req.Context(), http.MethodGet, r.ProjectPath("/llm/providers"), nil, &providers); err != nil || len(providers.Providers) == 0 {
				respond(w, 501, map[string]string{"error": "LLM provider unavailable"})
				return
			}
			payload := map[string]any{"messages": []map[string]string{{"role": "user", "content": "请用一句话概括：" + ex.Text(doc["title"]) + " " + ex.Text(doc["body"])}}}
			var summary struct {
				Content string `json:"content"`
			}
			if err := r.Request(req.Context(), http.MethodPost, r.ProjectPath("/llm/chat"), payload, &summary); err != nil {
				respond(w, 502, map[string]string{"error": "summary failed"})
				return
			}
			doc["summary"] = summary.Content
			if _, err := r.Client.UpdateDocument(req.Context(), "posts", id, doc); err != nil {
				respond(w, 502, map[string]string{"error": "summary save failed"})
				return
			}
			respond(w, 200, map[string]string{"summary": summary.Content})
			return
		}
		respond(w, 404, map[string]string{"error": "post not found"})
	})
	port := os.Getenv("EXAMPLE_PORT")
	if port == "" {
		port = "8096"
	}
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, mux))
}

func respond(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
