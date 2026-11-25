// 文件说明：client_stream.go — OpenAI SSE 流式读取（httpTokenStream）
//
// httpTokenStream 必须实现 agentcore.DeltaStream：Model 只有拿到 NextDelta 才会读取
// 工具调用增量与 usage，退化成 Next() 就只剩正文，工具调用会被整个丢掉。

package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// ---------------------------------------------------------------------------
// 流式（SSE）
// ---------------------------------------------------------------------------

// wireChunk 是 chat.completion.chunk 帧。
type wireChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
}

// httpTokenStream 把 OpenAI SSE 读取器包成 agentcore.TokenStream，并实现 DeltaStream。
//
// 必须实现 NextDelta：agentcore.Model 只有在拿到 DeltaStream 时才会读取工具调用增量
// 与 usage，退化成 Next() 就只剩正文——工具调用会被整个丢掉。
type httpTokenStream struct {
	body   io.ReadCloser
	reader *bufio.Reader
	closed bool
	// pending 缓存一帧里拆出的多个增量（一帧可能同时带正文与多个工具分片）。
	pending []agentcore.StreamDelta
	done    bool
}

// NextDelta 返回下一个增量；流结束返回 io.EOF。
func (s *httpTokenStream) NextDelta() (agentcore.StreamDelta, error) {
	for {
		if len(s.pending) > 0 {
			d := s.pending[0]
			s.pending = s.pending[1:]
			return d, nil
		}
		if s.done {
			return agentcore.StreamDelta{Finish: true}, io.EOF
		}
		line, err := s.reader.ReadString('\n')
		if err != nil {
			s.done = true
			if errors.Is(err, io.EOF) {
				return agentcore.StreamDelta{Finish: true}, io.EOF
			}
			return agentcore.StreamDelta{}, err
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue // 空行、注释行（": ping"）、event:/id: 行都忽略
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			s.done = true
			return agentcore.StreamDelta{Finish: true}, io.EOF
		}
		var chunk wireChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue // 非 JSON 帧忽略，与前端解析器的口径一致
		}
		s.pending = append(s.pending, deltasFromChunk(chunk)...)
	}
}

// deltasFromChunk 把一帧拆成若干增量。
func deltasFromChunk(chunk wireChunk) []agentcore.StreamDelta {
	var out []agentcore.StreamDelta
	for _, ch := range chunk.Choices {
		if ch.Delta.Content != "" {
			out = append(out, agentcore.StreamDelta{Content: ch.Delta.Content})
		}
		for _, tc := range ch.Delta.ToolCalls {
			out = append(out, agentcore.StreamDelta{ToolCall: &agentcore.ToolCallDelta{
				Index: tc.Index, ID: tc.ID, Name: tc.Function.Name, ArgsDelta: tc.Function.Arguments,
			}})
		}
	}
	if chunk.Usage != nil {
		u := chunk.Usage.toChatUsage()
		out = append(out, agentcore.StreamDelta{Usage: &u})
	}
	return out
}

// Next 是 TokenStream 的兼容形态：只返回正文增量。
func (s *httpTokenStream) Next() (string, bool, error) {
	for {
		d, err := s.NextDelta()
		if err != nil {
			return "", true, err
		}
		if d.Content != "" {
			return d.Content, false, nil
		}
	}
}

func (s *httpTokenStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}
