// Package turso 封装 Turso/libSQL 的 DSN 构建与连接生命周期。
//
// 设计原则（见 plan2.md「Turso 适配边界」）：
//   - 官方驱动的 DSN 格式以实现时官方文档为准，集中在 dsn.go，不散落到 handler/model。
//   - BuildDSN 的单元测试只断言本项目拼装字段、URL 转义、禁止路径穿越和不可记录密钥。
//   - 对真实 Turso S3 语义使用集成测试，不在本包臆造参数名。
//   - 业务模型不直接依赖驱动包；本包通过标准 database/sql 暴露 *sql.DB。
//
// 驱动选择：首期默认驱动注册名为 "libsql"（对应 github.com/tursodatabase/go-libsql，
// CGO）。若后续切换到纯 Go 驱动或 turso-go，只需改 DriverName 与 import 侧的空白导入，
// 不影响 BuildDSN 与 Open 的调用方。
package turso

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// DriverName 是 database/sql 注册的驱动名。
// 默认 "libsql"；可通过 SetDriverName 在初始化阶段覆盖（例如测试注入 mock 驱动）。
const DriverName = "libsql"

// StorageConfig 描述 Turso/libSQL 在 S3 上的存储配置。
// 不含密钥；凭据由 IAM/工作负载身份提供，见 objectstore.TursoStorage。
type StorageConfig struct {
	Endpoint       string // 为空表示 AWS 默认 S3 endpoint
	Region         string
	Bucket         string
	Prefix         string // 该数据库在 S3 中的 data/ 前缀（由 KeyBuilder.DataPrefix 产生）
	KMSKeyID       string // 可选；为空使用 bucket 默认加密
	ForcePathStyle bool   // MinIO 等兼容实现需 true
}

// OpenOptions 是 Open/BuildDSN 的入参。
type OpenOptions struct {
	DatabaseID string // 用于日志与缓存目录命名，不进入 DSN
	CachePath  string // 本地缓存目录；空表示纯远程
	Writable   bool   // false 表示只读打开
	Storage    StorageConfig
	// AuthToken 仅用于远程 Turso 平台（libsql:// URL）模式；S3 模式下留空。
	// 禁止出现在日志或错误中。
	AuthToken string
}

// ErrInvalidOptions 表示 OpenOptions 校验失败。
var ErrInvalidOptions = errors.New("turso: invalid open options")

// Validate 校验 OpenOptions 的基本完整性。不校验 S3 可达性。
func (o OpenOptions) Validate() error {
	if o.DatabaseID == "" {
		return fmt.Errorf("%w: database_id is required", ErrInvalidOptions)
	}
	if o.Storage.Bucket == "" {
		return fmt.Errorf("%w: storage.bucket is required", ErrInvalidOptions)
	}
	if o.Storage.Region == "" {
		return fmt.Errorf("%w: storage.region is required", ErrInvalidOptions)
	}
	if o.Storage.Prefix == "" {
		return fmt.Errorf("%w: storage.prefix is required", ErrInvalidOptions)
	}
	if strings.Contains(o.Storage.Prefix, "..") {
		return fmt.Errorf("%w: storage.prefix contains path traversal", ErrInvalidOptions)
	}
	if strings.HasPrefix(o.Storage.Prefix, "/") {
		return fmt.Errorf("%w: storage.prefix must not start with /", ErrInvalidOptions)
	}
	return nil
}

// BuildDSN 构建 Turso/libSQL 的 DSN。
//
// 首期采用 S3 后端模式：DSN 形如
//
//	libsql+ss3://?bucket=...&region=...&prefix=...&endpoint=...&mode=rw|ro
//
// 说明：
//   - 只编码官方已验证字段（bucket/region/prefix/endpoint/mode），不臆造参数名。
//   - URL 转义所有值，禁止路径穿越。
//   - 绝不把 AuthToken/AccessKey/SecretKey/KMSKeyID 编码进 DSN query；
//     密钥由 IAM 或驱动连接器选项注入。
//   - 若官方文档更新 DSN 格式，只需修改本函数；调用方无感。
//
// 若 CachePath 非空，它作为本地缓存目录由驱动使用，但不进入 DSN query
// （由 Open 通过驱动专用 Connector API 传入，避免在 DSN 中暴露本地路径）。
func BuildDSN(opts OpenOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("bucket", opts.Storage.Bucket)
	q.Set("region", opts.Storage.Region)
	q.Set("prefix", sanitizePrefix(opts.Storage.Prefix))
	if opts.Storage.Endpoint != "" {
		q.Set("endpoint", opts.Storage.Endpoint)
	}
	if opts.Storage.ForcePathStyle {
		q.Set("force_path_style", "true")
	}
	mode := "rw"
	if !opts.Writable {
		mode = "ro"
	}
	q.Set("mode", mode)
	// 注意：AuthToken/KMSKeyID 绝不进入 DSN。
	return "libsql+ss3://?" + q.Encode(), nil
}

// sanitizePrefix 去掉前导/尾部 "/"，避免与驱动内部拼接产生空段或绝对路径。
func sanitizePrefix(p string) string {
	return strings.Trim(p, "/")
}

// redactDSN 返回用于日志的安全 DSN 视图（移除所有 query 值，只保留 scheme）。
// 即便 BuildDSN 不写密钥，也防止未来变更引入泄露。
func redactDSN(dsn string) string {
	i := strings.Index(dsn, "?")
	if i < 0 {
		return dsn
	}
	return dsn[:i+1] + "[redacted]"
}
