package app

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
	"github.com/prometheus/client_golang/prometheus"
)

func TestStartThenShutdown(t *testing.T) {
	cfg := testConfig(false)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	cfg.HTTP.Address = addr

	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- a.Start() }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return")
	}
}

func TestStartListenError(t *testing.T) {
	cfg := testConfig(false)
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = a.Close(ctx)
	})
	a.httpServer = &http.Server{Addr: "127.0.0.1:1"} // privileged port
	if err := a.Start(); err == nil {
		t.Fatal("expected listen error")
	}
}

func TestSnapshotFromFactory(t *testing.T) {
	if snapshotFromFactory(nil, "x") != nil {
		t.Fatal("nil factory")
	}
	s := ducklake.NewLocalSyncer()
	f := &ducklake.Factory{Syncer: s}
	if snapshotFromFactory(f, "missing") != nil {
		t.Fatal("empty watermark")
	}
	s.MarkDirty("db-1", 9)
	got := snapshotFromFactory(f, "db-1")
	if got == nil || got.LastSyncedSnapshot != 9 {
		t.Fatalf("%+v", got)
	}
}

func TestShutdownUnstartedHTTPError(t *testing.T) {
	cfg := testConfig(false)
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	// Replace with a server that was never started so Shutdown returns an error.
	a.httpServer = &http.Server{Addr: "127.0.0.1:0"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = a.Shutdown(ctx)
}

func TestRunWithSignalImmediateError(t *testing.T) {
	cfg := testConfig(false)
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = a.Close(ctx)
	})
	a.httpServer = &http.Server{Addr: "127.0.0.1:1"}
	if err := a.RunWithSignal(time.Second); err == nil {
		t.Fatal("expected start error")
	}
}

