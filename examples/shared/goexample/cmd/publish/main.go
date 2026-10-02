package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	ex "github.com/linkxzhou/SimpleBase/examples/shared/goexample"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("usage: go run ./examples/shared/goexample/cmd/publish <mini-game-service|community-service> <case-directory> [--upload]")
	}
	name := os.Args[1]
	config := map[string]struct{ Function, Job, Cron string }{
		"mini-game-service": {"ex_game_season", "ex_game_rank_refresh", "0 * * * *"},
		"community-service": {"ex_community_trending", "ex_community_hot", "0 * * * *"},
	}[name]
	if config.Function == "" {
		log.Fatal("unknown example")
	}
	rt, err := ex.New()
	if err != nil {
		log.Fatal(err)
	}
	dir, err := filepath.Abs(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	upload := len(os.Args) > 3 && os.Args[3] == "--upload"
	if err := rt.Publish(context.Background(), dir, config.Function, config.Job, config.Cron, upload); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s function and cron configured; private upload=%v\n", name, upload)
}
