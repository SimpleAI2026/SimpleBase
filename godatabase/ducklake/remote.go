// 文件说明：remote.go — godatabase/ducklake 中 RemoteStorage 等符号的实现
//
// 用法
//
//	type RemoteStorage struct{…}  // RemoteStorage 描述 S3 持久层
//
// 调用架构
//
//	[上游] internal/app, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/ducklake/remote.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] c4_syncer_lease_test.go, c4_syncer_ordering_test.go, catalog_syncer.go, catalog_syncer_test.go, coverage_test.go 等 35 个
//	[关键引用] RemoteStorage ← catalog_syncer.go, catalog_syncer_test.go
//
// 主要符号
//
//	type RemoteStorage struct{…}
//	(r RemoteStorage) keyBuilder() blob.KeyBuilder
//	(r RemoteStorage) validate() error
//	(r RemoteStorage) s3EndpointHost() string
//	(r RemoteStorage) endpointUsesSSL() bool

package ducklake

import (
	"fmt"
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase/blob"
)

// RemoteStorage 描述 S3 持久层。Enabled=false 时走本地 DATA_PATH。
type RemoteStorage struct {
	Enabled        bool
	Endpoint       string
	Region         string
	Bucket         string
	RootPrefix     string
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool
	UseSSL         bool
}

func (r RemoteStorage) keyBuilder() blob.KeyBuilder {
	// RootPrefix 为空表示桶根目录；不再为环境名追加一层。
	return blob.KeyBuilder{RootPrefix: r.RootPrefix}
}

func (r RemoteStorage) validate() error {
	if !r.Enabled {
		return nil
	}
	if r.Bucket == "" {
		return fmt.Errorf("ducklake: remote bucket is required")
	}
	if r.Region == "" {
		return fmt.Errorf("ducklake: remote region is required")
	}
	return nil
}

func (r RemoteStorage) s3EndpointHost() string {
	ep := strings.TrimSpace(r.Endpoint)
	ep = strings.TrimPrefix(ep, "https://")
	ep = strings.TrimPrefix(ep, "http://")
	ep = strings.TrimSuffix(ep, "/")
	return ep
}

func (r RemoteStorage) endpointUsesSSL() bool {
	ep := strings.TrimSpace(r.Endpoint)
	if strings.HasPrefix(ep, "http://") {
		return false
	}
	if strings.HasPrefix(ep, "https://") {
		return true
	}
	if ep == "" {
		return true
	}
	return r.UseSSL
}
