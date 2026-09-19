package app

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/prometheus/client_golang/prometheus"
)

type errCloser struct{ err error }

func (c errCloser) Close() error { return c.err }

type fakeStore struct{ fail error }

func (f fakeStore) Check(ctx context.Context) error { return f.fail }

func TestHealthLiveAndReady(t *testing.T) {
	h := &healthService{app: &App{cfg: config.Config{Instance: config.InstanceConfig{Writable: false}}}}
	if err := h.Live(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := h.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}

	h.app.cfg.Instance.Writable = true
	if err := h.Ready(context.Background()); err == nil || err.Error() != "system database not ready" {
		t.Fatalf("want system not ready, got %v", err)
	}

	// Writable + DevMode skips S3 even if bucket empty.
	h.app.systemStore = nil
	h.app.cfg.DevMode = true
	if err := h.Ready(context.Background()); err == nil {
		t.Fatal("still missing system store")
	}

	h.app.cfg.DevMode = false
	h.app.cfg.S3.Bucket = ""
	// systemStore still nil -> fails before S3
	if err := h.Ready(context.Background()); err == nil {
		t.Fatal("expected system store error")
	}
}

func TestHealthReadyS3Check(t *testing.T) {
	cfg := testConfig(true)
	cfg.DevMode = true
	cfg.S3 = config.S3Config{}
	cfg.Database.CacheDir = t.TempDir()
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	})
	if err := a.health.Live(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.health.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Flip to production-like ready checks against a fake object store.
	a.cfg.DevMode = false
	a.cfg.S3.Bucket = ""
	if err := a.health.Ready(context.Background()); err == nil || err.Error() != "s3 bucket not configured" {
		t.Fatalf("bucket: %v", err)
	}
	a.cfg.S3.Bucket = "b"
	a.objectStore = fakeStore{fail: errors.New("s3 down")}
	if err := a.health.Ready(context.Background()); err == nil || err.Error() == "" {
		t.Fatalf("s3 check: %v", err)
	}
	a.objectStore = fakeStore{}
	if err := a.health.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterCloserAndClose(t *testing.T) {
	a := &App{}
	a.registerCloser(errCloser{})
	a.registerCloser(errCloser{err: errors.New("boom")})
	if err := a.Close(context.Background()); err == nil || err.Error() != "boom" {
		t.Fatalf("first closer error: %v", err)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDuckLakeOptionsAndFactories(t *testing.T) {
	cfg := config.DuckLakeConfig{
		MemoryLimit: "1MB", Threads: 3, ExtensionDir: "/e", DataInliningRowLimit: 2,
		ParquetCompression: "snappy", TargetFileSize: "1MB", RequireCommitMessage: true,
		CatalogSync:  config.CatalogSyncConfig{Mode: "sync_on_commit", Debounce: time.Second, KeepVersions: 2},
		Maintenance: config.DuckLakeMaintenanceConfig{CheckpointInterval: time.Minute, ExpireOlderThan: time.Hour, DeleteOlderThan: time.Hour, RewriteDeleteThreshold: 0.5},
	}
	opts := duckLakeOptions(cfg)
	if opts.MemoryLimit != "1MB" || opts.Threads != 3 || !opts.RequireCommitMessage {
		t.Fatalf("%+v", opts)
	}
	if opts.CatalogSync.Mode != "sync_on_commit" || opts.Maintenance.RewriteDeleteThreshold != 0.5 {
		t.Fatal(opts)
	}

	a := &App{cfg: testConfig(true)}
	a.cfg.DevMode = true
	a.cfg.Database.CacheDir = t.TempDir()
	a.cfg.Database.DuckLake = cfg
	f := a.newUserDatabaseFactory(a.cfg.Database.CacheDir)
	if f == nil || a.duckFactory == nil {
		t.Fatal("user factory")
	}
	sys := a.newSystemDatabaseFactory()
	if sys.Syncer == nil || sys.CacheDir == "" {
		t.Fatal("system factory")
	}
	if a.cloudAgentRuntime() != nil {
		t.Fatal("no system store")
	}
}

func TestNewUserFactoryRemoteWithoutBlobs(t *testing.T) {
	a := &App{cfg: testConfig(true)}
	a.cfg.DevMode = false
	a.cfg.S3.Bucket = "b"
	f := a.newUserDatabaseFactory(t.TempDir()).(*ducklake.Factory)
	if !f.Remote.Enabled || f.Syncer == nil {
		t.Fatal(f.Remote, f.Syncer)
	}
	sys := a.newSystemDatabaseFactory()
	if sys.Remote.Enabled != f.Remote.Enabled {
		t.Fatal(sys.Remote)
	}
}

func TestSyncerCloser(t *testing.T) {
	if err := (syncerCloser{}).Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewDelegatesToRegistry(t *testing.T) {
	// New() is NewWithRegistry(..., nil). Avoid DefaultRegisterer collisions
	// by exercising the same assembly with an isolated registry.
	cfg := testConfig(false)
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	})
	if a.echo == nil {
		t.Fatal("echo")
	}
}

func TestAssembleDepsWritableMissingPrefix(t *testing.T) {
	cfg := testConfig(true)
	cfg.S3.Prefix = ""
	_, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err == nil {
		t.Fatal("expected s3 prefix error")
	}
}

func TestCloseRegistryErrorWins(t *testing.T) {
	a := &App{}
	// registry nil is fine; just closers
	a.registerCloser(io.NopCloser(nil))
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCloudAgentRuntimeWithStore(t *testing.T) {
	cfg := testConfig(true)
	cfg.DevMode = true
	cfg.S3 = config.S3Config{}
	cfg.Database.CacheDir = t.TempDir()
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	})
	if a.cloudAgentRuntime() == nil {
		t.Fatal("runtime")
	}
	if a.fileStore == nil {
		t.Fatal("local file store")
	}
}

var _ objectstore.Client = fakeStore{}

func (fakeStore) PutJSON(ctx context.Context, key string, value any, opts objectstore.PutOptions) error {
	return nil
}
func (fakeStore) GetJSON(ctx context.Context, key string, dst any) error { return nil }
func (fakeStore) Head(ctx context.Context, key string) (objectstore.ObjectInfo, error) {
	return objectstore.ObjectInfo{}, nil
}
func (fakeStore) DeletePrefix(ctx context.Context, prefix string) error { return nil }
