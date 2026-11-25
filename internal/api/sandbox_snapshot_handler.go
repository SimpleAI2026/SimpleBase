// 文件说明：sandbox_snapshot_handler.go — 快照与 fork 的 HTTP 面
//
// 取代原来的 501 占位：
//
//	POST   /e2b/sandboxes/{id}/snapshots   给沙盒拍快照
//	GET    /e2b/sandboxes/{id}/snapshots   列出快照
//	DELETE /e2b/snapshots/{snapshotID}     删快照
//	POST   /e2b/sandboxes/{id}/fork        从沙盒的最近快照 fork
//	POST   /e2b/snapshots/{snapshotID}/fork  从指定快照 fork
//
// 语义边界（响应里 processes=false 也在说这件事）：快照只有文件系统。
// fork 出来的沙盒没有原沙盒里正在跑的进程，需要重新启动。这是共享内核
// 的容器做不到内存快照的直接后果，不隐藏在请求参数里。
//
// 调用架构
//
//	[上游] internal/api/router.go（路由注册）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/sandbox_snapshot_handler.go
//	         │
//	         ▼
//	[下游] internal/sandbox（Snapshot / Snapshots / Fork）
//	[同包] sandbox_handler.go, sandbox_logs_handler.go
//
// 主要符号
//
//	(h *SandboxHandler) SnapshotCreate(c echo.Context) error
//	(h *SandboxHandler) SnapshotList(c echo.Context) error
//	(h *SandboxHandler) SnapshotRemove(c echo.Context) error
//	(h *SandboxHandler) Fork(c echo.Context) error
//	(h *SandboxHandler) SnapshotFork(c echo.Context) error

package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/sandbox"
)

// e2bSnapshot 是快照的 E2B 形状。
type e2bSnapshot struct {
	SnapshotID string `json:"snapshotID"`
	// Names 与 E2B 一致：快照可以对应多个名字（这里恒为沙盒名）。
	Names []string `json:"names"`
	// Label 是说明。
	Label string `json:"label,omitempty"`
	// Parent 是父快照 id。
	Parent string `json:"parent,omitempty"`
	// SizeBytes 是归档大小。
	SizeBytes int64 `json:"sizeBytes"`
	// Digest 是归档摘要。
	Digest string `json:"digest"`
	// Processes 恒为 false：只有文件系统。
	Processes bool `json:"processes"`
	// CreatedAt 是 ISO8601。
	CreatedAt string `json:"createdAt"`
}

