// 文件说明：route_meta.go — internal/api 中 RouteMetadata 等符号的实现
//
// 用法
//
//	type RouteMetadata（导出类型，同包与上层均可使用）
//	type openAPIDocument（包内使用）
//	同包调用：buildOpenAPI() → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/route_meta.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] buildOpenAPI() ← router.go
//
// 主要符号
//
//	type RouteMetadata struct {…}
//	... 另有 11 个顶层符号

package api

import (
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
)

type RouteMetadata struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	OperationID string `json:"operation_id"`
	Tryable     bool   `json:"tryable"`
}

type openAPIDocument struct {
	OpenAPI    string                          `json:"openapi"`
	Info       map[string]any                  `json:"info"`
	Servers    []map[string]string             `json:"servers"`
	Paths      map[string]map[string]operation `json:"paths"`
	Components map[string]any                  `json:"components"`
}

type operation struct {
	OperationID string                `json:"operationId"`
	Tryable     bool                  `json:"x-tryable"`
	Parameters  []operationParameter  `json:"parameters,omitempty"`
	RequestBody *operationBody        `json:"requestBody,omitempty"`
	Responses   map[string]any        `json:"responses"`
	Security    []map[string][]string `json:"security,omitempty"`
}

type operationParameter struct {
	Name     string         `json:"name"`
	In       string         `json:"in"`
	Required bool           `json:"required"`
	Schema   map[string]any `json:"schema"`
}

type operationBody struct {
	Required bool           `json:"required"`
	Content  map[string]any `json:"content"`
}

var routeParamPattern = regexp.MustCompile(`:([A-Za-z][A-Za-z0-9_]*)`)

func routeMetadata(routes []*echo.Route) []RouteMetadata {
	items := []RouteMetadata{}
	seen := map[string]bool{}
	for _, route := range routes {
		if route == nil || route.Name == "echo_route_not_found" || route.Method == "echo_route_not_found" || !publicAPIRoute(route.Path) {
			continue
		}
		path := openAPIPath(route.Path)
		key := route.Method + " " + path
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, RouteMetadata{Method: route.Method, Path: path, OperationID: operationID(route.Method, path), Tryable: routeTryable(route.Method, route.Path)})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Path == items[j].Path {
			return items[i].Method < items[j].Method
		}
		return items[i].Path < items[j].Path
	})
	return items
}

func publicAPIRoute(path string) bool {
	return (strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/go/") || path == "/health/live" || path == "/health/ready") && !strings.Contains(path, "*")
}

func openAPIPath(path string) string {
	return routeParamPattern.ReplaceAllString(path, "{$1}")
}

func operationID(method, path string) string {
	replacer := strings.NewReplacer("/", "_", "{", "", "}", "", "-", "_", ".", "_")
	return strings.ToLower(method) + replacer.Replace(path)
}

func routeTryable(method, path string) bool {
	if method != http.MethodGet && method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
		return false
	}
	for _, suffix := range []string{"/realtime", "/export", "/objects/content", "/download"} {
		if strings.HasSuffix(path, suffix) {
			return false
		}
	}
	return true
}

func buildOpenAPI(routes []*echo.Route) openAPIDocument {
	document := openAPIDocument{
		OpenAPI:    "3.0.3",
		Info:       map[string]any{"title": "SimpleBase API", "version": "1"},
		Servers:    []map[string]string{{"url": "/"}},
		Paths:      map[string]map[string]operation{},
		Components: map[string]any{"securitySchemes": map[string]any{"bearerAuth": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}}},
	}
	for _, metadata := range routeMetadata(routes) {
		if document.Paths[metadata.Path] == nil {
			document.Paths[metadata.Path] = map[string]operation{}
		}
		item := operation{OperationID: metadata.OperationID, Tryable: metadata.Tryable, Responses: map[string]any{"200": map[string]any{"description": "Success"}}, Security: []map[string][]string{{"bearerAuth": {}}}}
		for _, segment := range strings.Split(metadata.Path, "/") {
			if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
				item.Parameters = append(item.Parameters, operationParameter{Name: strings.Trim(segment, "{}"), In: "path", Required: true, Schema: map[string]any{"type": "string"}})
			}
		}
		if metadata.Method == http.MethodPost || metadata.Method == http.MethodPut || metadata.Method == http.MethodPatch {
			contentType := "application/json"
			if strings.HasSuffix(metadata.Path, "/import") {
				contentType = "application/x-ndjson"
			}
			item.RequestBody = &operationBody{Required: true, Content: map[string]any{contentType: map[string]any{"schema": map[string]any{}}}}
		}
		if metadata.Path == "/v1/auth/login" || metadata.Path == "/v1/auth/refresh" || metadata.Path == "/v1/openapi.json" || strings.HasPrefix(metadata.Path, "/health/") {
			item.Security = []map[string][]string{}
		}
		document.Paths[metadata.Path][strings.ToLower(metadata.Method)] = item
	}
	return document
}
