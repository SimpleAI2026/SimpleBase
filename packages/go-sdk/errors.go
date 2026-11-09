package gosdk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIError describes a non-2xx response. Details are intentionally not retained.
type APIError struct {
	Status    int
	Code      string
	Message   string
	RequestID string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("gosdk: HTTP %d (%s): %s", e.Status, e.Code, e.Message)
}

func decodeAPIError(res *http.Response) error {
	data, err := io.ReadAll(io.LimitReader(res.Body, maxErrorBytes+1))
	if err != nil {
		return fmt.Errorf("gosdk: read HTTP %d error: %w", res.StatusCode, err)
	}
	out := &APIError{Status: res.StatusCode, Code: "http_error", Message: http.StatusText(res.StatusCode), RequestID: res.Header.Get("X-Request-Id")}
	if len(data) > maxErrorBytes {
		return out
	}
	var payload struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
		// E2B 兼容面的 code 是整数（HTTP 状态），业务码在 error_code；其余接口的 code 是字符串。
		Code      json.RawMessage `json:"code"`
		ErrorCode string          `json:"error_code"`
		Message   string          `json:"message"`
		RequestID string          `json:"request_id"`
	}
	if json.Unmarshal(data, &payload) == nil {
		var codeStr string
		if json.Unmarshal(payload.Code, &codeStr) == nil && codeStr != "" {
			out.Code = codeStr
		}
		if payload.ErrorCode != "" {
			out.Code = payload.ErrorCode
		}
		if payload.Message != "" {
			out.Message = payload.Message
		}
		if payload.Error.Code != "" {
			out.Code = payload.Error.Code
		}
		if payload.Error.Message != "" {
			out.Message = payload.Error.Message
		}
		if out.RequestID == "" {
			out.RequestID = payload.Error.RequestID
			if out.RequestID == "" {
				out.RequestID = payload.RequestID
			}
		}
	}
	return out
}
