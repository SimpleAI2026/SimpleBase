// Package perfbench 实现 api-db-perf-validation-plan §2.2/§2.4 的压测 harness。
//
// 定位：独立测量工具包，不进生产二进制（仅 cmd/perfbench 引用）。
// 装配真实链路：真实 Router + catalog + registry + ducklake factory（DevMode
// 本地盘，DATA_PATH 落临时目录），用 httptest.Server 提供真实 HTTP 语义
// （含 echo 中间件、认证、分段计时），排除外网噪声。
//
// 认证：DevMode 种子 Key（systemdb.DevRawAPIKey），绑定 dev-shop 项目。
package perfbench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/app"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/prometheus/client_golang/prometheus"
)

// benchEnv 是一次压测会话的全部环境。
type benchEnv struct {
	App      *app.App
	Server   *httptest.Server
	Registry *prometheus.Registry
	Client   *http.Client

	APIKey    string // DevMode 种子明文 Key
	ProjectID string // dev-shop（种子项目，持有 kv + 用户库）
	// 用户压测库：env.Setup 内通过 HTTP 建库后回填。
	UserDBID string

	baseCtx context.Context
}

// newEnv 装配真实 app（DevMode 本地盘）+ httptest.Server。
func newEnv(ctx context.Context, cacheRoot string) (*benchEnv, error) {
	cfg := devBenchConfig(cacheRoot)
	reg := prometheus.NewRegistry()
	a, err := app.NewWithRegistry(ctx, cfg, reg)
	if err != nil {
		return nil, fmt.Errorf("perfbench: assemble app: %w", err)
	}
	ts := httptest.NewServer(a.Handler())
	env := &benchEnv{
		App:       a,
		Server:    ts,
		Registry:  reg,
		Client:    &http.Client{Timeout: 60 * time.Second},
		APIKey:    systemdb.DevRawAPIKey,
		ProjectID: catalog.DevProjectID,
		baseCtx:   ctx,
	}
	return env, nil
}

// devBenchConfig 构造压测专用配置：DevMode、临时缓存目录、分段计时开启。
func devBenchConfig(cacheRoot string) config.Config {
	cfg := config.Config{}
	// 借用 config 包默认值再覆盖（不读取仓库 config.yaml，保证可复现）。
	cfg = defaultBenchConfig()
	cfg.DevMode = true
	cfg.Instance.Writable = true
	cfg.Database.CacheDir = cacheRoot
	cfg.Observability.PerfStageTiming = true
	cfg.Observability.LogLevel = "error" // 压测期压低日志噪声
	return cfg
}

// defaultBenchConfig 复刻 config.Load 的代码默认值（不读 YAML/env），
// 仅调整压测相关字段；与 internal/config.defaults() 保持语义一致。
func defaultBenchConfig() config.Config {
	return config.Config{
		HTTP: config.HTTPConfig{
			Address:         ":0",
			ReadTimeout:     15 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     60 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Instance: config.InstanceConfig{
			ID:       "perfbench-instance",
			Writable: true,
			Lease: config.InstanceLeaseConfig{
				Enabled: true,
				TTL:     30 * time.Second,
				Grace:   10 * time.Second,
				OnLost:  "release_db",
			},
		},
		Database: config.DatabaseConfig{
			Engine:            config.EngineDuckLake,
			CacheMaxBytes:     config.DefaultCacheMaxBytes,
			CacheMaxDatabases: config.DefaultCacheMaxDatabases,
			IdleTimeout:       5 * time.Minute,
			MaxOpen:           8,
			DuckLake: config.DuckLakeConfig{
				MemoryLimit:          "512MB",
				Threads:              2,
				DataInliningRowLimit: 100,
				ParquetCompression:   "zstd",
				TargetFileSize:       "64MB",
				CatalogSync: config.CatalogSyncConfig{
					Mode:         "debounce",
					Debounce:     200 * time.Millisecond,
					KeepVersions: 10,
				},
				Maintenance: config.DuckLakeMaintenanceConfig{
					CheckpointInterval:     time.Hour,
					ExpireOlderThan:        7 * 24 * time.Hour,
					DeleteOlderThan:        7 * 24 * time.Hour,
					RewriteDeleteThreshold: 0.95,
				},
			},
		},
		Auth: config.AuthConfig{APIKeyHashSecret: "perfbench-secret"},
		Limits: config.LimitsConfig{
			MaxRequestBytes:      1 << 20,
			MaxQueryRows:         1000,
			QueryTimeout:         30 * time.Second,
			MaxConcurrentQueries: 64,
			MaxBatchStatements:   100,
			MaxSQLBytes:          65536,
		},
		Observability: config.ObservabilityConfig{
			LogLevel:    "error",
			LogFormat:   "json",
			LogOutput:   "stderr",
			MetricsPath: "/metrics",
		},
		SystemDatabase: config.SystemDatabaseConfig{
			Name:                 "simplebase-system",
			MetricsFlushInterval: 2 * time.Second,
			LogFlushInterval:     2 * time.Second,
			LogKeepDays:          14,
		},
	}
}

// Close 释放环境（HTTP server 先关，再走 app.Shutdown 收敛后台任务）。
func (e *benchEnv) Close() {
	if e.Server != nil {
		e.Server.Close()
	}
	if e.App != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = e.App.Shutdown(ctx)
	}
}

