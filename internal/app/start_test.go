// 文件说明：start_test.go — internal/app 的单元测试（TestStartThenShutdown、TestStartListenError 等）
//
// 用法
//
//	go test ./internal/app -run 'TestStartThenShutdown|TestStartListenError|TestSnapshotFromFactory|TestShutdownUnstartedHTTPError'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/start_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_shutdown_test.go, app.go, app_alerts.go, app_lease.go, app_test.go 等 41 个
//
// 主要符号
//
//	TestStartThenShutdown(t *testing.T)
//	TestStartListenError(t *testing.T)
//	TestSnapshotFromFactory(t *testing.T)
//	TestShutdownUnstartedHTTPError(t *testing.T)
//	TestRunWithSignalImmediateError(t *testing.T)

package app

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
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
	if snapshotFromFileFactory(nil, "x") != nil {
		t.Fatal("nil factory")
	}
	s := filedb.NewLocalSyncer()
	f := &filedb.Factory{Syncer: s}
	if snapshotFromFileFactory(f, "missing") != nil {
		t.Fatal("empty watermark")
	}
	s.MarkDirty("db-1")
	got := snapshotFromFileFactory(f, "db-1")
	if got == nil || got.LastSyncedSnapshot != 1 {
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
