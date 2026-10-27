// 文件说明：e2b_envd_handler.go — internal/api 中 Health、Envs 等符号的实现
//
// 用法
//
//	(h *SandboxHandler) Health(c echo.Context) error  // Health 对应 envd GET /health：沙盒存在且运行中返回 204，否则 502（SDK 把 502 视为未运行）
//	(h *SandboxHandler) Envs(c echo.Context) error  // Envs 对应 envd GET /envs：沙盒的默认环境变量（创建时的 envVars）
//	同包调用：Error() → agent_handler.go, agent_schedule_handler.go 等；Envs() → e2b_process_handler.go, router.go 等；envdScope() → e2b_process_handler.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/e2b_envd_handler.go
//	         │ 依赖
//	         ▼
//	[下游] gosandbox/vm, internal/auth, internal/catalog, internal/e2bproto 等
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] Error() ← agent_handler.go, agent_schedule_handler.go, api_keys_handler.go
//
// 主要符号
//
//	(h *SandboxHandler) Health(c echo.Context) error
//	(h *SandboxHandler) Envs(c echo.Context) error
//	(h *SandboxHandler) EnvdMetrics(c echo.Context) error
//	(h *SandboxHandler) FilesGet(c echo.Context) error
//	(h *SandboxHandler) FilesPost(c echo.Context) error
//	(h *SandboxHandler) FsStat(c echo.Context) error
//	... 另有 20 个顶层符号

package api

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/e2bproto"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

// envdAuth 是 envd 路由的认证中间件。两条路：
//
//  1. X-Access-Token：无状态 HMAC 令牌，绑定项目 + 沙盒；必须与请求头 E2b-Sandbox-Id 一致（SDK 路径）。
//  2. 其余（控制台 / sbcli）：走正常的 Bearer / JWT 认证，沙盒 ID 同样取自 E2b-Sandbox-Id，项目走 X-SimpleBase-Project 规则。
//
// 通过后把项目与沙盒 ID 注入 ctx；写操作由各 handler 再校验写权限。
// 令牌方式下主体没有 Principal，因此令牌只授予「该沙盒的数据面」，不能调控制面。
func (h *SandboxHandler) envdAuth(authMW echo.MiddlewareFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		withBearer := authMW(func(c echo.Context) error {
			return e2bProjectMiddleware(h.deps)(func(c echo.Context) error {
				return h.bindSandbox(c, next, "")
			})(c)
		})
		return func(c echo.Context) error {
			if tok := strings.TrimSpace(c.Request().Header.Get("X-Access-Token")); tok != "" {
				claims, err := h.signer.Verify(tok, h.now())
				if err != nil {
					return writeE2BErr(c, auth.ErrInvalidCredentials)
				}
				ctx := WithProject(c.Request().Context(), ProjectContext{ID: claims.Project})
				c.SetRequest(c.Request().WithContext(ctx))
				c.Set(ctxTokenAuth, true)
				return h.bindSandbox(c, next, claims.Sandbox)
			}
			return withBearer(c)
		}
	}
}

const (
	ctxTokenAuth = "e2b.token_auth"
	ctxSandboxID = "e2b.sandbox_id"
)

// bindSandbox 取 E2b-Sandbox-Id，校验它与令牌绑定的沙盒一致，然后放进 echo.Context。
func (h *SandboxHandler) bindSandbox(c echo.Context, next echo.HandlerFunc, bound string) error {
	id := strings.TrimSpace(c.Request().Header.Get("E2b-Sandbox-Id"))
	if id == "" {
		id = c.QueryParam("sandboxID")
	}
	if id == "" {
		return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "E2b-Sandbox-Id header is required"))
	}
	if bound != "" && bound != id {
		// 令牌属于另一个沙盒：当作凭据无效，不暴露存在性。
		return writeE2BErr(c, auth.ErrInvalidCredentials)
	}
	c.Set(ctxSandboxID, id)
	return next(c)
}