// Do 发一次带认证的 JSON 请求，返回状态码与 body。
func (e *benchEnv) Do(method, path string, body any) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		bs, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = &byteReader{data: bs}
	}
	req, err := http.NewRequest(method, e.Server.URL+path, rd)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.Client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	return resp.StatusCode, bs, err
}

// DoRaw 发一次带认证的请求（body 由调用方构造）。
func (e *benchEnv) DoRaw(method, path string, body []byte) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		rd = &byteReader{data: body}
	}
	req, err := http.NewRequest(method, e.Server.URL+path, rd)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.Client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	return resp.StatusCode, bs, err
}

// byteReader 是可复用的 bytes.Reader 包装（避免每次分配新类型）。
type byteReader struct {
	data []byte
	off  int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

// createDatabase 通过 HTTP 建用户库并回填 env.UserDBID。同名已存在时
// 从列表接口解析现有库 ID（幂等；压测环境隔离，重复名仅来自本进程重跑）。
func (e *benchEnv) createDatabase(name string) (string, error) {
	code, body, err := e.Do("POST", "/v1/projects/"+e.ProjectID+"/databases", map[string]string{"name": name})
	if err != nil {
		return "", err
	}
	if code == http.StatusOK || code == http.StatusCreated {
		var resp struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return "", err
		}
		if resp.ID == "" {
			return "", fmt.Errorf("perfbench: create database %s: empty id", name)
		}
		return resp.ID, nil
	}
	if code != http.StatusConflict {
		return "", fmt.Errorf("perfbench: create database %s: http %d: %s", name, code, body)
	}
	// 409 database_already_exists：按名查找现有库。
	listCode, listBody, err := e.Do("GET", "/v1/projects/"+e.ProjectID+"/databases?limit=100", nil)
	if err != nil {
		return "", err
	}
	if listCode != http.StatusOK {
		return "", fmt.Errorf("perfbench: list databases: http %d: %s", listCode, listBody)
	}
	var list struct {
		Databases []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"databases"`
	}
	if err := json.Unmarshal(listBody, &list); err != nil {
		return "", err
	}
	for _, db := range list.Databases {
		if db.Name == name {
			return db.ID, nil
		}
	}
	return "", fmt.Errorf("perfbench: database %s conflict but not found in list", name)
}

// setupUserDB 建压测用户库并建基准表（幂等）。
func (e *benchEnv) setupUserDB(name string) error {
	id, err := e.createDatabase(name)
	if err != nil {
		return err
	}
	e.UserDBID = id
	code, body, err := e.Do("POST", e.sqlPath("execute"), map[string]any{
		"sql": "CREATE TABLE IF NOT EXISTS bench_rows (id VARCHAR NOT NULL, payload VARCHAR NOT NULL, seq BIGINT NOT NULL)",
	})
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("perfbench: setup table: http %d: %s", code, body)
	}
	return nil
}

func (e *benchEnv) sqlPath(action string) string {
	return fmt.Sprintf("/v1/projects/%s/databases/%s/%s", e.ProjectID, e.UserDBID, action)
}

func (e *benchEnv) dataPath(parts ...string) string {
	p := fmt.Sprintf("/v1/projects/%s/databases/%s/data", e.ProjectID, e.UserDBID)
	for _, s := range parts {
		p += "/" + s
	}
	return p
}

func (e *benchEnv) kvPath() string {
	return "/v1/projects/" + e.ProjectID + "/kv"
}

// ensureOutputDir 创建原始数据落盘目录并返回路径。
func ensureOutputDir(root string) (string, error) {
	if root == "" {
		root = "output/perf"
	}
	dir := filepath.Join(root, time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
