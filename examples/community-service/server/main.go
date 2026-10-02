package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	ex "github.com/linkxzhou/SimpleBase/examples/shared/goexample"
	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func uuid() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func jsonResponse(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
func list(ctx context.Context, r *ex.Runtime, sql string, args []any, max int) ([]map[string]any, error) {
	result, err := r.Client.Query(ctx, sql, args, max)
	if err != nil {
		return nil, err
	}
	return ex.Rows(result)
}
func initialize(ctx context.Context, r *ex.Runtime) error {
	collections, err := r.Client.ListCollections(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, name := range collections.Collections {
		if name == "ex_community_posts" {
			found = true
		}
	}
	if !found {
		if err := r.Client.CreateCollection(ctx, "ex_community_posts"); err != nil {
			return err
		}
	}
	for _, ddl := range []string{
		"CREATE TABLE IF NOT EXISTS ex_community_likes (post_id VARCHAR NOT NULL, actor_id VARCHAR NOT NULL, created_at VARCHAR NOT NULL, PRIMARY KEY (post_id, actor_id))",
		"CREATE TABLE IF NOT EXISTS ex_community_trending (window_start VARCHAR NOT NULL, post_id VARCHAR NOT NULL, score BIGINT NOT NULL, source_run_id VARCHAR NOT NULL, PRIMARY KEY (window_start, post_id))",
		"CREATE TABLE IF NOT EXISTS ex_community_processed_runs (run_id VARCHAR PRIMARY KEY, window_start VARCHAR NOT NULL)",
	} {
		if _, err := r.Client.Execute(ctx, ddl, nil); err != nil {
			return err
		}
	}
	return nil
}
func apply(ctx context.Context, r *ex.Runtime, win ex.Window, run ex.Run) error {
	seen, err := list(ctx, r, "SELECT run_id FROM ex_community_processed_runs WHERE run_id = ?", []any{run.ID}, 1)
	if err != nil {
		return err
	}
	if len(seen) > 0 {
		return nil
	}
	likes, err := list(ctx, r, "SELECT post_id, count(*) AS like_count FROM ex_community_likes WHERE created_at >= ? AND created_at < ? GROUP BY post_id LIMIT 101", []any{win.Start, win.End}, 101)
	if err != nil {
		return err
	}
	if len(likes) > 100 {
		return errors.New("too many posts; manual reconciliation required")
	}
	docs, err := r.Client.ListDocuments(ctx, "ex_community_posts")
	if err != nil {
		return err
	}
	valid := map[string]bool{}
	for _, doc := range docs.Rows {
		valid[ex.Text(doc["id"])] = true
	}
	type Row struct {
		PostID     string `json:"post_id"`
		LikeCount  int64  `json:"like_count"`
		AgeMinutes int64  `json:"age_minutes"`
	}
	input := struct {
		Rows []Row `json:"rows"`
	}{Rows: []Row{}}
	for _, item := range likes {
		postID := ex.Text(item["post_id"])
		if valid[postID] {
			input.Rows = append(input.Rows, Row{postID, ex.Int(item["like_count"]), 0})
		}
	}
	var output struct {
		Items []struct {
			PostID string `json:"post_id"`
			Score  int64  `json:"score"`
		} `json:"items"`
	}
	if err := r.Compute(ctx, "ex_community_trending", input, &output); err != nil {
		return err
	}
	statements := make([]gosdk.SQLStatement, 0, len(output.Items)+1)
	for _, item := range output.Items {
		statements = append(statements, gosdk.SQLStatement{SQL: "INSERT INTO ex_community_trending SELECT ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM ex_community_trending WHERE window_start = ? AND post_id = ?)", Args: []any{win.Start, item.PostID, item.Score, run.ID, win.Start, item.PostID}})
	}
	statements = append(statements, gosdk.SQLStatement{SQL: "INSERT INTO ex_community_processed_runs (run_id, window_start) VALUES (?, ?)", Args: []any{run.ID, win.Start}})
	batch, err := r.Client.Batch(ctx, statements, true)
	if err := ex.CheckBatch(batch, err); err != nil {
		return err
	}
	data, _ := json.Marshal(output.Items)
	_, err = r.KV(ctx, "SET", "ex:community:trend", string(data), "EX", "3600")
	return err
}
func main() {
	r, err := ex.New()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	if err := initialize(ctx, r); err != nil {
		log.Fatal(err)
	}
	if os.Getenv("EXAMPLE_WORKER") == "1" {
		r.StartWorker(ctx, "ex_community_hot", func(ctx context.Context, w ex.Window, run ex.Run) error { return apply(ctx, r, w, run) })
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { jsonResponse(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /api/posts", func(w http.ResponseWriter, req *http.Request) {
		docs, err := r.Client.ListDocuments(req.Context(), "ex_community_posts")
		if err != nil {
			jsonResponse(w, 500, map[string]string{"error": "Posts unavailable"})
			return
		}
		jsonResponse(w, 200, docs.Rows)
	})
	mux.HandleFunc("POST /api/posts", func(w http.ResponseWriter, req *http.Request) {
		req.Body = http.MaxBytesReader(w, req.Body, 8192)
		var input struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil || len(strings.TrimSpace(input.Body)) < 2 || len(input.Body) > 500 {
			jsonResponse(w, 400, map[string]string{"error": "Invalid post"})
			return
		}
		doc, err := r.Client.InsertDocument(req.Context(), "ex_community_posts", gosdk.Document{"id": uuid(), "author_id": "local-demo", "body": strings.TrimSpace(input.Body), "created_at": time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			jsonResponse(w, 500, map[string]string{"error": "Post failed"})
			return
		}
		jsonResponse(w, 201, doc)
	})
	mux.HandleFunc("POST /api/posts/{id}/likes", func(w http.ResponseWriter, req *http.Request) {
		postID := req.PathValue("id")
		if len(postID) != 32 {
			jsonResponse(w, 400, map[string]string{"error": "Invalid post"})
			return
		}
		docs, err := r.Client.ListDocuments(req.Context(), "ex_community_posts")
		if err != nil {
			jsonResponse(w, 500, map[string]string{"error": "Posts unavailable"})
			return
		}
		found := false
		for _, doc := range docs.Rows {
			if doc["id"] == postID {
				found = true
			}
		}
		if !found {
			jsonResponse(w, 404, map[string]string{"error": "Post not found"})
			return
		}
		_, err = r.Client.Execute(req.Context(), "INSERT INTO ex_community_likes (post_id, actor_id, created_at) VALUES (?, ?, ?)", []any{postID, "local-demo", time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			jsonResponse(w, 409, map[string]string{"error": "Already liked"})
			return
		}
		_, _ = r.KV(req.Context(), "DEL", "ex:community:trend")
		jsonResponse(w, 201, map[string]bool{"liked": true})
	})
	mux.HandleFunc("GET /api/trending", func(w http.ResponseWriter, req *http.Request) {
		items, err := list(req.Context(), r, "SELECT post_id, score FROM ex_community_trending ORDER BY window_start DESC, score DESC LIMIT 20", nil, 20)
		if err != nil {
			jsonResponse(w, 500, map[string]string{"error": "Trends unavailable"})
			return
		}
		jsonResponse(w, 200, items)
	})
	port := os.Getenv("EXAMPLE_PORT")
	if port == "" {
		port = "8096"
	}
	log.Printf("Community demo listening on 127.0.0.1:%s", port)
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, ex.WithAssets(ex.LocalOnly(mux), "examples/community-service")))
}