// envdScope 取 envd 请求的项目与沙盒，并校验权限：读需要读权限，写需要写权限且实例可写、非系统项目。
// 令牌方式视为已授权写（令牌只在写权限调用方的响应里下发），仍受实例只读与系统项目限制。
func (h *SandboxHandler) envdScope(c echo.Context, write bool) (projectID, sandboxID string, err error) {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return "", "", echo.NewHTTPError(http.StatusBadRequest, "project context missing")
	}
	sandboxID, _ = c.Get(ctxSandboxID).(string)
	if tokenAuth, _ := c.Get(ctxTokenAuth).(bool); tokenAuth {
		if write {
			if !h.writable {
				return "", "", echo.NewHTTPError(http.StatusServiceUnavailable, "instance is read-only")
			}
			if catalog.IsSystemProject(pc.ID) {
				return "", "", NewAPIError(http.StatusForbidden, "system_project_protected", "system project does not support sandboxes", "")
			}
		}
		return pc.ID, sandboxID, nil
	}
	if write {
		if _, err := h.requireWrite(c); err != nil {
			return "", "", err
		}
	} else if _, err := h.requireRead(c); err != nil {
		return "", "", err
	}
	return pc.ID, sandboxID, nil
}

// ---- /health /envs /metrics ----

// Health 对应 envd GET /health：沙盒存在且运行中返回 204，否则 502（SDK 把 502 视为未运行）。
func (h *SandboxHandler) Health(c echo.Context) error {
	p, id, err := h.envdScope(c, false)
	if err != nil {
		return writeE2BErr(c, err)
	}
	row, err := h.svc.Get(c.Request().Context(), p, id, false)
	if err != nil {
		return writeE2BErr(c, err)
	}
	if stateOf(row.Status) != "running" {
		return c.NoContent(http.StatusBadGateway)
	}
	return c.NoContent(http.StatusNoContent)
}

// Envs 对应 envd GET /envs：沙盒的默认环境变量（创建时的 envVars）。
func (h *SandboxHandler) Envs(c echo.Context) error {
	p, id, err := h.envdScope(c, false)
	if err != nil {
		return writeE2BErr(c, err)
	}
	if _, err := h.svc.Get(c.Request().Context(), p, id, false); err != nil {
		return writeE2BErr(c, err)
	}
	return c.JSON(http.StatusOK, h.svc.Envs(id))
}

// EnvdMetrics 对应 envd GET /metrics：不实现。
func (h *SandboxHandler) EnvdMetrics(c echo.Context) error { return h.Unsupported(c) }

// ---- /files ----

// e2bPath 把 E2B 的路径换成沙盒内的绝对路径：相对路径相对工作目录 /workspace。
func e2bPath(workdir, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return workdir
	}
	if !strings.HasPrefix(p, "/") {
		p = path.Join(workdir, p)
	}
	return path.Clean(p)
}

func (h *SandboxHandler) workdir() string {
	if w := h.deps.Config.Sandbox.Workdir; w != "" {
		return path.Clean(w)
	}
	return "/workspace"
}

// FilesGet 对应 envd GET /files：下载文件。被截断（超过 max_file_bytes）返回 413 而不是残缺文件。
func (h *SandboxHandler) FilesGet(c echo.Context) error {
	p, id, err := h.envdScope(c, false)
	if err != nil {
		return writeE2BErr(c, err)
	}
	f, err := h.svc.ReadFile(c.Request().Context(), p, id, e2bPath(h.workdir(), c.QueryParam("path")))
	if err != nil {
		return writeE2BErr(c, err)
	}
	if f.Truncated {
		return writeE2BErr(c, sandbox.ErrFileTooLarge)
	}
	return c.Blob(http.StatusOK, "application/octet-stream", f.Content)
}

