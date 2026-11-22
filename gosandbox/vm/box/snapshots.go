//go:build linux

package box

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/linkxzhou/SimpleBase/gosandbox/snapshot"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

// boxMeta 是传给沙盒的快照血缘，落在 <box>/meta.json。
type boxMeta struct {
	// Parent 是父快照 id。
	Parent string `json:"parent,omitempty"`
	// Source 是父沙盒名（排查用）。
	Source string `json:"source,omitempty"`
	// Created 是 fork 时间。
	Created string `json:"created,omitempty"`
}

// store 返回快照 store；未配置时为 nil。
func (d *Driver) snapshotStore() (*snapshot.Store, error) {
	if d.opts.Snapshots == nil {
		return nil, fmt.Errorf("%w: snapshots", vm.ErrNotSupported)
	}
	return d.opts.Snapshots, nil
}

// Snapshot 给一个沙盒拍文件系统快照。
func (d *Driver) Snapshot(ctx context.Context, t vm.Target, label string) (vm.SnapshotInfo, error) {
	store, err := d.snapshotStore()
	if err != nil {
		return vm.SnapshotInfo{}, err
	}
	root := filepath.Join(d.opts.Root, t.Name)
	if _, serr := os.Stat(root); serr != nil {
		if errors.Is(serr, os.ErrNotExist) {
			return vm.SnapshotInfo{}, fmt.Errorf("%w: sandbox %q does not exist", vm.ErrNotFound, t.Name)
		}
		return vm.SnapshotInfo{}, fmt.Errorf("%w: %v", vm.ErrBackend, serr)
	}
	// 血缘：这个沙盒自己可能是 fork 出来的。
	parent := ""
	if meta, merr := d.boxMeta(t.Name); merr == nil {
		parent = meta.Parent
	}
	info, err := store.CreateFrom(ctx, root, t.Name, label, parent)
	if err != nil {
		switch {
		case errors.Is(err, snapshot.ErrEmpty):
			// 空沙盒（没启动过、也没写过文件）：不是错误，是没有可拍的内容。
			return vm.SnapshotInfo{}, fmt.Errorf("%w: sandbox has nothing to save", vm.ErrInvalid)
		}
		return vm.SnapshotInfo{}, fmt.Errorf("%w: %v", vm.ErrBackend, err)
	}
	return toVMSnapshot(info), nil
}

// Snapshots 列出全部快照。
func (d *Driver) Snapshots(context.Context) ([]vm.SnapshotInfo, error) {
	store, err := d.snapshotStore()
	if err != nil {
		return nil, err
	}
	list := store.List()
	out := make([]vm.SnapshotInfo, 0, len(list))
	for _, it := range list {
		out = append(out, toVMSnapshot(it))
	}
	return out, nil
}

// RemoveSnapshot 删一个快照。
func (d *Driver) RemoveSnapshot(_ context.Context, id string) error {
	store, err := d.snapshotStore()
	if err != nil {
		return err
	}
	if err := store.Remove(id); err != nil {
		if errors.Is(err, snapshot.ErrNotFound) {
			return fmt.Errorf("%w: snapshot %q", vm.ErrNotFound, id)
		}
		return fmt.Errorf("%w: %v", vm.ErrBackend, err)
	}
	return nil
}

// Fork 从快照建一个新沙盒。
//
// 目标名字已有会话时拒绝：会话的进程会把内容写回去，与刚解出来的快照冲突。
func (d *Driver) Fork(ctx context.Context, snapshotID string, t vm.Target) (vm.SnapshotInfo, error) {
	store, err := d.snapshotStore()
	if err != nil {
		return vm.SnapshotInfo{}, err
	}
	if t.Name == "" {
		return vm.SnapshotInfo{}, fmt.Errorf("%w: fork needs a target name", vm.ErrInvalid)
	}
	l := d.lock(t.Name)
	l.Lock()
	defer l.Unlock()
	if s := d.session(t.Name); s != nil {
		return vm.SnapshotInfo{}, fmt.Errorf("%w: sandbox %q is running; stop it before forking into it",
			vm.ErrInvalid, t.Name)
	}

	root := filepath.Join(d.opts.Root, t.Name)
	// 目标目录里如果有 overlay 的 work，先删掉：它是内核的挂载工作区，
	// 残留会让下一次 mount 失败。
	_ = os.RemoveAll(filepath.Join(root, "work"))

	info, rerr := store.Restore(ctx, snapshotID, root)
	if rerr != nil {
		if errors.Is(rerr, snapshot.ErrNotFound) {
			return vm.SnapshotInfo{}, fmt.Errorf("%w: snapshot %q", vm.ErrNotFound, snapshotID)
		}
		return vm.SnapshotInfo{}, fmt.Errorf("%w: %v", vm.ErrBackend, rerr)
	}
	// 写血缘：这个沙盒 fork 自 info.ID。
	meta := boxMeta{Parent: info.ID, Source: info.BoxName}
	body, _ := json.Marshal(meta)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return vm.SnapshotInfo{}, fmt.Errorf("%w: %v", vm.ErrBackend, err)
	}
	if err := os.WriteFile(filepath.Join(root, "meta.json"), body, 0o600); err != nil {
		return vm.SnapshotInfo{}, fmt.Errorf("%w: %v", vm.ErrBackend, err)
	}
	// Stop 标记要清掉：fork 出来的沙盒是「待启动」，不是「已停止」。
	_ = os.Remove(filepath.Join(root, "stopped"))
	return toVMSnapshot(info), nil
}

// boxMeta 读一个沙盒的血缘；文件不存在返回空值。
func (d *Driver) boxMeta(name string) (boxMeta, error) {
	var m boxMeta
	body, err := os.ReadFile(filepath.Join(d.opts.Root, name, "meta.json"))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return boxMeta{}, err
	}
	return m, nil
}

// toVMSnapshot 把 snapshot.Info 转成 vm.SnapshotInfo。
func toVMSnapshot(it snapshot.Info) vm.SnapshotInfo {
	return vm.SnapshotInfo{
		ID: it.ID, Name: it.BoxName, Label: it.Label, Parent: it.Parent,
		Size: it.Size, Digest: it.Digest, Created: it.Created,
	}
}

var _ vm.Snapshotter = (*Driver)(nil)
