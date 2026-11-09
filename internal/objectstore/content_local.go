// 文件说明：content_local.go — internal/objectstore 的内部实现（GetContent、GetContent 等）
//
// 用法
//
//	type memoryReader（包内使用）
//	同包调用：GetContent() → content.go, content.go 等
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/objectstore/content_local.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] blob.go, client.go, content.go, content_s3.go, descriptor.go 等 10 个
//	[关键引用] GetContent() ← content.go, content_s3.go
//
// 主要符号
//
//	(s *localFileStore) GetContent(_ context.Context, key, rangeValue string) ...
//	(m *memoryFileStore) GetContent(_ context.Context, key, rangeValue string) ...
//	type memoryReader struct {…}
//	(r *memoryReader) ReadAt(p []byte, off int64) (int, error)

package objectstore

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
)

func (s *localFileStore) GetContent(_ context.Context, key, rangeValue string) (FileContent, error) {
	path, err := s.physical(key)
	if err != nil {
		return FileContent{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return FileContent{}, ErrNotFound
		}
		return FileContent{}, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return FileContent{}, err
	}
	start, end, err := contentRange(info.Size(), rangeValue)
	if err != nil {
		file.Close()
		return FileContent{}, err
	}
	body := struct {
		io.Reader
		io.Closer
	}{Reader: io.NewSectionReader(file, start, end-start+1), Closer: file}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return FileContent{Body: body, Size: info.Size(), RangeStart: start, RangeEnd: end, ContentType: contentType, ETag: fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size()), LastModified: info.ModTime()}, nil
}

func (m *memoryFileStore) GetContent(_ context.Context, key, rangeValue string) (FileContent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.objects[key]
	if !ok {
		return FileContent{}, ErrNotFound
	}
	start, end, err := contentRange(int64(len(item.data)), rangeValue)
	if err != nil {
		return FileContent{}, err
	}
	return FileContent{Body: io.NopCloser(io.NewSectionReader(&memoryReader{data: item.data}, start, end-start+1)), Size: int64(len(item.data)), RangeStart: start, RangeEnd: end, ContentType: item.contentType, LastModified: item.lastMod}, nil
}

type memoryReader struct{ data []byte }

func (r *memoryReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
