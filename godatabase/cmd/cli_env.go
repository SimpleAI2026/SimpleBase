// 文件说明：cli_env.go — godatabase/cmd 的内部实现（env、dataRoots 等）
//
// 用法
//
//	同包调用：env → cli_output.go, godb-cli_test.go；dataRoots() → godb-cli.go；fileDefaultRoot() → godb-cli.go
//
// 调用架构
//
//	[上游] 无（可执行入口）
//	[本文件] godatabase/cmd/cli_env.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] cli_output.go, godb-cli.go, godb-cli_test.go
//	[关键引用] env ← cli_output.go, godb-cli_test.go；dataRoots ← godb-cli.go；fileDefaultRoot ← godb-cli.go
//
// 主要符号
//
//	type env struct{…}
//	dataRoots() []string
//	fileDefaultRoot() string
//	parseEnv(jsonOut bool) env
//	(e env) spec(projectID string) (godatabase.Spec, error)
//	(e env) engine() string
//	(e env) openReg(ctx context.Context, projectID string, rw bool) (*registry.Registry, *registry.Lease, func(), ...
//	dbFilePath(e env, h *registry.Handle) string

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
)

// env 返回数据根目录与缓存目录（环境变量 GODB_DATA_ROOT / GODB_CACHE_DIR，
// 默认 ./godb-data 与 ./godb-cache）。
type env struct {
	dataRoot string
	cacheDir string
	jsonOut  bool
}

func dataRoots() []string {
	return []string{
		os.Getenv("GODB_DATA_ROOT"),
		filepath.Join(".", "godb-data"),
	}
}

func fileDefaultRoot() string { return filepath.Join(".", "godb-data") }

func parseEnv(jsonOut bool) env {
	root := os.Getenv("GODB_DATA_ROOT")
	if root == "" {
		root = filepath.Join(".", "godb-data")
	}
	cache := os.Getenv("GODB_CACHE_DIR")
	if cache == "" {
		cache = filepath.Join(".", "godb-cache")
	}
	return env{dataRoot: root, cacheDir: cache, jsonOut: jsonOut}
}

func (e env) spec(projectID string) (godatabase.Spec, error) {
	if err := blob.ValidateProjectID(projectID); err != nil {
		return godatabase.Spec{}, fmt.Errorf("invalid project id: %w", errors.Unwrap(err))
	}
	path, err := godatabase.PathForProject(projectID, e.engine())
	if err != nil {
		return godatabase.Spec{}, err
	}
	return godatabase.Spec{
		ID:           "db-" + projectID,
		ProjectID:    projectID,
		Kind:         godatabase.KindUser,
		Status:       godatabase.StatusReady,
		DatabasePath: path,
	}, nil
}

// engine 由环境变量 GODB_ENGINE 指定；默认 duckdb。
func (e env) engine() string {
	if p := os.Getenv("GODB_ENGINE"); p != "" {
		return p
	}
	return godatabase.EngineDuckDB
}

// openReg 是本地模式 Factory+Registry 的唯一装配核心：Acquire 一个租约并
// 返回清理函数（Release + Shutdown）。用完必须调用 cleanup。
// Acquire 失败时用 Background context 关停注册表（请求 ctx 可能已被取消，
// 失败清理不依赖它）。
func (e env) openReg(ctx context.Context, projectID string, rw bool) (*registry.Registry, *registry.Lease, func(), error) {
	spec, err := e.spec(projectID)
	if err != nil {
		return nil, nil, nil, err
	}
	factory := &filedb.Factory{
		DataRoot: e.dataRoot,
		CacheDir: e.cacheDir,
		Remote:   false,
	}
	reg := registry.New(factory, nil, registry.Options{Writable: true}, nil, nil)
	mode := godatabase.ReadOnly
	if rw {
		mode = godatabase.ReadWrite
	}
	lease, err := reg.Acquire(ctx, spec, mode)
	if err != nil {
		_ = reg.Shutdown(context.Background())
		return nil, nil, nil, err
	}
	cleanup := func() {
		lease.Release()
		_ = reg.Shutdown(context.Background())
	}
	return reg, lease, cleanup, nil
}

// dbFilePath 返回项目主文件的绝对路径（DataRoot + Spec.DatabasePath）。
func dbFilePath(e env, h *registry.Handle) string {
	return filepath.Join(e.dataRoot, h.Database.DatabasePath)
}
