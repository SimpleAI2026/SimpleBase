package objectstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
)

type fakeS3API struct {
	putErr    error
	getErr    error
	getBody   []byte
	getMeta   *s3.GetObjectOutput
	headErr   error
	headOut   *s3.HeadObjectOutput
	delErr    error
	delOut    *s3.DeleteObjectsOutput
	listErr   error
	listPages []s3.ListObjectsV2Output
	listIdx   int
	putCalled int
}

func (f *fakeS3API) PutObject(ctx context.Context, in *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putCalled++
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &s3.PutObjectOutput{}, nil
}
func (f *fakeS3API) GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	out := &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(f.getBody))}
	if f.getMeta != nil {
		out.LastModified = f.getMeta.LastModified
		out.ETag = f.getMeta.ETag
	}
	return out, nil
}
func (f *fakeS3API) HeadObject(ctx context.Context, in *s3.HeadObjectInput, opts ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if f.headErr != nil {
		return nil, f.headErr
	}
	if f.headOut != nil {
		return f.headOut, nil
	}
	return &s3.HeadObjectOutput{}, nil
}
func (f *fakeS3API) HeadBucket(ctx context.Context, in *s3.HeadBucketInput, opts ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return &s3.HeadBucketOutput{}, nil
}
func (f *fakeS3API) DeleteObjects(ctx context.Context, in *s3.DeleteObjectsInput, opts ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	if f.delErr != nil {
		return nil, f.delErr
	}
	if f.delOut != nil {
		return f.delOut, nil
	}
	return &s3.DeleteObjectsOutput{}, nil
}
func (f *fakeS3API) ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, opts ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if len(f.listPages) == 0 {
		return &s3.ListObjectsV2Output{}, nil
	}
	if f.listIdx >= len(f.listPages) {
		return &s3.ListObjectsV2Output{}, nil
	}
	out := f.listPages[f.listIdx]
	f.listIdx++
	return &out, nil
}

func testS3Client(api s3API) *s3Client {
	reg := prometheus.NewRegistry()
	return &s3Client{
		api:      api,
		bucket:   "b",
		kmsKeyID: "kms-default",
		logger:   observability.NewLogger("debug", "json", io.Discard),
		metrics:  observability.NewMetrics(reg),
		redacted: redactor{},
	}
}

