// 文件说明：store_test.go — gosandbox/image 的单元测试（TestDigestHexRejectsPaths、TestBlobWriterRoundTrip 等）
//
// 用法
//
//	go test ./gosandbox/image -run 'TestDigestHexRejectsPaths|TestBlobWriterRoundTrip|TestBlobWriterDigestMismatch|TestBlobWriterAbort'
//
// 调用架构
//
//	[上游] gosandbox/vm/box, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/image/store_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] arch.go, import.go, manifest.go, manifest_test.go, pull.go 等 16 个
//
// 主要符号
//
//	TestDigestHexRejectsPaths(t *testing.T)
//	TestBlobWriterRoundTrip(t *testing.T)
//	TestBlobWriterDigestMismatch(t *testing.T)
//	TestBlobWriterAbort(t *testing.T)
//	TestStoreFind(t *testing.T)
//	TestStorePersistsIndex(t *testing.T)
//	TestStoreRejectsBrokenIndex(t *testing.T)
//	TestStoreGC(t *testing.T)
//	... 另有 2 个顶层符号

package image

// store_test.go：blob store 与索引的行为。
//
//   1. 摘要必须能安全地拼进路径（拒绝 "../"）；
//   2. BlobWriter 校验摘要，不符时不留残留；
//   3. 索引按引用与摘要两种方式查找；
//   4. GC 只删无引用的 blob；
//   5. 索引损坏时明确报错，而不是静默清空。

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDigestHexRejectsPaths(t *testing.T) {
	for _, bad := range []string{
		"", "sha256:", "sha256:../../etc/passwd", "md5:" + hex64("a"),
		"sha256:" + hex64("a") + "x", "sha256:" + strings.Repeat("g", 64),
	} {
		if _, err := digestHex(bad); err == nil {
			t.Errorf("digestHex(%q) must fail", bad)
		}
	}
	good := "sha256:" + hex64("a")
	if h, err := digestHex(good); err != nil || len(h) != 64 {
		t.Fatalf("digestHex(%q) = %q, %v", good, h, err)
	}
}

func TestBlobWriterRoundTrip(t *testing.T) {
	st := newStore(t)
	body := []byte("hello blob")
	digest, err := digestOf(body)
	if err != nil {
		t.Fatal(err)
	}
	w, err := st.BlobWriter(digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	if !st.HasBlob(digest) {
		t.Fatal("blob must exist after commit")
	}
	got, err := st.ReadBlob(digest)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("read = %q, %v", got, err)
	}
}

// 摘要不符：Commit 失败且不留文件。
func TestBlobWriterDigestMismatch(t *testing.T) {
	st := newStore(t)
	digest, _ := digestOf([]byte("expected"))
	w, err := st.BlobWriter(digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("something else")); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("err = %v", err)
	}
	if st.HasBlob(digest) {
		t.Fatal("mismatched blob must not be committed")
	}
	// tmp 目录里不留残留。
	entries, err := os.ReadDir(filepath.Join(st.root, "tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("tmp not cleaned: %v", entries)
	}
}

// Abort 删掉临时文件。
func TestBlobWriterAbort(t *testing.T) {
	st := newStore(t)
	digest, _ := digestOf([]byte("x"))
	w, err := st.BlobWriter(digest)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("x"))
	w.Abort()
	entries, _ := os.ReadDir(filepath.Join(st.root, "tmp"))
	if len(entries) != 0 {
		t.Fatalf("tmp not cleaned: %v", entries)
	}
}

// PutImage / Find 支持完整引用与摘要两种查找。
func TestStoreFind(t *testing.T) {
	st := newStore(t)
	manifestDigest, _ := digestOf([]byte("m"))
	indexDigest, _ := digestOf([]byte("i"))
	img := Image{
		Reference:      "docker.io/library/alpine:3.19",
		ManifestDigest: manifestDigest,
		IndexDigest:    indexDigest,
		ConfigDigest:   "sha256:" + hex64("c"),
		Platform:       "linux/arm64",
		Layers:         []Layer{{Digest: "sha256:" + hex64("l"), Size: 10}},
	}
	if err := st.PutImage(img); err != nil {
		t.Fatal(err)
	}

	for _, ref := range []string{
		"alpine:3.19",
		"docker.io/library/alpine:3.19",
		"alpine@" + manifestDigest,
		"alpine@" + indexDigest,
	} {
		got, err := st.Find(ref)
		if err != nil {
			t.Errorf("Find(%q): %v", ref, err)
			continue
		}
		if got.Reference != img.Reference {
			t.Errorf("Find(%q) = %q", ref, got.Reference)
		}
	}
	if _, err := st.Find("alpine:3.20"); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("err = %v", err)
	}
}

// 索引持久化：重新打开 store 仍能找到。
func TestStorePersistsIndex(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	img := Image{Reference: "docker.io/library/redis:7", ConfigDigest: "sha256:" + hex64("c"),
		ManifestDigest: "sha256:" + hex64("m"), Layers: []Layer{{Digest: "sha256:" + hex64("l"), Size: 1}}}
	if err := st.PutImage(img); err != nil {
		t.Fatal(err)
	}
	st2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st2.Find("redis:7"); err != nil {
		t.Fatalf("reopened store must find the image: %v", err)
	}
}

// 索引损坏时 Open 报错，不静默清空。
func TestStoreRejectsBrokenIndex(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "images", "index.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("broken index must be reported")
	}
}

// GC 只删无引用的 blob，并保留调用方声明的。
func TestStoreGC(t *testing.T) {
	st := newStore(t)
	used := []byte("used")
	orphan := []byte("orphan")
	usedDigest, _ := digestOf(used)
	orphanDigest, _ := digestOf(orphan)
	_ = st.PutBlob(usedDigest, used)
	_ = st.PutBlob(orphanDigest, orphan)

	img := Image{Reference: "docker.io/library/x:1", ConfigDigest: usedDigest,
		ManifestDigest: usedDigest, Layers: []Layer{{Digest: usedDigest, Size: 4}}}
	if err := st.PutImage(img); err != nil {
		t.Fatal(err)
	}
	removed, err := st.GC(InUse{})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if !st.HasBlob(usedDigest) {
		t.Fatal("referenced blob must be kept")
	}
	if st.HasBlob(orphanDigest) {
		t.Fatal("unreferenced blob must be gone")
	}
}

// Remove 删掉索引条目，但 blob 留给 GC。
func TestStoreRemoveKeepsBlob(t *testing.T) {
	st := newStore(t)
	body := []byte("layer")
	digest, _ := digestOf(body)
	_ = st.PutBlob(digest, body)
	img := Image{Reference: "docker.io/library/y:1", ConfigDigest: digest, ManifestDigest: digest,
		Layers: []Layer{{Digest: digest, Size: int64(len(body))}}}
	if err := st.PutImage(img); err != nil {
		t.Fatal(err)
	}
	if err := st.Remove("y:1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Find("y:1"); !errors.Is(err, ErrImageNotFound) {
		t.Fatal("image must be gone from the index")
	}
	if !st.HasBlob(digest) {
		t.Fatal("blob must stay until GC")
	}
}

// SizeOf 累加层大小。
func TestImageSizeOf(t *testing.T) {
	img := Image{Layers: []Layer{{Size: 10}, {Size: 20}}}
	if img.SizeOf() != 30 {
		t.Fatalf("size = %d", img.SizeOf())
	}
}
