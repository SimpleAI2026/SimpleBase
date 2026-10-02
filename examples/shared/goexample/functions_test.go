package goexample

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gofunction"
	_ "github.com/linkxzhou/SimpleBase/gofunction/packages"
)

func TestCloudFunctionSources(t *testing.T) {
	samples := []struct {
		path  string
		input string
	}{
		{"shop-service/functions/ex_shop_metrics.go", `{"rows":[{"qty":2,"amount_minor":4900}]}`},
		{"mini-game-service/functions/ex_game_season.go", `{"rows":[{"player_id":"a","score":100}]}`},
		{"booking-service/functions/ex_booking_expiry.go", `{"cutoff":"2026-09-29T00:00:00Z","rows":[]}`},
		{"community-service/functions/ex_community_trending.go", `{"rows":[]}`},
		{"ticket-service/functions/ex_ticket_sla.go", `{"now":"2026-09-29T00:00:00Z","rows":[]}`},
	}
	// 发现由 ./build.sh init-example 生成、经 case.json 声明函数与样例的案例。
	discovered, err := filepath.Glob(filepath.Join("..", "..", "*", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, s := range samples {
		known[s.path] = true
	}
	for _, meta := range discovered {
		caseDir := filepath.Base(filepath.Dir(meta))
		if strings.HasPrefix(caseDir, "_") {
			continue
		}
		raw, err := os.ReadFile(meta)
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Fn     string          `json:"fn"`
			Sample json.RawMessage `json:"sample"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("%s: %v", meta, err)
		}
		if cfg.Fn == "" {
			continue
		}
		path := filepath.Join(caseDir, "functions", cfg.Fn+".go")
		if known[path] {
			continue
		}
		input := "{}"
		if len(cfg.Sample) > 0 {
			input = string(cfg.Sample)
		}
		samples = append(samples, struct {
			path  string
			input string
		}{path, input})
	}
	for _, s := range samples {
		t.Run(s.path, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("..", "..", s.path))
			if err != nil {
				t.Fatal(err)
			}
			for _, call := range []struct {
				name string
				body string
			}{{"Tick", `{}`}, {"Compute", s.input}} {
				result, err := gofunction.RunJSON(context.Background(), "example-test", filepath.Base(s.path), string(source), call.name, json.RawMessage(call.body))
				if err != nil {
					t.Fatalf("%s: %v", call.name, err)
				}
				if !json.Valid(result) {
					t.Fatalf("%s produced invalid JSON: %s", call.name, result)
				}
			}
		})
	}
}
