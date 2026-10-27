// 文件说明：harness.go — api-db-perf-validation-plan §2.2/§2.4 的压测 harness
//
// 用法
//
//	type benchEnv（包内使用）
//	type byteReader（包内使用）
//	同包调用：benchEnv → cases.go, helpers.go 等；Close() → cases.go, runner.go；Do() → cases.go, helpers.go
//
// 调用架构
//
//	[上游] cmd/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/perfbench/harness.go
//	         │ 依赖
//	         ▼
//	[下游] internal/app, internal/auth, internal/config
//	[同包] cases.go, helpers.go, loadgen.go, runner.go, stage_report.go
//	[关键引用] benchEnv ← cases.go, helpers.go, loadgen.go
//
// 主要符号
//
//	type benchEnv struct {…}
//	const benchProjectName
//	newEnv(ctx context.Context, cacheRoot string) (*benchEnv, error)
//	(e *benchEnv) bootstrapProject(ctx context.Context) error
//	(e *benchEnv) login(username, password string) (string, error)
//	(e *benchEnv) createProject() (string, error)
//	... 另有 14 个顶层符号

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
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

// benchEnv 是一次压测会话的全部环境。
type benchEnv struct {
	App      *app.App
	Server   *httptest.Server
	Registry *prometheus.Registry
	Client   *http.Client

	// AccessToken 是超管登录后拿到的 access token（用于建项目 / 签发 Key）。
	AccessToken string
	APIKey      string // 为压测项目签发的 Key
	ProjectID   string // 压测项目（由超管通过 API 创建）
	// 用户压测库：Setup 内通过 HTTP 建库后回填。
	UserDBID string

	baseCtx context.Context
}

// benchProjectName 是压测项目展示名（每次压测新建，包含随机后缀）。
const benchProjectName = "perfbench"

// newEnv 装配真实 app（本地存储）+ httptest.Server，并以超管身份
// 通过公开 API 建好项目与 API Key（不依赖任何开发模式种子）。
func newEnv(ctx context.Context, cacheRoot string) (*benchEnv, error) {
	cfg := devBenchConfig(cacheRoot)
	reg := prometheus.NewRegistry()
	a, err := app.NewWithRegistry(ctx, cfg, reg)
	if err != nil {
		return nil, fmt.Errorf("perfbench: assemble app: %w", err)
	}
	ts := httptest.NewServer(a.Handler())
	env := &benchEnv{
		App:      a,
		Server:   ts,
		Registry: reg,
		Client:   &http.Client{Timeout: 60 * time.Second},
		baseCtx:  ctx,
	}
	if err := env.bootstrapProject(ctx); err != nil {
		env.Close()
		return nil, err
	}
	return env, nil
}

// bootstrapProject 登录超管 → 建项目 → 签发全权限 Key。
func (e *benchEnv) bootstrapProject(ctx context.Context) error {
	token, err := e.login(auth.BootstrapUsername, auth.BootstrapPassword)
	if err != nil {
		return fmt.Errorf("perfbench: login: %w", err)
	}
	e.AccessToken = token

	projectID, err := e.createProject()
	if err != nil {
		return err
	}
	e.ProjectID = projectID

	key, err := e.issueAPIKey()
	if err != nil {
		return err
	}
	e.APIKey = key
	return nil
}

func (e *benchEnv) login(username, password string) (string, error) {
	code, body, err := e.doWithToken("", "POST", "/v1/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("http %d: %s", code, body)
	}
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("empty access_token")
	}
	return resp.AccessToken, nil
}

func (e *benchEnv) createProject() (string, error) {
	name := fmt.Sprintf("%s-%d", benchProjectName, time.Now().UnixNano())
	code, body, err := e.doWithToken(e.AccessToken, "POST", "/v1/projects", map[string]string{"name": name})
	if err != nil {
		return "", err
	}
	if code != http.StatusOK && code != http.StatusCreated {
		return "", fmt.Errorf("create project: http %d: %s", code, body)
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if resp.ID == "" {
		return "", fmt.Errorf("create project: empty id")
	}
	return resp.ID, nil
}

// issueAPIKey 为压测项目签发全权限 Key（登录态 + ProjectAdmin 权限）。
func (e *benchEnv) issueAPIKey() (string, error) {
	path := "/v1/projects/" + e.ProjectID + "/api-keys"
	code, body, err := e.doWithToken(e.AccessToken, "POST", path, map[string]any{
		"permissions": []string{
			"database:read", "database:write", "database:admin",
			"llm:invoke", "project:admin",
		},
	})
	if err != nil {
		return "", err
	}
	if code != http.StatusOK && code != http.StatusCreated {
		return "", fmt.Errorf("issue api key: http %d: %s", code, body)
	}
	var resp struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if resp.Secret == "" {
		return "", fmt.Errorf("issue api key: empty secret")
	}
	return resp.Secret, nil
}

// doWithToken 发一次请求；token 为空时用 env.APIKey。
func (e *benchEnv) doWithToken(token, method, path string, body any) (int, []byte, error) {
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
	if token == "" {
		token = e.APIKey
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.Client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	return resp.StatusCode, bs, err
}

// devBenchConfig 构造压测专用配置：本地存储、临时目录、分段计时开启。
func devBenchConfig(cacheRoot string) config.Config {
	// 借用 config 包默认值再覆盖（不读取仓库 config.yaml，保证可复现）。
	cfg := defaultBenchConfig()
	cfg.Storage = config.StorageConfig{
		Database: config.StorageModeLocal,
		Files:    config.StorageModeLocal,
		DataDir:  filepath.Join(cacheRoot, "data"),
	}
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
			Engine:            config.EngineDuckDB,
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
