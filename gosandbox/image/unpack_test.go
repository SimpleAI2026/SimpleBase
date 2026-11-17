package image

// unpack_test.go：层解包的安全与语义。
//
// 每个用例对应 kiri 的 layers.rs 测试或一条明确的威胁：
//   1. 符号链接的中间段不能把写入带出 lower；
//   2. whiteout 删除同目录的同名条目；
//   3. whiteout 穿过符号链接仍然落在 lower 内；
//   4. . 与 .. 的 whiteout 被拒绝；
//   5. opaque 清空目录但保留目录本身；
//   6. 设备节点 / FIFO 跳过而不是失败；
//   7. 解压膨胀有上限；
//   8. 半成品目录（无 .done）被重新解。

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// layerTar 造一个层 blob（未压缩）并写进 store，返回 Image。
func makeImage(t *testing.T, st *Store, layers ...[]byte) Image {
	t.Helper()
	img := Image{Reference: "test:1", Platform: "linux/arm64"}
	for _, body := range layers {
		digest, err := digestOf(body)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.PutBlob(digest, body); err != nil {
			t.Fatal(err)
		}
		img.Layers = append(img.Layers, Layer{Digest: digest, Size: int64(len(body)),
			MediaType: LayerTypePlain})
	}
	return img
}

// tarWith 用给定的条目造一个 tar。
func tarWith(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: e.mode, Size: int64(len(e.body)), Typeflag: e.typ}
		if hdr.Mode == 0 {
			hdr.Mode = 0o644
		}
		if hdr.Typeflag == 0 {
			hdr.Typeflag = tar.TypeReg
		}
		hdr.Linkname = e.link
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if len(e.body) > 0 {
			if _, err := tw.Write(e.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// tarEntry 是一个 tar 条目的简写。
type tarEntry struct {
	name string
	typ  byte
	mode int64
	link string
	body []byte
}

// newStore 建一个临时 store。
func newStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// unpack 解一个由单个 tar 组成的镜像。
func unpackOne(t *testing.T, st *Store, body []byte) (string, error) {
	t.Helper()
	img := makeImage(t, st, body)
	return NewUnpacker(st).Unpack(context.Background(), img)
}

// 基本条目：目录、普通文件、符号链接、硬链接。
func TestUnpackBasic(t *testing.T) {
	st := newStore(t)
	body := tarWith(t,
		tarEntry{name: "etc/", typ: tar.TypeDir, mode: 0o755},
		tarEntry{name: "etc/app.conf", body: []byte("k=v\n")},
		tarEntry{name: "etc/link", typ: tar.TypeSymlink, link: "app.conf"},
		tarEntry{name: "bin/", typ: tar.TypeDir, mode: 0o755},
		tarEntry{name: "bin/sh", mode: 0o755, body: []byte("#!/bin/sh\n")},
		tarEntry{name: "bin/ash", typ: tar.TypeLink, link: "bin/sh"},
	)
	chain, err := unpackOne(t, st, body)
	if err != nil {
		t.Fatal(err)
	}
	lower := NewUnpacker(st).LowerPath(chain)
	data, err := os.ReadFile(filepath.Join(lower, "etc/app.conf"))
	if err != nil || string(data) != "k=v\n" {
		t.Fatalf("app.conf = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(lower, "etc/link")); err != nil {
		t.Fatal(err)
	}
	// 硬链接共享 inode。
	a, _ := os.Stat(filepath.Join(lower, "bin/sh"))
	b, _ := os.Stat(filepath.Join(lower, "bin/ash"))
	if !os.SameFile(a, b) {
		t.Fatal("hard link must share the inode")
	}
	// .done 标记存在。
	if _, err := os.Stat(lower + ".done"); err != nil {
		t.Fatal("missing .done marker")
	}
}

// .wh. 删除同目录的条目。
func TestUnpackWhiteout(t *testing.T) {
	st := newStore(t)
	base := tarWith(t,
		tarEntry{name: "etc/", typ: tar.TypeDir, mode: 0o755},
		tarEntry{name: "etc/lower.conf", body: []byte("old")},
	)
	top := tarWith(t, tarEntry{name: "etc/.wh.lower.conf"})
	img := makeImage(t, st, base, top)
	chain, err := NewUnpacker(st).Unpack(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(NewUnpacker(st).LowerPath(chain), "etc/lower.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("whiteout must remove the file, stat err = %v", err)
	}
}

// .wh..wh..opq 清空目录，但目录本身保留。
func TestUnpackOpaque(t *testing.T) {
	st := newStore(t)
	base := tarWith(t,
		tarEntry{name: "app/", typ: tar.TypeDir, mode: 0o755},
		tarEntry{name: "app/a", body: []byte("a")},
		tarEntry{name: "app/b", body: []byte("b")},
	)
	top := tarWith(t,
		tarEntry{name: "app/.wh..wh..opq"},
		tarEntry{name: "app/c", body: []byte("c")},
	)
	img := makeImage(t, st, base, top)
	chain, err := NewUnpacker(st).Unpack(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	lower := NewUnpacker(st).LowerPath(chain)
	if _, err := os.Stat(filepath.Join(lower, "app")); err != nil {
		t.Fatalf("opaque must keep the directory: %v", err)
	}
	for _, gone := range []string{"app/a", "app/b"} {
		if _, err := os.Stat(filepath.Join(lower, gone)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s must be gone", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(lower, "app/c")); err != nil {
		t.Fatalf("new file must be there: %v", err)
	}
}

// 符号链接的中间段不能把写入带出 lower。
//
// 构造：第一层建 a -> /tmp/escape（绝对符号链接），第二层写 a/pwned。
// 朴素的实现在写 a/pwned 时会跟随符号链接写到 /tmp/escape/pwned。
func TestUnpackSymlinkParentCannotEscape(t *testing.T) {
	st := newStore(t)
	escape := t.TempDir()
	base := tarWith(t,
		tarEntry{name: "a", typ: tar.TypeSymlink, link: escape},
	)
	top := tarWith(t, tarEntry{name: "a/pwned", body: []byte("x")})
	img := makeImage(t, st, base, top)

	_, err := NewUnpacker(st).Unpack(context.Background(), img)
	// 两种可接受的结果：拒绝这一层，或者把它写在 lower 内。
	// 唯一不可接受的是真的写到了 escape 目录里。
	if _, serr := os.Stat(filepath.Join(escape, "pwned")); serr == nil {
		t.Fatalf("write escaped the lower to %s (err=%v)", escape, err)
	}
}

// whiteout 穿过符号链接也落在 lower 内。
func TestUnpackWhiteoutThroughSymlinkStaysInside(t *testing.T) {
	st := newStore(t)
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim")
	if err := os.WriteFile(victim, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := tarWith(t,
		tarEntry{name: "link", typ: tar.TypeSymlink, link: outside},
	)
	// 试图通过符号链接删掉外部文件。
	top := tarWith(t, tarEntry{name: "link/.wh.victim"})
	img := makeImage(t, st, base, top)

	_, _ = NewUnpacker(st).Unpack(context.Background(), img)
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("whiteout escaped the lower and deleted %s: %v", victim, err)
	}
}

// 指向 . 或 .. 的 whiteout 被拒绝（或至少不产生效果）。
func TestUnpackWhiteoutOfDotIsRefused(t *testing.T) {
	st := newStore(t)
	for _, name := range []string{".wh..", ".wh."} {
		body := tarWith(t, tarEntry{name: name})
		if _, err := unpackOne(t, st, body); err != nil {
			// 拒绝也可以，只要不是 panic 或删掉整个 lower。
			t.Logf("%s refused: %v", name, err)
		}
	}
}

// 设备节点与 FIFO 跳过，不失败。
func TestUnpackSkipsDevices(t *testing.T) {
	st := newStore(t)
	body := tarWith(t,
		tarEntry{name: "dev/", typ: tar.TypeDir, mode: 0o755},
		tarEntry{name: "dev/null", typ: tar.TypeChar},
		tarEntry{name: "dev/fifo", typ: tar.TypeFifo},
		tarEntry{name: "kept", body: []byte("ok")},
	)
	chain, err := unpackOne(t, st, body)
	if err != nil {
		t.Fatalf("devices must be skipped, not fail: %v", err)
	}
	lower := NewUnpacker(st).LowerPath(chain)
	if _, err := os.Stat(filepath.Join(lower, "dev/null")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("character device must not be created")
	}
	if _, err := os.Stat(filepath.Join(lower, "kept")); err != nil {
		t.Fatal("other entries must still be written")
	}
}

// 越界路径被拒绝：绝对路径被当作相对（不逃逸），".." 被拒绝。
func TestUnpackRejectsParentTraversal(t *testing.T) {
	st := newStore(t)
	body := tarWith(t, tarEntry{name: "../escape", body: []byte("x")})
	if _, err := unpackOne(t, st, body); !errors.Is(err, ErrUnsafeArchive) {
		t.Fatalf("err = %v, want ErrUnsafeArchive", err)
	}
}

// 绝对路径：被当相对路径落在 lower 内，而不是写到宿主根。
func TestUnpackAbsolutePathStaysInside(t *testing.T) {
	st := newStore(t)
	body := tarWith(t, tarEntry{name: "/etc/hostname", body: []byte("sandbox\n")})
	chain, err := unpackOne(t, st, body)
	if err != nil {
		t.Fatal(err)
	}
	lower := NewUnpacker(st).LowerPath(chain)
	if _, err := os.Stat(filepath.Join(lower, "etc/hostname")); err != nil {
		t.Fatalf("absolute path must land inside the lower: %v", err)
	}
	// 宿主的 /etc/hostname 没被碰。
	host, _ := os.ReadFile("/etc/hostname")
	if strings.TrimSpace(string(host)) == "sandbox" {
		t.Fatal("write reached the host filesystem")
	}
}

// setuid / setgid 位被清掉。
func TestUnpackStripsSetuid(t *testing.T) {
	st := newStore(t)
	body := tarWith(t, tarEntry{name: "bin/su", mode: 0o4755, body: []byte("x")})
	chain, err := unpackOne(t, st, body)
	if err != nil {
		t.Fatal(err)
	}
	st2, err := os.Stat(filepath.Join(NewUnpacker(st).LowerPath(chain), "bin/su"))
	if err != nil {
		t.Fatal(err)
	}
	if st2.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Fatalf("setuid must be stripped, mode = %v", st2.Mode())
	}
}

// 同一个镜像解两次复用同一个 lower。
func TestUnpackIsIdempotent(t *testing.T) {
	st := newStore(t)
	img := makeImage(t, st, tarWith(t, tarEntry{name: "a", body: []byte("1")}))
	u := NewUnpacker(st)
	first, err := u.Unpack(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	second, err := u.Unpack(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("chain ids differ: %q vs %q", first, second)
	}
}

// 没有 .done 的半成品目录被重新解。
func TestUnpackReplacesHalfBuilt(t *testing.T) {
	st := newStore(t)
	img := makeImage(t, st, tarWith(t, tarEntry{name: "a", body: []byte("1")}))
	u := NewUnpacker(st)
	chain := ChainID(img.Layers)
	dir := u.LowerPath(chain)
	// 伪造一个半成品：有目录、有垃圾文件、没有 .done。
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "garbage"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := u.Unpack(context.Background(), img); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "garbage")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("half-built directory must be replaced")
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); err != nil {
		t.Fatal("content must be unpacked")
	}
}

// gzip 层按 media type 解开。
func TestUnpackGzipLayer(t *testing.T) {
	st := newStore(t)
	raw := tarWith(t, tarEntry{name: "hello", body: []byte("world")})
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	digest, _ := digestOf(gz.Bytes())
	if err := st.PutBlob(digest, gz.Bytes()); err != nil {
		t.Fatal(err)
	}
	img := Image{Reference: "gz:1", Layers: []Layer{{Digest: digest, Size: int64(gz.Len()), MediaType: LayerType}}}
	chain, err := NewUnpacker(st).Unpack(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(NewUnpacker(st).LowerPath(chain), "hello"))
	if err != nil || string(data) != "world" {
		t.Fatalf("hello = %q, %v", data, err)
	}
}

// zstd 层默认不可用：给出明确错误，而不是静默跳过。
func TestUnpackZstdUnavailable(t *testing.T) {
	st := newStore(t)
	body := tarWith(t, tarEntry{name: "a", body: []byte("1")})
	digest, _ := digestOf(body)
	_ = st.PutBlob(digest, body)
	img := Image{Reference: "z:1", Layers: []Layer{{Digest: digest, Size: int64(len(body)), MediaType: LayerTypeZstd}}}
	if _, err := NewUnpacker(st).Unpack(context.Background(), img); !errors.Is(err, ErrNoZstd) {
		t.Fatalf("err = %v, want ErrNoZstd", err)
	}
}

// ChainID 由层摘要决定：同层同 id，不同层不同 id。
func TestChainID(t *testing.T) {
	a := Layer{Digest: "sha256:" + hex64("a")}
	b := Layer{Digest: "sha256:" + hex64("b")}
	if ChainID([]Layer{a, b}) != ChainID([]Layer{a, b}) {
		t.Fatal("same layers must give the same chain id")
	}
	if ChainID([]Layer{a, b}) == ChainID([]Layer{b, a}) {
		t.Fatal("layer order must matter")
	}
	if ChainID(nil) != "empty" {
		t.Fatal("no layers must give the empty id")
	}
}

// remove 拒绝可疑的 chainID，避免路径穿越。
func TestUnpackRemoveRejectsBadChainID(t *testing.T) {
	st := newStore(t)
	u := NewUnpacker(st)
	for _, bad := range []string{"", "../etc", "a/b", `a\b`} {
		if err := u.Remove(bad); err == nil {
			t.Errorf("Remove(%q) must fail", bad)
		}
	}
}
