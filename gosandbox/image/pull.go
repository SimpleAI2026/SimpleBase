// 文件说明：pull.go — gosandbox/image 中 Progress、PullOptions 等符号的实现
//
// 用法
//
//	type Progress struct{…}  // Progress 是一次拉取的进度回调参数
//	type PullOptions struct{…}  // PullOptions 是拉取选项
//	type Puller struct{…}  // Puller 负责拉取与解包
//	NewPuller(s *Store) *Puller  // NewPuller 建一个 Puller
//
// 调用架构
//
//	[上游] gosandbox/vm/box, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/pull.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] arch.go, import.go, manifest.go, manifest_test.go, pull_test.go 等 16 个
//	[关键引用] Progress ← pull_test.go；PullOptions ← pull_test.go；Puller ← import.go, pull_test.go
//
// 主要符号
//
//	type Progress struct{…}
//	type PullOptions struct{…}
//	type Puller struct{…}
//	type pullCall struct{…}
//	NewPuller(s *Store) *Puller
//	(p *Puller) Client() *Client
//	(p *Puller) Store() *Store
//	(p *Puller) Unpack(ctx context.Context, img Image) (string, error)
//	... 另有 6 个顶层符号

package image

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
)

// Progress 是一次拉取的进度回调参数。
type Progress struct {
	// Stage 是当前阶段：resolve / config / layer / unpack / done。
	Stage string
	// Digest 是当前处理的 blob（Stage=layer/config 时有效）。
	Digest string
	// Current / Total 是当前 blob 已下载字节与总字节。
	Current int64
	Total   int64
	// Index / Count 是第几层 / 共几层。
	Index int
	Count int
}

// PullOptions 是拉取选项。
type PullOptions struct {
	// Arch 是目标架构；空取 runtime.GOARCH。
	Arch string
	// OS 是目标系统；空取 "linux"。
	OS string
	// Progress 是进度回调；nil 表示不回报。
	Progress func(Progress)
	// Unpack 为真时拉取后立刻解包。
	Unpack bool
	// Credentials 是 registry 凭据。
	Credentials map[string]Credentials
}

// Puller 负责拉取与解包。
type Puller struct {
	store  *Store
	client *Client
	unpack *Unpacks

	// inflight 是正在进行的拉取：同一引用同一平台只跑一次。
	mu       sync.Mutex
	inflight map[string]*pullCall
}

// pullCall 是一次进行中的拉取，后来者等它的结果。
type pullCall struct {
	done chan struct{}
	img  Image
	err  error
}

// NewPuller 建一个 Puller。
func NewPuller(s *Store) *Puller {
	return &Puller{store: s, client: NewClient(), unpack: NewUnpacker(s), inflight: map[string]*pullCall{}}
}

// Client 暴露 registry 客户端，供设置凭据。
func (p *Puller) Client() *Client { return p.client }

// Store 暴露底层 store，供上层查询索引与 GC。
func (p *Puller) Store() *Store { return p.store }

// Unpack 把一个已拉取的镜像压平成 lower，返回 chainID。
func (p *Puller) Unpack(ctx context.Context, img Image) (string, error) {
	chainID, err := p.unpack.Unpack(ctx, img)
	if err != nil {
		return "", err
	}
	if img.ChainID != chainID {
		img.ChainID = chainID
		// 索引更新失败不影响本次返回：lower 已经在了，下次解包会复用。
		_ = p.store.PutImage(img)
	}
	return chainID, nil
}

// LowerPath 返回某个 chainID 的 lower 目录。
func (p *Puller) LowerPath(chainID string) string { return p.unpack.LowerPath(chainID) }

// Pull 拉取一个镜像。
//
// 同一引用同一平台的并发拉取会合并：第二个调用者等第一个完成后拿到同样的
// 结果，而不是并行下载两遍。
func (p *Puller) Pull(ctx context.Context, ref string, opt PullOptions) (Image, error) {
	parsed, err := Parse(ref)
	if err != nil {
		return Image{}, err
	}
	if opt.Arch == "" {
		opt.Arch = runtime.GOARCH
	}
	if opt.OS == "" {
		opt.OS = "linux"
	}
	key := parsed.String() + "|" + opt.OS + "/" + opt.Arch

	p.mu.Lock()
	if call, ok := p.inflight[key]; ok {
		p.mu.Unlock()
		select {
		case <-call.done:
			return call.img, call.err
		case <-ctx.Done():
			// 跟随者自己取消：不等了，但也不影响正在跑的那次。
			return Image{}, ctx.Err()
		}
	}
	call := &pullCall{done: make(chan struct{})}
	p.inflight[key] = call
	p.mu.Unlock()

	call.img, call.err = p.pull(ctx, parsed, opt)
	close(call.done)
	p.mu.Lock()
	delete(p.inflight, key)
	p.mu.Unlock()
	return call.img, call.err
}

