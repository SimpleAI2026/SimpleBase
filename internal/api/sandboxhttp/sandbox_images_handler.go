package sandboxhttp

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gosandbox/image"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

// ImageInfo 是镜像的只读视图。
type ImageInfo struct {
	Reference string    `json:"reference"`
	Digest    string    `json:"digest"`
	Platform  string    `json:"platform"`
	SizeBytes int64     `json:"size_bytes"`
	Layers    int       `json:"layers"`
	ChainID   string    `json:"chain_id,omitempty"`
	Created   time.Time `json:"created"`
}

// imageStore 返回镜像 store；未配置时 nil。
//
// 用惰性函数而不是字段：没有配 images_dir 的部署不该在启动时创建目录。
func (h *SandboxHandler) imageStore() *image.Store {
	if h.deps.Images == nil {
		return nil
	}
	return h.deps.Images
}

// Images 对应 GET /e2b/ext/images。
func (h *SandboxHandler) Images(c echo.Context) error {
	store := h.imageStore()
	if store == nil {
		return c.JSON(http.StatusNotImplemented, E2BErrorBody{Code: http.StatusNotImplemented,
			Message: "this instance has no image store (sandbox.images_dir is unset)", ErrorCode: "unsupported_feature"})
	}
	imgs := store.Images()
	out := make([]ImageInfo, 0, len(imgs))
	for _, img := range imgs {
		out = append(out, ImageInfo{
			Reference: img.Reference, Digest: img.ManifestDigest, Platform: img.Platform,
			SizeBytes: img.Size, Layers: len(img.Layers), ChainID: img.ChainID, Created: img.Created,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"images": out})
}

// ImagePull 对应 POST /e2b/ext/images/pull。
//
// 同步拉取：镜像大小从几 MB 到几 GB，这里不返回 202 + 轮询，
// 因为 CLI 与运维脚本更希望「命令返回时已经可用」。HTTP 层的超时由
// 服务器配置决定，拉大镜像时需要放宽。
func (h *SandboxHandler) ImagePull(c echo.Context) error {
	store := h.imageStore()
	if store == nil {
		return c.JSON(http.StatusNotImplemented, E2BErrorBody{Code: http.StatusNotImplemented,
			Message: "this instance has no image store (sandbox.images_dir is unset)", ErrorCode: "unsupported_feature"})
	}
	var req struct {
		Reference    string `json:"reference"`
		Architecture string `json:"architecture,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return WriteE2BErr(c, httpkit.NewAPIError(http.StatusBadRequest, "invalid_body", "invalid JSON", ""))
	}
	if req.Reference == "" {
		return WriteE2BErr(c, httpkit.NewAPIError(http.StatusBadRequest, "invalid_reference", "reference is required", ""))
	}
	puller := image.NewPuller(store)
	img, err := puller.Pull(c.Request().Context(), req.Reference, image.PullOptions{
		Arch: req.Architecture, OS: "linux", Unpack: true,
	})
	if err != nil {
		return imageError(c, err)
	}
	return c.JSON(http.StatusCreated, ImageInfo{
		Reference: img.Reference, Digest: img.ManifestDigest, Platform: img.Platform,
		SizeBytes: img.Size, Layers: len(img.Layers), ChainID: img.ChainID, Created: img.Created,
	})
}

// ImageRemove 对应 DELETE /e2b/ext/images?reference=…
//
// 只删索引：blob 与 lower 由 GC 回收（运行中的沙盒可能还在用它们）。
func (h *SandboxHandler) ImageRemove(c echo.Context) error {
	store := h.imageStore()
	if store == nil {
		return c.JSON(http.StatusNotImplemented, E2BErrorBody{Code: http.StatusNotImplemented,
			Message: "this instance has no image store (sandbox.images_dir is unset)", ErrorCode: "unsupported_feature"})
	}
	ref := c.QueryParam("reference")
	if ref == "" {
		return WriteE2BErr(c, httpkit.NewAPIError(http.StatusBadRequest, "invalid_reference", "reference is required", ""))
	}
	if err := store.Remove(ref); err != nil {
		return imageError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// imageError 把镜像层的错误翻成 HTTP。
//
// 404 与 401 要分开：前者是「引用写错了」，后者是「需要先登录」，
// 调用方的处理完全不同。
func imageError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, image.ErrImageNotFound), errors.Is(err, image.ErrNotFound):
		return WriteE2BErr(c, httpkit.NewAPIError(http.StatusNotFound, "image_not_found", err.Error(), ""))
	case errors.Is(err, image.ErrUnauthorized):
		return WriteE2BErr(c, httpkit.NewAPIError(http.StatusUnauthorized, "registry_unauthorized", err.Error(), ""))
	case errors.Is(err, image.ErrInvalidReference):
		return WriteE2BErr(c, httpkit.NewAPIError(http.StatusBadRequest, "invalid_reference", err.Error(), ""))
	case errors.Is(err, image.ErrUnsupportedPlatform):
		return WriteE2BErr(c, httpkit.NewAPIError(http.StatusBadRequest, "unsupported_platform", err.Error(), ""))
	case errors.Is(err, image.ErrDigestMismatch), errors.Is(err, image.ErrUnsafeArchive):
		// 内容与声明不符 / 归档越界：这是安全性错误，不能当 500 忽略。
		return WriteE2BErr(c, httpkit.NewAPIError(http.StatusBadGateway, "image_integrity", err.Error(), ""))
	}
	return WriteE2BErr(c, httpkit.NewAPIError(http.StatusBadGateway, "image_pull_failed", err.Error(), ""))
}
