package app

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/prometheus/client_golang/prometheus"
)

type clientBlob struct {
	fakeStore
	blob objectstore.BlobStore
}

func (c clientBlob) Head(ctx context.Context, key string) (objectstore.ObjectInfo, error) {
	return c.blob.Head(ctx, key)
}
func (c clientBlob) PutBytes(ctx context.Context, key string, data []byte, ct string) error {
	return c.blob.PutBytes(ctx, key, data, ct)
}
func (c clientBlob) GetBytes(ctx context.Context, key string) ([]byte, objectstore.ObjectInfo, error) {
	return c.blob.GetBytes(ctx, key)
}
func (c clientBlob) DownloadFile(ctx context.Context, key, dest string) (objectstore.ObjectInfo, error) {
	return c.blob.DownloadFile(ctx, key, dest)
}
func (c clientBlob) Delete(ctx context.Context, key string) error { return c.blob.Delete(ctx, key) }

func TestNewUserFactoryWithBlobStore(t *testing.T) {
	a := &App{cfg: testConfig(true), logger: nil, metrics: nil}
	a.cfg.DevMode = false
	a.cfg.S3.Bucket = "b"
	a.objectStore = clientBlob{blob: objectstore.NewMemoryBlobStore()}
	f := a.newUserDatabaseFactory(t.TempDir()).(*ducklake.Factory)
	if f.Blobs == nil || a.catalogSyncer == nil {
		t.Fatal("expected catalog syncer")
	}
	if cs, ok := a.catalogSyncer.(*ducklake.CatalogSyncer); ok {
		if err := (syncerCloser{cs: cs}).Close(); err != nil {
			t.Fatal(err)
		}
	}
	sys := a.newSystemDatabaseFactory()
	if sys.Syncer == nil {
		t.Fatal("system syncer")
	}
}

func TestAssembleDepsCreatesS3ClientsThenFailsBootstrap(t *testing.T) {
	cfg := testConfig(true)
	cfg.S3 = config.S3Config{
		Endpoint: "http://127.0.0.1:1", Region: "us-east-1", Bucket: "b", Prefix: "simplebase",
		AccessKey: "ak", SecretKey: "sk", ForcePathStyle: true,
	}
	cfg.Database.CacheDir = t.TempDir()
	_, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err == nil {
		t.Fatal("expected bootstrap/open failure without reachable S3")
	}
}

func TestDevModeLLMEnabledAndCronRunner(t *testing.T) {
	cfg := testConfig(true)
	cfg.DevMode = true
	cfg.S3 = config.S3Config{}
	cfg.Database.CacheDir = t.TempDir()
	cfg.LLM.Enabled = true
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	})
	if a.llmSvc == nil {
		t.Fatal("llm service")
	}

	r := &systemDBRunner{}
	if _, err := r.RunFunction(context.Background(), "p", "f.go", "F", nil); err == nil {
		t.Fatal("nil store")
	}
	r.store = a.systemStore
	_, err = r.RunFunction(context.Background(), "p", "missing.go", "Hello", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("missing function")
	}

	src := "package main\nfunc Hello(m map[string]interface{}) string { return \"ok\" }\n"
	g, err := a.systemStore.CreateGoFunction(context.Background(), systemdb.GoFunction{
		ProjectID: uuid.NewString(),
		Name:      "hello.go",
		Source:    src,
		Exports:   []string{"Hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.RunFunction(context.Background(), g.ProjectID, "hello.go", "Hello", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `"ok"` {
		t.Fatalf("out=%s", out)
	}

	// empty system name uses default
	cfg2 := testConfig(true)
	cfg2.DevMode = true
	cfg2.S3 = config.S3Config{}
	cfg2.Database.CacheDir = t.TempDir()
	cfg2.SystemDatabase.Name = ""
	a2, err := NewWithRegistry(context.Background(), cfg2, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a2.Shutdown(ctx)
	})
}

func TestNewWrapperAndShutdownTimeouts(t *testing.T) {
	cfg := testConfig(false)
	func() {
		defer func() { _ = recover() }()
		a, err := New(context.Background(), cfg)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	}()

	cfg = testConfig(true)
	cfg.DevMode = true
	cfg.S3 = config.S3Config{}
	cfg.Database.CacheDir = t.TempDir()
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	_ = a.Shutdown(ctx)
}

func TestCloseEmptyApp(t *testing.T) {
	a := &App{}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestReadyPingFailAndCronOtherError(t *testing.T) {
	h := &healthService{app: &App{
		cfg:         config.Config{Instance: config.InstanceConfig{Writable: true}},
		systemStore: &systemdb.Store{},
	}}
	if err := h.Ready(context.Background()); err == nil {
		t.Fatal("ping fail")
	}
	r := &systemDBRunner{store: &systemdb.Store{}}
	if _, err := r.RunFunction(context.Background(), "p", "f.go", "F", nil); err == nil {
		t.Fatal("unavailable store")
	}
}

func TestAssembleDepsDevModeMkdirFail(t *testing.T) {
	cfg := testConfig(true)
	cfg.DevMode = true
	cfg.S3 = config.S3Config{}
	dir := t.TempDir()
	file := dir + "/notdir"
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.Database.CacheDir = file
	if _, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry()); err == nil {
		t.Fatal("expected mkdir fail")
	}
}
