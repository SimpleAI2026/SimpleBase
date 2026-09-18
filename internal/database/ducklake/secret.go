package ducklake

import (
	"fmt"
	"strings"
	"unicode"
)

// buildCreateSecretSQL 生成 httpfs S3 SECRET（在 ATTACH 之前执行）。
// 凭据只出现在 boot SQL 中，绝不写入日志（summarizeSQL 截断）。
func buildCreateSecretSQL(remote RemoteStorage) string {
	parts := []string{
		"CREATE OR REPLACE SECRET simplebase_s3 (",
		"TYPE S3",
		", KEY_ID " + quoteSQLString(remote.AccessKey),
		", SECRET " + quoteSQLString(remote.SecretKey),
		", REGION " + quoteSQLString(remote.Region),
	}
	if host := remote.s3EndpointHost(); host != "" {
		parts = append(parts, ", ENDPOINT "+quoteSQLString(host))
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

func buildDataURI(remote RemoteStorage, tenantID, databaseID string) (string, error) {
	kb := remote.keyBuilder()
	uri, err := kb.DuckLakeDataURI(remote.Bucket, tenantID, databaseID)
	if err != nil {
		return "", fmt.Errorf("ducklake: data uri: %w", err)
	}
	return uri, nil
}

// —— SQL 字面量与标识符构造（原 sqlquote.go）——

func quoteSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func sqlPath(p string) string {
	return quoteSQLString(filepathToSlash(p))
}

func isSafeIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}