// FilesPost 对应 envd POST /files：上传文件，自动建父目录，已存在则覆盖。
// 支持 multipart/form-data（字段 file）与 application/octet-stream（需 path 查询参数）。
func (h *SandboxHandler) FilesPost(c echo.Context) error {
	p, id, err := h.envdScope(c, true)
	if err != nil {
		return writeE2BErr(c, err)
	}
	max := int64(h.deps.Config.Sandbox.MaxFileBytes)
	if max <= 0 {
		max = 1 << 20
	}
	req := c.Request()
	ct := req.Header.Get("Content-Type")
	var out []e2bproto.EntryInfo
	if strings.HasPrefix(ct, "multipart/form-data") {
		req.Body = http.MaxBytesReader(c.Response(), req.Body, max+(64<<10))
		mr, merr := req.MultipartReader()
		if merr != nil {
			return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "invalid multipart body"))
		}
		for {
			part, perr := mr.NextPart()
			if errors.Is(perr, io.EOF) {
				break
			}
			if perr != nil {
				return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "invalid multipart body"))
			}
			if part.FormName() != "file" {
				continue
			}
			target := c.QueryParam("path")
			if target == "" {
				target = part.FileName()
			}
			entry, uerr := h.putFile(c, p, id, e2bPath(h.workdir(), target), part, max)
			if uerr != nil {
				return writeE2BErr(c, uerr)
			}
			out = append(out, entry)
		}
		if len(out) == 0 {
			return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "multipart field \"file\" is required"))
		}
		return c.JSON(http.StatusOK, out)
	}
	target := c.QueryParam("path")
	if target == "" {
		return writeE2BErr(c, echo.NewHTTPError(http.StatusBadRequest, "path query parameter is required"))
	}
	entry, uerr := h.putFile(c, p, id, e2bPath(h.workdir(), target), req.Body, max)
	if uerr != nil {
		return writeE2BErr(c, uerr)
	}
	return c.JSON(http.StatusOK, []e2bproto.EntryInfo{entry})
}

func (h *SandboxHandler) putFile(c echo.Context, projectID, id, target string, r io.Reader, max int64) (e2bproto.EntryInfo, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return e2bproto.EntryInfo{}, sandbox.ErrFileTooLarge
		}
		return e2bproto.EntryInfo{}, echo.NewHTTPError(http.StatusBadRequest, "cannot read upload")
	}
	if int64(len(data)) > max {
		return e2bproto.EntryInfo{}, sandbox.ErrFileTooLarge
	}
	if err := h.svc.WriteFile(c.Request().Context(), projectID, id, target, data); err != nil {
		return e2bproto.EntryInfo{}, err
	}
	h.record(c, projectID, id, "file.write", "bytes="+itoa(len(data)))
	return e2bproto.EntryInfo{Name: path.Base(target), Type: e2bproto.FileTypeFile, Path: target,
		Size: e2bproto.Int64(len(data)), Mode: 0o644, Permissions: e2bproto.PermString(0o644, false),
		ModifiedTime: e2bproto.FormatTimestamp(h.now())}, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// ---- filesystem.Filesystem（一元 Connect-JSON）----

func entryOf(fe vm.FileEntry) e2bproto.EntryInfo {
	t := e2bproto.FileTypeFile
	switch fe.Kind {
	case "directory":
		t = e2bproto.FileTypeDirectory
	case "symlink":
		t = e2bproto.FileTypeSymlink
	}
	mode := fe.Mode
	if mode == 0 {
		if fe.Kind == "directory" {
			mode = 0o755
		} else {
			mode = 0o644
		}
	}
	return e2bproto.EntryInfo{Name: fe.Name, Type: t, Path: fe.Path, Size: e2bproto.Int64(fe.Size), Mode: mode,
		Permissions: e2bproto.PermString(mode, fe.Kind == "directory"), Owner: "user", Group: "user",
		ModifiedTime: e2bproto.FormatTimestamp(fe.ModTime), IsSymlink: fe.Kind == "symlink"}
}

