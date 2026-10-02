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
	"strconv"
	"time"

	ex "github.com/linkxzhou/SimpleBase/examples/shared/goexample"
	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

func id() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func validScore(score int64) bool { return score >= 0 && score <= 1000 }
func jsonResponse(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
func list(ctx context.Context, r *ex.Runtime, q string, args []any, max int) ([]map[string]any, error) {
	result, err := r.Client.Query(ctx, q, args, max)
	if err != nil {
		return nil, err
	}
	return ex.Rows(result)
}
func initialize(ctx context.Context, r *ex.Runtime) error {
	for _, ddl := range []string{
		"CREATE TABLE IF NOT EXISTS ex_game_levels (id VARCHAR PRIMARY KEY, title VARCHAR NOT NULL, answer VARCHAR NOT NULL)",
		"CREATE TABLE IF NOT EXISTS ex_game_scores (id VARCHAR PRIMARY KEY, session_id VARCHAR NOT NULL UNIQUE, player_id VARCHAR NOT NULL, score BIGINT NOT NULL, created_at VARCHAR NOT NULL)",
		"CREATE TABLE IF NOT EXISTS ex_game_seasons (window_start VARCHAR PRIMARY KEY, rank_json VARCHAR NOT NULL, source_run_id VARCHAR NOT NULL)",
		"CREATE TABLE IF NOT EXISTS ex_game_processed_runs (run_id VARCHAR PRIMARY KEY, window_start VARCHAR NOT NULL)",
	} {
		if _, err := r.Client.Execute(ctx, ddl, nil); err != nil {
			return err
		}
	}
	_, err := r.Client.Execute(ctx, "INSERT INTO ex_game_levels SELECT ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM ex_game_levels WHERE id = ?)", []any{"quiz-1", "SimpleBase 的项目 KV 属于哪里？", "项目", "quiz-1"})
	return err
}
func apply(ctx context.Context, r *ex.Runtime, win ex.Window, run ex.Run) error {
	seen, err := list(ctx, r, "SELECT run_id FROM ex_game_processed_runs WHERE run_id = ?", []any{run.ID}, 1)
	if err != nil {
		return err
	}
	if len(seen) > 0 {
		return nil
	}
	scores, err := list(ctx, r, "SELECT player_id, max(score) AS score FROM ex_game_scores WHERE created_at >= ? AND created_at < ? GROUP BY player_id LIMIT 101", []any{win.Start, win.End}, 101)
	if err != nil {
		return err
	}
	if len(scores) > 100 {
		return errors.New("ranking input exceeds demo limit")
	}
	type Row struct {
		PlayerID string `json:"player_id"`
		Score    int64  `json:"score"`
	}
	input := struct {
		Rows []Row `json:"rows"`
	}{Rows: []Row{}}
	for _, item := range scores {
		input.Rows = append(input.Rows, Row{ex.Text(item["player_id"]), ex.Int(item["score"])})
	}
	var output struct {
		Ranks []Row `json:"ranks"`
	}
	if err := r.Compute(ctx, "ex_game_season", input, &output); err != nil {
		return err
	}
	rank, err := json.Marshal(output)
	if err != nil {
		return err
	}
	batch, err := r.Client.Batch(ctx, []gosdk.SQLStatement{
		{SQL: "INSERT INTO ex_game_seasons SELECT ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM ex_game_seasons WHERE window_start = ?)", Args: []any{win.Start, string(rank), run.ID, win.Start}},
		{SQL: "INSERT INTO ex_game_processed_runs (run_id, window_start) VALUES (?, ?)", Args: []any{run.ID, win.Start}},
	}, true)
	if err := ex.CheckBatch(batch, err); err != nil {
		return err
	}
	_, err = r.KV(ctx, "SET", "ex:game:rank", string(rank), "EX", "3600")
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
		r.StartWorker(ctx, "ex_game_rank_refresh", func(ctx context.Context, w ex.Window, run ex.Run) error { return apply(ctx, r, w, run) })
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { jsonResponse(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /api/levels", func(w http.ResponseWriter, req *http.Request) {
		data, err := list(req.Context(), r, "SELECT id, title FROM ex_game_levels LIMIT 20", nil, 20)
		if err != nil {
			jsonResponse(w, 500, map[string]string{"error": "Query failed"})
			return
		}
		jsonResponse(w, 200, data)
	})
	mux.HandleFunc("POST /api/sessions", func(w http.ResponseWriter, req *http.Request) {
		session := id()
		_, err := r.KV(req.Context(), "SET", "ex:game:session:"+session, "quiz-1", "EX", "300")
		if err != nil {
			jsonResponse(w, 500, map[string]string{"error": "Session unavailable"})
			return
		}
		jsonResponse(w, 201, map[string]string{"sessionId": session})
	})
	mux.HandleFunc("POST /api/scores", func(w http.ResponseWriter, req *http.Request) {
		req.Body = http.MaxBytesReader(w, req.Body, 8192)
		var input struct {
			SessionID string `json:"sessionId"`
			Answer    string `json:"answer"`
			PlayerID  string `json:"playerId"`
		}
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil || len(input.SessionID) != 32 || len(input.PlayerID) < 1 || len(input.PlayerID) > 32 {
			jsonResponse(w, 400, map[string]string{"error": "Invalid session or player"})
			return
		}
		session, err := r.KV(req.Context(), "GET", "ex:game:session:"+input.SessionID)
		if err != nil || session == nil {
			jsonResponse(w, 400, map[string]string{"error": "Session expired"})
			return
		}
		score := int64(0)
		if input.Answer == "项目" {
			score = 100
		}
		if !validScore(score) {
			jsonResponse(w, 400, map[string]string{"error": "Invalid score"})
			return
		}
		_, err = r.Client.Execute(req.Context(), "INSERT INTO ex_game_scores (id, session_id, player_id, score, created_at) VALUES (?, ?, ?, ?, ?)", []any{id(), input.SessionID, input.PlayerID, score, time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			jsonResponse(w, 409, map[string]string{"error": "Session already settled"})
			return
		}
		_, _ = r.KV(req.Context(), "DEL", "ex:game:session:"+input.SessionID)
		_, _ = r.KV(req.Context(), "ZADD", "ex:game:leaderboard", strconv.FormatInt(score, 10), input.PlayerID)
		jsonResponse(w, 201, map[string]any{"score": score})
	})
	mux.HandleFunc("GET /api/leaderboard", func(w http.ResponseWriter, req *http.Request) {
		data, err := list(req.Context(), r, "SELECT player_id, max(score) AS score FROM ex_game_scores GROUP BY player_id ORDER BY score DESC LIMIT 20", nil, 20)
		if err != nil {
			jsonResponse(w, 500, map[string]string{"error": "Query failed"})
			return
		}
		jsonResponse(w, 200, data)
	})
	port := os.Getenv("EXAMPLE_PORT")
	if port == "" {
		port = "8094"
	}
	log.Printf("Game demo listening on 127.0.0.1:%s", port)
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, ex.WithAssets(ex.LocalOnly(mux), "examples/mini-game-service")))
}
