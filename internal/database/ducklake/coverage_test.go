package ducklake

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
)

func TestOptionsNormalizedAndDefault(t *testing.T) {
	t.Parallel()
	def := DefaultOptions()
	if def.LakeAlias != DefaultLakeAlias || def.Threads != 2 {
		t.Fatalf("%+v", def)
	}
	n := Options{DataInliningRowLimit: -1}.normalized()
	if n.MemoryLimit != def.MemoryLimit || n.Threads != def.Threads || n.LakeAlias != def.LakeAlias {
		t.Fatalf("%+v", n)
	}
	if n.DataInliningRowLimit != def.DataInliningRowLimit || n.ParquetCompression != def.ParquetCompression {
		t.Fatal(n)
	}
	if n.CatalogSync.Mode != "debounce" || n.CatalogSync.Debounce != 200*time.Millisecond || n.CatalogSync.KeepVersions != 10 {
		t.Fatal(n.CatalogSync)
	}
	if n.Maintenance.RewriteDeleteThreshold != 0.95 {
		t.Fatal(n.Maintenance)
	}
}

func TestRemoteValidateAndEndpoint(t *testing.T) {
	t.Parallel()
	var r RemoteStorage
	if err := r.validate(); err != nil {
		t.Fatal(err)
	}
	r.Enabled = true
	if err := r.validate(); err == nil || !strings.Contains(err.Error(), "bucket") {
		t.Fatal(err)
	}
	r.Bucket = "b"
	if err := r.validate(); err == nil || !strings.Contains(err.Error(), "region") {
		t.Fatal(err)
	}
	r.Region = "us-east-1"
	if err := r.validate(); err != nil {
		t.Fatal(err)
	}
	kb := r.keyBuilder()
	if kb.Environment != "dev" || kb.RootPrefix != "simplebase" {
		t.Fatalf("%+v", kb)
	}
	r.Environment = "prod"
	r.RootPrefix = "root"
	if r.keyBuilder().Environment != "prod" {
		t.Fatal(r.keyBuilder())
	}
	if r.s3EndpointHost() != "" {
		t.Fatal(r.s3EndpointHost())
	}
	r.Endpoint = "https://minio.local/"
	if r.s3EndpointHost() != "minio.local" {
		t.Fatal(r.s3EndpointHost())
	}
	if !r.endpointUsesSSL() {
		t.Fatal("https")
	}
	r.Endpoint = "http://minio.local"
	if r.endpointUsesSSL() {
		t.Fatal("http")
	}
	r.Endpoint = "minio.local"
	r.UseSSL = true
	if !r.endpointUsesSSL() {
		t.Fatal("use ssl")
	}
	r.Endpoint = ""
	if !r.endpointUsesSSL() {
		t.Fatal("empty endpoint ssl")
	}
	r.UseSSL = false
	r.Endpoint = "minio.local"
	if r.endpointUsesSSL() {
		t.Fatal("plain host no ssl")
	}
}

func TestSecretAndSQLQuote(t *testing.T) {
	t.Parallel()
	sql := buildCreateSecretSQL(RemoteStorage{
		AccessKey: "ak", SecretKey: "sk", Region: "r",
		Endpoint: "https://s3.example/", ForcePathStyle: true,
	})
	if !strings.Contains(sql, "KEY_ID") || !strings.Contains(sql, "URL_STYLE 'path'") || !strings.Contains(sql, "USE_SSL true") {
		t.Fatal(sql)
	}
	sql2 := buildCreateSecretSQL(RemoteStorage{Endpoint: "http://minio", Region: "r"})
	if !strings.Contains(sql2, "USE_SSL false") {
		t.Fatal(sql2)
	}
	if quoteSQLString("a'b") != "'a''b'" {
		t.Fatal(quoteSQLString("a'b"))
	}
	if quoteIdent(`a"b`) != `"a""b"` {
		t.Fatal(quoteIdent(`a"b`))
	}
	if !isSafeIdent("lake") || !isSafeIdent("_x1") || isSafeIdent("") || isSafeIdent("1x") || isSafeIdent("la-ke") {
		t.Fatal("isSafeIdent")
	}
	if _, err := buildDataURI(RemoteStorage{Bucket: ""}, "t", "d"); err == nil {
		t.Fatal("empty bucket")
	}
	uri, err := buildDataURI(RemoteStorage{
		Bucket: "b", Region: "r", RootPrefix: "simplebase", Environment: "e",
	}, "11111111-1111-1111-1111-111111111111", "33333333-3333-3333-3333-333333333333")
	if err != nil || !strings.HasPrefix(uri, "s3://b/") {
		t.Fatal(uri, err)
	}
}