// toE2BSnapshot 转成 E2B 形状。
//
// displayName 是用户可见的沙盒名：快照里存的是驱动层的目录名（CloudName），
// 直接回给调用方会暴露内部命名。
func toE2BSnapshot(s sandbox.Snapshot, displayName string) e2bSnapshot {
	if displayName == "" {
		displayName = s.Name
	}
	names := []string{}
	if displayName != "" {
		names = append(names, displayName)
	}
	return e2bSnapshot{
		SnapshotID: s.ID, Names: names, Label: s.Label, Parent: s.Parent,
		SizeBytes: s.Size, Digest: s.Digest, Processes: false,
		CreatedAt: s.Created.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// SnapshotCreate 对应 POST /sandboxes/{id}/snapshots。
//
// 请求体可空；{"name": "..."} 作为标签。E2B 的 name 是「快照名」，
// 这里当标签用：真正的 id 由内容决定，名字只是方便人看。
func (h *SandboxHandler) SnapshotCreate(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	if !h.svc.CanSnapshot() {
		return c.JSON(http.StatusNotImplemented, e2bErrorBody{Code: http.StatusNotImplemented,
			Message: "snapshots are not available on this backend", ErrorCode: "unsupported_feature"})
	}
	var req struct {
		Name  string `json:"name"`
		Label string `json:"label"`
	}
	// 空 body 是合法的：拍快照不需要参数。
	if c.Request().ContentLength > 0 {
		if err := decodeE2BBody(c, &req); err != nil {
			return writeE2BErr(c, err)
		}
	}
	label := req.Label
	if label == "" {
		label = req.Name
	}
	id := c.Param("sandboxID")
	snap, err := h.svc.Snapshot(c.Request().Context(), p, id, label)
	if err != nil {
		return writeE2BErr(c, err)
	}
	// 沙盒名用于展示，取不到就退回快照里记的目录名。
	display := snap.Name
	if row, gerr := h.svc.Get(c.Request().Context(), p, id, false); gerr == nil {
		display = row.Name
	}
	h.record(c, p, id, "snapshot.create", snap.ID)
	return c.JSON(http.StatusCreated, toE2BSnapshot(snap, display))
}

// SnapshotList 对应 GET /sandboxes/{id}/snapshots。
//
// 返回「这个沙盒产生的快照」：按沙盒名过滤。fork 出来的沙盒名不同，
// 因此父子关系要靠 parent 字段看。
func (h *SandboxHandler) SnapshotList(c echo.Context) error {
	p, err := h.requireRead(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	if !h.svc.CanSnapshot() {
		return c.JSON(http.StatusNotImplemented, e2bErrorBody{Code: http.StatusNotImplemented,
			Message: "snapshots are not available on this backend", ErrorCode: "unsupported_feature"})
	}
	id := c.Param("sandboxID")
	row, err := h.svc.Get(c.Request().Context(), p, id, false)
	if err != nil {
		return writeE2BErr(c, err)
	}
	all, err := h.svc.Snapshots(c.Request().Context())
	if err != nil {
		return writeE2BErr(c, err)
	}
	out := make([]e2bSnapshot, 0, len(all))
	for _, s := range all {
		// 按 CloudName 归属：驱动层的沙盒名就是 CloudName（目录名），
		// 不是用户可见的 Name——用 Name 比会永远匹配不上。
		if s.Name != "" && s.Name != row.CloudName {
			continue
		}
		out = append(out, toE2BSnapshot(s, row.Name))
	}
	return c.JSON(http.StatusOK, out)
}

// SnapshotRemove 对应 DELETE /snapshots/{snapshotID}。
func (h *SandboxHandler) SnapshotRemove(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	if !h.svc.CanSnapshot() {
		return c.JSON(http.StatusNotImplemented, e2bErrorBody{Code: http.StatusNotImplemented,
			Message: "snapshots are not available on this backend", ErrorCode: "unsupported_feature"})
	}
	snapID := c.Param("snapshotID")
	if snapID == "" {
		return writeE2BErr(c, NewAPIError(http.StatusBadRequest, "invalid_snapshot", "snapshotID is required", ""))
	}
	if err := h.svc.RemoveSnapshot(c.Request().Context(), snapID); err != nil {
		return writeE2BErr(c, err)
	}
	h.record(c, p, "", "snapshot.remove", snapID)
	return c.NoContent(http.StatusNoContent)
}

// Fork 对应 POST /sandboxes/{id}/fork：把沙盒拍成快照，再从快照建一个新沙盒。
//
// 一次往返完成两件事：调用方不必先调 snapshots 再调 fork，
// 也不必处理「快照建好了但 fork 失败」的中间状态。
func (h *SandboxHandler) Fork(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	if !h.svc.CanSnapshot() {
		return c.JSON(http.StatusNotImplemented, e2bErrorBody{Code: http.StatusNotImplemented,
			Message: "fork is not available on this backend", ErrorCode: "unsupported_feature"})
	}
	var req e2bForkReq
	if c.Request().ContentLength > 0 {
		if err := decodeE2BBody(c, &req); err != nil {
			return writeE2BErr(c, err)
		}
	}
	id := c.Param("sandboxID")
	ctx := c.Request().Context()
	src, err := h.svc.Get(ctx, p, id, false)
	if err != nil {
		return writeE2BErr(c, err)
	}
	snap, err := h.svc.Snapshot(ctx, p, id, "fork:"+src.Name)
	if err != nil {
		return writeE2BErr(c, err)
	}
	row, err := h.forkInto(c, p, src, snap.ID, req)
	if err != nil {
		// fork 失败时快照留着：它是有效的状态，调用方可以重试或手动删。
		return err
	}
	return c.JSON(http.StatusCreated, h.detail(c, row))
}

// SnapshotFork 对应 POST /snapshots/{snapshotID}/fork：从指定快照 fork。
func (h *SandboxHandler) SnapshotFork(c echo.Context) error {
	p, err := h.requireWrite(c)
	if err != nil {
		return writeE2BErr(c, err)
	}
	if !h.svc.CanSnapshot() {
		return c.JSON(http.StatusNotImplemented, e2bErrorBody{Code: http.StatusNotImplemented,
			Message: "fork is not available on this backend", ErrorCode: "unsupported_feature"})
	}
	var req e2bForkReq
	if c.Request().ContentLength > 0 {
		if err := decodeE2BBody(c, &req); err != nil {
			return writeE2BErr(c, err)
		}
	}
	snapID := c.Param("snapshotID")
	// 快照的沙盒名用来继承镜像与规格：元数据里没写就用它。
	var src sandbox.Sandbox
	if all, lerr := h.svc.Snapshots(c.Request().Context()); lerr == nil {
		for _, s := range all {
			if s.ID == snapID {
				src.Name = s.Name
				break
			}
		}
	}
	row, err := h.forkInto(c, p, src, snapID, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, h.detail(c, row))
}

// e2bForkReq 是 fork 的请求体。字段都可空：不写就继承源沙盒。
type e2bForkReq struct {
	// Name 是新沙盒的名字（可选）。
	Name string `json:"name"`
	// Timeout 是存活秒数。
	Timeout *int64 `json:"timeout"`
	// Metadata 是用户自定义键值。
	Metadata map[string]string `json:"metadata"`
	// EnvVars 只做校验：fork 不会把环境写进磁盘。
	EnvVars map[string]string `json:"envVars"`
}

// forkInto 组装 CreateInput 并调用 Fork。
func (h *SandboxHandler) forkInto(c echo.Context, p string, src sandbox.Sandbox, snapshotID string, req e2bForkReq) (sandbox.Sandbox, error) {
	in := sandbox.CreateInput{Metadata: req.Metadata, Start: false}
	// 名字：请求体优先；否则在源名字上加后缀，避免与源冲突。
	if req.Name != "" {
		in.Name = req.Name
	} else if src.Name != "" {
		in.Name = forkName(src.Name)
	}
	// 镜像继承源沙盒：fork 出来的沙盒要与源同根。
	in.Image = src.Image
	if req.Timeout != nil {
		in.TimeoutS = *req.Timeout
	}
	if len(req.EnvVars) > 0 {
		if err := sandbox.ValidateEnvForE2B(req.EnvVars); err != nil {
			return sandbox.Sandbox{}, writeE2BErr(c, err)
		}
	}
	principal := principalOf(c)
	in.Source = sourceFor(principal)
	id, err := h.svc.Fork(c.Request().Context(), p, snapshotID, p, in)
	if err != nil {
		return sandbox.Sandbox{}, writeE2BErr(c, err)
	}
	h.record(c, p, id.ID, "sandbox.fork", snapshotID)
	return id, nil
}

// forkName 给 fork 出来的沙盒起一个不与源冲突的名字。
func forkName(base string) string {
	// 名字有长度上限；截断再拼后缀，避免超长被拒。
	const max = 48
	if len(base) > max {
		base = strings.TrimRight(base[:max], "-_.")
	}
	return base + "-fork"
}
