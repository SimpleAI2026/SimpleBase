package goexample

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	gosdk "github.com/linkxzhou/SimpleBase/packages/go-sdk"
)

type Runtime struct {
	Client                      *gosdk.Client
	URL, Key, Project, Database string
	HTTP                        *http.Client
}

func New() (*Runtime, error) {
	keys := []string{"SIMPLEBASE_URL", "SIMPLEBASE_API_KEY", "SIMPLEBASE_PROJECT_ID", "SIMPLEBASE_DATABASE_ID"}
	for _, k := range keys {
		if os.Getenv(k) == "" {
			return nil, fmt.Errorf("missing %s", k)
		}
	}
	c, err := gosdk.NewClient(gosdk.Options{URL: os.Getenv(keys[0]), APIKey: os.Getenv(keys[1]), ProjectID: os.Getenv(keys[2]), DatabaseID: os.Getenv(keys[3])})
	if err != nil {
		return nil, err
	}
	return &Runtime{Client: c, URL: os.Getenv(keys[0]), Key: os.Getenv(keys[1]), Project: os.Getenv(keys[2]), Database: os.Getenv(keys[3]), HTTP: &http.Client{Timeout: 12 * time.Second}}, nil
}
func (r *Runtime) Request(ctx context.Context, method, path string, payload any, result any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(r.URL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.Key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := r.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("project API returned HTTP %d", res.StatusCode)
	}
	if result != nil {
		return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(result)
	}
	return nil
}
func (r *Runtime) ProjectPath(path string) string {
	return "/v1/projects/" + url.PathEscape(r.Project) + path
}
func (r *Runtime) KV(ctx context.Context, argv ...string) (any, error) {
	var out any
	err := r.Request(ctx, http.MethodPost, r.ProjectPath("/kv"), map[string]any{"type": "cmd", "argvs": argv}, &out)
	return out, err
}
func (r *Runtime) Compute(ctx context.Context, name string, input any, output any) error {
	return r.Request(ctx, http.MethodPost, "/go/"+url.PathEscape(r.Project)+"/"+url.PathEscape(name)+"/Compute", input, output)
}
func Rows(result *gosdk.QueryResult) ([]map[string]any, error) {
	if result == nil {
		return nil, errors.New("nil query result")
	}
	output := make([]map[string]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) != len(result.Columns) {
			return nil, errors.New("column mismatch")
		}
		item := map[string]any{}
		for i, k := range result.Columns {
			item[k] = row[i]
		}
		output = append(output, item)
	}
	return output, nil
}
func Int(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	default:
		return 0
	}
}
func Text(v any) string { s, _ := v.(string); return s }
func CheckBatch(batch *gosdk.BatchResult, err error) error {
	if err != nil {
		return err
	}
	if batch == nil {
		return errors.New("empty batch")
	}
	if batch.Error != nil {
		return fmt.Errorf("batch error: %s", batch.Error.Code)
	}
	for _, item := range batch.Results {
		if item.ErrorCode != "" {
			return fmt.Errorf("statement error: %s", item.ErrorCode)
		}
	}
	return nil
}
