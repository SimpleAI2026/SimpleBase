package objectstore

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // 仅用于 COS 要求的 Content-MD5 请求头，非安全用途
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// BlobStore 读写二进制对象（DuckLake catalog.sqlite 等）。
// 与 Client（JSON 平台对象）分离，避免把大文件塞进 PutJSON。
//
// PutIfAbsent 是唯一支持条件写的原语（multi-instance-consistency-plan §4.5）：
// 仅当 key 不存在时写入，已存在返回 ErrPreconditionFailed。
// 故意不提供 IfMatch 能力——目标端点 COS 会静默忽略它，制造假保护。
type BlobStore interface {
	Head(ctx context.Context, key string) (ObjectInfo, error)
	PutBytes(ctx context.Context, key string, data []byte, contentType string) error
	PutIfAbsent(ctx context.Context, key string, data []byte, contentType string) (ObjectInfo, error)
	GetBytes(ctx context.Context, key string) ([]byte, ObjectInfo, error)
	DownloadFile(ctx context.Context, key, destPath string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	// List 分页列举前缀下对象（ducklake-duckdb-catalog-plan §6）。
	// cursor 为空从头开始；返回本页 keys、下一页 cursor（空表示结束）。
	List(ctx context.Context, prefix, cursor string, limit int) (keys []string, next string, err error)
	// DeleteMany 批量删除（S3 上限 1000/批）；容忍 key 不存在。
	DeleteMany(ctx context.Context, keys []string) error
}

// ListResult 是 List 的单页结果。
type ListResult struct {
	Keys []string
	Next string
}

// ErrPreconditionFailed 表示 create-if-absent 写入冲突（对象已存在）。
// COS 返回 409 ObjectAlreadyExists、S3/R2 返回 412，统一映射到本错误。
var ErrPreconditionFailed = errors.New("objectstore: object already exists")

// putIfAbsentInput 构造条件写的公共输入。
// S3/GCS/R2 → If-None-Match: *；腾讯云 COS → x-cos-forbid-overwrite: true
// （私有头，冲突返回 409 ObjectAlreadyExists；存储桶开启版本控制时该头失效，
// 由启动探针 ProbePutIfAbsent 检测）。
//
// 注意：COS 的 x-cos-forbid-overwrite 是独立请求头，不能用 PutObjectInput.Metadata
// 发送——SDK 会把它包装成 x-amz-meta-x-cos-forbid-overwrite，COS 不识别
// （该 bug 会导致 COS 上 create-if-absent 完全不生效，探针正确报错）。
// COS 分支由 PutIfAbsent 通过 middleware 注入独立头（见 cosForbidOverwriteOption）。
func (c *s3Client) putIfAbsentInput(key string, data []byte, contentType string) *s3.PutObjectInput {
	in := &s3.PutObjectInput{
		Bucket:            aws.String(c.bucket),
		Key:               aws.String(key),
		Body:              bytes.NewReader(data),
		ContentLength:     aws.Int64(int64(len(data))),
		IfNoneMatch:       aws.String("*"),
		ChecksumAlgorithm: types.ChecksumAlgorithmCrc32,
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if c.isCOSLike() {
		// COS 不识别 If-None-Match；冲突返回 409 ObjectAlreadyExists，非标准 412。
		in.IfNoneMatch = nil
	}
	return in
}

// cosForbidOverwriteOption 返回 s3 调用选项：以独立请求头注入
// x-cos-forbid-overwrite: true（Build 阶段、签名之前，随请求一起签名）。
func cosForbidOverwriteOption(o *s3.Options) {
	o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
		return stack.Build.Add(middleware.BuildMiddlewareFunc("COSForbidOverwrite",
			func(ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler) (middleware.BuildOutput, middleware.Metadata, error) {
				if req, ok := in.Request.(*smithyhttp.Request); ok {
					req.Header.Set("x-cos-forbid-overwrite", "true")
				}
				return next.HandleBuild(ctx, in)
			}), middleware.After)
	})
}

// deleteObjectsMD5Option 为 DeleteObjects 补 Content-MD5 头（Build 阶段，签名之前）。
// 腾讯云 COS 的批量删除强制要求 Content-MD5，只带 x-amz-checksum-* 会返回
// 400 InvalidRequest: Missing required header for this request: Content-MD5。
// 对 AWS S3 / MinIO 同样合法（S3 规范本就要求 MD5 或 checksum 二选一）。
func deleteObjectsMD5Option(o *s3.Options) {
	o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
		return stack.Build.Add(middleware.BuildMiddlewareFunc("DeleteObjectsContentMD5",
			func(ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler) (middleware.BuildOutput, middleware.Metadata, error) {
				req, ok := in.Request.(*smithyhttp.Request)
				if !ok || req.GetStream() == nil {
					return next.HandleBuild(ctx, in)
				}
				body, err := io.ReadAll(req.GetStream())
				if err != nil {
					return middleware.BuildOutput{}, middleware.Metadata{}, fmt.Errorf("objectstore: read delete body: %w", err)
				}
				sum := md5.Sum(body)
				req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(sum[:]))
				if req, err = req.SetStream(bytes.NewReader(body)); err != nil {
					return middleware.BuildOutput{}, middleware.Metadata{}, err
				}
				in.Request = req
				return next.HandleBuild(ctx, in)
			}), middleware.After)
	})
}

