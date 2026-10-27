// 文件说明：secret.go — godatabase/ducklake 的内部实现（buildCreateSecretSQL、buildDataURI 等）
//
// 用法
//
//	同包调用：sqlPath() → catalog_syncer.go, factory.go；buildCreateSecretSQL() → factory.go；buildDataURI() → factory.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/app, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/ducklake/secret.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/sqlx
//	[同包] catalog_syncer.go, errors.go, extension.go, factory.go, gated_conn.go 等 14 个
//	[关键引用] sqlPath() ← catalog_syncer.go, factory.go
//
// 主要符号
//
//	buildCreateSecretSQL(remote RemoteStorage) string
//	buildDataURI(remote RemoteStorage, projectID, databaseID string) (string, error)
//	sqlPath(p string) string

package ducklake

import (
	"fmt"

	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
)

// buildCreateSecretSQL 生成 httpfs S3 SECRET（在 ATTACH 之前执行）。
// 凭据只出现在 boot SQL 中，绝不写入日志（summarizeSQL 截断）。
func buildCreateSecretSQL(remote RemoteStorage) string {
	parts := []string{
		"CREATE OR REPLACE SECRET simplebase_s3 (",
		"TYPE S3",
		", KEY_ID " + sqlx.Literal(remote.AccessKey),
		", SECRET " + sqlx.Literal(remote.SecretKey),
		", REGION " + sqlx.Literal(remote.Region),
	}
	if host := remote.s3EndpointHost(); host != "" {
		parts = append(parts, ", ENDPOINT "+sqlx.Literal(host))
	}
	if remote.ForcePathStyle {
		parts = append(parts, ", URL_STYLE 'path'")
	}
	if remote.endpointUsesSSL() {
		parts = append(parts, ", USE_SSL true")
	} else {
		parts = append(parts, ", USE_SSL false")
	}
	parts = append(parts, ")")
	out := ""
	for i, p := range parts {
		if i == 0 {
			out = p
			continue
		}
		if p == ")" {
			out += p
			continue
		}
		out += "\n" + p
	}
	return out
}

func buildDataURI(remote RemoteStorage, projectID, databaseID string) (string, error) {
	kb := remote.keyBuilder()
	uri, err := kb.DuckLakeDataURI(remote.Bucket, projectID, databaseID)
	if err != nil {
		return "", fmt.Errorf("ducklake: data uri: %w", err)
	}
	return uri, nil
}

// —— SQL 字面量与标识符构造（原 sqlquote.go）——

func sqlPath(p string) string {
	return sqlx.Literal(filepathToSlash(p))
}
