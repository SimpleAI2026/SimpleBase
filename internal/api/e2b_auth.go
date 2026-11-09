// 文件说明：e2b_auth.go — internal/api 的内部实现（e2bPrefix、e2bEnvdPrefix 等）
//
// 用法
//
//	type e2bErrorBody（包内使用）
//	type e2bMapping（包内使用）
//	同包调用：writeE2BErr() → e2b_envd_handler.go, e2b_terminal.go 等；e2bProjectMiddleware() → e2b_envd_handler.go, router.go 等；Issue() → e2b_terminal.go, e2b_tickets.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/e2b_auth.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/vm, internal/auth, internal/catalog, internal/e2bproto 等
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] writeE2BErr() ← e2b_envd_handler.go, e2b_terminal.go, router.go
//
// 主要符号
//
//	const e2bPrefix
//	const e2bEnvdPrefix
//	const defaultEnvdVersion
//	const e2bProjectHeader
//	type e2bErrorBody struct {…}
//	isE2BPath(p string) bool
//	... 另有 16 个顶层符号

package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/e2bproto"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

// E2B 兼容 API 的路径前缀。控制面和控制台用的 envd 副本挂在这里；
// SDK 用的 envd 路径挂在根（SDK 拼 /files 会丢掉基址里的路径前缀）。
const (
	e2bPrefix     = "/e2b"
	e2bEnvdPrefix = "/e2b/envd"
)

// E2B 响应里的 envd 版本。取一个满足官方 SDK 最低版本检查（>=0.1.0）的值，
// 可由 sandbox.e2b.envd_version 覆盖。
const defaultEnvdVersion = "0.2.4"

// e2bProjectHeader 是 SimpleBase 对标准 E2B 请求的唯一扩展：E2B 请求里没有项目，
// 主体授权了多个项目时用它指明。
const e2bProjectHeader = "X-SimpleBase-Project"

// ---- 错误体 ----

// e2bErrorBody 是 E2B 控制面 / envd HTTP 面的错误体：{code, message, error_code?}。
// 不带 request_id 包装；X-Request-ID 响应头保留。
type e2bErrorBody struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	ErrorCode string `json:"error_code,omitempty"`
}

// isE2BPath 判断请求是否属于 E2B 兼容面，用于让全局错误处理器输出 E2B 形状。
func isE2BPath(p string) bool {
	if strings.HasPrefix(p, e2bPrefix+"/") || p == e2bPrefix {
		return true
	}
	return isEnvdRootPath(p)
}

// isEnvdRootPath 判断是否是根路径上的 envd 路由。
func isEnvdRootPath(p string) bool {
	return p == "/files" || p == "/envs" ||
		strings.HasPrefix(p, "/process.Process/") || strings.HasPrefix(p, "/filesystem.Filesystem/")
}

// isConnectPath 判断是否是 Connect RPC 路径（错误体用 Connect 形状）。
func isConnectPath(p string) bool {
	for _, base := range []string{"", e2bEnvdPrefix} {
		if strings.HasPrefix(p, base+"/process.Process/") || strings.HasPrefix(p, base+"/filesystem.Filesystem/") {
			return true
		}
	}
	return false
}

type e2bMapping struct {
	status    int
	errorCode string
	message   string
	connect   string
}

// mapE2BError 把领域错误映射成 E2B 的状态码、error_code 与 Connect 错误码。
// 返回的 message 是固定文案，不透传内部细节。
func mapE2BError(err error) e2bMapping {
	switch {
	case errors.Is(err, sandbox.ErrNotFound):
		return e2bMapping{http.StatusNotFound, "sandbox_not_found", "sandbox not found", e2bproto.CodeNotFound}
	case errors.Is(err, vm.ErrFileNotFound):
		return e2bMapping{http.StatusNotFound, "file_not_found", "file not found", e2bproto.CodeNotFound}
	case errors.Is(err, sandbox.ErrNameConflict):
		return e2bMapping{http.StatusConflict, "sandbox_name_conflict", "sandbox name already exists", e2bproto.CodeAlreadyExists}
	case errors.Is(err, sandbox.ErrLimitExceeded):
		return e2bMapping{http.StatusTooManyRequests, "sandbox_limit_exceeded", "sandbox limit exceeded", e2bproto.CodeResourceExhausted}
	case errors.Is(err, sandbox.ErrInvalidSpec):
		return e2bMapping{http.StatusBadRequest, "invalid_request", "invalid request", e2bproto.CodeInvalidArgument}
	case errors.Is(err, sandbox.ErrInvalidPath):
		return e2bMapping{http.StatusBadRequest, "invalid_path", "invalid path", e2bproto.CodeInvalidArgument}
	case errors.Is(err, sandbox.ErrFileTooLarge):
		return e2bMapping{http.StatusRequestEntityTooLarge, "file_too_large", "file too large", e2bproto.CodeResourceExhausted}
	case errors.Is(err, sandbox.ErrBusy):
		return e2bMapping{http.StatusConflict, "sandbox_busy", "sandbox is busy", e2bproto.CodeFailedPrecondition}
	case errors.Is(err, sandbox.ErrGone):
		return e2bMapping{http.StatusGone, "sandbox_gone", "sandbox has expired", e2bproto.CodeNotFound}
	case errors.Is(err, vm.ErrNotSupported):
		return e2bMapping{http.StatusNotImplemented, "unsupported_feature", "feature is not supported by this backend", e2bproto.CodeUnimplemented}
	case errors.Is(err, vm.ErrUnavailable):
		return e2bMapping{http.StatusServiceUnavailable, "sandbox_unavailable", "sandbox is unavailable", e2bproto.CodeUnavailable}
	case errors.Is(err, vm.ErrBackend):
		return e2bMapping{http.StatusBadGateway, "sandbox_backend_error", "sandbox backend error", e2bproto.CodeInternal}
	}
	return e2bMapping{}
}

