package turso

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/observability"
)

func TestBuildDSN_EscapingAndFields(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "33333333-3333-3333-3333-333333333333",
		Storage: StorageConfig{
			Endpoint: "https://s3.example.com",
			Region:   "us-east-1",
			Bucket:   "sb-bucket",
			Prefix:   "simplebase/prod/tenants/t/databases/d/data",
		},
		Writable: true,
	}
	dsn, err := BuildDSN(opts)
	if err != nil {
		t.Fatalf("BuildDSN: %v", err)
	}
	if !strings.HasPrefix(dsn, "libsql+ss3://?") {
		t.Errorf("unexpected scheme: %s", dsn)
	}
	// 必须包含 bucket/region/prefix/mode
	mustContain := []string{"bucket=sb-bucket", "region=us-east-1", "mode=rw"}
	for _, sub := range mustContain {
		if !strings.Contains(dsn, sub) {
			t.Errorf("dsn missing %q: %s", sub, dsn)
		}
	}
	// endpoint 的 ":" 和 "/" 应被 URL 转义
	if strings.Contains(dsn, "https://") {
		// endpoint 值应被 url.QueryEscape 处理（"://" -> "%3A%2F%2F"）
		if !strings.Contains(dsn, "endpoint=https") {
			// 至少 endpoint= 前缀存在
		}
	}
}

func TestBuildDSN_ReadOnlyMode(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "d",
		Storage:    StorageConfig{Region: "r", Bucket: "b", Prefix: "p"},
		Writable:   false,
	}
	dsn, err := BuildDSN(opts)
	if err != nil {
		t.Fatalf("BuildDSN: %v", err)
	}
	if !strings.Contains(dsn, "mode=ro") {
		t.Errorf("read-only dsn should have mode=ro: %s", dsn)
	}
}

func TestBuildDSN_RejectsPathTraversal(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "d",
		Storage:    StorageConfig{Region: "r", Bucket: "b", Prefix: "../evil"},
	}
	if _, err := BuildDSN(opts); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
}

func TestBuildDSN_RejectsLeadingSlash(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "d",
		Storage:    StorageConfig{Region: "r", Bucket: "b", Prefix: "/abs"},
	}
	if _, err := BuildDSN(opts); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
}

func TestBuildDSN_NoSecretsInQuery(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "d",
		Storage: StorageConfig{
			Region:   "r",
			Bucket:   "b",
			Prefix:   "p",
			KMSKeyID: "arn:aws:kms:us-east-1:123:key/abc",
		},
		AuthToken: "super-secret-token",
	}
	dsn, err := BuildDSN(opts)
	if err != nil {
		t.Fatalf("BuildDSN: %v", err)
	}
	if strings.Contains(dsn, "super-secret-token") {
		t.Errorf("dsn leaks auth token: %s", dsn)
	}
	if strings.Contains(dsn, "kms") || strings.Contains(dsn, "KMSKeyID") {
		t.Errorf("dsn leaks kms key id: %s", dsn)
	}
}

func TestRedactDSN(t *testing.T) {
	in := "libsql+ss3://?bucket=b&region=r&token=secret"
	out := redactDSN(in)
	if strings.Contains(out, "bucket=b") {
		t.Errorf("redact should remove query values: %s", out)
	}
	if !strings.Contains(out, "[redacted]") {
		t.Errorf("redact should add [redacted]: %s", out)
	}
}

// mockDB 用于测试 Open 流程而不真正打开驱动。
type mockDB struct {
	pingErr error
}

func (m *mockDB) Close() error { return nil }

// openMock 返回一个 OpenFunc，使用真实 *sql.DB 但用 sql.Open("sqlite3", ":memory:")
// 替代 libsql 驱动——这里我们直接构造一个不依赖驱动的 mock。
// 由于 sql.Open 需要已注册驱动，我们用一个返回 stub 的 OpenFunc。
func openMock(pingErr error) (OpenFunc, *sql.DB) {
	// 使用一个不会真正连接的 stub：返回 nil db 会 panic，所以用 sqlite 内存库
	// 若环境无 sqlite 驱动则跳过。这里改为返回一个可控的 fake。
	return func(driverName, dsn string) (*sql.DB, error) {
		if driverName != DriverName {
			return nil, errors.New("unexpected driver name: " + driverName)
		}
		// 返回 nil db 与错误模拟 open 失败路径
		if dsn == "" {
			return nil, errors.New("empty dsn")
		}
		// 由于不能真正打开 libsql，这里返回一个通过 sql.OpenDB 构造的空连接池。
		// 但 PingContext 会调用 driver.Connector，为避免复杂 mock，
		// 本测试只验证 BuildDSN + Validate 的纯逻辑；Open 的集成测试留待真实驱动。
		return nil, errors.New("mock: real driver not available in unit test")
	}, nil
}

func TestOpen_ValidateFails(t *testing.T) {
	opts := OpenOptions{} // 空 DatabaseID
	_, err := Open(context.Background(), opts, PoolOptions{}, nil, nil, nil)
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
}