func TestNewClientValidationAndLoadAWS(t *testing.T) {
	ctx := context.Background()
	if _, err := NewClient(ctx, Config{Region: "r"}, nil, nil); err == nil {
		t.Fatal("bucket required")
	}
	if _, err := NewClient(ctx, Config{Bucket: "b"}, nil, nil); err == nil {
		t.Fatal("region required")
	}
	cfg := Config{
		Endpoint: "https://minio.local:9000", Region: "us-east-1", Bucket: "b",
		AccessKey: "ak", SecretKey: "sk", ForcePathStyle: true, KMSKeyID: "k",
	}
	c, err := NewClient(ctx, cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.(*s3Client).Close(); err != nil {
		t.Fatal(err)
	}
	if !isAWSHost("https://s3.amazonaws.com") || isAWSHost("https://minio.local") {
		t.Fatal("isAWSHost")
	}
	if isAWSHost("") {
		// empty is AWS host per implementation
	}
	awsCfg, err := loadAWSConfig(ctx, Config{Region: "us-east-1", Endpoint: "https://s3.us-east-1.amazonaws.com"})
	if err != nil {
		t.Fatalf("loadAWSConfig aws host: %v", err)
	}
	_ = awsCfg
}

func TestS3ClientJSONAndHead(t *testing.T) {
	ctx := context.Background()
	api := &fakeS3API{getBody: []byte(`{"ok":true}`)}
	c := testS3Client(api)
	if err := c.PutJSON(ctx, "k", map[string]int{"a": 1}, PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := c.PutJSON(ctx, "k", map[string]int{"a": 1}, PutOptions{ContentType: "application/x", KMSKeyID: "override"}); err != nil {
		t.Fatal(err)
	}
	var dst map[string]any
	if err := c.GetJSON(ctx, "k", &dst); err != nil {
		t.Fatal(err)
	}
	sz := int64(3)
	etag := `"abc"`
	now := time.Now()
	api.headOut = &s3.HeadObjectOutput{ContentLength: &sz, ETag: &etag, LastModified: &now}
	info, err := c.Head(ctx, "k")
	if err != nil || info.Size != 3 || info.ETag != `"abc"` {
		t.Fatalf("head: %+v %v", info, err)
	}

	api.putErr = errors.New("AccessKey=AKIA&SecretKey=SK&boom")
	if err := c.PutJSON(ctx, "k", 1, PutOptions{}); err == nil || strings.Contains(err.Error(), "AKIA") {
		t.Fatalf("put error should sanitize: %v", err)
	}
	api.getErr = &types.NoSuchKey{}
	if err := c.GetJSON(ctx, "missing", &dst); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get not found: %v", err)
	}
	api.getErr = errors.New("bad json body")
	api.getBody = []byte("not-json")
	// getErr takes precedence
	if err := c.GetJSON(ctx, "k", &dst); err == nil {
		t.Fatal("expected get error")
	}
	api.getErr = nil
	api.getBody = []byte("not-json")
	if err := c.GetJSON(ctx, "k", &dst); err == nil {
		t.Fatal("expected decode error")
	}
	api.headErr = &types.NotFound{}
	if _, err := c.Head(ctx, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("head notfound: %v", err)
	}
	if err := c.PutJSON(ctx, "k", make(chan int), PutOptions{}); err == nil {
		t.Fatal("marshal error")
	}
}

func TestS3ClientDeletePrefixAndCheck(t *testing.T) {
	ctx := context.Background()
	key := "a"
	trunc := true
	api := &fakeS3API{
		listPages: []s3.ListObjectsV2Output{
			{Contents: []types.Object{{Key: &key}}, IsTruncated: &trunc},
			{Contents: []types.Object{{Key: &key}}},
		},
		delOut: &s3.DeleteObjectsOutput{Errors: []types.Error{{Key: &key}}},
	}
	c := testS3Client(api)
	if err := c.DeletePrefix(ctx, "pref"); err != nil {
		t.Fatal(err)
	}
	api.listErr = errors.New("list fail")
	if err := c.DeletePrefix(ctx, "pref"); err == nil {
		t.Fatal("list error")
	}
	api.listErr = nil
	api.listPages = []s3.ListObjectsV2Output{{Contents: []types.Object{{Key: &key}}}}
	api.listIdx = 0
	api.delErr = errors.New("del fail")
	if err := c.DeletePrefix(ctx, "pref"); err == nil {
		t.Fatal("delete error")
	}
	api.listPages = nil
	api.listErr = nil
	api.delErr = nil
	if err := c.DeletePrefix(ctx, "empty"); err != nil {
		t.Fatal(err)
	}

	api.headErr = nil
	if err := c.Check(ctx); err != nil {
		t.Fatal(err)
	}
	api.headErr = &types.NotFound{}
	if err := c.Check(ctx); err != nil {
		t.Fatal(err)
	}
	api.headErr = errors.New("denied")
	if err := c.Check(ctx); err == nil {
		t.Fatal("check should fail")
	}
}

func TestS3BlobMethods(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	etag := "e"
	api := &fakeS3API{getBody: []byte("blob"), getMeta: &s3.GetObjectOutput{LastModified: &now, ETag: &etag}}
	c := testS3Client(api)
	if err := c.PutBytes(ctx, "k", []byte("hi"), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	if err := c.PutBytes(ctx, "k", []byte("hi"), ""); err != nil {
		t.Fatal(err)
	}
	data, info, err := c.GetBytes(ctx, "k")
	if err != nil || string(data) != "blob" || info.ETag != "e" {
		t.Fatalf("getbytes %q %+v %v", data, info, err)
	}
	dest := filepath.Join(t.TempDir(), "sub", "f.bin")
	if _, err := c.DownloadFile(ctx, "k", dest); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dest)
	if string(b) != "blob" {
		t.Fatalf("downloaded %q", b)
	}
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}

	api.putErr = errors.New("put")
	if err := c.PutBytes(ctx, "k", []byte("x"), ""); err == nil {
		t.Fatal("put error")
	}
	api.getErr = errors.New("StatusCode: 404")
	if _, _, err := c.GetBytes(ctx, "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get not found: %v", err)
	}
	c2 := testS3Client(&fakeS3API{delErr: errors.New("x")})
	if err := c2.Delete(ctx, "k"); err == nil {
		t.Fatal("delete error")
	}
	c3 := testS3Client(&errBodyAPI{})
	if _, _, err := c3.GetBytes(ctx, "k"); err == nil {
		t.Fatal("read body error")
	}
}

type errBodyAPI struct{ fakeS3API }

func (e *errBodyAPI) GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{Body: io.NopCloser(&errReader{})}, nil
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read fail") }

