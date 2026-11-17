// 文件说明：reference.go — 镜像引用解析与归一化
//
// 用法
//
//	Parse("alpine")                      → docker.io/library/alpine:latest
//	Parse("ghcr.io/o/r:1.2")             → ghcr.io/o/r:1.2
//	Parse("alpine@sha256:…")             → 按摘要引用
//	(r Reference) String()               → 归一化后的完整引用
//	(r Reference) Repo()                 → 不含 tag / digest 的仓库
//
// 调用架构
//
//	[上游] gosandbox/image（registry、store）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/reference.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] registry.go, store.go
//
// 主要符号
//
//	type Reference struct {…}
//	Parse(s string) (Reference, error)
//	(r Reference) String() string
//	(r Reference) Repo() string

package image

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Docker Hub 的规范化名字与 API 主机。
const (
	// DefaultRegistry 是没写主机时的注册表。
	DefaultRegistry = "docker.io"
	// DefaultNamespace 是 Docker Hub 上官方镜像的命名空间。
	DefaultNamespace = "library"
	// hubAPIHost 是 Docker Hub 的 registry API 主机（与 docker.io 不同）。
	hubAPIHost = "registry-1.docker.io"
)

// ErrInvalidReference 表示引用无法解析。
var ErrInvalidReference = errors.New("image: invalid reference")

// Reference 是解析后的镜像引用。
type Reference struct {
	// Registry 是注册表主机名（不带协议）。
	Registry string
	// Repository 是仓库路径，例如 library/alpine。
	Repository string
	// Tag 是标签；与 Digest 二选一，Tag 为空表示只按摘要引用。
	Tag string
	// Digest 是 sha256:… 形式的摘要。
	Digest string
}

// namePart 是仓库路径的一段：小写字母、数字与分隔符。
var namePart = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)

// digestPart 是摘要的完整形式。
var digestPart = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// Parse 解析并归一化镜像引用。
//
// 规则与 docker CLI 一致：没写主机时补 docker.io；Docker Hub 上单段名字补
// library/ 命名空间；没写 tag 也没写 digest 时补 :latest。
func Parse(s string) (Reference, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return Reference{}, fmt.Errorf("%w: empty", ErrInvalidReference)
	}
	// 大写会让同一镜像出现两个引用，直接拒绝而不是静默转小写。
	if raw != strings.ToLower(raw) {
		return Reference{}, fmt.Errorf("%w: %q must be lowercase", ErrInvalidReference, s)
	}

	var ref Reference
	body := raw

	// 先切摘要。
	if i := strings.Index(body, "@"); i >= 0 {
		ref.Digest = body[i+1:]
		body = body[:i]
		if !digestPart.MatchString(ref.Digest) {
			return Reference{}, fmt.Errorf("%w: bad digest %q", ErrInvalidReference, ref.Digest)
		}
	}

	// 再切主机与仓库：只有在第一个 / 之前出现 . 或 : 或等于 localhost 时，
	// 第一段才是主机名（与 docker 的判定一致）。
	first, rest, hasSlash := strings.Cut(body, "/")
	if hasSlash && (strings.ContainsAny(first, ".:") || first == "localhost") {
		ref.Registry = first
		body = rest
	} else {
		ref.Registry = DefaultRegistry
	}

	// 切 tag：最后一段里的冒号。主机已经切走，所以这里的冒号一定是 tag。
	if i := strings.LastIndex(body, ":"); i > strings.LastIndex(body, "/") {
		ref.Tag = body[i+1:]
		body = body[:i]
		if ref.Tag == "" || len(ref.Tag) > 128 {
			return Reference{}, fmt.Errorf("%w: bad tag", ErrInvalidReference)
		}
	}
	if body == "" {
		return Reference{}, fmt.Errorf("%w: empty repository", ErrInvalidReference)
	}

	parts := strings.Split(body, "/")
	for _, p := range parts {
		if !namePart.MatchString(p) {
			return Reference{}, fmt.Errorf("%w: bad path segment %q", ErrInvalidReference, p)
		}
	}
	if ref.Registry == DefaultRegistry && len(parts) == 1 {
		parts = append([]string{DefaultNamespace}, parts...)
	}
	ref.Repository = strings.Join(parts, "/")

	// 既没 tag 也没 digest：补 latest，与 docker 一致。
	if ref.Tag == "" && ref.Digest == "" {
		ref.Tag = "latest"
	}
	return ref, nil
}

// String 返回归一化后的完整引用。
func (r Reference) String() string {
	s := r.Registry + "/" + r.Repository
	if r.Tag != "" {
		s += ":" + r.Tag
	}
	if r.Digest != "" {
		s += "@" + r.Digest
	}
	return s
}

// Repo 返回不含 tag / digest 的仓库引用，用于 API 路径。
func (r Reference) Repo() string { return r.Registry + "/" + r.Repository }

// APIRegistry 返回 registry API 的主机名：Docker Hub 的 API 在 registry-1.docker.io。
func (r Reference) APIRegistry() string {
	if r.Registry == DefaultRegistry {
		return hubAPIHost
	}
	return r.Registry
}

// Reference 是「请求哪个 manifest」：优先摘要，其次 tag。
func (r Reference) manifestRef() string {
	if r.Digest != "" {
		return r.Digest
	}
	return r.Tag
}
