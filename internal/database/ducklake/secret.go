package ducklake

import (
	"fmt"

	"github.com/linkxzhou/SimpleBase/internal/utils/sqlx"
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