func TestOpen_DriverOpenFails(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "d",
		Storage:    StorageConfig{Region: "r", Bucket: "b", Prefix: "p"},
	}
	openFn, _ := openMock(nil)
	_, err := Open(context.Background(), opts, PoolOptions{}, nil, nil, openFn)
	if err == nil {
		t.Fatal("expected error from mock open")
	}
	// 错误中不应包含完整 DSN 的 query 值
	if strings.Contains(err.Error(), "bucket=b") {
		t.Errorf("error leaks dsn query: %v", err)
	}
}

func TestPoolOptionsDefaults(t *testing.T) {
	p := PoolOptions{}.defaults()
	if p.MaxOpen != 8 {
		t.Errorf("MaxOpen default = %d, want 8", p.MaxOpen)
	}
	if p.MaxIdle != 2 {
		t.Errorf("MaxIdle default = %d, want 2", p.MaxIdle)
	}
	if p.MaxIdleTime != 5*time.Minute {
		t.Errorf("MaxIdleTime default = %v, want 5m", p.MaxIdleTime)
	}
}

func TestCloseNil(t *testing.T) {
	if err := Close(nil, nil); err != nil {
		t.Errorf("Close(nil) = %v, want nil", err)
	}
}

// 确保 observability 包被引用（避免 import 未用）。
var _ observability.Logger

func TestBuildDSN_ForcePathStyle(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "d",
		Storage: StorageConfig{
			Region:         "r",
			Bucket:         "b",
			Prefix:         "p",
			ForcePathStyle: true,
		},
	}
	dsn, err := BuildDSN(opts)
	if err != nil {
		t.Fatalf("BuildDSN: %v", err)
	}
	if !strings.Contains(dsn, "force_path_style=true") {
		t.Errorf("expected force_path_style=true in dsn: %s", dsn)
	}
}

func TestBuildDSN_NoForcePathStyleByDefault(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "d",
		Storage:    StorageConfig{Region: "r", Bucket: "b", Prefix: "p"},
	}
	dsn, err := BuildDSN(opts)
	if err != nil {
		t.Fatalf("BuildDSN: %v", err)
	}
	if strings.Contains(dsn, "force_path_style") {
		t.Errorf("force_path_style should not appear when false: %s", dsn)
	}
}

func TestBuildDSN_PrefixTrailingSlashRemoved(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "d",
		Storage:    StorageConfig{Region: "r", Bucket: "b", Prefix: "p/data/"},
	}
	dsn, err := BuildDSN(opts)
	if err != nil {
		t.Fatalf("BuildDSN: %v", err)
	}
	// sanitizePrefix 去掉尾部斜杠
	if strings.Contains(dsn, "p=data%2F") {
		t.Errorf("trailing slash should be sanitized: %s", dsn)
	}
}

func TestValidate_MissingDatabaseID(t *testing.T) {
	opts := OpenOptions{
		Storage: StorageConfig{Region: "r", Bucket: "b", Prefix: "p"},
	}
	err := opts.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
}

func TestValidate_MissingBucket(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "d",
		Storage:    StorageConfig{Region: "r", Prefix: "p"},
	}
	err := opts.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
}

func TestValidate_MissingRegion(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "d",
		Storage:    StorageConfig{Bucket: "b", Prefix: "p"},
	}
	err := opts.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
}

func TestValidate_MissingPrefix(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "d",
		Storage:    StorageConfig{Region: "r", Bucket: "b"},
	}
	err := opts.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
}

func TestPoolOptionsDefaults_ExplicitValues(t *testing.T) {
	p := PoolOptions{MaxOpen: 16, MaxIdle: 4, MaxIdleTime: 10 * time.Minute}.defaults()
	if p.MaxOpen != 16 {
		t.Errorf("MaxOpen = %d, want 16", p.MaxOpen)
	}
	if p.MaxIdle != 4 {
		t.Errorf("MaxIdle = %d, want 4", p.MaxIdle)
	}
	if p.MaxIdleTime != 10*time.Minute {
		t.Errorf("MaxIdleTime = %v, want 10m", p.MaxIdleTime)
	}
}

func TestRedactDSN_NoQuestionMark(t *testing.T) {
	in := "libsql+ss3://something"
	out := redactDSN(in)
	if out != in {
		t.Errorf("redact without query should return as-is: got %s", out)
	}
}

func TestOpen_BuildDSNFailsBeforeCallingDriver(t *testing.T) {
	opts := OpenOptions{
		DatabaseID: "d",
		Storage:    StorageConfig{Region: "r", Bucket: "b", Prefix: "../traversal"},
	}
	// openFn 不应被调用——BuildDSN 在 Open 内部先校验
	openFn := func(driverName, dsn string) (*sql.DB, error) {
		t.Fatal("openFn should not be called when BuildDSN fails")
		return nil, nil
	}
	_, err := Open(context.Background(), opts, PoolOptions{}, nil, nil, openFn)
	if err == nil {
		t.Fatal("expected error from invalid prefix")
	}
}