// readConnectUnary 读一元 Connect 请求体。Content-Type 必须是 JSON；二进制 protobuf 不支持。
func (h *SandboxHandler) readConnectUnary(c echo.Context, dst any) error {
	ct := c.Request().Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/proto") || strings.HasPrefix(ct, "application/connect+proto") {
		return &connectErr{e2bproto.CodeUnimplemented, "binary protobuf is not supported; use the JSON encoding"}
	}
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, 1<<20))
	if err != nil {
		return &connectErr{e2bproto.CodeInvalidArgument, "cannot read request body"}
	}
	if err := e2bproto.DecodeUnary(body, dst); err != nil {
		return &connectErr{e2bproto.CodeInvalidArgument, "invalid JSON body"}
	}
	return nil
}

type connectErr struct{ code, msg string }

func (e *connectErr) Error() string { return e.code + ": " + e.msg }

// writeConnectErr 写一元 RPC 错误。
func writeConnectErr(c echo.Context, err error) error {
	var ce *connectErr
	if errors.As(err, &ce) {
		return c.JSON(e2bproto.HTTPStatus(ce.code), e2bproto.Error{Code: ce.code, Message: ce.msg})
	}
	return writeE2BErr(c, err)
}

// FsStat 对应 filesystem.Filesystem/Stat。
func (h *SandboxHandler) FsStat(c echo.Context) error {
	var req e2bproto.StatRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, false)
	if err != nil {
		return writeConnectErr(c, err)
	}
	target := e2bPath(h.workdir(), req.Path)
	entry, err := h.stat(c, p, id, target)
	if err != nil {
		return writeConnectErr(c, err)
	}
	return c.JSON(http.StatusOK, e2bproto.StatResponse{Entry: entry})
}

// stat 通过列父目录得到条目信息（驱动没有单独的 stat）。工作目录本身视为目录。
func (h *SandboxHandler) stat(c echo.Context, projectID, id, target string) (e2bproto.EntryInfo, error) {
	if target == h.workdir() {
		return e2bproto.EntryInfo{Name: path.Base(target), Type: e2bproto.FileTypeDirectory, Path: target,
			Mode: 0o755, Permissions: e2bproto.PermString(0o755, true), Owner: "user", Group: "user"}, nil
	}
	entries, err := h.svc.ListDir(c.Request().Context(), projectID, id, path.Dir(target))
	if err != nil {
		if errors.Is(err, vm.ErrBackend) {
			return e2bproto.EntryInfo{}, &connectErr{e2bproto.CodeNotFound, "path not found"}
		}
		return e2bproto.EntryInfo{}, err
	}
	for _, fe := range entries {
		if fe.Path == target || fe.Name == path.Base(target) {
			fe.Path = target
			return entryOf(fe), nil
		}
	}
	return e2bproto.EntryInfo{}, &connectErr{e2bproto.CodeNotFound, "path not found"}
}

