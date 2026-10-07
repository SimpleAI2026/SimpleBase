package objectstore

// COS 的 DeleteObjects 强制要求 Content-MD5；验证线上请求带了正确的头。

import (
	"context"
	"crypto/md5" //nolint:gosec // 测试对照 Content-MD5
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeleteManySendsContentMD5(t *testing.T) {
	var gotMD5 string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMD5 = r.Header.Get("Content-MD5")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><DeleteResult></DeleteResult>`))
	}))
	defer srv.Close()

	c, err := NewClient(context.Background(), Config{
		Endpoint:       srv.URL,
		Region:         "ap-guangzhou",
		Bucket:         "b-123",
		AccessKey:      "ak",
		SecretKey:      "sk",
		ForcePathStyle: true,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.(BlobStore).DeleteMany(context.Background(), []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if gotMD5 == "" {
		t.Fatal("DeleteObjects 缺少 Content-MD5 头")
	}
	sum := md5.Sum(gotBody) //nolint:gosec
	if want := base64.StdEncoding.EncodeToString(sum[:]); gotMD5 != want {
		t.Fatalf("Content-MD5 = %q, want %q (body %q)", gotMD5, want, gotBody)
	}
}
