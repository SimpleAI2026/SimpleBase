// 文件说明：content.go — internal/objectstore 中 FileContent、ContentStore 等符号的实现
//
// 用法
//
//	type FileContent（导出类型，同包与上层均可使用）
//	type ContentStore（导出类型，同包与上层均可使用）
//	同包调用：FileContent → content_local.go, content_s3.go；contentRange() → content_local.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/objectstore/content.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] blob.go, client.go, content_local.go, content_s3.go, descriptor.go 等 10 个
//	[关键引用] FileContent ← content_local.go, content_s3.go
//
// 主要符号
//
//	type FileContent struct {…}
//	type ContentStore interface {…}
//	... 另有 1 个顶层符号

package objectstore

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type FileContent struct {
	Body         io.ReadCloser
	Size         int64
	RangeStart   int64
	RangeEnd     int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

type ContentStore interface {
	GetContent(context.Context, string, string) (FileContent, error)
}

func contentRange(size int64, value string) (int64, int64, error) {
	start, end := int64(0), size-1
	if value == "" {
		return start, end, nil
	}
	if size <= 0 || !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return 0, 0, fmt.Errorf("invalid range")
	}
	parts := strings.SplitN(strings.TrimPrefix(value, "bytes="), "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range")
	}
	if parts[0] == "" {
		length, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || length <= 0 {
			return 0, 0, fmt.Errorf("invalid range")
		}
		start = size - length
		if start < 0 {
			start = 0
		}
	} else {
		var err error
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil || start < 0 || start >= size {
			return 0, 0, fmt.Errorf("invalid range")
		}
		if parts[1] != "" {
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil || end < start {
				return 0, 0, fmt.Errorf("invalid range")
			}
		}
	}
	if end >= size {
		end = size - 1
	}
	return start, end, nil
}
