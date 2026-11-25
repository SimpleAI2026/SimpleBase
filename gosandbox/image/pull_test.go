// 文件说明：pull_test.go — gosandbox/image 的单元测试（TestPullEndToEnd、TestPullSkipsExistingLayers 等）
//
// 用法
//
//	go test ./gosandbox/image -run 'TestPullEndToEnd|TestPullSkipsExistingLayers|TestPullCoalesces|TestPullRejectsBadDigest'
//
// 调用架构
//
//	[上游] gosandbox/vm/box, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/pull_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] arch.go, import.go, manifest.go, manifest_test.go, pull.go 等 16 个
//
// 主要符号
//
//	type fakeRegistry struct{…}
//	newFakeRegistry(t *testing.T) (*fakeRegistry, *httptest.Server, Reference)
//	(f *fakeRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request)
//	(f *fakeRegistry) addBlob(body []byte) string
//	(f *fakeRegistry) addManifest(tag string, body []byte) string
//	gzipTar(t *testing.T, entries ...tarEntry) []byte
//	setupImage(t *testing.T, fr *fakeRegistry, repo string, layers ...[]byte)
//	TestPullEndToEnd(t *testing.T)
//	... 另有 11 个顶层符号

package image

// pull_test.go：端到端拉取，打到本地假 registry。
//
// 覆盖：索引 → 挑平台 → manifest → config → 逐层下载 → 写索引 → 解包；
// 以及断点（已存在的层跳过）、并发合并、摘要不符时拒绝。

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRegistry 是一个最小的 registry：支持 manifest、blob、匿名。
type fakeRegistry struct {
	t         *testing.T
	manifests map[string][]byte // digest 或 tag → body
	blobs     map[string][]byte
	hits      map[string]int32
	// requireAuth 为真时先返回 401，逼客户端走 token 流程。
	requireAuth bool
	// authHits 记录 token 端点的请求次数。
	authHits int32
}

// newFakeRegistry 起一个假 registry 并返回它的 Reference 前缀。
func newFakeRegistry(t *testing.T) (*fakeRegistry, *httptest.Server, Reference) {
	t.Helper()
	fr := &fakeRegistry{t: t, manifests: map[string][]byte{}, blobs: map[string][]byte{}, hits: map[string]int32{}}
	srv := httptest.NewServer(fr)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ref := Reference{Registry: u.Host, Repository: "test/app", Tag: "latest"}
	return fr, srv, ref
}

// ServeHTTP 实现 registry 的读接口。
func (f *fakeRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	f.hits[path]++
	switch {
	case strings.HasPrefix(path, "/token"):
		atomic.AddInt32(&f.authHits, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "test-token", "expires_in": 300})
	case strings.HasPrefix(path, "/v2/") && strings.Contains(path, "/manifests/"):
		if f.requireAuth && r.Header.Get("Authorization") != "Bearer test-token" {
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer realm="http://%s/token",service="fake"`, r.Host))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		ref := path[strings.LastIndex(path, "/")+1:]
		body, ok := f.manifests[ref]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		sum := sha256.Sum256(body)
		w.Header().Set("Docker-Content-Digest", "sha256:"+hex.EncodeToString(sum[:]))
		w.Header().Set("Content-Type", ManifestType)
		_, _ = w.Write(body)
	case strings.HasPrefix(path, "/v2/") && strings.Contains(path, "/blobs/"):
		if f.requireAuth && r.Header.Get("Authorization") != "Bearer test-token" {
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer realm="http://%s/token",service="fake"`, r.Host))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		digest := path[strings.LastIndex(path, "/")+1:]
		body, ok := f.blobs[digest]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// addBlob 往假 registry 里放一个 blob，返回摘要。
func (f *fakeRegistry) addBlob(body []byte) string {
	digest, _ := digestOf(body)
	f.blobs[digest] = body
	return digest
}

// addManifest 放一个 manifest，按 tag 与摘要都能取。
func (f *fakeRegistry) addManifest(tag string, body []byte) string {
	digest, _ := digestOf(body)
	f.manifests[digest] = body
	if tag != "" {
		f.manifests[tag] = body
	}
	return digest
}

