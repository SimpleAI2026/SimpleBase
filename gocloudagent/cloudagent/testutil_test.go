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
