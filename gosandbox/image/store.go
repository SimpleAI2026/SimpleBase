package image

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/internal/fsx"
)

// 错误分类。
var (
	// ErrImageNotFound 表示索引里没有这个引用。
	ErrImageNotFound = errors.New("image: no such image")
	// ErrInUse 表示镜像仍被沙盒或快照引用。
	ErrInUse = errors.New("image: image is in use")
)

// Layer 是一个层。
type Layer struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	MediaType string `json:"media_type"`
}

// Image 是索引里的一条。
type Image struct {
	// Reference 是归一化引用（不含摘要时是 tag 形式）。
	Reference string `json:"reference"`
	// ManifestDigest 是单平台 manifest 的摘要。
	ManifestDigest string `json:"manifest_digest"`
	// IndexDigest 是索引的摘要（多架构镜像才有）。
	IndexDigest string `json:"index_digest,omitempty"`
	// ConfigDigest 是 config 的摘要，也是镜像 id。
	ConfigDigest string `json:"config_digest"`
	// Platform 是 "linux/arm64" 这类。
	Platform string `json:"platform"`
	// Layers 按叠加顺序，从底到顶。
	Layers []Layer `json:"layers"`
	// Size 是所有层的压缩大小之和。
	Size int64 `json:"size"`
	// ChainID 是压平后的只读根标识（见 unpack.go）。
	ChainID string `json:"chain_id,omitempty"`
	// Created 是导入/拉取时间。
	Created time.Time `json:"created"`
}

// SizeOf 返回层大小之和。
func (i Image) SizeOf() int64 {
	var n int64
	for _, l := range i.Layers {
		n += l.Size
	}
	return n
}

// Store 是内容寻址的 blob store 与索引。
type Store struct {
	root string
	// mu 保护 index.json 的读写；blob 写入用原子 rename，不需要锁。
	mu    sync.Mutex
	index map[string]Image
}

// Open 打开（或创建）一个 store。
func Open(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("image: empty root")
	}
	dir := filepath.Join(root, "images")
	for _, d := range []string{filepath.Join(dir, "blobs", "sha256"), filepath.Join(dir, "tmp")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	s := &Store{root: dir, index: map[string]Image{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Root 返回 store 目录。
func (s *Store) Root() string { return s.root }

// load 读索引；文件不存在或损坏都当作空索引（损坏时保留原文件以便排查）。
func (s *Store) load() error {
	path := filepath.Join(s.root, "index.json")
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var list []Image
	if err := json.Unmarshal(body, &list); err != nil {
		return fmt.Errorf("image: index.json is unreadable: %w", err)
	}
	for _, img := range list {
		s.index[img.Reference] = img
	}
	return nil
}

// save 原子地写索引（tmp + rename）。
func (s *Store) save() error {
	list := make([]Image, 0, len(s.index))
	for _, img := range s.index {
		list = append(list, img)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Reference < list[j].Reference })
	body, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return fsx.WriteAtomic(filepath.Join(s.root, "index.json"), body, 0o600)
}

// blobPath 返回一个摘要对应的文件路径。
func (s *Store) blobPath(digest string) (string, error) {
	hex, err := digestHex(digest)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.root, "blobs", "sha256", hex), nil
}

// digestHex 校验摘要形式并取出 hex 部分。
//
// 这一步是安全边界：摘要会拼进文件路径，绝不能放行 "../" 之类的东西。
func digestHex(digest string) (string, error) {
	h, ok := strings.CutPrefix(digest, "sha256:")
	if !ok {
		return "", fmt.Errorf("image: digest %q is not sha256", digest)
	}
	if len(h) != 64 {
		return "", fmt.Errorf("image: digest %q has wrong length", digest)
	}
	for _, c := range h {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", fmt.Errorf("image: digest %q has non-hex characters", digest)
		}
	}
	return h, nil
}

// HasBlob 报告 blob 是否已完整存在。
func (s *Store) HasBlob(digest string) bool {
	path, err := s.blobPath(digest)
	if err != nil {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

// BlobSize 返回已有 blob 的大小；不存在返回 -1。
func (s *Store) BlobSize(digest string) int64 {
	path, err := s.blobPath(digest)
	if err != nil {
		return -1
	}
	st, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return st.Size()
}

// BlobWriter 是一个待写入的 blob：先写 tmp，Commit 时校验摘要并 rename。
//
// 中途失败只需 Abort（或由 GC 清掉 tmp），不会留下半个 blob 让后续读取
// 拿到损坏数据。
type BlobWriter struct {
	store  *Store
	digest string
	tmp    *os.File
	hash   hash.Hash
	closed bool
}

// BlobWriter 打开一个 blob 的写端。
func (s *Store) BlobWriter(digest string) (*BlobWriter, error) {
	if _, err := digestHex(digest); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Join(s.root, "tmp"), "blob-")
	if err != nil {
		return nil, err
	}
	return &BlobWriter{store: s, digest: digest, tmp: tmp, hash: sha256.New()}, nil
}

// Write 实现 io.Writer。
func (w *BlobWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errors.New("image: blob writer closed")
	}
	n, err := w.tmp.Write(p)
	if n > 0 {
		_, _ = w.hash.Write(p[:n])
	}
	return n, err
}

