// 文件说明：unpack.go — gosandbox/image 中 Unpacks、NewUnpacker 等符号的实现
//
// 用法
//
//	type Unpacks struct{…}  // Unpacks 负责把层压平成 lower
//	NewUnpacker(s *Store) *Unpacks  // NewUnpacker 建一个解包器
//	ChainID(layers []Layer) string  // ChainID 是压平根的标识：由各层摘要拼接算出，同一组层永远得到同一个 id
//
// 调用架构
//
//	[上游] gosandbox/vm/box, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/unpack.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] arch.go, import.go, manifest.go, manifest_test.go, pull.go 等 16 个
//	[关键引用] Unpacks ← pull.go；NewUnpacker ← pull.go, pull_test.go；ChainID ← unpack_test.go
//
// 主要符号
//
//	const maxLayerBytes / maxEntries / maxFileBytes / maxPathDepth / maxSymlinkLen
//	var ErrUnsafeArchive
//	type Unpacks struct{…}
//	NewUnpacker(s *Store) *Unpacks
//	(u *Unpacks) LowerPath(chainID string) string
//	ChainID(layers []Layer) string
//	(u *Unpacks) Unpack(ctx context.Context, img Image) (string, error)
//	(u *Unpacks) Remove(chainID string) error
//	... 另有 7 个顶层符号

package image

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// 解包上限。超过就失败，而不是把盘写满。
const (
	// maxLayerBytes 是单层解压后的字节上限（压缩比的硬上限）。
	maxLayerBytes = 8 << 30
	// maxEntries 是单层条目数上限。
	maxEntries = 500_000
	// maxFileBytes 是单个文件的上限。
	maxFileBytes = 4 << 30
	// maxPathDepth 是路径深度上限，防止构造的超深路径。
	maxPathDepth = 128
	// maxSymlinkLen 是符号链接目标长度上限。
	maxSymlinkLen = 4096
)

// ErrUnsafeArchive 表示层里有越界条目，整个层被拒绝。
var ErrUnsafeArchive = errors.New("image: unsafe archive entry")

// Unpacks 负责把层压平成 lower。
type Unpacks struct{ store *Store }

// NewUnpacker 建一个解包器。
func NewUnpacker(s *Store) *Unpacks { return &Unpacks{store: s} }

// LowerPath 返回某个压平根的目录。
func (u *Unpacks) LowerPath(chainID string) string {
	return filepath.Join(u.store.root, "lower", chainID)
}

// ChainID 是压平根的标识：由各层摘要拼接算出，同一组层永远得到同一个 id。
//
// 用摘要而不是计数器：重复拉取同一镜像不会产生第二个 lower，也让 lower
// 能反过来核对它是由哪些层解出来的。
func ChainID(layers []Layer) string {
	if len(layers) == 0 {
		return "empty"
	}
	var b strings.Builder
	for _, l := range layers {
		b.WriteString(l.Digest)
		b.WriteByte('\n')
	}
	sum := sha256Sum(b.String())
	return sum[:32]
}