// isCOSLike 判断当前端点是否为腾讯云 COS。
// COS 域名形如 {bucket}-{appid}.cos.{region}.myqcloud.com。
func (c *s3Client) isCOSLike() bool {
	return isCOSEndpoint(c.endpoint)
}

// isCOSEndpoint 判断 endpoint 是否指向腾讯云 COS（*.myqcloud.com）。
func isCOSEndpoint(endpoint string) bool {
	return strings.Contains(endpointHost(endpoint), "myqcloud.com")
}

// endpointHost 返回脱敏后的 endpoint host（去 scheme 与路径，保留端口）。
func endpointHost(endpoint string) string {
	ep := strings.TrimSpace(endpoint)
	if i := strings.Index(ep, "://"); i >= 0 {
		ep = ep[i+3:]
	}
	if i := strings.IndexByte(ep, '/'); i >= 0 {
		ep = ep[:i]
	}
	return strings.ToLower(ep)
}

// PutBytes 上传二进制对象。
func (c *s3Client) PutBytes(ctx context.Context, key string, data []byte, contentType string) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	}
	if len(data) >= 0 {
		in.ContentLength = aws.Int64(int64(len(data)))
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if _, err := c.api.PutObject(ctx, in); err != nil {
		c.recordOp("put_bytes", "error")
		return c.sanitizeErr(err)
	}
	c.recordOp("put_bytes", "ok")
	return nil
}

// PutIfAbsent 仅当 key 不存在时写入；已存在返回 ErrPreconditionFailed。
func (c *s3Client) PutIfAbsent(ctx context.Context, key string, data []byte, contentType string) (ObjectInfo, error) {
	in := c.putIfAbsentInput(key, data, contentType)
	var opts []func(*s3.Options)
	if c.isCOSLike() {
		opts = append(opts, cosForbidOverwriteOption)
	}
	out, err := c.api.PutObject(ctx, in, opts...)
	if err != nil {
		c.recordOp("put_if_absent", "conflict_or_error")
		mapped := mapPreconditionErr(c.sanitizeErr(err))
		if errors.Is(mapped, ErrPreconditionFailed) {
			c.recordOp("put_if_absent", "conflict")
			return ObjectInfo{}, mapped
		}
		return ObjectInfo{}, mapped
	}
	c.recordOp("put_if_absent", "ok")
	return ObjectInfo{
		Key:  key,
		Size: int64(len(data)),
		ETag: derefStr(out.ETag),
	}, nil
}

// GetBytes 下载整个对象到内存。
func (c *s3Client) GetBytes(ctx context.Context, key string) ([]byte, ObjectInfo, error) {
	out, err := c.api.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		c.recordOp("get_bytes", "error")
		return nil, ObjectInfo{}, mapNotFoundErr(c.sanitizeErr(err))
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		c.recordOp("get_bytes", "error")
		return nil, ObjectInfo{}, c.sanitizeErr(err)
	}
	info := ObjectInfo{
		Key:          key,
		Size:         int64(len(data)),
		LastModified: derefTime(out.LastModified),
		ETag:         derefStr(out.ETag),
	}
	c.recordOp("get_bytes", "ok")
	return data, info, nil
}

// DownloadFile 下载对象到本地文件（先写 .tmp 再 rename）。
func (c *s3Client) DownloadFile(ctx context.Context, key, destPath string) (ObjectInfo, error) {
	data, info, err := c.GetBytes(ctx, key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return ObjectInfo{}, fmt.Errorf("objectstore: mkdir for download: %w", err)
	}
	tmp := destPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return ObjectInfo{}, fmt.Errorf("objectstore: write download tmp: %w", err)
	}
	if err := os.Rename(tmp, destPath); err != nil {
		_ = os.Remove(tmp)
		return ObjectInfo{}, fmt.Errorf("objectstore: rename download: %w", err)
	}
	return info, nil
}

// Delete 删除单个对象。
func (c *s3Client) Delete(ctx context.Context, key string) error {
	out, err := c.api.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(c.bucket),
		Delete: &types.Delete{
			Objects: []types.ObjectIdentifier{{Key: aws.String(key)}},
			Quiet:   aws.Bool(true),
		},
	}, deleteObjectsMD5Option)
	if err != nil {
		c.recordOp("delete", "error")
		return c.sanitizeErr(err)
	}
	if err := deleteObjectsError(out); err != nil {
		c.recordOp("delete", "error")
		return err
	}
	c.recordOp("delete", "ok")
	return nil
}

