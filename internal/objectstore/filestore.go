// filestore.go 提供面向用户文件的对象存储抽象（FileStore）。
//
// 与平台级 Client 的区别：
//   - Client 只读写 catalog/descriptor 等平台对象，key 由 KeyBuilder 生成；
//   - FileStore 服务用户上传的文件，key 是用户提供的相对路径，
//     所有 key 先经 ValidateFileKey 校验，再由实现拼接物理前缀（项目隔离）。
//
// 物理布局：{root}/{env}/files/{projectID}/{userKey}（前缀由 app 装配时注入）。
package objectstore

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// ErrInvalidKey 表示用户提供的文件 key 不合法（路径穿越/绝对路径/过长等）。
var ErrInvalidKey = errors.New("objectstore: invalid file key")

// maxFileKeyLen 是用户 key 的最大长度（字节）。
const maxFileKeyLen = 1024

// ValidateFileKey 校验用户相对 key：
// 非空、不含 NUL、不以 "/" 开头、不含 ".."、不含反斜杠、长度不超过 1024。
func ValidateFileKey(key string) error {
	if key == "" {
		return fmt.Errorf("%w: key is empty", ErrInvalidKey)
	}
	if len(key) > maxFileKeyLen {
		return fmt.Errorf("%w: key exceeds %d bytes", ErrInvalidKey, maxFileKeyLen)
	}
	if strings.ContainsRune(key, 0) {
		return fmt.Errorf("%w: key contains NUL", ErrInvalidKey)
	}
	if strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return fmt.Errorf("%w: key must be a relative path", ErrInvalidKey)
	}
	// 逐段校验，杜绝 "a/../b"、"a/./b" 等穿越变体
	for _, seg := range strings.Split(key, "/") {
		if seg == ".." || seg == "." {
			return fmt.Errorf("%w: key contains traversal segment", ErrInvalidKey)
		}
	}
	return nil
}

// FileObject 是用户文件的元数据。Key 为相对 key（不含物理前缀）。
type FileObject struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// FileStore 抽象用户文件存储。List 的 prefix 同样是相对前缀（如 "images/"）。
type FileStore interface {
	List(ctx context.Context, prefix string, maxKeys int) ([]FileObject, error)
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) (FileObject, error)
	Delete(ctx context.Context, key string) error
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

/* ---------- S3 实现 ---------- */

type s3FileStore struct {
	client   *s3.Client
	bucket   string
	prefix   string // 物理前缀（已含 files/ 段），可为空
	logger   observability.Logger
	redacted redactor
}

// NewS3FileStore 构造基于真实 S3 的 FileStore。
// basePrefix 为物理前缀（如 "simplebase/prod/files"），app 层负责拼接项目段。
func NewS3FileStore(ctx context.Context, cfg Config, basePrefix string, logger observability.Logger) (FileStore, error) {
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
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.ForcePathStyle
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
	return &s3FileStore{
		client:   client,
		bucket:   cfg.Bucket,
		prefix:   strings.Trim(basePrefix, "/"),
		logger:   logger,
		redacted: redactor{},
	}, nil
}

// physical 把相对 key 映射为物理 key。
func (s *s3FileStore) physical(key string) string {
	if s.prefix == "" {
		return key
	}
	return s.prefix + "/" + key
}

// relative 把物理 key 还原为相对 key。
func (s *s3FileStore) relative(key string) string {
	if s.prefix == "" {
		return key
	}
	return strings.TrimPrefix(key, s.prefix+"/")
}

func (s *s3FileStore) List(ctx context.Context, prefix string, maxKeys int) ([]FileObject, error) {
	if prefix != "" {
		if err := ValidateFileKey(prefix); err != nil {
			return nil, err
		}
	}
	if maxKeys <= 0 || maxKeys > 1000 {
		maxKeys = 1000
	}
	out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		Prefix:  aws.String(s.physical(prefix)),
		MaxKeys: aws.Int32(int32(maxKeys)),
	})
	if err != nil {
		return nil, s.sanitize(err)
	}
	objects := make([]FileObject, 0, len(out.Contents))
	for _, obj := range out.Contents {
		objects = append(objects, FileObject{
			Key:          s.relative(derefStr(obj.Key)),
			Size:         derefInt64(obj.Size),
			LastModified: derefTime(obj.LastModified),
		})
	}
	return objects, nil
}

func (s *s3FileStore) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) (FileObject, error) {
	if err := ValidateFileKey(key); err != nil {
		return FileObject{}, err
	}
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.physical(key)),
		Body:   body,
	}
	if size >= 0 {
		in.ContentLength = aws.Int64(size)
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if _, err := s.client.PutObject(ctx, in); err != nil {
		return FileObject{}, s.sanitize(err)
	}
	// Head 一次取准确的 LastModified
	head, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.physical(key)),
	})
	if err != nil {
		return FileObject{Key: key, Size: size, LastModified: time.Now().UTC()}, nil
	}
	return FileObject{
		Key:          key,
		Size:         derefInt64(head.ContentLength),
		LastModified: derefTime(head.LastModified),
	}, nil
}

func (s *s3FileStore) Delete(ctx context.Context, key string) error {
	if err := ValidateFileKey(key); err != nil {
		return err
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.physical(key)),
	})
	if err != nil {
		return s.sanitize(err)
	}
	return nil
}

func (s *s3FileStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := ValidateFileKey(key); err != nil {
		return "", err
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	presigner := s3.NewPresignClient(s.client)
	out, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.physical(key)),
	}, func(o *s3.PresignOptions) {
		o.Expires = ttl
	})
	if err != nil {
		return "", s.sanitize(err)
	}
	return out.URL, nil
}

func (s *s3FileStore) sanitize(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("objectstore: %s", s.redacted.cleanString(err.Error()))
}

/* ---------- 内存实现（DevMode） ---------- */

type memoryFile struct {
	data        []byte
	contentType string
	lastMod     time.Time
}

type memoryFileStore struct {
	mu      sync.RWMutex
	objects map[string]*memoryFile
}

// NewMemoryFileStore 构造 DevMode 使用的内存 FileStore。不持久化，进程退出即丢失。
func NewMemoryFileStore() FileStore {
	return &memoryFileStore{objects: make(map[string]*memoryFile)}
}

func (m *memoryFileStore) List(_ context.Context, prefix string, _ int) ([]FileObject, error) {
	if prefix != "" {
		if err := ValidateFileKey(prefix); err != nil {
			return nil, err
		}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]FileObject, 0, len(m.objects))
	for key, f := range m.objects {
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		out = append(out, FileObject{Key: key, Size: int64(len(f.data)), LastModified: f.lastMod})
	}
	return out, nil
}

func (m *memoryFileStore) Put(_ context.Context, key string, body io.Reader, _ int64, contentType string) (FileObject, error) {
	if err := ValidateFileKey(key); err != nil {
		return FileObject{}, err
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return FileObject{}, fmt.Errorf("objectstore: read body: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = &memoryFile{data: data, contentType: contentType, lastMod: time.Now().UTC()}
	return FileObject{Key: key, Size: int64(len(data)), LastModified: m.objects[key].lastMod}, nil
}

func (m *memoryFileStore) Delete(_ context.Context, key string) error {
	if err := ValidateFileKey(key); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

func (m *memoryFileStore) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	if err := ValidateFileKey(key); err != nil {
		return "", err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.objects[key]
	if !ok {
		return "", ErrNotFound
	}
	// 内存模式没有真实 URL，返回 data URL 便于本地预览
	ct := f.contentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(f.data), nil
}