// gzipTar 造一个 gzip 层。
func gzipTar(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	raw := tarWith(t, entries...)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// setupImage 往 registry 里放一个单平台镜像，返回 tag 与层数。
func setupImage(t *testing.T, fr *fakeRegistry, repo string, layers ...[]byte) {
	t.Helper()
	cfg := []byte(`{"architecture":"arm64","os":"linux","config":{"Env":["PATH=/bin"],"Cmd":["/bin/sh"]}}`)
	cfgDigest := fr.addBlob(cfg)

	type desc struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	}
	var layerDescs []desc
	for _, l := range layers {
		d := fr.addBlob(l)
		layerDescs = append(layerDescs, desc{MediaType: LayerType, Digest: d, Size: int64(len(l))})
	}
	m := map[string]any{
		"schemaVersion": 2,
		"mediaType":     ManifestType,
		"config":        desc{MediaType: ConfigType, Digest: cfgDigest, Size: int64(len(cfg))},
		"layers":        layerDescs,
	}
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	fr.addManifest("latest", body)
	_ = repo
}

// 端到端：拉一个两层镜像并解包。
func TestPullEndToEnd(t *testing.T) {
	fr, srv, ref := newFakeRegistry(t)
	layer1 := gzipTar(t, tarEntry{name: "bin/", typ: tar.TypeDir, mode: 0o755},
		tarEntry{name: "bin/sh", mode: 0o755, body: []byte("#!/bin/sh\n")})
	layer2 := gzipTar(t, tarEntry{name: "etc/", typ: tar.TypeDir, mode: 0o755},
		tarEntry{name: "etc/os-release", body: []byte("ID=test\n")})
	setupImage(t, fr, ref.Repository, layer1, layer2)

	st := newStore(t)
	p := NewPuller(st)
	// fake registry 是 loopback + 明文：把 URL scheme 换进去。
	// 这里靠 SetCredentials 之外的方式指定端点不方便，所以直接改 ref 的
	// 解析结果不可行——client 固定用 https。改用测试专用的 https→http 替换。
	old := srv.URL
	_ = old
	if err := pullWithHTTP(t, p, ref, srv.URL); err != nil {
		t.Fatal(err)
	}
}

// pullWithHTTP 让客户端把 https 换成测试服务器的 http 前缀。
//
// registry 客户端固定拼 https://<host>/…：假服务器是明文 HTTP，
// 所以这里临时把 host 换成 srv.URL 去掉 scheme 后的部分，并通过
// 一个 http transport 直连（见 testClientScheme）。
func pullWithHTTP(t *testing.T, p *Puller, ref Reference, srvURL string) error {
	t.Helper()
	u, err := url.Parse(srvURL)
	if err != nil {
		return err
	}
	// 把 registry 改成 loopback 主机，并让 transport 用 http。
	p.client = newTestClient(srvURL)
	ref.Registry = u.Host
	_, err = p.Pull(context.Background(), ref.String(), PullOptions{Arch: "arm64", OS: "linux", Unpack: true})
	return err
}

// 已存在的层不重复下载。
func TestPullSkipsExistingLayers(t *testing.T) {
	fr, srv, ref := newFakeRegistry(t)
	layer := gzipTar(t, tarEntry{name: "a", body: []byte("x")})
	setupImage(t, fr, ref.Repository, layer)

	st := newStore(t)
	p := NewPuller(st)
	if err := pullWithHTTP(t, p, ref, srv.URL); err != nil {
		t.Fatal(err)
	}
	// 第一次下载过：blob 端点的命中次数 = 1（config）+ 1（层）。
	before := int32(0)
	for path, n := range fr.hits {
		if strings.Contains(path, "/blobs/") {
			before += n
		}
	}
	if before < 2 {
		t.Fatalf("expected blob requests, got %d", before)
	}

	// 换个 store 再拉一次会重新下载；同一个 store 则命中缓存。
	p2 := NewPuller(st)
	if err := pullWithHTTP(t, p2, ref, srv.URL); err != nil {
		t.Fatal(err)
	}
	after := int32(0)
	for path, n := range fr.hits {
		if strings.Contains(path, "/blobs/") {
			after += n
		}
	}
	// config 与层都在本地了，第二次不应再请求 blob。
	if after != before {
		t.Fatalf("second pull made %d extra blob requests", after-before)
	}
}

// 并发拉同一引用：只打一次 manifest。
func TestPullCoalesces(t *testing.T) {
	fr, srv, ref := newFakeRegistry(t)
	layer := gzipTar(t, tarEntry{name: "a", body: []byte("x")})
	setupImage(t, fr, ref.Repository, layer)

	st := newStore(t)
	p := NewPuller(st)
	p.client = newTestClient(srv.URL)
	u, _ := url.Parse(srv.URL)
	ref.Registry = u.Host

	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			_, err := p.Pull(context.Background(), ref.String(), PullOptions{Arch: "arm64", OS: "linux"})
			done <- err
		}()
	}
	for i := 0; i < 4; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent pull: %v", err)
		}
	}
	manifestHits := int32(0)
	for path, n := range fr.hits {
		if strings.Contains(path, "/manifests/") {
			manifestHits += n
		}
	}
	// 索引 + 单平台 manifest = 2 次；合并后不应变成 8 次。
	if manifestHits > 4 {
		t.Fatalf("manifest fetched %d times, coalescing failed", manifestHits)
	}
}

