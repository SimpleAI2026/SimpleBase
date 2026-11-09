// 文件说明：content_s3.go — internal/objectstore 的内部实现（GetContent 等）
//
// 用法
//
//	同包调用：GetContent() → content.go, content_local.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/objectstore/content_s3.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] blob.go, client.go, content.go, content_local.go, descriptor.go 等 10 个
//	[关键引用] GetContent() ← content.go, content_local.go
//
// 主要符号
//
//	(s *s3FileStore) GetContent(ctx context.Context, key, rangeValue string) ...

package objectstore

import (
	"context"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (s *s3FileStore) GetContent(ctx context.Context, key, rangeValue string) (FileContent, error) {
	if err := ValidateFileKey(key); err != nil {
		return FileContent{}, err
	}
	input := &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(s.physical(key))}
	if rangeValue != "" {
		input.Range = aws.String(rangeValue)
	}
	output, err := s.client.GetObject(ctx, input)
	if err != nil {
		return FileContent{}, s.sanitize(err)
	}
	size := derefInt64(output.ContentLength)
	start, end := int64(0), size-1
	if output.ContentRange != nil {
		value := derefStr(output.ContentRange)
		if dash := strings.Index(value, "-"); dash >= 0 {
			start, _ = strconv.ParseInt(value[strings.LastIndex(value[:dash], " ")+1:dash], 10, 64)
			if slash := strings.Index(value[dash:], "/"); slash >= 0 {
				end, _ = strconv.ParseInt(value[dash+1:dash+slash], 10, 64)
				size, _ = strconv.ParseInt(value[dash+slash+1:], 10, 64)
			}
		}
	}
	return FileContent{Body: output.Body, Size: size, RangeStart: start, RangeEnd: end, ContentType: derefStr(output.ContentType), ETag: derefStr(output.ETag), LastModified: derefTime(output.LastModified)}, nil
}
