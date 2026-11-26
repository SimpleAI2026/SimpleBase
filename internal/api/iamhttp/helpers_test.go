package iamhttp

// helpers_test.go：子包内测试的最小 HTTP 夹具（与 api 同包测试的 doRequest 同形）。

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"

	"github.com/labstack/echo/v4"
)

func doRequest(e *echo.Echo, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}