// List 分页列举前缀下对象（ListObjectsV2）。limit<=0 或 >1000 时取 1000。
func (c *s3Client) List(ctx context.Context, prefix, cursor string, limit int) ([]string, string, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	in := &s3.ListObjectsV2Input{
		Bucket:  aws.String(c.bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(int32(limit)),
	}
	if cursor != "" {
		in.ContinuationToken = aws.String(cursor)
	}
	out, err := c.api.ListObjectsV2(ctx, in)
	if err != nil {
		c.recordOp("list", "error")
		return nil, "", c.sanitizeErr(err)
	}
	c.recordOp("list", "ok")
	keys := make([]string, 0, len(out.Contents))
	for _, obj := range out.Contents {
		if obj.Key != nil {
			keys = append(keys, *obj.Key)
		}
	}
	next := ""
	if out.IsTruncated != nil && *out.IsTruncated && out.NextContinuationToken != nil {
		next = *out.NextContinuationToken
	}
	return keys, next, nil
}

// DeleteMany 批量删除（S3 上限 1000/批，内部自动分批）。容忍 key 不存在。
func (c *s3Client) DeleteMany(ctx context.Context, keys []string) error {
	for len(keys) > 0 {
		n := len(keys)
		if n > 1000 {
			n = 1000
		}
		batch := keys[:n]
		objects := make([]types.ObjectIdentifier, 0, n)
		for _, k := range batch {
			objects = append(objects, types.ObjectIdentifier{Key: aws.String(k)})
		}
		out, err := c.api.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(c.bucket),
			Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)},
		}, deleteObjectsMD5Option)
		if err != nil {
			c.recordOp("delete_many", "error")
			return c.sanitizeErr(err)
		}
		if err := deleteObjectsError(out); err != nil {
			c.recordOp("delete_many", "error")
			return err
		}
		keys = keys[n:]
	}
	c.recordOp("delete_many", "ok")
	return nil
}

func deleteObjectsError(out *s3.DeleteObjectsOutput) error {
	if out == nil {
		return errors.New("objectstore: delete objects returned no response")
	}
	if len(out.Errors) == 0 {
		return nil
	}
	first := out.Errors[0]
	return fmt.Errorf("objectstore: delete objects failed for %d key(s), first key %q (code %q)",
		len(out.Errors), aws.ToString(first.Key), aws.ToString(first.Code))
}

/* ---------- 内存 BlobStore（单测 / DevMode 可选） ---------- */

type memoryBlobStore struct {
	mu    sync.RWMutex
	store map[string][]byte
	meta  map[string]time.Time
}

// NewMemoryBlobStore 构造进程内 BlobStore。
func NewMemoryBlobStore() BlobStore {
	return &memoryBlobStore{
		store: map[string][]byte{},
		meta:  map[string]time.Time{},
	}
}

func (m *memoryBlobStore) Head(_ context.Context, key string) (ObjectInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.store[key]
	if !ok {
		return ObjectInfo{}, ErrNotFound
	}
	return ObjectInfo{Key: key, Size: int64(len(b)), LastModified: m.meta[key]}, nil
}

func (m *memoryBlobStore) PutBytes(_ context.Context, key string, data []byte, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]byte(nil), data...)
	m.store[key] = cp
	m.meta[key] = time.Now().UTC()
	return nil
}

// PutIfAbsent 仅当 key 不存在时写入；ETag 用内容 SHA256 前 16 hex（便于测试断言）。
func (m *memoryBlobStore) PutIfAbsent(_ context.Context, key string, data []byte, contentType string) (ObjectInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.store[key]; ok {
		return ObjectInfo{}, ErrPreconditionFailed
	}
	cp := append([]byte(nil), data...)
	sum := sha256.Sum256(cp)
	m.store[key] = cp
	now := time.Now().UTC()
	m.meta[key] = now
	return ObjectInfo{Key: key, Size: int64(len(cp)), ETag: hex.EncodeToString(sum[:8]), LastModified: now}, nil
}

func (m *memoryBlobStore) GetBytes(_ context.Context, key string) ([]byte, ObjectInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.store[key]
	if !ok {
		return nil, ObjectInfo{}, ErrNotFound
	}
	cp := append([]byte(nil), b...)
	return cp, ObjectInfo{Key: key, Size: int64(len(cp)), LastModified: m.meta[key]}, nil
}

func (m *memoryBlobStore) DownloadFile(ctx context.Context, key, destPath string) (ObjectInfo, error) {
	data, info, err := m.GetBytes(ctx, key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return ObjectInfo{}, err
	}
	tmp := destPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return ObjectInfo{}, err
	}
	if err := os.Rename(tmp, destPath); err != nil {
		_ = os.Remove(tmp)
		return ObjectInfo{}, err
	}
	return info, nil
}

func (m *memoryBlobStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
	delete(m.meta, key)
	return nil
}

func (m *memoryBlobStore) List(_ context.Context, prefix, cursor string, limit int) ([]string, string, error) {
	if limit <= 0 {
		limit = 1000
	}
	m.mu.RLock()
	keys := make([]string, 0, len(m.store))
	for k := range m.store {
		if strings.HasPrefix(k, prefix) && k > cursor {
			keys = append(keys, k)
		}
	}
	m.mu.RUnlock()
	sort.Strings(keys)
	if len(keys) > limit {
		return keys[:limit], keys[limit-1], nil
	}
	return keys, "", nil
}

func (m *memoryBlobStore) DeleteMany(_ context.Context, keys []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.store, k)
		delete(m.meta, k)
	}
	return nil
}
