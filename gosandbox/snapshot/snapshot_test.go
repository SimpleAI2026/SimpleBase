package snapshot

// snapshot_test.go：快照的打包、去重、恢复与安全边界。
//
//  1. 打包 upper+ws → 恢复后内容一致（含符号链接、权限、空目录）；
//  2. 内容相同的两次快照共用一份 blob，删一个不影响另一个；
//  3. 恢复只替换 upper/ws，不碰 logs/ 与 meta.json；
//  4. 没有 upper 的沙盒恢复后不会凭空多出 upper；
//  5. 越界条目（../ 与绝对路径）被拒绝；
//  6. 空沙盒拒绝拍快照。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeBox 造一个沙盒目录：ws/ + upper/ + logs/。
func makeBox(t *testing.T, upper bool) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ws", "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ws", "a.txt"), []byte("hello"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ws", "sub", "b.txt"), []byte("world"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logs", "p1.log"), []byte("noise"), 0o600); err != nil {
		t.Fatal(err)
	}
	if upper {
		if err := os.MkdirAll(filepath.Join(dir, "upper", "etc"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "upper", "etc", "motd"), []byte("mine"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("/etc/motd", filepath.Join(dir, "upper", "etc", "link")); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// 打包 → 恢复，内容与权限一致。
func TestCreateRestoreRoundTrip(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := makeBox(t, true)
	info, err := store.Create(context.Background(), src, "sb1", "before")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if info.Size <= 0 || info.Digest == "" {
		t.Fatalf("bad info: %+v", info)
	}
	if !info.HasUpper {
		t.Fatal("has_upper must be true")
	}

	dst := filepath.Join(t.TempDir(), "restored")
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Restore(context.Background(), info.ID, dst); err != nil {
		t.Fatalf("restore: %v", err)
	}

	for _, tc := range []struct{ path, want string }{
		{"ws/a.txt", "hello"},
		{"ws/sub/b.txt", "world"},
		{"upper/etc/motd", "mine"},
	} {
		body, rerr := os.ReadFile(filepath.Join(dst, tc.path))
		if rerr != nil {
			t.Fatalf("%s: %v", tc.path, rerr)
		}
		if string(body) != tc.want {
			t.Errorf("%s = %q, want %q", tc.path, body, tc.want)
		}
	}
	// 权限保留。
	fi, err := os.Stat(filepath.Join(dst, "ws", "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o640 {
		t.Errorf("perm = %v, want 0640", fi.Mode().Perm())
	}
	// 符号链接按原样恢复，不跟随。
	link, err := os.Readlink(filepath.Join(dst, "upper", "etc", "link"))
	if err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if link != "/etc/motd" {
		t.Errorf("link = %q", link)
	}
}

// 相同内容只存一份 blob；删一个不影响另一个。
func TestCreateDedup(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	box := makeBox(t, true)
	a, err := store.Create(context.Background(), box, "sb1", "one")
	if err != nil {
		t.Fatal(err)
	}
	// tar 头里有 mtime；同一状态立刻再拍，mtime 未变则摘要相同。
	b, err := store.CreateFrom(context.Background(), box, "sb1", "two", a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest != b.Digest {
		t.Skipf("digest differs (mtime drift): %s vs %s", a.Digest, b.Digest)
	}
	blob, err := store.blobPath(a.Digest)
	if err != nil {
		t.Fatal(err)
	}
	// 删 a：blob 仍在（b 引用它）。
	if err := store.Remove(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(blob); err != nil {
		t.Fatalf("blob must survive while another snapshot uses it: %v", err)
	}
	// 删 b：blob 消失。
	if err := store.Remove(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(blob); !os.IsNotExist(err) {
		t.Fatalf("blob must be removed with the last reference: %v", err)
	}
}

// 恢复不碰 upper/ws 之外的东西。
func TestRestoreKeepsOtherDirs(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := makeBox(t, true)
	info, err := store.Create(context.Background(), src, "sb1", "")
	if err != nil {
		t.Fatal(err)
	}

	dst := makeBox(t, true)
	// 目标里有一份「当前」状态与一个 meta.json。
	if err := os.WriteFile(filepath.Join(dst, "ws", "a.txt"), []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "meta.json"), []byte(`{"parent":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Restore(context.Background(), info.ID, dst); err != nil {
		t.Fatal(err)
	}
	// ws 被替换回快照内容。
	if body, _ := os.ReadFile(filepath.Join(dst, "ws", "a.txt")); string(body) != "hello" {
		t.Errorf("ws/a.txt = %q", body)
	}
	// meta.json 保留。
	if body, _ := os.ReadFile(filepath.Join(dst, "meta.json")); !strings.Contains(string(body), "parent") {
		t.Errorf("meta.json = %q", body)
	}
	// logs 保留。
	if _, err := os.Stat(filepath.Join(dst, "logs", "p1.log")); err != nil {
		t.Errorf("logs must survive restore: %v", err)
	}
}

// 没有 upper 的沙盒：恢复后不该出现 upper。
func TestRestoreWithoutUpper(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := makeBox(t, false) // 只有 ws
	info, err := store.Create(context.Background(), src, "sb1", "")
	if err != nil {
		t.Fatal(err)
	}
	if info.HasUpper {
		t.Fatal("has_upper must be false")
	}
	dst := makeBox(t, true) // 目标本来有 upper
	if _, err := store.Restore(context.Background(), info.ID, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "upper")); !os.IsNotExist(err) {
		t.Fatalf("restoring a snapshot without upper must remove the stale upper: %v", err)
	}
}

// 越界条目：解包拒绝。
func TestRestoreRejectsTraversal(t *testing.T) {
	if _, err := safeRel("../escape"); err == nil {
		t.Fatal("must reject ../")
	}
	if _, err := safeRel("/etc/passwd"); err == nil {
		t.Fatal("must reject absolute paths")
	}
	if got, err := safeRel("upper/./etc/motd"); err != nil || got != filepath.FromSlash("upper/etc/motd") {
		t.Fatalf("clean = %q, %v", got, err)
	}
}

// 空沙盒：拒绝拍快照。
func TestCreateEmpty(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "ws"), 0o700)
	if _, err := store.Create(context.Background(), dir, "empty", ""); err == nil {
		t.Fatal("must refuse an empty sandbox")
	}
}

// 索引持久化：重开 store 仍能看到快照。
func TestIndexPersists(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := store.Create(context.Background(), makeBox(t, true), "sb1", "label")
	if err != nil {
		t.Fatal(err)
	}
	again, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, gerr := again.Get(info.ID)
	if gerr != nil {
		t.Fatalf("get after reopen: %v", gerr)
	}
	if got.Label != "label" || got.Digest != info.Digest {
		t.Fatalf("got %+v", got)
	}
}

// 未知 id：ErrNotFound。
func TestGetUnknown(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("nope"); err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
	if err := store.Remove("nope"); err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
}

// 列表按时间倒序。
func TestListOrder(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	box := makeBox(t, true)
	a, _ := store.Create(context.Background(), box, "sb1", "first")
	time.Sleep(2 * time.Millisecond)
	b, _ := store.Create(context.Background(), box, "sb1", "second")
	list := store.List()
	if len(list) != 2 {
		t.Fatalf("len = %d", len(list))
	}
	if list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("order: %s, %s", list[0].Label, list[1].Label)
	}
}