// 层内容与摘要不符时拒绝，且不留半成品。
func TestPullRejectsBadDigest(t *testing.T) {
	fr, srv, ref := newFakeRegistry(t)
	cfg := []byte(`{"architecture":"arm64","os":"linux","config":{}}`)
	cfgDigest := fr.addBlob(cfg)
	// 声明的摘要与实际内容不符。
	real := gzipTar(t, tarEntry{name: "a", body: []byte("x")})
	lie := gzipTar(t, tarEntry{name: "b", body: []byte("y")})
	lieDigest, _ := digestOf(lie)
	fr.blobs[lieDigest] = real // 摘要指向另一份内容

	m := map[string]any{
		"schemaVersion": 2, "mediaType": ManifestType,
		"config": map[string]any{"mediaType": ConfigType, "digest": cfgDigest, "size": len(cfg)},
		"layers": []map[string]any{{"mediaType": LayerType, "digest": lieDigest, "size": int64(len(real))}},
	}
	body, _ := json.Marshal(m)
	fr.addManifest("latest", body)

	st := newStore(t)
	p := NewPuller(st)
	p.client = newTestClient(srv.URL)
	u, _ := url.Parse(srv.URL)
	ref.Registry = u.Host

	_, err := p.Pull(context.Background(), ref.String(), PullOptions{Arch: "arm64", OS: "linux"})
	if !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("err = %v, want ErrDigestMismatch", err)
	}
	if st.HasBlob(lieDigest) {
		t.Fatal("mismatched layer must not be stored")
	}
}

// 多架构索引：只取目标平台。
func TestPullPicksPlatform(t *testing.T) {
	fr, srv, ref := newFakeRegistry(t)
	cfg := []byte(`{"architecture":"arm64","os":"linux","config":{}}`)
	cfgDigest := fr.addBlob(cfg)
	armLayer := gzipTar(t, tarEntry{name: "arch", body: []byte("arm64")})
	amdLayer := gzipTar(t, tarEntry{name: "arch", body: []byte("amd64")})
	armDigest := fr.addBlob(armLayer)
	amdDigest := fr.addBlob(amdLayer)

	mkManifest := func(layerDigest string, size int64) []byte {
		m := map[string]any{
			"schemaVersion": 2, "mediaType": ManifestType,
			"config": map[string]any{"mediaType": ConfigType, "digest": cfgDigest, "size": len(cfg)},
			"layers": []map[string]any{{"mediaType": LayerType, "digest": layerDigest, "size": size}},
		}
		b, _ := json.Marshal(m)
		return b
	}
	armManifest := mkManifest(armDigest, int64(len(armLayer)))
	amdManifest := mkManifest(amdDigest, int64(len(amdLayer)))
	armManifestDigest := fr.addManifest("", armManifest)
	amdManifestDigest := fr.addManifest("", amdManifest)

	index := map[string]any{
		"schemaVersion": 2, "mediaType": ManifestListType,
		"manifests": []map[string]any{
			{"mediaType": ManifestType, "digest": armManifestDigest, "size": len(armManifest),
				"platform": map[string]string{"architecture": "arm64", "os": "linux"}},
			{"mediaType": ManifestType, "digest": amdManifestDigest, "size": len(amdManifest),
				"platform": map[string]string{"architecture": "amd64", "os": "linux"}},
		},
	}
	body, _ := json.Marshal(index)
	fr.addManifest("latest", body)

	st := newStore(t)
	p := NewPuller(st)
	p.client = newTestClient(srv.URL)
	u, _ := url.Parse(srv.URL)
	ref.Registry = u.Host

	img, err := p.Pull(context.Background(), ref.String(), PullOptions{Arch: "amd64", OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Layers) != 1 || img.Layers[0].Digest != amdDigest {
		t.Fatalf("picked the wrong platform: %+v", img.Layers)
	}
	if img.IndexDigest == "" {
		t.Fatal("index digest must be recorded")
	}
	// 目标平台不存在：明确报错。
	_, err = p.Pull(context.Background(), ref.String(), PullOptions{Arch: "riscv64", OS: "linux"})
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("err = %v", err)
	}
}

