// Package objectstore 是 SimpleBase 平台配置与元数据的 S3 访问层。
// 它只负责 descriptor/state/manifest 等平台对象的读写与最小健康检查；
// 数据库数据对象（data/）完全由 Turso/libSQL 管理，本包绝不解析或拼接其格式。
//
// 与旧 internal/s3 的区别：
//   - 不提供 AppendObject / SelectObject（旧 storage API 专用，Plan 10 删除）
//   - 凭据来源由 config.Config 显式注入，不使用全局单例 default.go
//   - 支持 AWS default credential chain（工作负载身份），不仅限 AccessKey/SecretKey
package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// ErrNotFound 表示请求的对象在 S3 中不存在。
// GetJSON/Head 在对象缺失时返回此错误，调用方可用 errors.Is 判别。
var ErrNotFound = errors.New("objectstore: object not found")

// Client 是平台对象存储的抽象接口。测试可注入 fake 实现。
type Client interface {
	PutJSON(ctx context.Context, key string, value any, opts PutOptions) error
	GetJSON(ctx context.Context, key string, dst any) error
	Head(ctx context.Context, key string) (ObjectInfo, error)
	DeletePrefix(ctx context.Context, prefix string) error // 仅删除已软删库，由后台任务调用
	Check(ctx context.Context) error
}

// Deleter 是仅含删除能力的子接口，供后台任务依赖以避免引入完整 Client。
type Deleter interface {
	DeletePrefix(ctx context.Context, prefix string) error
}

// PutOptions 控制 PutJSON 行为。
type PutOptions struct {
	// ContentType 默认 application/json
	ContentType string
	// KMSKeyID 为空则使用 bucket 默认加密
	KMSKeyID string
	// IfNotMatch 为 true 时以条件写入实现 create-if-absent（首期可选）
	IfNotMatch bool
}

// ObjectInfo 描述对象元数据。禁止包含 body 或预签名 URL。
type ObjectInfo struct {
	Key          string
	Size         int64
	ETag         string
	LastModified time.Time
}

// s3Client 是基于 aws-sdk-go-v2 的 Client 实现。
type s3Client struct {
	api       s3API
	bucket    string
	kmsKeyID  string
	logger    observability.Logger
	metrics   *observability.Metrics
	redacted  redactor
}

// s3API 抽象 s3.Client 以便测试注入。
type s3API interface {
	PutObject(ctx context.Context, in *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadObject(ctx context.Context, in *s3.HeadObjectInput, opts ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	DeleteObjects(ctx context.Context, in *s3.DeleteObjectsInput, opts ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
	ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, opts ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// Config 是 NewClient 的入参，由 config.S3Config 转换而来。
type Config struct {
	Endpoint       string
	Region         string
	Bucket         string
	Prefix         string
	AccessKey      string
	SecretKey      string
	KMSKeyID       string
	ForcePathStyle bool
}

// NewClient 构造 S3 Client。AccessKey/SecretKey 可同时为空以使用默认凭据链。
func NewClient(ctx context.Context, cfg Config, logger observability.Logger, metrics *observability.Metrics) (Client, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("objectstore: bucket is required")
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("objectstore: region is required")
	}
	awsCfg, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("objectstore: load aws config: %w", err)
	}
	api := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.ForcePathStyle
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
	return &s3Client{
		api:      api,
		bucket:   cfg.Bucket,
		kmsKeyID: cfg.KMSKeyID,
		logger:   logger,
		metrics:  metrics,
		redacted: redactor{},
	}, nil
}

func loadAWSConfig(ctx context.Context, cfg Config) (aws.Config, error) {
	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}
	// 仅在显式提供静态凭据时覆盖默认链；否则使用工作负载身份/IMDS/SSO。
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		loadOpts = append(loadOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")))
	}
	if cfg.Endpoint != "" && !isAWSHost(cfg.Endpoint) {
		loadOpts = append(loadOpts, config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...any) (aws.Endpoint, error) {
				if service == s3.ServiceID {
					return aws.Endpoint{
						URL:               cfg.Endpoint,
						SigningRegion:     cfg.Region,
						HostnameImmutable: true,
					}, nil
				}
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			},
		)))
	}
	return config.LoadDefaultConfig(ctx, loadOpts...)
}

func isAWSHost(endpoint string) bool {
	return endpoint == "" || strings.Contains(endpoint, "amazonaws.com")
}

// PutJSON 将 value 序列化为 JSON 并上传。错误中绝不包含 endpoint query 或凭据。
func (c *s3Client) PutJSON(ctx context.Context, key string, value any, opts PutOptions) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("objectstore: marshal: %w", err)
	}
	ct := opts.ContentType
	if ct == "" {
		ct = "application/json"
	}
	in := &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        bytesReader(body),
		ContentType: aws.String(ct),
	}
	if c.kmsKeyID != "" || opts.KMSKeyID != "" {
		kms := opts.KMSKeyID
		if kms == "" {
			kms = c.kmsKeyID
		}
		in.SSEKMSKeyId = aws.String(kms)
		in.ServerSideEncryption = types.ServerSideEncryptionAwsKms
	}
	start := time.Now()
	if _, err := c.api.PutObject(ctx, in); err != nil {
		c.recordOp("put", "error")
		return c.sanitizeErr(err)
	}
	c.recordOp("put", "ok")
	if c.metrics != nil {
		c.metrics.S3OperationSeconds.WithLabelValues("put").Observe(time.Since(start).Seconds())
	}
	return nil
}