// FsListDir 对应 filesystem.Filesystem/ListDir。depth 大于 1 时递归（上限 8 层，总数 500）。
func (h *SandboxHandler) FsListDir(c echo.Context) error {
	var req e2bproto.ListDirRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, false)
	if err != nil {
		return writeConnectErr(c, err)
	}
	depth := int(req.Depth)
	if depth < 1 {
		depth = 1
	}
	if depth > 8 {
		depth = 8
	}
	var out []e2bproto.EntryInfo
	var walk func(dir string, level int) error
	walk = func(dir string, level int) error {
		entries, err := h.svc.ListDir(c.Request().Context(), p, id, dir)
		if err != nil {
			return err
		}
		for _, fe := range entries {
			if len(out) >= 500 {
				return nil
			}
			out = append(out, entryOf(fe))
			if fe.Kind == "directory" && level < depth {
				if err := walk(fe.Path, level+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(e2bPath(h.workdir(), req.Path), 1); err != nil {
		return writeConnectErr(c, err)
	}
	if out == nil {
		out = []e2bproto.EntryInfo{}
	}
	return c.JSON(http.StatusOK, e2bproto.ListDirResponse{Entries: out})
}

// FsMakeDir 对应 filesystem.Filesystem/MakeDir：用写入占位文件再删除的方式创建目录。
func (h *SandboxHandler) FsMakeDir(c echo.Context) error {
	var req e2bproto.MakeDirRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, true)
	if err != nil {
		return writeConnectErr(c, err)
	}
	target := e2bPath(h.workdir(), req.Path)
	if target == h.workdir() {
		return writeConnectErr(c, &connectErr{e2bproto.CodeAlreadyExists, "directory already exists"})
	}
	marker := path.Join(target, ".sb-mkdir")
	ctx := c.Request().Context()
	if err := h.svc.WriteFile(ctx, p, id, marker, []byte{}); err != nil {
		return writeConnectErr(c, err)
	}
	_ = h.svc.RemoveFile(ctx, p, id, marker)
	h.record(c, p, id, "mkdir", "")
	return c.JSON(http.StatusOK, e2bproto.MakeDirResponse{Entry: e2bproto.EntryInfo{Name: path.Base(target),
		Type: e2bproto.FileTypeDirectory, Path: target, Mode: 0o755, Permissions: e2bproto.PermString(0o755, true),
		Owner: "user", Group: "user", ModifiedTime: e2bproto.FormatTimestamp(h.now())}})
}

// FsMove 对应 filesystem.Filesystem/Move：读源、写目标、删源。只支持文件（驱动没有 rename）。
func (h *SandboxHandler) FsMove(c echo.Context) error {
	var req e2bproto.MoveRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, true)
	if err != nil {
		return writeConnectErr(c, err)
	}
	src, dst := e2bPath(h.workdir(), req.Source), e2bPath(h.workdir(), req.Destination)
	ctx := c.Request().Context()
	f, err := h.svc.ReadFile(ctx, p, id, src)
	if err != nil {
		return writeConnectErr(c, err)
	}
	if f.Truncated {
		return writeConnectErr(c, sandbox.ErrFileTooLarge)
	}
	if err := h.svc.WriteFile(ctx, p, id, dst, f.Content); err != nil {
		return writeConnectErr(c, err)
	}
	if err := h.svc.RemoveFile(ctx, p, id, src); err != nil {
		return writeConnectErr(c, err)
	}
	h.record(c, p, id, "file.move", "")
	return c.JSON(http.StatusOK, e2bproto.MoveResponse{Entry: e2bproto.EntryInfo{Name: path.Base(dst),
		Type: e2bproto.FileTypeFile, Path: dst, Size: e2bproto.Int64(len(f.Content)), Mode: 0o644,
		Permissions: e2bproto.PermString(0o644, false), Owner: "user", Group: "user",
		ModifiedTime: e2bproto.FormatTimestamp(h.now())}})
}

// FsRemove 对应 filesystem.Filesystem/Remove。
func (h *SandboxHandler) FsRemove(c echo.Context) error {
	var req e2bproto.RemoveRequest
	if err := h.readConnectUnary(c, &req); err != nil {
		return writeConnectErr(c, err)
	}
	p, id, err := h.envdScope(c, true)
	if err != nil {
		return writeConnectErr(c, err)
	}
	if err := h.svc.RemoveFile(c.Request().Context(), p, id, e2bPath(h.workdir(), req.Path)); err != nil {
		return writeConnectErr(c, err)
	}
	h.record(c, p, id, "file.delete", "")
	return c.JSON(http.StatusOK, e2bproto.RemoveResponse{})
}

// FsUnimplemented 对应 WatchDir / CreateWatcher / GetWatcherEvents / RemoveWatcher。
func (h *SandboxHandler) FsUnimplemented(c echo.Context) error {
	return writeConnectErr(c, &connectErr{e2bproto.CodeUnimplemented, "directory watching is not supported"})
}
