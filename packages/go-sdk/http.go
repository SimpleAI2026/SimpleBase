package gosdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxResponseBytes = 32 << 20
const maxErrorBytes = 64 << 10

func (c *Client) request(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string, result any) error {
	if ctx == nil {
		return errors.New("gosdk: context is required")
	}
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return fmt.Errorf("gosdk: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if strings.HasPrefix(path, "/e2b/") {
		// E2B 的请求里没有项目：用头指明（见 internal/api/e2b_auth.go）。
		req.Header.Set("X-SimpleBase-Project", c.projectID)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gosdk: request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return decodeAPIError(res)
	}
	if result == nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("gosdk: read response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return errors.New("gosdk: response exceeds 32 MiB")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("gosdk: decode response: %w", err)
	}
	return nil
}

func (c *Client) requestStream(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string) (*http.Response, error) {
	if ctx == nil {
		return nil, errors.New("gosdk: context is required")
	}
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, fmt.Errorf("gosdk: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gosdk: request: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		return nil, decodeAPIError(res)
	}
	return res, nil
}

func (c *Client) requestJSON(ctx context.Context, method, path string, query url.Values, input, output any) error {
	if input == nil {
		return c.request(ctx, method, path, query, nil, "", output)
	}
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("gosdk: encode request: %w", err)
	}
	return c.request(ctx, method, path, query, bytes.NewReader(body), "application/json", output)
}