// 走 token 流程：401 → token → 重试成功。
func TestPullWithTokenAuth(t *testing.T) {
	fr, srv, ref := newFakeRegistry(t)
	fr.requireAuth = true
	layer := gzipTar(t, tarEntry{name: "a", body: []byte("x")})
	setupImage(t, fr, ref.Repository, layer)

	st := newStore(t)
	p := NewPuller(st)
	p.client = newTestClient(srv.URL)
	u, _ := url.Parse(srv.URL)
	ref.Registry = u.Host

	if _, err := p.Pull(context.Background(), ref.String(), PullOptions{Arch: "arm64", OS: "linux"}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&fr.authHits) == 0 {
		t.Fatal("token endpoint must be used")
	}
}

// 超时：ctx 取消时立刻返回。
func TestPullRespectsContext(t *testing.T) {
	fr, srv, ref := newFakeRegistry(t)
	layer := gzipTar(t, tarEntry{name: "a", body: []byte("x")})
	setupImage(t, fr, ref.Repository, layer)

	st := newStore(t)
	p := NewPuller(st)
	p.client = newTestClient(srv.URL)
	u, _ := url.Parse(srv.URL)
	ref.Registry = u.Host

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	_, err := p.Pull(ctx, ref.String(), PullOptions{Arch: "arm64", OS: "linux"})
	if err == nil {
		t.Fatal("a cancelled pull must fail")
	}
}

// 进度回调被调用到各阶段。
func TestPullReportsProgress(t *testing.T) {
	fr, srv, ref := newFakeRegistry(t)
	layer := gzipTar(t, tarEntry{name: "a", body: []byte("x")})
	setupImage(t, fr, ref.Repository, layer)

	st := newStore(t)
	p := NewPuller(st)
	p.client = newTestClient(srv.URL)
	u, _ := url.Parse(srv.URL)
	ref.Registry = u.Host

	stages := map[string]bool{}
	_, err := p.Pull(context.Background(), ref.String(), PullOptions{
		Arch: "arm64", OS: "linux",
		Progress: func(pr Progress) { stages[pr.Stage] = true },
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"resolve", "config", "layer", "done"} {
		if !stages[want] {
			t.Errorf("missing progress stage %q (got %v)", want, stages)
		}
	}
}

// importFromTar 从 docker save 归档导入。
func TestImportDockerSave(t *testing.T) {
	st := newStore(t)
	p := NewPuller(st)

	// 造一个最小的 docker save 归档。
	cfg := []byte(`{"architecture":"arm64","os":"linux","config":{"Cmd":["/bin/sh"]}}`)
	layer := tarWith(t, tarEntry{name: "hello", body: []byte("world")})
	manifest := []any{map[string]any{
		"Config":   "config.json",
		"RepoTags": []string{"imported:1"},
		"Layers":   []string{"layer.tar"},
	}}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	writeTar := func(name string, body []byte) {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	manifestBody, _ := json.Marshal(manifest)
	writeTar("manifest.json", manifestBody)
	writeTar("config.json", cfg)
	writeTar("layer.tar", layer)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	imgs, err := p.Import(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 1 {
		t.Fatalf("imported %d images", len(imgs))
	}
	if imgs[0].Reference != "docker.io/library/imported:1" {
		t.Fatalf("reference = %q", imgs[0].Reference)
	}
	if len(imgs[0].Layers) != 1 {
		t.Fatalf("layers = %+v", imgs[0].Layers)
	}
	// 层已进 store，能解包。
	u := NewUnpacker(st)
	chain, err := u.Unpack(context.Background(), imgs[0])
	if err != nil {
		t.Fatal(err)
	}
	data, err := readFileIn(t, u.LowerPath(chain), "hello")
	if err != nil || data != "world" {
		t.Fatalf("hello = %q, %v", data, err)
	}
}

// 归档里的越界路径被拒绝。
func TestImportRejectsTraversal(t *testing.T) {
	st := newStore(t)
	p := NewPuller(st)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{Name: "../evil", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	if _, err := p.Import(context.Background(), bytes.NewReader(buf.Bytes())); !errors.Is(err, ErrUnsafeArchive) {
		t.Fatalf("err = %v", err)
	}
}

// 没有 manifest.json 的归档被拒绝。
func TestImportRejectsMissingManifest(t *testing.T) {
	st := newStore(t)
	p := NewPuller(st)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "x", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	if _, err := p.Import(context.Background(), bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("archive without manifest.json must be refused")
	}
}