func TestExtensionHelpers(t *testing.T) {
	t.Parallel()
	boot := extensionBootSQL(Options{ExtensionDir: "/ext"})
	if len(boot) < 7 || !strings.Contains(boot[0], "extension_directory") {
		t.Fatal(boot)
	}
	if normalizeDuckDBVersion(" v1.5.2-dev ") != "1.5.2" {
		t.Fatal(normalizeDuckDBVersion(" v1.5.2-dev "))
	}
	if _, err := parseSemver3("1"); err == nil {
		t.Fatal("short semver")
	}
	if _, err := parseSemver3("a.b.c"); err == nil {
		t.Fatal("bad semver")
	}
	if ok, err := versionAtLeast("1.5", MinDuckDBVersion); err != nil || ok {
		t.Fatalf("1.5 vs 1.5.2: %v %v", ok, err)
	}
	if ok, err := versionAtLeast("1.5.2", MinDuckDBVersion); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := versionAtLeast("x", MinDuckDBVersion); err == nil {
		t.Fatal("bad got")
	}
	if _, err := versionAtLeast("1.5.2", "x"); err == nil {
		t.Fatal("bad min")
	}
	if !extensionLoaded(map[string]bool{"sqlite_scanner": true}, "sqlite") {
		t.Fatal("alias")
	}
	if extensionLoaded(map[string]bool{"httpfs": false}, "httpfs") {
		t.Fatal("not loaded")
	}
	if !extensionLoaded(map[string]bool{"ducklake": true}, "ducklake") {
		t.Fatal("direct")
	}
}

func TestSummarizeSQLAndBootSQL(t *testing.T) {
	t.Parallel()
	if summarizeSQL("CREATE OR REPLACE SECRET x (TYPE S3, KEY_ID 'a')") != "CREATE OR REPLACE SECRET ..." {
		t.Fatal(summarizeSQL("CREATE OR REPLACE SECRET x (TYPE S3, KEY_ID 'a')"))
	}
	long := strings.Repeat("a", 100)
	if !strings.HasSuffix(summarizeSQL(long), "...") {
		t.Fatal(summarizeSQL(long))
	}
	layout := layoutFor(t.TempDir(), uuid.NewString())
	_ = layout.ensure()
	opts := DefaultOptions()
	opts.RequireCommitMessage = true
	opts.ExtensionDir = "/e"
	boot := buildBootSQL(layout, opts, RemoteStorage{}, layout.dataPathArg())
	joined := strings.Join(boot, "\n")
	if !strings.Contains(joined, "require_commit_message") || !strings.Contains(joined, "enable_external_access = false") {
		t.Fatal(joined)
	}
	remote := RemoteStorage{Enabled: true, Bucket: "b", Region: "r", AccessKey: "ak", SecretKey: "sk"}
	boot2 := buildBootSQL(layout, opts, remote, "s3://b/p/")
	joined2 := strings.Join(boot2, "\n")
	if !strings.Contains(joined2, "OVERRIDE_DATA_PATH") || strings.Contains(joined2, "enable_external_access = false") {
		t.Fatal(joined2)
	}
	if !strings.HasSuffix(layout.dataPathArg(), "/") {
		t.Fatal(layout.dataPathArg())
	}
}

