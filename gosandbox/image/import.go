package image

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// importManifest 是 docker save 的 manifest.json 的一条。
type importManifest struct {
	Config   string   `json:"Config"`
	RepoTags []string `json:"RepoTags"`
	Layers   []string `json:"Layers"`
}

// importLimits 限制一个归档的规模，避免构造的 tar 撑爆盘。
const (
	// importMaxManifest 是 manifest.json 的上限。
	importMaxManifest = 8 << 20
	// importMaxBytes 是整个归档解出的字节上限。
	importMaxBytes = 16 << 30
	// importMaxEntries 是条目数上限。
	importMaxEntries = 200_000
)

// Import 从一个 docker-save 格式的 tar 导入全部镜像。
//
// 大归档是流式处理的：每个条目来了就落地，不把整个 tar 放进内存。
// 层文件先写进 store 的 tmp，算完摘要再 rename 成正式 blob。
func (p *Puller) Import(ctx context.Context, r io.Reader) ([]Image, error) {
	tr := tar.NewReader(r)

	// 第一遍：把条目按名字落到临时目录，同时记下 manifest.json。
	staging, err := os.MkdirTemp(p.store.root, "import-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)

	var (
		rawManifest []byte
		total       int64
		entries     int
	)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hdr, terr := tr.Next()
		if errors.Is(terr, io.EOF) {
			break
		}
		if terr != nil {
			return nil, fmt.Errorf("image: read archive: %w", terr)
		}
		entries++
		if entries > importMaxEntries {
			return nil, fmt.Errorf("%w: archive has too many entries", ErrUnsafeArchive)
		}
		name, werr := safeArchiveName(hdr.Name)
		if werr != nil {
			return nil, werr
		}
		if name == "" {
			continue
		}
		total += hdr.Size
		if total > importMaxBytes {
			return nil, fmt.Errorf("%w: archive is larger than %d bytes", ErrUnsafeArchive, int64(importMaxBytes))
		}

		if name == "manifest.json" {
			rawManifest, werr = readLimited(tr, importMaxManifest)
			if werr != nil {
				return nil, werr
			}
			continue
		}
		// 其余条目落到 staging；目录不建（层与 config 都是文件）。
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		dst := path.Join(staging, name)
		if err := os.MkdirAll(path.Dir(dst), 0o700); err != nil {
			return nil, err
		}
		f, ferr := os.Create(dst)
		if ferr != nil {
			return nil, ferr
		}
		if _, cerr := io.CopyN(f, tr, hdr.Size); cerr != nil {
			f.Close()
			return nil, cerr
		}
		if cerr := f.Close(); cerr != nil {
			return nil, cerr
		}
	}
	if len(rawManifest) == 0 {
		return nil, fmt.Errorf("image: archive has no manifest.json")
	}

	var entriesManifest []importManifest
	if err := json.Unmarshal(rawManifest, &entriesManifest); err != nil {
		return nil, fmt.Errorf("image: parse manifest.json: %w", err)
	}
	if len(entriesManifest) == 0 {
		return nil, fmt.Errorf("image: manifest.json lists no images")
	}

	out := make([]Image, 0, len(entriesManifest))
	for _, im := range entriesManifest {
		img, ierr := p.importOne(ctx, staging, im)
		if ierr != nil {
			return nil, ierr
		}
		out = append(out, img)
	}
	return out, nil
}

