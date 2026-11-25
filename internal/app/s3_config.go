// 文件说明：s3_config.go — config.S3Config 到对象存储配置的转换（单一事实来源）
// 用法：app 装配与 reset 共用 s3ConfigFrom；catalog descriptor 用 duckLakeStorageFrom。
// 调用架构：assembleDeps / Reset → s3ConfigFrom → objectstore.NewClient。
// 主要符号：s3ConfigFrom、duckLakeStorageFrom。

package app

import (
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// s3ConfigFrom 把配置里的 S3 段转为对象存储客户端配置。
func s3ConfigFrom(cfg config.Config) objectstore.Config {
	return objectstore.Config{
		Endpoint:       cfg.S3.Endpoint,
		Region:         cfg.S3.Region,
		Bucket:         cfg.S3.Bucket,
		Prefix:         cfg.S3.Prefix,
		AccessKey:      cfg.S3.AccessKey,
		SecretKey:      cfg.S3.SecretKey,
		KMSKeyID:       cfg.S3.KMSKeyID,
		ForcePathStyle: cfg.S3.ForcePathStyle,
	}
}

// duckLakeStorageFrom 生成写入 descriptor 的 DuckLake 存储描述。
// 只带密钥引用，不含凭据；Prefix 由 KeyBuilder.DataPrefix 按库生成，此处刻意留空。
func duckLakeStorageFrom(cfg config.Config) objectstore.DuckLakeStorage {
	return objectstore.DuckLakeStorage{
		Endpoint:       cfg.S3.Endpoint,
		Region:         cfg.S3.Region,
		Bucket:         cfg.S3.Bucket,
		ForcePathStyle: cfg.S3.ForcePathStyle,
		KMSKeyIDRef:    cfg.S3.KMSKeyID,
	}
}
