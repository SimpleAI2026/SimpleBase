// 文件说明：gofunction_errors.go — Go 云函数的源码校验与错误映射（gofunction 解释器错误 → API 错误协议）
//
// 用法
//
//	同包调用：validateSource() / mapGoFunctionValidationError() ← gofunction_handler.go 的 Create / CreateVersion；
//	mapGoFunctionRunError() ← gofunction_invoke.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/gofunction_errors.go
//	         │ 依赖
//	         ▼
//	[下游] gofunction, internal/systemdb
//	[同包] gofunction_handler.go, gofunction_invoke.go
//
// 主要符号
//
//	validateGoFunctionSource(...) / mapNotFound(...)
//	(*GoFunctionHandler).validateSource(...)
//	mapGoFunctionValidationError(...) / mapGoFunctionRunError(...)

package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gofunction"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

func mapNotFound(err error) error {
	rid := "" // filled by WriteError via Error()
	if errors.Is(err, sql.ErrNoRows) {
		return NewAPIError(http.StatusNotFound, "not_found", "resource not found", rid)
	}
	return err
}

func (h *GoFunctionHandler) validateSource(c echo.Context, source string) ([]gofunction.FuncInfo, error) {
	start := time.Now()
	infos, err := gofunction.ValidateHTTPFuncs(source)
	if h.store != nil {
		pc := projectOf(c)
		if pc.ID != "" && !systemdb.IsAdminProject(pc.ID) {
			h.store.RecordMetric(systemdb.MetricSample{
				ProjectID: pc.ID, Name: "gofunction_compile_ms", Value: float64(time.Since(start).Milliseconds()),
			})
		}
	}
	return infos, err
}

// validateGoFunctionSource 校验源码非空与体积上限。
func validateGoFunctionSource(source string) error {
	rid := ""
	if source == "" {
		return badRequestWithID("source is required", rid)
	}
	if len(source) > gofunctionMaxSourceBytes {
		return NewAPIError(http.StatusRequestEntityTooLarge, "request_too_large", "source exceeds 256KB limit", rid)
	}
	return nil
}

// mapGoFunctionValidationError 映射 ValidateHTTPFuncs 错误。
// 约定/签名问题 → gofunction_signature_invalid；编译错误 → gofunction_compile_error。
func mapGoFunctionValidationError(err error, rid string) error {
	if err == nil {
		return nil
	}
	msg := strutil.TruncateEllipsis(err.Error(), 512)
	// 编译/语法错误才用 compile_error；签名与导出约定问题统一 signature_invalid
	if strings.Contains(err.Error(), "syntax error") ||
		strings.Contains(err.Error(), "expected") ||
		strings.Contains(err.Error(), "解析失败") {
		return NewAPIError(http.StatusBadRequest, "gofunction_compile_error", msg, rid)
	}
	return NewAPIError(http.StatusBadRequest, "gofunction_signature_invalid", msg, rid)
}

// mapGoFunctionRunError 映射 RunJSON 错误。
func mapGoFunctionRunError(err error, rid string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gofunction.ErrFunctionNotFound) {
		return NewAPIError(http.StatusNotFound, "function_not_found", "function not found", rid)
	}
	if errors.Is(err, gofunction.ErrTimeout) {
		return NewAPIError(http.StatusGatewayTimeout, "request_timeout", "go function execution timed out", rid)
	}
	if errors.Is(err, gofunction.ErrBind) {
		return badRequestWithID(strutil.TruncateEllipsis(err.Error(), 512), rid)
	}
	if errors.Is(err, gofunction.ErrCompile) {
		return NewAPIError(http.StatusInternalServerError, "gofunction_compile_error", strutil.TruncateEllipsis(err.Error(), 512), rid)
	}
	return NewAPIError(http.StatusInternalServerError, "gofunction_runtime_error", strutil.TruncateEllipsis(err.Error(), 512), rid)
}
