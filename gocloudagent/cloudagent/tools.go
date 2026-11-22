package cloudagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/cloudwego/eino/components/tool"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

func wrapToolErrors(tools []tool.BaseTool) []tool.BaseTool {
	out := make([]tool.BaseTool, 0, len(tools))
	for _, t := range tools {
		inv, ok := t.(tool.InvokableTool)
		if !ok {
			out = append(out, t)
			continue
		}
		out = append(out, &errorTolerantTool{InvokableTool: inv})
	}
	return out
}

type errorTolerantTool struct {
	tool.InvokableTool
}

func (w *errorTolerantTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	out, err := w.InvokableTool.InvokableRun(ctx, args, opts...)
	if err == nil {
		return out, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", err
	}
	return marshalToolJSON(map[string]any{"error": err.Error(), "is_error": true})
}

func toolFailure(code, message string) string {
	body, _ := json.Marshal(map[string]any{
		"ok":    false,
		"error": map[string]any{"code": code, "message": agentcore.RedactSecrets(message)},
	})
	return string(body)
}

func toolResultIsError(content string) bool {
	var envelope struct {
		OK      *bool `json:"ok"`
		IsError bool  `json:"is_error"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(content)), &envelope) != nil {
		return false
	}
	return envelope.IsError || envelope.OK != nil && !*envelope.OK
}

func marshalToolJSON(v any) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return agentcore.RedactSecrets(string(body)), nil
}
