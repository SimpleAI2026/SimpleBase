// 文件说明：delete_md5_test.go — internal/objectstore 的单元测试（TestDeleteManySendsContentMD5 等）
//
// 用法
//
//	go test ./internal/objectstore -run 'TestDeleteManySendsContentMD5'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/api, internal/app, internal/backup 等 6 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/objectstore/delete_md5_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] blob.go, blob_test.go, branches_test.go, client.go, client_test.go 等 24 个
//
// 主要符号
//
//	TestDeleteManySendsContentMD5(t *testing.T)

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
