package main

import (
	"encoding/json"

	"github.com/google/uuid"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	ex "github.com/linkxzhou/SimpleBase/examples/lib/goexample"
	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func main() {
	r, err := ex.New()
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
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
	port := os.Getenv("EXAMPLE_PORT")
	if port == "" {
		port = "8097"
	}
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, mux))
}

func reply(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