// pull 是实际的拉取流程。
func (p *Puller) pull(ctx context.Context, ref Reference, opt PullOptions) (Image, error) {
	for host, cred := range opt.Credentials {
		p.client.SetCredentials(host, cred)
	}
	progress := opt.Progress
	report := func(pr Progress) {
		if progress != nil {
			progress(pr)
		}
	}

	// 1. 解析到单平台 manifest。
	report(Progress{Stage: "resolve"})
	resolved, err := p.client.Resolve(ctx, ref, opt.Arch, opt.OS)
	if err != nil {
		return Image{}, err
	}
	m := resolved.Manifest
	if len(m.Layers) == 0 {
		return Image{}, fmt.Errorf("image: %s has no layers", ref)
	}

	// 2. manifest 本身进 store：镜像引用按摘要查找时要用到它。
	if err := p.store.PutBlob(resolved.Digest, m.Raw); err != nil {
		return Image{}, err
	}

	// 3. config。
	report(Progress{Stage: "config", Digest: m.Config.Digest})
	if _, err := p.store.ReadBlob(m.Config.Digest); err != nil {
		// 本地没有就从 registry 取。
		cfgBody, err := p.client.GetSmall(ctx, ref, m.Config)
		if err != nil {
			return Image{}, err
		}
		if err := p.store.PutBlob(m.Config.Digest, cfgBody); err != nil {
			return Image{}, err
		}
	}

	// 4. 层：已存在的跳过，其余流式下载并按摘要校验。
	layers := make([]Layer, 0, len(m.Layers))
	for i, d := range m.Layers {
		if err := ctx.Err(); err != nil {
			return Image{}, err
		}
		layers = append(layers, Layer{Digest: d.Digest, Size: d.Size, MediaType: d.MediaType})
		if p.store.HasBlob(d.Digest) {
			report(Progress{Stage: "layer", Digest: d.Digest, Total: d.Size,
				Current: d.Size, Index: i + 1, Count: len(m.Layers)})
			continue
		}
		report(Progress{Stage: "layer", Digest: d.Digest, Total: d.Size, Index: i + 1, Count: len(m.Layers)})
		w, werr := p.store.BlobWriter(d.Digest)
		if werr != nil {
			return Image{}, werr
		}
		cw := &countingWriter{w: w, total: d.Size, index: i + 1, count: len(m.Layers),
			digest: d.Digest, report: report}
		if derr := p.client.GetBlob(ctx, ref, d, cw); derr != nil {
			w.Abort()
			return Image{}, derr
		}
		if cerr := w.Commit(); cerr != nil {
			return Image{}, cerr
		}
	}

	img := Image{
		Reference:      ref.String(),
		ManifestDigest: resolved.Digest,
		IndexDigest:    resolved.IndexDigest,
		ConfigDigest:   m.Config.Digest,
		Platform:       opt.OS + "/" + opt.Arch,
		Layers:         layers,
	}
	img.Size = img.SizeOf()
	if err := p.store.PutImage(img); err != nil {
		return Image{}, err
	}

	if opt.Unpack {
		report(Progress{Stage: "unpack"})
		chainID, uerr := p.unpack.Unpack(ctx, img)
		if uerr != nil {
			return Image{}, uerr
		}
		img.ChainID = chainID
		_ = p.store.PutImage(img)
	}
	report(Progress{Stage: "done"})
	return img, nil
}

// countingWriter 在下载过程中回报进度。
type countingWriter struct {
	w       io.Writer
	written int64
	total   int64
	index   int
	count   int
	digest  string
	report  func(Progress)
	// last 是上次回报时的字节数，避免每写一小块就回调一次。
	last int64
}

// Write 实现 io.Writer。
func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.written += int64(n)
	// 每 4 MiB 回报一次，避免回调成为瓶颈。
	if c.written-c.last >= 4<<20 {
		c.last = c.written
		c.report(Progress{Stage: "layer", Digest: c.digest, Current: c.written,
			Total: c.total, Index: c.index, Count: c.count})
	}
	return n, err
}

// ErrImportUnsupported 表示导入功能尚未实现。
var ErrImportUnsupported = errors.New("image: import from a docker-save tarball is not implemented yet")