func TestMemoryBlobRemaining(t *testing.T) {
	s := NewMemoryBlobStore()
	ctx := context.Background()
	if _, _, err := s.GetBytes(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.DownloadFile(ctx, "missing", filepath.Join(t.TempDir(), "x")); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	_ = s.PutBytes(ctx, "k", []byte("v"), "")
	info, err := s.Head(ctx, "k")
	if err != nil || info.Size != 1 {
		t.Fatal(err, info)
	}
}

func TestDescriptorValidateRemaining(t *testing.T) {
	ok := func() *Descriptor {
		return &Descriptor{
			FormatVersion:   2,
			TenantID:        "11111111-1111-1111-1111-111111111111",
			ProjectID:       "22222222-2222-2222-2222-222222222222",
			DatabaseID:      "33333333-3333-3333-3333-333333333333",
			CreatedAt:       time.Now(),
			DuckLakeStorage: DuckLakeStorage{Bucket: "b", Region: "r", Prefix: "p"},
			DataPrefix:      "p",
			Status:          StatusReady,
			Engine:          "ducklake",
		}
	}
	cases := []struct {
		name string
		d    *Descriptor
		sub  string
	}{
		{"nil", nil, "nil"},
		{"ver0", func() *Descriptor { d := ok(); d.FormatVersion = 0; return d }(), "positive"},
		{"no created", func() *Descriptor { d := ok(); d.CreatedAt = time.Time{}; return d }(), "created_at"},
		{"no bucket", func() *Descriptor { d := ok(); d.DuckLakeStorage.Bucket = ""; return d }(), "bucket"},
		{"no region", func() *Descriptor { d := ok(); d.DuckLakeStorage.Region = ""; return d }(), "region"},
		{"no prefix", func() *Descriptor { d := ok(); d.DuckLakeStorage.Prefix = ""; return d }(), "prefix"},
		{"no data prefix", func() *Descriptor { d := ok(); d.DataPrefix = ""; return d }(), "data_prefix"},
		{"no status", func() *Descriptor { d := ok(); d.Status = ""; return d }(), "status"},
		{"bad tenant", func() *Descriptor { d := ok(); d.TenantID = "not-uuid"; return d }(), "tenant_id"},
	}
	for _, tc := range cases {
		err := tc.d.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.sub) {
			t.Errorf("%s: want %q got %v", tc.name, tc.sub, err)
		}
	}
	if err := ok().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestKeyBuilderRemaining(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "/simplebase/", Environment: "/prod/"}
	tenant := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	db := "ffffffff-0000-1111-2222-333333333333"
	if _, err := kb.DuckLakeCatalogVersionKey(tenant, db, 0); err == nil {
		t.Fatal("snapshot must be positive")
	}
	if _, err := kb.DuckLakeDataURI("", tenant, db); err == nil {
		t.Fatal("bucket required")
	}
	if _, err := kb.DataPrefix("bad", db); err == nil {
		t.Fatal("invalid tenant")
	}
	if _, err := kb.DescriptorKey(tenant, "short"); err == nil {
		t.Fatal("invalid db")
	}
	// invalid dash positions / non-hex
	if err := validateID("id", "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"); err == nil {
		t.Fatal("non-hex")
	}
	if err := validateID("id", "11111111x1111-1111-1111-111111111111"); err == nil {
		t.Fatal("missing dash")
	}
	if joinKey("", "/a/", "", "b") != "a/b" {
		t.Fatalf("joinKey: %q", joinKey("", "/a/", "", "b"))
	}
	p, err := kb.DatabasePrefix(tenant, db)
	if err != nil || strings.Contains(p, "//") || strings.HasPrefix(p, "/") {
		t.Fatalf("prefix %q %v", p, err)
	}
}

func TestHelpersSanitizeAndDeref(t *testing.T) {
	c := &s3Client{redacted: redactor{}}
	if c.sanitizeErr(nil) != nil {
		t.Fatal("nil")
	}
	err := c.sanitizeErr(errors.New("access_key=ABC&secret_key=DEF&X-Amz-Credential=GHI&end"))
	if strings.Contains(err.Error(), "ABC") || strings.Contains(err.Error(), "DEF") {
		t.Fatalf("not redacted: %v", err)
	}
	if mapNotFoundErr(nil) != nil {
		t.Fatal("nil map")
	}
	if !errors.Is(mapNotFoundErr(errors.New("StatusCode: 404")), ErrNotFound) {
		t.Fatal("404")
	}
	if mapNotFoundErr(errors.New("other")) == ErrNotFound {
		t.Fatal("other")
	}
	if derefStr(nil) != "" || derefInt64(nil) != 0 || !derefTime(nil).IsZero() {
		t.Fatal("deref nil")
	}
	s := "x"
	n := int64(2)
	tm := time.Unix(1, 0)
	if derefStr(&s) != "x" || derefInt64(&n) != 2 || derefTime(&tm).Unix() != 1 {
		t.Fatal("deref")
	}
	r := bytesReader([]byte("ab"))
	buf := make([]byte, 1)
	if n, _ := r.Read(buf); n != 1 {
		t.Fatal(n)
	}
	_, _ = r.Read(buf)
	if n, err := r.Read(buf); n != 0 || err != io.EOF {
		t.Fatal(n, err)
	}
	c.recordOp("x", "ok") // metrics set
	c2 := &s3Client{}
	c2.recordOp("x", "ok") // nil metrics
}