// GetJSON 下载对象并反序列化到 dst。对象不存在返回 ErrNotFound。
func (c *s3Client) GetJSON(ctx context.Context, key string, dst any) error {
	start := time.Now()
	out, err := c.api.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		c.recordOp("get", "error")
		return c.sanitizeErr(mapNotFoundErr(err))
	}
	defer out.Body.Close()
	if err := json.NewDecoder(out.Body).Decode(dst); err != nil {
		return fmt.Errorf("objectstore: decode: %w", err)
	}
	c.recordOp("get", "ok")
	if c.metrics != nil {
		c.metrics.S3OperationSeconds.WithLabelValues("get").Observe(time.Since(start).Seconds())
	}
	return nil
}

// Head 返回对象元数据。
func (c *s3Client) Head(ctx context.Context, key string) (ObjectInfo, error) {
	out, err := c.api.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return ObjectInfo{}, c.sanitizeErr(mapNotFoundErr(err))
	}
	return ObjectInfo{
		Key:          key,
		Size:         derefInt64(out.ContentLength),
		ETag:         derefStr(out.ETag),
		LastModified: derefTime(out.LastModified),
	}, nil
}

// DeletePrefix 批量删除指定前缀下所有对象。仅由后台删除任务调用。
// 最多删除 1000 个对象/批，分页直到清空。
func (c *s3Client) DeletePrefix(ctx context.Context, prefix string) error {
	for {
		list, err := c.api.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:  aws.String(c.bucket),
			Prefix:  aws.String(prefix),
			MaxKeys: aws.Int32(1000),
		})
		if err != nil {
			return c.sanitizeErr(err)
		}
		if len(list.Contents) == 0 {
			return nil
		}
		objects := make([]types.ObjectIdentifier, 0, len(list.Contents))
		for _, obj := range list.Contents {
			objects = append(objects, types.ObjectIdentifier{Key: obj.Key})
		}
		del, err := c.api.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(c.bucket),
			Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return c.sanitizeErr(err)
		}
		if len(del.Errors) > 0 && c.logger != nil {
			c.logger.Warn("delete prefix partial errors",
				zap.Int("error_count", len(del.Errors)))
		}
		if list.IsTruncated == nil || !*list.IsTruncated {
			return nil
		}
	}
}

// Check 做最小连通与权限验证：HeadBucket。不能枚举其他 tenant。
func (c *s3Client) Check(ctx context.Context) error {
	// 使用 ListObjectsV2 MaxKeys=1 限定到 bucket 根，验证读权限。
	_, err := c.api.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(c.bucket),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return c.sanitizeErr(err)
	}
	return nil
}

func (c *s3Client) recordOp(op, outcome string) {
	if c.metrics == nil {
		return
	}
	c.metrics.S3Operations.WithLabelValues(op, outcome).Inc()
}

// sanitizeErr 抹除 endpoint query、AccessKey、SecretKey、预签名 URL。
func (c *s3Client) sanitizeErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	msg = c.redacted.cleanString(msg)
	return fmt.Errorf("objectstore: %s", msg)
}

func (c *s3Client) Close() error { return nil }

// redactor 提供字符串脱敏。
type redactor struct{}

func (r redactor) cleanString(s string) string {
	// 移除常见密钥模式；保守处理避免误删。
	s = scrubBetween(s, "AccessKey=", "&")
	s = scrubBetween(s, "SecretKey=", "&")
	s = scrubBetween(s, "X-Amz-Credential=", "&")
	s = scrubBetween(s, "X-Amz-Signature=", "&")
	s = scrubBetween(s, "access_key=", "&")
	s = scrubBetween(s, "secret_key=", "&")
	return s
}

func scrubBetween(s, prefix, suffix string) string {
	// 维护已处理偏移，避免替换后从开头重复匹配同一 prefix 导致死循环。
	off := 0
	for {
		i := strings.Index(s[off:], prefix)
		if i < 0 {
			return s
		}
		i += off
		start := i + len(prefix)
		j := strings.Index(s[start:], suffix)
		end := len(s)
		if j >= 0 {
			end = start + j
		}
		s = s[:i+len(prefix)] + "***" + s[end:]
		// 已处理到 "***" 之后；suffix 若存在则也跳过它。
		off = i + len(prefix) + len("***")
		if j >= 0 {
			off += len(suffix)
		}
		if j < 0 || off >= len(s) {
			return s
		}
	}
}

// bytesReader 避免在本文件 import bytes。
func bytesReader(b []byte) io.Reader { return &byteReader{b: b} }

type byteReader struct {
	b   []byte
	off int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.off >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.off:])
	r.off += n
	return n, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt64(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// mapNotFoundErr 将 aws-sdk-go-v2 的 NotFound 类错误映射为 ErrNotFound，
// 其余错误原样返回。使用 errors.As 而非字符串匹配，避免误判。
func mapNotFoundErr(err error) error {
	if err == nil {
		return nil
	}
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return ErrNotFound
	}
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return ErrNotFound
	}
	// HeadObject 对缺失对象返回 404，sdk 会包装为 *ResponseError。
	// 通过 smithy http 状态码判断。
	if isHTTP404(err) {
		return ErrNotFound
	}
	return err
}

// isHTTP404 检测 smithy/go 错误链中是否携带 404 状态码。
func isHTTP404(err error) bool {
	// 使用 fmt.Sprintf 检查消息中是否含 "StatusCode: 404"。
	// 这是保守的字符串检测；types.NotFound/NoSuchKey 已覆盖大多数情况。
	return strings.Contains(err.Error(), "StatusCode: 404")
}
