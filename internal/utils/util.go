package utils

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func IsDir(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

func RaftPort(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	p, err := StrconvAtoi(port)
	if err != nil {
		return 0
	}
	return p
}

func StrconvAtoi(s string) (int, error) {
	var n int
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// IsZipFile checks if the file path has a zip extension.
func IsZipFile(path string) bool {
	p := strings.ToLower(path)
	return strings.HasSuffix(p, ".zip")
}

// IsTarGzipFile checks if the file path has a tar.gz or tgz extension.
func IsTarGzipFile(path string) bool {
	p := strings.ToLower(path)
	return strings.HasSuffix(p, ".tar.gz") || strings.HasSuffix(p, ".tgz")
}

// CheckDbName 校验数据库名是否合法
func CheckDbName(name string) error {
	if strings.Contains(name, " ") {
		return fmt.Errorf("db name dont allow `Space`")
	}
	if strings.Contains(name, "\"") ||
		strings.Contains(name, "*") ||
		strings.Contains(name, "`") ||
		strings.Contains(name, "<") ||
		strings.Contains(name, ">") ||
		strings.Contains(name, "?") ||
		strings.Contains(name, "\\") ||
		strings.Contains(name, "|") ||
		strings.Contains(name, ":") {
		return fmt.Errorf("db name contains invalid characters")
	}
	return nil
}

// ValidSQLiteFile 检查字节流是否为合法的 SQLite 文件头
func ValidSQLiteFile(b []byte) bool {
	return len(b) > 13 && string(b[0:13]) == "SQLite format"
}

// ContextTimeout 从 context 推导超时时长
func ContextTimeout(ctx context.Context) time.Duration {
	if dl, ok := ctx.Deadline(); ok {
		return dl.Sub(time.Now())
	}
	return 0
}

// ComputeCoastTime 计算从 t 到当前时间的耗时（毫秒）
func ComputeCoastTime(t time.Time) float64 {
	return float64(time.Since(t).Nanoseconds()) / 1e6
}

// GetSqliteTypeFromKindName 将 Go 类型名映射为 SQLite 类型名
func GetSqliteTypeFromKindName(kind string) string {
	typeName, ok := sqliteKindMap[kind]
	if !ok {
		return ""
	}
	return typeName
}

var sqliteKindMap = map[string]string{
	"bool":    "real",
	"int":     "integer",
	"int8":    "integer",
	"int16":   "integer",
	"int32":   "integer",
	"int64":   "integer",
	"uint":    "integer",
	"uint8":   "integer",
	"uint16":  "integer",
	"uint32":  "integer",
	"uint64":  "integer",
	"float32": "number",
	"float64": "number",
	"string":  "text",
}