func TestAssertHealthFail(t *testing.T) {
	c := newFakeClient()
	c.failOn["check"] = errors.New("down")
	if err := AssertHealth(context.Background(), c); err == nil {
		t.Fatal("expected fail")
	}
}

func TestLocalFileStoreRemaining(t *testing.T) {
	ctx := context.Background()
	fs, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// maxKeys <= 0 defaults to 100; prefix skip
	_, _ = fs.Put(ctx, "keep/a.txt", bytes.NewReader([]byte("a")), 1, "")
	_, _ = fs.Put(ctx, "other/b.txt", bytes.NewReader([]byte("b")), 1, "")
	objs, err := fs.List(ctx, "keep/", 0)
	if err != nil || len(objs) != 1 {
		t.Fatalf("list: %v %+v", err, objs)
	}
	objs, err = fs.List(ctx, "keep/", 1)
	if err != nil || len(objs) != 1 {
		t.Fatal(err, objs)
	}
	if _, err := fs.PresignGet(ctx, "keep/a.txt", time.Minute); err != nil {
		t.Fatal(err)
	}
	// unknown extension uses DetectContentType
	_, _ = fs.Put(ctx, "bin/x.unknownext", bytes.NewReader([]byte("hello")), 5, "")
	url, err := fs.PresignGet(ctx, "bin/x.unknownext", 0)
	if err != nil || !strings.HasPrefix(url, "data:") {
		t.Fatal(err, url)
	}
	if err := fs.Delete(ctx, "keep/a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Put(ctx, "../x", bytes.NewReader([]byte("x")), 1, ""); err == nil {
		t.Fatal("invalid put")
	}
}

func TestMemoryFileStorePresignDefaultCT(t *testing.T) {
	fs := NewMemoryFileStore()
	ctx := context.Background()
	_, _ = fs.Put(ctx, "x.bin", bytes.NewReader([]byte("z")), 1, "")
	url, err := fs.PresignGet(ctx, "x.bin", 0)
	if err != nil || !strings.Contains(url, "application/octet-stream") {
		t.Fatal(err, url)
	}
}

func TestS3FileStoreHelpersAndNew(t *testing.T) {
	ctx := context.Background()
	if _, err := NewS3FileStore(ctx, Config{Region: "r"}, "p", nil); err == nil {
		t.Fatal("bucket")
	}
	if _, err := NewS3FileStore(ctx, Config{Bucket: "b"}, "p", nil); err == nil {
		t.Fatal("region")
	}
	fs, err := NewS3FileStore(ctx, Config{
		Bucket: "b", Region: "us-east-1", AccessKey: "ak", SecretKey: "sk",
		Endpoint: "http://127.0.0.1:1", ForcePathStyle: true,
	}, "root/pref", nil)
	if err != nil {
		t.Fatalf("NewS3FileStore: %v", err)
	}
	s := fs.(*s3FileStore)
	if s.physical("a") != "root/pref/a" || s.relative("root/pref/a") != "a" {
		t.Fatalf("physical/relative %q %q", s.physical("a"), s.relative("root/pref/a"))
	}
	s.prefix = ""
	if s.physical("a") != "a" || s.relative("a") != "a" {
		t.Fatal("empty prefix")
	}
	if err := s.sanitize(nil); err != nil {
		t.Fatal(err)
	}
	if err := s.sanitize(errors.New("AccessKey=AK&x")); err == nil || strings.Contains(err.Error(), "AK") && !strings.Contains(err.Error(), "***") {
		t.Fatalf("sanitize: %v", err)
	}
	if _, err := s.List(ctx, "../bad", 10); err == nil {
		t.Fatal("invalid list prefix")
	}
	if _, err := s.Put(ctx, "../bad", bytes.NewReader(nil), 0, ""); err == nil {
		t.Fatal("invalid put")
	}
	if err := s.Delete(ctx, "/abs"); err == nil {
		t.Fatal("invalid delete")
	}
	if _, err := s.PresignGet(ctx, "..", 0); err == nil {
		t.Fatal("invalid presign")
	}
}
