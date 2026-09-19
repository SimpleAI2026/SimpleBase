package objectstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type memS3 struct {
	mu   sync.Mutex
	data map[string][]byte
	fail string
}

func (m *memS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.fail != "" {
		http.Error(w, m.fail, http.StatusInternalServerError)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/b/")
	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("list-type") == "2" || r.URL.Query().Has("prefix") || r.URL.RawQuery != "" {
			prefix := r.URL.Query().Get("prefix")
			m.mu.Lock()
			var body strings.Builder
			body.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
			body.WriteString("<Name>b</Name><IsTruncated>false</IsTruncated>")
			for k, v := range m.data {
				if prefix != "" && !strings.HasPrefix(k, prefix) {
					continue
				}
				fmt.Fprintf(&body, "<Contents><Key>%s</Key><Size>%d</Size><LastModified>2020-01-01T00:00:00.000Z</LastModified></Contents>", k, len(v))
			}
			body.WriteString("</ListBucketResult>")
			m.mu.Unlock()
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(body.String()))
			return
		}
	case http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		if m.data == nil {
			m.data = map[string][]byte{}
		}
		m.data[key] = b
		m.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodHead:
		m.mu.Lock()
		b, ok := m.data[key]
		m.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(b)))
		w.Header().Set("Last-Modified", "Wed, 01 Jan 2020 00:00:00 GMT")
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodDelete:
		m.mu.Lock()
		delete(m.data, key)
		m.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func newTestS3FileStore(t *testing.T, backend *memS3) *s3FileStore {
	t.Helper()
	srv := httptest.NewServer(backend)
	t.Cleanup(srv.Close)
	client := s3.New(s3.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(srv.URL),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider("ak", "sk", ""),
		HTTPClient:   srv.Client(),
	})
	return &s3FileStore{client: client, bucket: "b", prefix: "root"}
}

func TestS3FileStoreCRUDViaFakeHTTP(t *testing.T) {
	ctx := context.Background()
	backend := &memS3{data: map[string][]byte{}}
	fs := newTestS3FileStore(t, backend)

	body := []byte("hello")
	obj, err := fs.Put(ctx, "docs/a.txt", bytes.NewReader(body), int64(len(body)), "text/plain")
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if obj.Key != "docs/a.txt" {
		t.Fatalf("key %s", obj.Key)
	}

	// Head failure after successful put still returns object.
	fs2 := newTestS3FileStore(t, &memS3{fail: "head-down"})
	// Put will fail entirely because PutObject hits the same backend.
	if _, err := fs2.Put(ctx, "x.txt", bytes.NewReader([]byte("x")), 1, ""); err == nil {
		t.Fatal("expected put error")
	}

	objs, err := fs.List(ctx, "docs/", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(objs) != 1 || objs[0].Key != "docs/a.txt" {
		t.Fatalf("list %+v", objs)
	}
	if _, err := fs.List(ctx, "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.List(ctx, "", 2000); err != nil {
		t.Fatal(err)
	}

	url, err := fs.PresignGet(ctx, "docs/a.txt", 0)
	if err != nil || url == "" {
		t.Fatalf("presign: %v %s", err, url)
	}
	if err := fs.Delete(ctx, "docs/a.txt"); err != nil {
		t.Fatal(err)
	}

	bad := newTestS3FileStore(t, &memS3{fail: "down"})
	if _, err := bad.List(ctx, "", 1); err == nil {
		t.Fatal("list error")
	}
	if err := bad.Delete(ctx, "x.txt"); err == nil {
		t.Fatal("delete error")
	}
}

func TestS3FileStorePutHeadFallback(t *testing.T) {
	// Custom handler: Put succeeds, Head fails.
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
		case http.MethodHead:
			http.Error(w, "no", http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	fs := &s3FileStore{
		client: s3.New(s3.Options{
			Region: "us-east-1", BaseEndpoint: aws.String(srv.URL), UsePathStyle: true,
			Credentials: credentials.NewStaticCredentialsProvider("ak", "sk", ""),
			HTTPClient:  srv.Client(),
		}),
		bucket: "b",
	}
	obj, err := fs.Put(context.Background(), "only.txt", bytes.NewReader([]byte("z")), 1, "text/plain")
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if obj.Key != "only.txt" || obj.Size != 1 {
		t.Fatalf("%+v", obj)
	}
}

func TestDownloadFileErrorAndMemoryPutRead(t *testing.T) {
	ctx := context.Background()
	c := testS3Client(&fakeS3API{getBody: []byte("x")})
	// dest is a directory name that cannot be a file: use a path whose parent is a file
	parent := t.TempDir()
	fileAsDir := parent + "/notdir"
	if err := os.WriteFile(fileAsDir, []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DownloadFile(ctx, "k", fileAsDir+"/child"); err == nil {
		t.Fatal("expected mkdir fail")
	}

	m := NewMemoryBlobStore()
	_ = m.PutBytes(ctx, "k", []byte("v"), "")
	if _, err := m.DownloadFile(ctx, "k", fileAsDir+"/child"); err == nil {
		t.Fatal("memory download mkdir fail")
	}

	fs := NewMemoryFileStore()
	if _, err := fs.Put(ctx, "a.txt", errReader{}, 1, ""); err == nil {
		t.Fatal("put read error")
	}
	lfs, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lfs.Put(ctx, "a.txt", errReader{}, 1, ""); err == nil {
		t.Fatal("local put read")
	}
}

func TestDuckLakeKeyErrors(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "r", Environment: "e"}
	if _, err := kb.DuckLakeCatalogKey("bad", "33333333-3333-3333-3333-333333333333"); err == nil {
		t.Fatal("catalog key")
	}
	if _, err := kb.DuckLakeCatalogVersionKey("bad", "33333333-3333-3333-3333-333333333333", 1); err == nil {
		t.Fatal("version key")
	}
	if _, err := kb.DuckLakeDataURI("b", "bad", "33333333-3333-3333-3333-333333333333"); err == nil {
		t.Fatal("data uri")
	}
}

func TestValidateIDUpperHex(t *testing.T) {
	if err := validateID("id", "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"); err != nil {
		t.Fatal(err)
	}
}