// writeE2BErr 写 E2B 形状的错误。Connect 路径用 Connect 错误体，其余用 {code, message, error_code}。
// 未映射的错误记录日志并返回 500，不泄露细节。
func writeE2BErr(c echo.Context, err error) error {
	var he *echo.HTTPError
	var ae *APIError
	m := mapE2BError(err)
	switch {
	case m.status != 0:
	case errors.As(err, &ae):
		m = e2bMapping{status: ae.HTTPStatus, errorCode: ae.Body.Error.Code, message: ae.Body.Error.Message, connect: connectFor(ae.HTTPStatus)}
	case errors.As(err, &he):
		m = e2bMapping{status: he.Code, errorCode: "http_error", message: safeMessage(he), connect: connectFor(he.Code)}
	case errors.Is(err, auth.ErrMissingCredentials), errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrKeyRevoked):
		m = e2bMapping{http.StatusUnauthorized, "unauthorized", "invalid or missing credentials", e2bproto.CodeUnauthenticated}
	case errors.Is(err, auth.ErrForbidden), errors.Is(err, catalog.ErrCrossProject):
		m = e2bMapping{http.StatusForbidden, "forbidden", "access denied", e2bproto.CodePermissionDenied}
	default:
		if ae2 := Error(err, RequestIDFromContext(c.Request().Context())); ae2 != nil {
			m = e2bMapping{status: ae2.HTTPStatus, errorCode: ae2.Body.Error.Code, message: ae2.Body.Error.Message, connect: connectFor(ae2.HTTPStatus)}
			break
		}
		logUnhandledError(c, err, RequestIDFromContext(c.Request().Context()))
		m = e2bMapping{http.StatusInternalServerError, "internal_error", "internal error", e2bproto.CodeInternal}
	}
	if m.status == http.StatusTooManyRequests {
		c.Response().Header().Set("Retry-After", "5")
	}
	if isConnectPath(c.Request().URL.Path) {
		return c.JSON(e2bproto.HTTPStatus(m.connect), e2bproto.Error{Code: m.connect, Message: m.message})
	}
	return c.JSON(m.status, e2bErrorBody{Code: m.status, Message: m.message, ErrorCode: m.errorCode})
}

func connectFor(status int) string {
	switch status {
	case http.StatusBadRequest:
		return e2bproto.CodeInvalidArgument
	case http.StatusUnauthorized:
		return e2bproto.CodeUnauthenticated
	case http.StatusForbidden:
		return e2bproto.CodePermissionDenied
	case http.StatusNotFound:
		return e2bproto.CodeNotFound
	case http.StatusConflict:
		return e2bproto.CodeAlreadyExists
	case http.StatusTooManyRequests, http.StatusRequestEntityTooLarge:
		return e2bproto.CodeResourceExhausted
	case http.StatusNotImplemented:
		return e2bproto.CodeUnimplemented
	case http.StatusServiceUnavailable:
		return e2bproto.CodeUnavailable
	}
	return e2bproto.CodeInternal
}

// ---- 凭据归一 ----

