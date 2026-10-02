package objectstore

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPutIfAbsentFirstWinsSecondConflicts(t *testing.T) {
	store := NewMemoryBlobStore()
	ctx := context.Background()

	if _, err := store.PutIfAbsent(ctx, "a/b.bin", []byte("v1"), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutIfAbsent(ctx, "a/b.bin", []byte("v2"), "application/octet-stream"); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("want ErrPreconditionFailed, got %v", err)
	}
	// 已存在的对象不受影响。
	data, _, err := store.GetBytes(ctx, "a/b.bin")
	if err != nil || string(data) != "v1" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

func TestMapPreconditionErr(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"operation error S3: PutObject, https response error StatusCode: 412, PreconditionFailed: ...", true},
		{"operation error S3: PutObject, https response error StatusCode: 409, ObjectAlreadyExists: ...", true},
		{"api error PreconditionFailed: at least one of the pre-conditions you specified did not hold", true},
		{"some other error NoSuchKey", false},
		{"", false},
	}
	for _, c := range cases {
		err := errorStr(c.in)
		got := errors.Is(mapPreconditionErr(err), ErrPreconditionFailed)
		if got != c.want {
			t.Fatalf("mapPreconditionErr(%q) conflict=%v want %v", c.in, got, c.want)
		}
	}
}

type errorStr string

func (e errorStr) Error() string { return string(e) }

func TestProbePutIfAbsentPassesOnRealStore(t *testing.T) {
	store := NewMemoryBlobStore()
	res := ProbePutIfAbsent(context.Background(), store, "sb/env/catalog")
	if !res.CASSupported {
		t.Fatalf("memory store must pass probe: %v", res.Err)
	}
}

// overwriteStore 忽略 create-if-absent：模拟 COS 版本控制失效。
type overwriteStore struct{ memoryBlobStore }

func (o *overwriteStore) PutIfAbsent(ctx context.Context, key string, data []byte, contentType string) (ObjectInfo, error) {
	_ = o.memoryBlobStore.PutBytes(ctx, key, data, contentType)
	return o.memoryBlobStore.Head(ctx, key)
}

func TestProbePutIfAbsentFailsOnOverwriteStore(t *testing.T) {
	store := &overwriteStore{memoryBlobStore{
		store: map[string][]byte{},
		meta:  map[string]time.Time{},
	}}
	res := ProbePutIfAbsent(context.Background(), store, "sb/env/catalog")
	if res.CASSupported {
		t.Fatal("probe must fail when endpoint allows overwrite")
	}
	if res.Err == nil {
		t.Fatal("probe must report the failure reason")
	}
}
