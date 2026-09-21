package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/app"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "simplebased: %v\n", err)
		os.Exit(1)
	}

	a, err := app.New(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "simplebased: %v\n", err)
		os.Exit(1)
	}

	timeout := cfg.HTTP.ShutdownTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if err := a.RunWithSignal(timeout); err != nil {
		fmt.Fprintf(os.Stderr, "simplebased: %v\n", err)
		os.Exit(1)
	}
}
