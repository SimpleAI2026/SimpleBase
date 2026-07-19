package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// fakeClient 是内存版 Client，用于测试 descriptor put/get 与错误映射。
type fakeClient struct {
	store map[string][]byte
	// failOn 决定对某 op 返回 err，用于测试错误路径
	failOn map[string]error
}

func newFakeClient() *fakeClient {
	return &fakeClient{store: map[string][]byte{}, failOn: map[string]error{}}
}

func (f *fakeClient) PutJSON(ctx context.Context, key string, value any, opts PutOptions) error {
	if err, ok := f.failOn["put"]; ok {
		return err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.store[key] = b
	return nil
}

func (f *fakeClient) GetJSON(ctx context.Context, key string, dst any) error {
	if err, ok := f.failOn["get"]; ok {
		return err
	}
	b, ok := f.store[key]
	if !ok {
		return ErrNotFound
	}
	return json.Unmarshal(b, dst)
}

func (f *fakeClient) Head(ctx context.Context, key string) (ObjectInfo, error) {
	if err, ok := f.failOn["head"]; ok {
		return ObjectInfo{}, err
	}
	b, ok := f.store[key]
	if !ok {
		return ObjectInfo{}, ErrNotFound
	}
	return ObjectInfo{Key: key, Size: int64(len(b)), LastModified: time.Now()}, nil
}

func (f *fakeClient) DeletePrefix(ctx context.Context, prefix string) error {
	if err, ok := f.failOn["delete"]; ok {
		return err
	}
	for k := range f.store {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(f.store, k)
		}
	}
	return nil
}

func (f *fakeClient) Check(ctx context.Context) error {
	if err, ok := f.failOn["check"]; ok {
		return err
	}
	return nil
}

func TestDescriptorPutGetRoundTrip(t *testing.T) {
	c := newFakeClient()
	kb := KeyBuilder{RootPrefix: "simplebase", Environment: "test"}

	desc := &Descriptor{
		FormatVersion: DescriptorFormatVersion,
		TenantID:      "11111111-1111-1111-1111-111111111111",
		ProjectID:     "22222222-2222-2222-2222-222222222222",
		DatabaseID:    "33333333-3333-3333-3333-333333333333",
		Name:          "mydb",
		CreatedAt:     time.Now().UTC(),
		TursoStorage: TursoStorage{
			Region: "us-east-1",
			Bucket: "sb-test",
			Prefix: "simplebase/test/tenants/11111111-1111-1111-1111-111111111111/databases/33333333-3333-3333-3333-333333333333/data",
		},
		DataPrefix: "simplebase/test/tenants/11111111-1111-1111-1111-111111111111/databases/33333333-3333-3333-3333-333333333333/data",
		Status:     StatusCreating,
	}
	if err := desc.Validate(); err != nil {
		t.Fatalf("validate descriptor: %v", err)
	}

	key, err := kb.DescriptorKey(desc.TenantID, desc.DatabaseID)
	if err != nil {
		t.Fatalf("descriptor key: %v", err)
	}
	if err := c.PutJSON(context.Background(), key, desc, PutOptions{}); err != nil {
		t.Fatalf("put descriptor: %v", err)
	}

	var got Descriptor
	if err := c.GetJSON(context.Background(), key, &got); err != nil {
		t.Fatalf("get descriptor: %v", err)
	}
	if got.DatabaseID != desc.DatabaseID {
		t.Errorf("database id mismatch: got %s want %s", got.DatabaseID, desc.DatabaseID)
	}
	if got.DataPrefix != desc.DataPrefix {
		t.Errorf("data prefix mismatch: got %s want %s", got.DataPrefix, desc.DataPrefix)
	}
	if got.Status != StatusCreating {
		t.Errorf("status mismatch: got %s want %s", got.Status, StatusCreating)
	}
}

func TestGetJSONNotFound(t *testing.T) {
	c := newFakeClient()
	var dst Descriptor
	err := c.GetJSON(context.Background(), "missing-key", &dst)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestHeadNotFound(t *testing.T) {
	c := newFakeClient()
	_, err := c.Head(context.Background(), "missing-key")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeletePrefix(t *testing.T) {
	c := newFakeClient()
	// 放入两个 descriptor key，一个属于 db A，一个属于 db B
	c.store["simplebase/test/tenants/t/databases/a/descriptor.json"] = []byte("{}")
	c.store["simplebase/test/tenants/t/databases/b/descriptor.json"] = []byte("{}")
	if err := c.DeletePrefix(context.Background(), "simplebase/test/tenants/t/databases/a"); err != nil {
		t.Fatalf("delete prefix: %v", err)
	}
	if _, ok := c.store["simplebase/test/tenants/t/databases/a/descriptor.json"]; ok {
		t.Error("a descriptor should be deleted")
	}
	if _, ok := c.store["simplebase/test/tenants/t/databases/b/descriptor.json"]; !ok {
		t.Error("b descriptor should remain")
	}
}

func TestAssertHealthNil(t *testing.T) {
	if err := AssertHealth(context.Background(), nil); err == nil {
		t.Error("expected error for nil client")
	}
}

func TestAssertHealthOK(t *testing.T) {
	c := newFakeClient()
	if err := AssertHealth(context.Background(), c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSanitizeErrRedactsKeys(t *testing.T) {
	r := redactor{}
	out := r.cleanString("AccessKey=AKIAFAKE12345&SecretKey=FAKESECRET&X-Amz-Signature=abc123")
	if containsAny(out, "AKIAFAKE12345", "FAKESECRET", "abc123") {
		t.Errorf("redaction failed: %s", out)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) && indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