// e2bCredentialMiddleware 把 E2B 的 X-API-Key 归一成 Authorization: Bearer，
// 然后由后面的 authMW 统一认证。已有 Authorization 时不覆盖。
// 必须在 authMW 之前。
func e2bCredentialMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			if req.Header.Get(echo.HeaderAuthorization) == "" {
				if k := strings.TrimSpace(req.Header.Get("X-API-Key")); k != "" {
					req.Header.Set(echo.HeaderAuthorization, "Bearer "+k)
				}
			}
			return next(c)
		}
	}
}

// ---- envd 访问令牌 ----

// envdClaims 是 X-Access-Token 的载荷。无状态：项目 + 沙盒 + 过期时间，用 HMAC 签名。
type envdClaims struct {
	Project string `json:"p"`
	Sandbox string `json:"s"`
	Exp     int64  `json:"exp"`
}

// envdTokenSigner 签发 / 校验 envd 访问令牌。密钥取 api_key_hash_secret 派生，
// 加上固定前缀做域分离，避免与 API Key 的 HMAC 互相冒用。
type envdTokenSigner struct{ key []byte }

func newEnvdTokenSigner(secret string) *envdTokenSigner {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte("simplebase-e2b-envd-token-v1"))
	return &envdTokenSigner{key: h.Sum(nil)}
}

func (s *envdTokenSigner) sign(payload []byte) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Issue 为沙盒签发令牌，到期时间不晚于沙盒到期时间。
func (s *envdTokenSigner) Issue(projectID, sandboxID string, exp time.Time) string {
	b, _ := json.Marshal(envdClaims{Project: projectID, Sandbox: sandboxID, Exp: exp.Unix()})
	return s.sign(b)
}

var errEnvdToken = errors.New("invalid envd access token")

// Verify 校验令牌；now 注入便于测试。
func (s *envdTokenSigner) Verify(token string, now time.Time) (envdClaims, error) {
	i := strings.IndexByte(token, '.')
	if i <= 0 {
		return envdClaims{}, errEnvdToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(token[:i])
	if err != nil {
		return envdClaims{}, errEnvdToken
	}
	got, err := base64.RawURLEncoding.DecodeString(token[i+1:])
	if err != nil {
		return envdClaims{}, errEnvdToken
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write(payload)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return envdClaims{}, errEnvdToken
	}
	var c envdClaims
	if json.Unmarshal(payload, &c) != nil || c.Project == "" || c.Sandbox == "" {
		return envdClaims{}, errEnvdToken
	}
	if now.Unix() >= c.Exp {
		return envdClaims{}, errEnvdToken
	}
	return c, nil
}

// ---- 项目解析 ----

// resolveE2BProject 解析 E2B 请求所属的项目，并校验主体的访问权。
//
//  1. 请求头 X-SimpleBase-Project 存在则取它；
//  2. 否则主体恰好授权一个项目则取它；
//  3. 否则 400。
//
// 返回的 ProjectContext 与 projectContextMiddlewareEcho 注入的一致，可直接放进 ctx。
func resolveE2BProject(c echo.Context, deps Dependencies) (ProjectContext, error) {
	ctx := c.Request().Context()
	p, ok := PrincipalFromContext(ctx)
	if !ok {
		return ProjectContext{}, auth.ErrMissingCredentials
	}
	projectID := strings.TrimSpace(c.Request().Header.Get(e2bProjectHeader))
	if projectID == "" {
		switch len(p.ProjectIDs) {
		case 1:
			for id := range p.ProjectIDs {
				projectID = id
			}
		case 0:
			return ProjectContext{}, echo.NewHTTPError(http.StatusBadRequest, "no project is authorized for this credential")
		default:
			return ProjectContext{}, echo.NewHTTPError(http.StatusBadRequest, "multiple projects authorized: set the "+e2bProjectHeader+" header")
		}
	}
	if deps.Catalog == nil {
		return ProjectContext{}, echo.NewHTTPError(http.StatusServiceUnavailable, "catalog not configured")
	}
	tenantID, err := deps.Catalog.ResolveProjectTenant(ctx, projectID)
	if err != nil {
		return ProjectContext{}, err
	}
	if !auth.CheckProjectAccess(p, projectID, tenantID, catalog.IsSystemProject(projectID)) {
		return ProjectContext{}, catalog.ErrCrossProject
	}
	return ProjectContext{ID: projectID, TenantID: tenantID}, nil
}

// e2bProjectMiddleware 在认证之后解析项目并注入 ctx。
func e2bProjectMiddleware(deps Dependencies) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			pc, err := resolveE2BProject(c, deps)
			if err != nil {
				return writeE2BErr(c, err)
			}
			ctx := catalog.WithResolvedProjectTenant(c.Request().Context(), pc.ID, pc.TenantID)
			ctx = WithProject(ctx, pc)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