// importOne 导入 manifest.json 里的一条。
func (p *Puller) importOne(ctx context.Context, staging string, im importManifest) (Image, error) {
	// config：读出来算摘要，写进 store。
	cfgPath, err := safeArchiveName(im.Config)
	if err != nil {
		return Image{}, err
	}
	cfgBody, err := os.ReadFile(path.Join(staging, cfgPath))
	if err != nil {
		return Image{}, fmt.Errorf("image: config %q: %w", im.Config, err)
	}
	cfgDigest, err := digestOf(cfgBody)
	if err != nil {
		return Image{}, err
	}
	if err := p.store.PutBlob(cfgDigest, cfgBody); err != nil {
		return Image{}, err
	}

	// 层：按顺序算摘要、落 blob。层在归档里是文件，摘要只能算出来。
	layers := make([]Layer, 0, len(im.Layers))
	for i, layerFile := range im.Layers {
		if err := ctx.Err(); err != nil {
			return Image{}, err
		}
		name, nerr := safeArchiveName(layerFile)
		if nerr != nil {
			return Image{}, nerr
		}
		src := path.Join(staging, name)
		digest, size, mediaType, derr := p.ingestLayerFile(src)
		if derr != nil {
			return Image{}, fmt.Errorf("image: layer %d (%s): %w", i, layerFile, derr)
		}
		layers = append(layers, Layer{Digest: digest, Size: size, MediaType: mediaType})
	}

	// 没有 RepoTags 的镜像（`docker save <id>`）用配置摘要当引用。
	tags := im.RepoTags
	if len(tags) == 0 {
		tags = []string{"<none>@" + cfgDigest}
	}
	var first Image
	for _, tag := range tags {
		ref, perr := Parse(tag)
		if perr != nil {
			// "<none>@sha256:…" 这种合成引用不走 Parse。
			ref = Reference{Registry: DefaultRegistry, Repository: "<none>", Digest: cfgDigest}
		}
		img := Image{
			Reference:    ref.String(),
			ConfigDigest: cfgDigest,
			// 导入的镜像没有 registry manifest：用自己的摘要占位，
			// 这样按引用查找、GC 与删除都能正常工作。
			ManifestDigest: cfgDigest,
			Platform:       "linux/" + goArch(),
			Layers:         layers,
		}
		img.Size = img.SizeOf()
		if err := p.store.PutImage(img); err != nil {
			return Image{}, err
		}
		if first.Reference == "" {
			first = img
		}
	}
	return first, nil
}

// ingestLayerFile 把一个层文件写进 store，返回摘要、大小与媒体类型。
//
// 媒体类型按内容判断：gzip 魔数开头就是 gzip，否则当未压缩 tar。
func (p *Puller) ingestLayerFile(src string) (string, int64, string, error) {
	f, err := os.Open(src)
	if err != nil {
		return "", 0, "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", 0, "", err
	}

	// 先读两个字节判断压缩格式，再决定媒体类型。
	var magic [2]byte
	n, _ := io.ReadFull(f, magic[:])
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", 0, "", err
	}
	mediaType := LayerTypePlain
	if n == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		mediaType = LayerType
	}

	// 层文件已经在暂存目录里，摘要可以直接算；不值得为了「不读两遍」
	// 牺牲清晰度（大层读两遍的代价是本地磁盘 IO）。
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", 0, "", err
	}
	digest := "sha256:" + hex.EncodeToString(h.Sum(nil))
	if p.store.HasBlob(digest) {
		return digest, st.Size(), mediaType, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", 0, "", err
	}
	w, err := p.store.BlobWriter(digest)
	if err != nil {
		return "", 0, "", err
	}
	if _, err := io.Copy(w, f); err != nil {
		w.Abort()
		return "", 0, "", err
	}
	if err := w.Commit(); err != nil {
		return "", 0, "", err
	}
	return digest, st.Size(), mediaType, nil
}

// safeArchiveName 归一化归档里的条目名，拒绝越界。
func safeArchiveName(raw string) (string, error) {
	clean := path.Clean(strings.TrimPrefix(strings.TrimSpace(raw), "/"))
	if clean == "." || clean == "" {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q escapes the archive root", ErrUnsafeArchive, raw)
	}
	return clean, nil
}

// goArch 返回 GOARCH 对应的 OCI 架构名。
func goArch() string {
	switch a := runtimeArch(); a {
	case "amd64", "arm64", "386", "arm", "ppc64le", "s390x", "riscv64":
		return a
	default:
		return a
	}
}
