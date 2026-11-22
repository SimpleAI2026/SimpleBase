package snapshot

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	// ErrNotFound 表示快照不存在。
	ErrNotFound = errors.New("snapshot: no such snapshot")
	// ErrEmpty 表示沙盒里没有任何可保存的内容（既没有 upper 也没有 ws）。
	ErrEmpty = errors.New("snapshot: sandbox has nothing to save")
)

// Info 是一个快照的元数据。
type Info struct {
	// ID 是快照 id（内容无关的随机串，便于同一个沙盒有多份快照）。
	ID string `json:"id"`
	// BoxName 是拍快照时的沙盒名（排查用）。
	BoxName string `json:"box_name"`
	// Label 是调用方给的说明（可空）。
	Label string `json:"label,omitempty"`
	// Parent 是父快照的 id（fork 出来的沙盒再拍快照时填）。
	Parent string `json:"parent,omitempty"`
	// Size 是归档压缩后的大小。
	Size int64 `json:"size"`
	// Digest 是归档内容的 sha256，用于校验与去重诊断。
	Digest string `json:"digest"`
	// Created 是创建时间。
	Created time.Time `json:"created"`
	// HasUpper 表示归档里含镜像写时复制层（否则只有工作区）。
	HasUpper bool `json:"has_upper"`
}

// Store 是快照的持久层。
//
// 布局：
//
//	<root>/snapshots/blobs/<digest>.tar.gz   内容寻址的归档
//	<root>/snapshots/index.json              元数据索引
//	<root>/snapshots/tmp/                    打包中的临时文件
//
// 归档按内容寻址：同一个状态拍两次只占一份空间，删除时按引用计数清理。
type Store struct {
	root string

	mu    sync.Mutex
	index map[string]Info
}

// Open 打开（或创建）快照 store。
func Open(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("snapshot: empty root")
	}
	dir := filepath.Join(root, "snapshots")
	for _, d := range []string{filepath.Join(dir, "blobs"), filepath.Join(dir, "tmp")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	s := &Store{root: dir, index: map[string]Info{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load 读索引；损坏时当作空索引（保留原文件以便排查）。
func (s *Store) load() error {
	body, err := os.ReadFile(filepath.Join(s.root, "index.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var list []Info
	if err := json.Unmarshal(body, &list); err != nil {
		return nil
	}
	for _, it := range list {
		s.index[it.ID] = it
	}
	return nil
}

// save 落索引。写临时文件再 rename，避免半截文件。
func (s *Store) save() error {
	list := make([]Info, 0, len(s.index))
	for _, it := range s.index {
		list = append(list, it)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Created.After(list[j].Created) })
	body, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return fsx.WriteAtomic(filepath.Join(s.root, "index.json"), body, 0o600)
}

// blobPath 返回摘要对应的归档路径。
func (s *Store) blobPath(digest string) (string, error) {
	hexPart, ok := strings.CutPrefix(digest, "sha256:")
	if !ok || len(hexPart) != 64 {
		return "", fmt.Errorf("snapshot: bad digest %q", digest)
	}
	return filepath.Join(s.root, "blobs", hexPart+".tar.gz"), nil
}

// Create 给一个沙盒目录拍快照。
func (s *Store) Create(ctx context.Context, boxDir, name, label string) (Info, error) {
	return s.CreateFrom(ctx, boxDir, name, label, "")
}

// CreateFrom 拍快照并记下父快照（fork 出的沙盒再拍时用）。
func (s *Store) CreateFrom(ctx context.Context, boxDir, name, label, parent string) (Info, error) {
	upper := filepath.Join(boxDir, "upper")
	ws := filepath.Join(boxDir, "ws")
	hasUpper := dirNonEmpty(upper)
	if !hasUpper && !dirNonEmpty(ws) {
		return Info{}, ErrEmpty
	}

	tmp, err := os.CreateTemp(filepath.Join(s.root, "tmp"), "snap-*.tar.gz")
	if err != nil {
		return Info{}, err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	// 边打包边算摘要：写出去的就是存下来的，不必再读一遍。
	h := sha256.New()
	if err := packAll(ctx, []packEntry{{dir: upper, prefix: "upper"}, {dir: ws, prefix: "ws"}},
		io.MultiWriter(h, tmp)); err != nil {
		tmp.Close()
		return Info{}, err
	}
	if err := tmp.Close(); err != nil {
		return Info{}, err
	}
	size, err := fileSize(tmpName)
	if err != nil {
		return Info{}, err
	}
	digest := "sha256:" + hex.EncodeToString(h.Sum(nil))

	dst, err := s.blobPath(digest)
	if err != nil {
		return Info{}, err
	}
	// 内容已存在：直接复用，删掉这次的临时文件。
	if _, serr := os.Stat(dst); serr == nil {
		_ = os.Remove(tmpName)
	} else if err := os.Rename(tmpName, dst); err != nil {
		return Info{}, err
	}

	info := Info{
		ID:       newID(),
		BoxName:  name,
		Label:    label,
		Parent:   parent,
		Size:     size,
		Digest:   digest,
		Created:  time.Now().UTC(),
		HasUpper: hasUpper,
	}
	s.mu.Lock()
	s.index[info.ID] = info
	err = s.save()
	s.mu.Unlock()
	if err != nil {
		return Info{}, err
	}
	return info, nil
}

// List 返回全部快照（新→旧）。
func (s *Store) List() []Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Info, 0, len(s.index))
	for _, it := range s.index {
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// Get 按 id 取元数据。
func (s *Store) Get(id string) (Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.index[id]
	if !ok {
		return Info{}, ErrNotFound
	}
	return it, nil
}

// Remove 删一个快照。归档只有在没有其他快照引用时才删。
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	it, ok := s.index[id]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	delete(s.index, id)
	stillUsed := false
	for _, other := range s.index {
		if other.Digest == it.Digest {
			stillUsed = true
			break
		}
	}
	err := s.save()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if stillUsed {
		return nil
	}
	if p, perr := s.blobPath(it.Digest); perr == nil {
		_ = os.Remove(p)
	}
	return nil
}

// Restore 把一个快照解进 boxDir（会清空 boxDir 的 upper 与 ws）。
//
// 调用方负责保证 boxDir 下没有正在运行的沙盒：恢复是「替换磁盘内容」，
// 运行中的进程不会感知到，写回去的内容会覆盖快照。
func (s *Store) Restore(ctx context.Context, id, boxDir string) (Info, error) {
	info, err := s.Get(id)
	if err != nil {
		return Info{}, err
	}
	path, err := s.blobPath(info.Digest)
	if err != nil {
		return Info{}, err
	}
	if err := unpackAll(ctx, path, boxDir); err != nil {
		return Info{}, err
	}
	return info, nil
}

// dirNonEmpty 报告目录存在且至少有一个条目。
func dirNonEmpty(p string) bool {
	entries, err := os.ReadDir(p)
	return err == nil && len(entries) > 0
}

// fileSize 返回文件大小。
func fileSize(p string) (int64, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// newID 生成一个短随机 id。
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("s%d", time.Now().UnixNano())
	}
	return "snap-" + hex.EncodeToString(b[:])
}
