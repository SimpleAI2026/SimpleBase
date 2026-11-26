package functionhttp

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

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

func mapNotFound(err error) error {
	rid := "" // filled by WriteError via httpkit.Error()
	if errors.Is(err, sql.ErrNoRows) {
		return httpkit.NewAPIError(http.StatusNotFound, "not_found", "resource not found", rid)
	}
	return err
}

func (h *GoFunctionHandler) validateSource(c echo.Context, source string) ([]gofunction.FuncInfo, error) {
	start := time.Now()
	infos, err := gofunction.ValidateHTTPFuncs(source)
	if h.store != nil {
		pc := httpkit.ProjectOf(c)
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
		return httpkit.BadRequestWithID("source is required", rid)
	}
	if len(source) > gofunctionMaxSourceBytes {
		return httpkit.NewAPIError(http.StatusRequestEntityTooLarge, "request_too_large", "source exceeds 256KB limit", rid)
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
		return httpkit.NewAPIError(http.StatusBadRequest, "gofunction_compile_error", msg, rid)
	}
	return httpkit.NewAPIError(http.StatusBadRequest, "gofunction_signature_invalid", msg, rid)
}

// mapGoFunctionRunError 映射 RunJSON 错误。
func mapGoFunctionRunError(err error, rid string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gofunction.ErrFunctionNotFound) {
		return httpkit.NewAPIError(http.StatusNotFound, "function_not_found", "function not found", rid)
	}
	if errors.Is(err, gofunction.ErrTimeout) {
		return httpkit.NewAPIError(http.StatusGatewayTimeout, "request_timeout", "go function execution timed out", rid)
	}
	if errors.Is(err, gofunction.ErrBind) {
		return httpkit.BadRequestWithID(strutil.TruncateEllipsis(err.Error(), 512), rid)
	}
	if errors.Is(err, gofunction.ErrCompile) {
		return httpkit.NewAPIError(http.StatusInternalServerError, "gofunction_compile_error", strutil.TruncateEllipsis(err.Error(), 512), rid)
	}
	return httpkit.NewAPIError(http.StatusInternalServerError, "gofunction_runtime_error", strutil.TruncateEllipsis(err.Error(), 512), rid)
}
