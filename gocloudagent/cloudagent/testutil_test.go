// 文件说明：testutil_test.go — gocloudagent/cloudagent 的测试辅助代码
//
// 用法
//
//	同包调用：scriptedChat → model_test.go, runtime_optimization_test.go
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/testutil_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//	[关键引用] scriptedChat ← model_test.go, runtime_optimization_test.go
//
// 主要符号
//
//	type scriptedChat struct{…}
//	(s *scriptedChat) Chat(_ context.Context, _ string, req ChatRequest) (ChatResponse, error)
//	(s *scriptedChat) Stream(_ context.Context, _ string, req ChatRequest) (TokenStream, error)
//	type seqStream struct{…}
//	(s *seqStream) Next() (string, bool, error)
//	(s *seqStream) Close() error

package cloudagent

import (
	"context"
	"io"
)

type scriptedChat struct {
	content       string
	err           error
	streamErr     error
	streamNextErr error
	streamChunks  []string
	last          ChatRequest
}

func (s *scriptedChat) Chat(_ context.Context, _ string, req ChatRequest) (ChatResponse, error) {
	s.last = req
	if s.err != nil {
		return ChatResponse{}, s.err
	}
	return ChatResponse{Content: s.content}, nil
}

func (s *scriptedChat) Stream(_ context.Context, _ string, req ChatRequest) (TokenStream, error) {
	s.last = req
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	return &seqStream{chunks: s.streamChunks, err: s.streamNextErr}, nil
}

type seqStream struct {
	chunks []string
	i      int
	err    error
}

func (s *seqStream) Next() (string, bool, error) {
	if s.i >= len(s.chunks) {
		if s.err != nil {
			return "", false, s.err
		}
		return "", true, io.EOF
	}
	content := s.chunks[s.i]
	s.i++
	return content, s.i >= len(s.chunks), nil
}

func (s *seqStream) Close() error { return nil }