// Commit 校验摘要并原子落盘。校验失败时返回 ErrDigestMismatch 并删除临时文件。
func (w *BlobWriter) Commit() error {
	if w.closed {
		return errors.New("image: blob writer closed")
	}
	w.closed = true
	got := "sha256:" + hex.EncodeToString(w.hash.Sum(nil))
	if got != w.digest {
		name := w.tmp.Name()
		w.tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("%w: %s got %s", ErrDigestMismatch, w.digest, got)
	}
	if err := w.tmp.Sync(); err != nil {
		name := w.tmp.Name()
		w.tmp.Close()
		_ = os.Remove(name)
		return err
	}
	path, err := w.store.blobPath(w.digest)
	if err != nil {
		w.tmp.Close()
		return err
	}
	if err := w.tmp.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.Rename(w.tmp.Name(), path)
}

// Abort 放弃写入并删掉临时文件；可重复调用。
func (w *BlobWriter) Abort() {
	if w.closed {
		return
	}
	w.closed = true
	name := w.tmp.Name()
	_ = w.tmp.Close()
	_ = os.Remove(name)
}

// PutBlob 一次性写入一个小对象（config、manifest）。已存在则跳过。
func (s *Store) PutBlob(digest string, body []byte) error {
	if s.HasBlob(digest) {
		return nil
	}
	w, err := s.BlobWriter(digest)
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		w.Abort()
		return err
	}
	return w.Commit()
}

// ReadBlob 读一个 blob 的全部内容（用于 config 与 manifest）。
func (s *Store) ReadBlob(digest string) ([]byte, error) {
	path, err := s.blobPath(digest)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// PutImage 写入或更新索引里的一条。
func (s *Store) PutImage(img Image) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if img.Created.IsZero() {
		img.Created = time.Now().UTC()
	}
	if img.Size == 0 {
		img.Size = img.SizeOf()
	}
	s.index[img.Reference] = img
	return s.save()
}

// Images 返回按引用排序的全部镜像副本。
func (s *Store) Images() []Image {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Image, 0, len(s.index))
	for _, img := range s.index {
		out = append(out, img)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Reference < out[j].Reference })
	return out
}

// Find 按引用查镜像。接受 tag 与摘要两种形式：
// 先按完整引用精确匹配，再按「仓库 + 摘要」匹配（拉取时按摘要记录过）。
func (s *Store) Find(ref string) (Image, error) {
	parsed, err := Parse(ref)
	if err != nil {
		return Image{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if img, ok := s.index[parsed.String()]; ok {
		return img, nil
	}
	// 按摘要查：任何引用了该 manifest 的镜像都算命中。
	if parsed.Digest != "" {
		for _, img := range s.index {
			if img.ManifestDigest == parsed.Digest || img.IndexDigest == parsed.Digest {
				return img, nil
			}
		}
	}
	return Image{}, fmt.Errorf("%w: %s", ErrImageNotFound, ref)
}

// Remove 删除索引里的一条；图层 blob 由 GC 回收。
func (s *Store) Remove(ref string) error {
	img, err := s.Find(ref)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.index, img.Reference)
	return s.save()
}

// InUse 是 GC 时需要保留的引用集合，由调用方提供（运行中的沙盒、快照）。
type InUse struct {
	// ChainIDs 是仍被占用的压平根。
	ChainIDs map[string]bool
	// Blobs 是仍被引用的 blob 摘要（config、manifest）。
	Blobs map[string]bool
}

// GC 删除无引用的 blob 与残留的临时文件，返回删除的 blob 数。
//
// 保守策略：只删确实没有任何索引条目或调用方集合引用的 blob。
func (s *Store) GC(inUse InUse) (int, error) {
	keep := map[string]bool{}
	s.mu.Lock()
	for _, img := range s.index {
		keep[img.ConfigDigest] = true
		keep[img.ManifestDigest] = true
		if img.IndexDigest != "" {
			keep[img.IndexDigest] = true
		}
		for _, l := range img.Layers {
			keep[l.Digest] = true
		}
	}
	s.mu.Unlock()
	for d := range inUse.Blobs {
		keep[d] = true
	}

	dir := filepath.Join(s.root, "blobs", "sha256")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		digest := "sha256:" + e.Name()
		if keep[digest] {
			continue
		}
		// 压平根（lower）也按 chainID 引用 blob，保留它们。
		if inUse.ChainIDs[e.Name()] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	// 清掉下载中崩溃留下的临时文件（一小时以前的）。
	tmpDir := filepath.Join(s.root, "tmp")
	if entries, err := os.ReadDir(tmpDir); err == nil {
		cutoff := time.Now().Add(-time.Hour)
		for _, e := range entries {
			if info, ierr := e.Info(); ierr == nil && info.ModTime().Before(cutoff) {
				_ = os.Remove(filepath.Join(tmpDir, e.Name()))
			}
		}
	}
	return removed, nil
}

// CopyBlob 把 store 里的 blob 拷进 dst。
func (s *Store) CopyBlob(digest string, dst io.Writer) error {
	path, err := s.blobPath(digest)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(dst, f)
	return err
}
