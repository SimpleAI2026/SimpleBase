// 文件说明：sandbox_logs_handler.go — E2B logs 与入站端口转发的 HTTP 面
//
// 两个端点：
//
//	GET  /e2b/sandboxes/{id}/logs        历史输出（text/plain；?follow=1 时为 SSE）
//	POST /e2b/sandboxes/{id}/host/{port} 沙盒内 127.0.0.1:port 的请求代理
//
// 为什么不直接暴露沙盒端口：沙盒默认没有网络栈，宿主也不该为每个沙盒开
// 监听。反向代理收到请求后经控制通道开一条中继，沙盒内进程看到的是来自
// 127.0.0.1 的连接——这正是 E2B getHost(port) 的语义。
//
// 调用架构
//
//	[上游] internal/api/router.go（路由注册）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sandbox_logs_handler.go
//	         │
//	         ▼
//	[下游] internal/sandbox（Logs / DialPort）, internal/utils/strutil
//	[同包] sandbox_handler.go, e2b_envd_handler.go
//
// 主要符号
//
//	(h *SandboxHandler) Logs(c echo.Context) error
//	(h *SandboxHandler) HostProxy(c echo.Context) error

package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// Logs 对应 GET /sandboxes/{id}/logs。
//
// 默认返回 text/plain（历史输出，一次性）；follow=1 时用 SSE 持续推送。
// 不用 chunked 直接写是因为调用方要能区分「还没结束」与「已经结束」，
// SSE 的 event 边界更明确。
func (h *SandboxHandler) Logs(c echo.Context) error {
	p, err := h.requireRead(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	if !h.svc.CanLogs() {
		return c.JSON(http.StatusNotImplemented, e2bErrorBody{Code: http.StatusNotImplemented,
			Message: "sandbox logs are not available on this backend", ErrorCode: "unsupported_feature"})
	}
	id := c.Param("sandboxID")
	lines, _ := strconv.Atoi(c.QueryParam("lines"))
	if lines < 0 {
		lines = 0
	}
	var since time.Time
	if s := c.QueryParam("since"); s != "" {
		if v, perr := strconv.ParseInt(s, 10, 64); perr == nil {
			since = time.Unix(v, 0).UTC()
		}
	}
	var streams []string
	for _, s := range strutil.SplitCSV(c.QueryParam("stream")) {
		switch s {
		case "stdout", "stderr":
			streams = append(streams, s)
		}
	}

	recs, err := h.svc.Logs(c.Request().Context(), p, id, vm.LogQuery{
		Lines: lines, Since: since, Streams: streams,
	})
	if err != nil {
		return writeE2BErr(c, err)
	}
	h.record(c, p, id, "logs", "")

	if c.QueryParam("follow") != "1" {
		c.Response().Header().Set(echo.HeaderContentType, "text/plain; charset=utf-8")
		c.Response().WriteHeader(http.StatusOK)
		for _, r := range recs {
			if _, werr := c.Response().Write(r.Data); werr != nil {
				return nil
			}
		}
		return nil
	}

	// follow：先回放历史，再按时间推送新记录（轮询日志文件的尾部）。
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)
	flusher, _ := c.Response().Writer.(http.Flusher)
	writeEvent := func(r vm.LogRecord) bool {
		// SSE 的 data 不能含裸换行：按行拆成多条 data。
		for _, line := range strings.Split(strings.TrimSuffix(string(r.Data), "\n"), "\n") {
			if _, werr := fmt.Fprintf(c.Response().Writer, "data: %s\n", line); werr != nil {
				return false
			}
		}
		if _, werr := fmt.Fprint(c.Response().Writer, "\n"); werr != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	}
	for _, r := range recs {
		if !writeEvent(r) {
			return nil
		}
	}
	// 增量：每 500ms 拉一次比上次更新的记录。
	last := since
	if len(recs) > 0 {
		last = recs[len(recs)-1].Time
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case <-ticker.C:
			more, lerr := h.svc.Logs(c.Request().Context(), p, id, vm.LogQuery{
				Since: last.Add(time.Nanosecond), Streams: streams,
			})
			if lerr != nil {
				if errors.Is(lerr, vm.ErrNotSupported) {
					return nil
				}
				continue
			}
			for _, r := range more {
				if !writeEvent(r) {
					return nil
				}
				if r.Time.After(last) {
					last = r.Time
				}
			}
		}
	}
}

// HostProxy 对应 * /sandboxes/{id}/host/{port}：把请求隧道进沙盒内的端口。
//
// 全双工隧道（M3）：
//
//   - 请求体边读边转发，不缓冲（大上传不占内存）；
//   - 响应边收边写，首字节一到就 flush（流式响应、SSE 立刻可见）；
//   - 请求体发完时对隧道做半关闭（CloseWrite），让沙盒内的 HTTP 服务端
//     知道请求结束——这是 keep-alive 与 HTTP/1.1 能正常工作的前提；
//   - 任何一侧断开都结束整条隧道。
//
// 不支持 WebSocket 升级：那需要劫持连接（http.Hijacker）并把两个方向都
// 提升为裸字节流。响应里的 Upgrade 会被原样带回，但当前实现会把它当
// 普通响应体读完——所以 WebSocket 需要在网关层处理，不在这里。
func (h *SandboxHandler) HostProxy(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	if !h.svc.CanPorts() {
		return c.JSON(http.StatusNotImplemented, e2bErrorBody{Code: http.StatusNotImplemented,
			Message: "port forwarding is not available on this backend", ErrorCode: "unsupported_feature"})
	}
	port, perr := strconv.Atoi(c.Param("port"))
	if perr != nil || port < 1 || port > 65535 {
		return writeE2BErr(c, NewAPIError(http.StatusBadRequest, "invalid_port", "port must be 1-65535", ""))
	}
	id := c.Param("sandboxID")
	conn, err := h.svc.DialPort(c.Request().Context(), p, id, uint16(port))
	if err != nil {
		return writeE2BErr(c, err)
	}
	defer conn.Close()
	h.record(c, p, id, "host.connect", strconv.Itoa(port))

	// 请求体 → 沙盒。发完做半关闭：沙盒内的服务端据此结束「读请求」。
	go func() {
		_, _ = io.Copy(conn, c.Request().Body)
		if cw, ok := conn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	}()

	c.Response().Header().Set(echo.HeaderContentType, "application/octet-stream")
	c.Response().WriteHeader(http.StatusOK)
	// 响应 → 客户端，逐块 flush：流式接口不能等到缓冲满。
	buf := make([]byte, 32<<10)
	flusher, _ := c.Response().Writer.(http.Flusher)
	for {
		n, rerr := conn.Read(buf)
		if n > 0 {
			if _, werr := c.Response().Writer.Write(buf[:n]); werr != nil {
				return nil
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return nil
			}
			// 隧道中途出错：响应头已经发出，只能断开让客户端感知。
			return nil
		}
	}
}