// Unpack 把镜像的层依次解到一个新的 lower 目录，返回 chainID。
//
// 已存在且带 .done 标记时直接复用；没有标记的目录视为半成品，先删掉重来。
func (u *Unpacks) Unpack(ctx context.Context, img Image) (string, error) {
	id := ChainID(img.Layers)
	dir := u.LowerPath(id)
	done := dir + ".done"
	if _, err := os.Stat(done); err == nil {
		return id, nil
	}
	// 半成品：上一次解到一半崩了。
	if _, err := os.Stat(dir); err == nil {
		if err := os.RemoveAll(dir); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// os.Root 把下面所有写入限制在 dir 内（底层 openat2 RESOLVE_IN_ROOT）。
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()

	for i, layer := range img.Layers {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := u.applyLayer(ctx, root, layer); err != nil {
			// 解失败就删掉整个目录，不留半成品。
			_ = os.RemoveAll(dir)
			return "", fmt.Errorf("image: layer %d (%s): %w", i, layer.Digest, err)
		}
	}
	// 最后写标记：目录出现不代表完整，标记出现才代表完整。
	if err := os.WriteFile(done, []byte(id+"\n"), 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return id, nil
}

// Remove 删除一个压平根（连同 .done 标记）。仍有沙盒引用时调用方不该调它。
func (u *Unpacks) Remove(chainID string) error {
	if chainID == "" || strings.ContainsAny(chainID, `/\`) {
		return fmt.Errorf("image: bad chain id %q", chainID)
	}
	dir := u.LowerPath(chainID)
	_ = os.Remove(dir + ".done")
	return os.RemoveAll(dir)
}

// applyLayer 把一层解到 root。
func (u *Unpacks) applyLayer(ctx context.Context, root *os.Root, layer Layer) error {
	path, err := u.store.blobPath(layer.Digest)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var r io.Reader = f
	// 按 media type 决定怎么解压；导入的镜像常给未压缩的 tar。
	switch layer.MediaType {
	case LayerType, DockerLayerType:
		gz, gerr := gzip.NewReader(f)
		if gerr != nil {
			return fmt.Errorf("gzip: %w", gerr)
		}
		defer gz.Close()
		r = gz
	case LayerTypeZstd:
		zr, zerr := newZstdReader(f)
		if zerr != nil {
			return zerr
		}
		defer zr.Close()
		r = zr
	case LayerTypePlain, "":
		// 原样读。
	default:
		// 不认识的类型：按 gzip 试一次，失败就报错而不是静默跳过。
		gz, gerr := gzip.NewReader(f)
		if gerr != nil {
			return fmt.Errorf("unsupported layer type %q", layer.MediaType)
		}
		defer gz.Close()
		r = gz
	}

	tr := tar.NewReader(r)
	var (
		total   int64
		entries int
	)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		entries++
		if entries > maxEntries {
			return fmt.Errorf("%w: too many entries", ErrUnsafeArchive)
		}
		total += hdr.Size
		if total > maxLayerBytes {
			return fmt.Errorf("%w: layer expands past %d bytes", ErrUnsafeArchive, int64(maxLayerBytes))
		}
		if err := applyEntry(root, tr, hdr); err != nil {
			return err
		}
	}
	return nil
}

// applyEntry 把一个 tar 条目写进 root。
func applyEntry(root *os.Root, tr *tar.Reader, hdr *tar.Header) error {
	name, whiteout, opaque, err := cleanEntryName(hdr.Name)
	if err != nil {
		return err
	}
	// whiteout：删除上一层的同名条目。
	if whiteout {
		return applyWhiteout(root, name, opaque)
	}
	if name == "" {
		return nil // 根目录本身
	}

	mode := hdr.FileInfo().Mode().Perm()
	// 清掉 setuid / setgid / sticky：镜像里的这些位在沙盒里没有意义，
	// 留着只会给提权留想象空间。
	mode &^= os.ModeSetuid | os.ModeSetgid

	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := mkdirAll(root, name, mode|0o700); err != nil {
			return err
		}
	case tar.TypeReg:
		if hdr.Size > maxFileBytes {
			return fmt.Errorf("%w: %s is %d bytes", ErrUnsafeArchive, name, hdr.Size)
		}
		if err := mkdirAll(root, path.Dir(name), 0o755); err != nil {
			return err
		}
		// 先删掉同名条目：直接覆盖会跟随已存在的符号链接。
		_ = root.Remove(name)
		f, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			return err
		}
		if _, err := io.CopyN(f, tr, hdr.Size); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	case tar.TypeSymlink:
		if len(hdr.Linkname) > maxSymlinkLen {
			return fmt.Errorf("%w: symlink target too long", ErrUnsafeArchive)
		}
		// 符号链接的目标可以是任意字符串（它只是个字符串），但链接本身
		// 必须在 root 内。真正的逃逸防护靠后续写入走 os.Root。
		if err := mkdirAll(root, path.Dir(name), 0o755); err != nil {
			return err
		}
		_ = root.Remove(name)
		if err := root.Symlink(hdr.Linkname, name); err != nil {
			return err
		}
	case tar.TypeLink:
		// 硬链接的目标必须已在同一个 root 内。
		target, werr := cleanLinkTarget(hdr.Linkname)
		if werr != nil {
			return werr
		}
		if err := mkdirAll(root, path.Dir(name), 0o755); err != nil {
			return err
		}
		_ = root.Remove(name)
		if err := root.Link(target, name); err != nil {
			return err
		}
	case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
		// rootless 建不了真设备；跳过而不是失败，否则大量镜像解不开。
		return nil
	case tar.TypeXGlobalHeader:
		return nil
	default:
		// 不认识的类型（如 GNU 稀疏文件）：跳过内容。
		return nil
	}
	return nil
}

// applyWhiteout 执行 .wh. 的语义。
//
//	.wh.<name>        删除同目录下的 <name>
//	.wh..wh..opq      清空该目录（opaque）
func applyWhiteout(root *os.Root, name string, opaque bool) error {
	if opaque {
		// opaque：目录内容全部作废。保留目录本身。
		entries, err := rootFS(root, name)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		for _, e := range entries {
			if err := root.RemoveAll(path.Join(name, e)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	}
	if name == "" {
		return nil
	}
	err := root.RemoveAll(name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// rootFS 列出 root 内一个目录的条目名。
func rootFS(root *os.Root, dir string) ([]string, error) {
	d, err := root.Open(dir)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return names, nil
}

// cleanEntryName 归一化条目路径，并识别 whiteout。
//
// 返回的 name 是相对 root 的干净路径（可能为空 = 根），不会含 ".." 或
// 前导 "/"。任何试图越界的写法都被拒绝，而不是「尽量修好」。
func cleanEntryName(raw string) (name string, whiteout bool, opaque bool, err error) {
	if raw == "" {
		return "", false, false, nil
	}
	// tar 里可能是 Windows 风格的反斜杠；按 POSIX 处理，把它当普通字符
	// 反而更安全（会成为一个奇怪的文件名，不会变成路径分隔符）。
	clean := path.Clean(strings.TrimPrefix(raw, "/"))
	if clean == "." {
		return "", false, false, nil
	}
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return "", false, false, fmt.Errorf("%w: %q escapes the root", ErrUnsafeArchive, raw)
	}
	if strings.Count(clean, "/") > maxPathDepth {
		return "", false, false, fmt.Errorf("%w: %q is too deep", ErrUnsafeArchive, raw)
	}

	dir, base := path.Split(clean)
	switch {
	case base == ".wh..wh..opq":
		// 清空 dir（可能是根）。它本身也是一个 whiteout 条目：
		// 不能落到下面按普通文件处理，否则会去 OpenFile 一个已存在的目录。
		return strings.TrimSuffix(dir, "/"), true, true, nil
	case strings.HasPrefix(base, ".wh."):
		// 删除 dir 下的 base[4:]。
		target := path.Join(strings.TrimSuffix(dir, "/"), strings.TrimPrefix(base, ".wh."))
		if target == "." || target == "" {
			return "", false, false, nil
		}
		return target, true, false, nil
	}
	return clean, false, false, nil
}

// cleanLinkTarget 归一化硬链接目标。硬链接不能指向 root 之外。
func cleanLinkTarget(raw string) (string, error) {
	clean := path.Clean(strings.TrimPrefix(raw, "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: link target %q escapes the root", ErrUnsafeArchive, raw)
	}
	return clean, nil
}

// mkdirAll 在 root 内递归建目录。
func mkdirAll(root *os.Root, dir string, mode os.FileMode) error {
	if dir == "" || dir == "." || dir == "/" {
		return nil
	}
	// 逐段建：os.Root.MkdirAll 在 Go 1.24+ 才有，这里显式走一遍，
	// 顺便保证每一段都在 root 内解析。
	parts := strings.Split(strings.Trim(dir, "/"), "/")
	cur := ""
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		cur = path.Join(cur, p)
		if err := root.Mkdir(cur, mode); err != nil && !errors.Is(err, os.ErrExist) {
			// 已存在的可能是文件：报错，让上层看到真实原因。
			return err
		}
	}
	return nil
}
