// 文件说明：cos_header_verify_test.go — internal/objectstore 的单元测试（TestCOSForbidOverwriteHeaderOnWire 等）
//
// 用法
//
//	go test ./internal/objectstore -run 'TestCOSForbidOverwriteHeaderOnWire'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/api, internal/app, internal/backup 等 6 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/objectstore/cos_header_verify_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] blob.go, blob_test.go, branches_test.go, client.go, client_test.go 等 24 个
//
// 主要符号
//
//	TestCOSForbidOverwriteHeaderOnWire(t *testing.T)

package objectstore

// 验证 COS 分支 PutIfAbsent 在真实 HTTP 线上发出的头：
// x-cos-forbid-overwrite 必须是独立请求头；若经由 SDK Metadata 发送，
// 线上实际会变成 x-amz-meta-x-cos-forbid-overwrite，COS 不识别（探针失败的根因）。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCOSForbidOverwriteHeaderOnWire(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("ETag", `"abc"`)
		w.WriteHeader(http.StatusOK)
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
	// 把判定用 endpoint 换成 COS 域名以触发 COS 分支；
	// 真实请求仍发往 httptest（endpoint 字段不参与路由）。
	sc := c.(*s3Client)
	sc.endpoint = "https://b-123.cos.ap-guangzhou.myqcloud.com"

	if _, err := c.(BlobStore).PutIfAbsent(context.Background(), "k", []byte("v"), "text/plain"); err != nil {
		t.Fatal(err)
	}
	t.Logf("wire headers: x-cos-forbid-overwrite=%q x-amz-meta-x-cos-forbid-overwrite=%q if-none-match=%q",
		got.Get("x-cos-forbid-overwrite"), got.Get("x-amz-meta-x-cos-forbid-overwrite"), got.Get("If-None-Match"))
	if got.Get("x-cos-forbid-overwrite") != "true" {
		t.Fatalf("x-cos-forbid-overwrite 必须作为独立请求头发送；实际收到 %q（被 SDK 包装进 x-amz-meta-*: %q）",
			got.Get("x-cos-forbid-overwrite"), got.Get("x-amz-meta-x-cos-forbid-overwrite"))
	}
}