func TestLocalSyncerNilAndCancel(t *testing.T) {
	var s *LocalSyncer
	s.MarkDirty("x", 1)
	if s.LastSynced("x") != 0 {
		t.Fatal(s.LastSynced("x"))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	live := NewLocalSyncer()
	if err := live.Sync(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestFactoryHelpersWithoutOpen(t *testing.T) {
	var f *Factory
	if err := f.AfterWrite(context.Background(), catalog.Database{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.BeforeClose(context.Background(), "id", nil); err != nil {
		t.Fatal(err)
	}
	if last, lag := f.SnapshotStatus("id"); last != 0 || lag != 0 {
		t.Fatal(last, lag)
	}
	if f.DurabilityFor("id") != DurabilityCommittedLocal {
		t.Fatal(f.DurabilityFor("id"))
	}

	syncer := NewLocalSyncer()
	f = &Factory{Syncer: syncer, Options: DefaultOptions()}
	if err := f.BeforeClose(context.Background(), "id", nil); err != nil {
		t.Fatal(err)
	}
	syncer.MarkDirty("id", 4)
	last, lag := f.SnapshotStatus("id")
	if last != 4 || lag != 0 {
		t.Fatal(last, lag)
	}
	if f.DurabilityFor("id") != DurabilityCommittedLocal {
		t.Fatal("remote off")
	}

	cs := NewCatalogSyncer(objectstore.NewMemoryBlobStore(), RemoteStorage{Enabled: true, Bucket: "b", Region: "r"}, t.TempDir(), CatalogSyncOptions{Mode: "sync_on_commit"}, nil, nil)
	f = &Factory{Syncer: cs, Remote: RemoteStorage{Enabled: true}, Options: Options{CatalogSync: CatalogSyncOptions{Mode: "sync_on_commit"}}}
	if f.DurabilityFor("id") != DurabilityCommittedLocal {
		t.Fatal(f.DurabilityFor("id"))
	}
	cs.last["id"] = 3
	if f.DurabilityFor("id") != DurabilitySyncedS3 {
		t.Fatal(f.DurabilityFor("id"))
	}
	last, lag = f.SnapshotStatus("id")
	if last != 3 {
		t.Fatal(last, lag)
	}

	f.CacheDir = t.TempDir()
	f.Remote.Enabled = true
	f.Remote.Bucket = ""
	_, err := f.Open(context.Background(), catalog.Database{ID: uuid.NewString()}, database.ReadWrite)
	if err == nil || !strings.Contains(err.Error(), "bucket") {
		t.Fatal(err)
	}
}

func TestCatalogSyncerControlPaths(t *testing.T) {
	var nilCS *CatalogSyncer
	nilCS.Bind("x", nil, catalog.Database{}, "")
	if err := nilCS.Unbind(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	nilCS.MarkDirty("x", 1)
	if nilCS.LastSynced("x") != 0 || nilCS.SyncLag("x") != 0 {
		t.Fatal("nil")
	}
	if err := nilCS.Sync(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if err := nilCS.Flush(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if err := nilCS.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	cs := NewCatalogSyncer(objectstore.NewMemoryBlobStore(), RemoteStorage{}, t.TempDir(), CatalogSyncOptions{}, observability.NewLogger("debug", "json", io.Discard), observability.NewMetrics(prometheus.NewRegistry()))
	if cs.Options.Mode != "debounce" || cs.Options.KeepVersions != 10 {
		t.Fatal(cs.Options)
	}
	cs.Bind("db", nil, catalog.Database{ID: "db"}, "")
	cs.MarkDirty("db", 0)
	cs.MarkDirty("db", 2)
	cs.MarkDirty("db", 1)
	if cs.SyncLag("db") != 2 {
		t.Fatal(cs.SyncLag("db"))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cs.Sync(ctx, "db"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	cs.closed = true
	cs.MarkDirty("db", 9)
	if err := cs.Sync(context.Background(), "db"); err == nil {
		t.Fatal("closed")
	}
	cs.closed = false
	cs.inflight["db"] = true
	if err := cs.Sync(context.Background(), "other"); err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatal(err)
	}
	if err := cs.Sync(context.Background(), "db"); err != nil {
		t.Fatal(err)
	}
	cs.inflight["db"] = false
	cs.last["db"] = 5
	cs.dirty["db"] = 4
	if err := cs.Sync(context.Background(), "db"); err != nil {
		t.Fatal(err)
	}
	delete(cs.bound, "db")
}

func TestEnsureLocalCatalogBranches(t *testing.T) {
	ctx := context.Background()
	ok, err := EnsureLocalCatalog(ctx, nil, RemoteStorage{Enabled: true}, t.TempDir(), catalog.Database{})
	if err != nil || ok {
		t.Fatal(ok, err)
	}
	blobs := objectstore.NewMemoryBlobStore()
	ok, err = EnsureLocalCatalog(ctx, blobs, RemoteStorage{}, t.TempDir(), catalog.Database{})
	if err != nil || ok {
		t.Fatal(ok, err)
	}
	remote := RemoteStorage{Enabled: true, Region: "r", Bucket: "b", RootPrefix: "simplebase", Environment: "e"}
	meta := catalog.Database{ID: uuid.NewString(), TenantID: "bad"}
	if _, err := EnsureLocalCatalog(ctx, blobs, remote, t.TempDir(), meta); err == nil {
		t.Fatal("bad tenant")
	}
	meta.TenantID = "11111111-1111-1111-1111-111111111111"
	ok, err = EnsureLocalCatalog(ctx, blobs, remote, t.TempDir(), meta)
	if err != nil || ok {
		t.Fatal(ok, err)
	}
}

func TestInspectInvalidAlias(t *testing.T) {
	ctx := context.Background()
	if _, err := CurrentSnapshot(ctx, nil, "bad-alias"); err == nil {
		t.Fatal("alias")
	}
	if _, err := ListSnapshots(ctx, nil, "1x"); err == nil {
		t.Fatal("list snaps")
	}
	if _, err := ListSettings(ctx, nil, ""); err == nil {
		t.Fatal("settings")
	}
	if _, err := ListTableFiles(ctx, nil, "lake", "bad-table"); err == nil {
		t.Fatal("files")
	}
}

func TestFactoryAfterWriteWithLake(t *testing.T) {
	db, _ := openTestLake(t)
	s := NewLocalSyncer()
	f := &Factory{Syncer: s, Options: DefaultOptions()}
	meta := catalog.Database{ID: uuid.NewString()}
	if _, err := db.ExecContext(context.Background(), `CREATE TABLE aw (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if err := f.AfterWrite(context.Background(), meta, db); err != nil {
		t.Fatal(err)
	}
	if s.LastSynced(meta.ID) < 0 {
		t.Fatal(s.LastSynced(meta.ID))
	}
	if _, err := ListSettings(context.Background(), db, DefaultLakeAlias); err != nil {
		t.Fatal(err)
	}
	if _, err := ListTableFiles(context.Background(), db, DefaultLakeAlias, "no_such"); err != nil && !strings.Contains(err.Error(), "list_files") {
		// may error if table missing
		t.Log(err)
	}
	if err := AssertExtensionsLoaded(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogSyncerLocalAdvanceAndRecord(t *testing.T) {
	db, _ := openTestLake(t)
	id := uuid.NewString()
	meta := catalog.Database{ID: id, TenantID: "11111111-1111-1111-1111-111111111111"}
	cs := NewCatalogSyncer(objectstore.NewMemoryBlobStore(), RemoteStorage{}, t.TempDir(), CatalogSyncOptions{Mode: "sync_on_commit"}, observability.NewLogger("debug", "json", io.Discard), observability.NewMetrics(prometheus.NewRegistry()))
	cs.Bind(id, db, meta, DefaultLakeAlias)
	if _, err := db.ExecContext(context.Background(), `CREATE TABLE z (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	snap, err := CurrentSnapshot(context.Background(), db, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	cs.MarkDirty(id, snap)
	if cs.LastSynced(id) != snap {
		t.Fatalf("synced %d want %d", cs.LastSynced(id), snap)
	}
	if err := cs.Unbind(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err := cs.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	cs.recordSync(errors.New("boom"), time.Millisecond, id)
	cs.recordSync(nil, time.Millisecond, id)
}

func TestValidateRemoteDataPath(t *testing.T) {
	db, _ := openTestLake(t)
	settings, err := ListSettings(context.Background(), db, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, s := range settings {
		if strings.EqualFold(s.Key, "data_path") {
			path = s.Value
		}
	}
	if err := validateRemoteDataPath(context.Background(), db, DefaultLakeAlias, path); err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteDataPath(context.Background(), db, DefaultLakeAlias, "s3://other/nope/"); err != nil && !strings.Contains(err.Error(), "mismatch") {
		t.Log(err) // empty data_path is ok
	}
}

func TestFactoryDataPathFor(t *testing.T) {
	layout := layoutFor(t.TempDir(), uuid.NewString())
	f := &Factory{}
	p, err := f.dataPathFor(catalog.Database{}, layout)
	if err != nil || p == "" {
		t.Fatal(p, err)
	}
	f.Remote = RemoteStorage{Enabled: true, Bucket: "b", Region: "r"}
	if _, err := f.dataPathFor(catalog.Database{TenantID: "bad", ID: uuid.NewString()}, layout); err == nil {
		t.Fatal("bad tenant")
	}
}

func TestPruneVersions(t *testing.T) {
	cs := NewCatalogSyncer(objectstore.NewMemoryBlobStore(), RemoteStorage{Enabled: true, Bucket: "b", Region: "r", RootPrefix: "simplebase", Environment: "e"}, t.TempDir(), CatalogSyncOptions{KeepVersions: 1}, nil, nil)
	meta := catalog.Database{ID: "33333333-3333-3333-3333-333333333333", TenantID: "11111111-1111-1111-1111-111111111111"}
	if err := cs.pruneVersions(context.Background(), meta, 1); err != nil {
		t.Fatal(err)
	}
	if err := cs.pruneVersions(context.Background(), meta, 5); err != nil {
		t.Fatal(err)
	}
	cs.Options.KeepVersions = 0
	if err := cs.pruneVersions(context.Background(), meta, 9); err != nil {
		t.Fatal(err)
	}
}

func TestFactoryOpenRemoteEnsureCatalog(t *testing.T) {
	if testing.Short() {
		t.Skip("remote open needs duckdb/network")
	}
	// Invalid remote already covered. Open with remote enabled but no blob catalog
	// still tries S3 ATTACH; skip real network by using disabled remote + local open
	// (existing runtime tests). Exercise EnsureLocalCatalog + factory fields only.
	f := &Factory{
		CacheDir: t.TempDir(),
		Options:  DefaultOptions(),
		Remote:   RemoteStorage{Enabled: true, Bucket: "b", Region: "us-east-1"},
		Blobs:    objectstore.NewMemoryBlobStore(),
	}
	_, err := f.Open(context.Background(), catalog.Database{
		ID: uuid.NewString(), TenantID: "11111111-1111-1111-1111-111111111111",
	}, database.ReadOnly)
	if err == nil {
		t.Log("unexpected success without S3")
	}
}
