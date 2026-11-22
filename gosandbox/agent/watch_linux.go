//go:build linux

package agent

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// watchBatch 是事件合并窗口：一个解压任务会瞬间产生几千个事件，
// 逐条发帧会把控制通道占满。
const watchBatch = 200 * time.Millisecond

// maxWatches 是同时存在的监听数上限。
const maxWatches = 64

// watcher 是一个进行中的目录监听。
type watcher struct {
	id   string
	root string
	fd   int
	done chan struct{}
	// closed 防止重复关闭 fd。
	closed chan struct{}
}

// watch 开始监听一个目录。递归时把现有子目录一并向内核登记。
func (s *Server) watch(m proto.Msg) {
	if m.ID == "" || m.Path == "" {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Err: "watch needs an id and a path"})
		return
	}
	s.mu.Lock()
	n := len(s.watches)
	s.mu.Unlock()
	if n >= maxWatches {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Err: "too many directory watches"})
		return
	}

	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Err: "inotify: " + err.Error()})
		return
	}
	mask := uint32(unix.IN_CREATE | unix.IN_DELETE | unix.IN_MODIFY |
		unix.IN_MOVED_FROM | unix.IN_MOVED_TO | unix.IN_ATTRIB | unix.IN_DELETE_SELF)
	// 只看目录本身；递归时再逐个登记子目录。
	if _, err := unix.InotifyAddWatch(fd, m.Path, mask); err != nil {
		_ = unix.Close(fd)
		_ = s.send(&proto.Msg{Kind: proto.KindError, Err: "inotify: " + err.Error()})
		return
	}
	if m.Recursive {
		_ = filepath.WalkDir(m.Path, func(p string, d os.DirEntry, werr error) error {
			if werr != nil || !d.IsDir() || p == m.Path {
				return nil
			}
			_, _ = unix.InotifyAddWatch(fd, p, mask)
			return nil
		})
	}

	w := &watcher{id: m.ID, root: m.Path, fd: fd, done: make(chan struct{}), closed: make(chan struct{})}
	s.mu.Lock()
	if s.watches == nil {
		s.watches = map[string]*watcher{}
	}
	// 同 id 重复注册：先停旧的。
	if old := s.watches[m.ID]; old != nil {
		old.stop()
	}
	s.watches[m.ID] = w
	s.mu.Unlock()

	go s.watchLoop(w, m.Recursive, mask)
}

// watchLoop 读 inotify 事件并成批推给宿主。
func (s *Server) watchLoop(w *watcher, recursive bool, mask uint32) {
	defer func() {
		select {
		case <-w.closed:
		default:
			close(w.closed)
		}
		_ = unix.Close(w.fd)
	}()

	buf := make([]byte, 64<<10)
	// pending 按路径去重：同一个文件在一批里被改十次只报一次。
	pending := map[string]proto.FileKind{}
	timer := time.NewTimer(watchBatch)
	defer timer.Stop()

	flush := func() {
		if len(pending) == 0 {
			return
		}
		entries := make([]proto.Entry, 0, len(pending))
		for name, kind := range pending {
			entries = append(entries, proto.Entry{Name: filepath.Base(name), Target: name, Kind: kind})
		}
		// 排序让测试与调用方的输出稳定。
		sort.Slice(entries, func(i, j int) bool { return entries[i].Target < entries[j].Target })
		pending = map[string]proto.FileKind{}
		_ = s.send(&proto.Msg{Kind: proto.KindWatchEvent, ID: w.id, Entries: entries})
	}

	for {
		select {
		case <-w.done:
			flush()
			return
		case <-timer.C:
			flush()
			timer.Reset(watchBatch)
			continue
		default:
		}
		n, err := unix.Read(w.fd, buf)
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) {
				// 没有事件：等一小会儿再看 done。
				select {
				case <-w.done:
					flush()
					return
				case <-time.After(50 * time.Millisecond):
				}
				continue
			}
			return
		}
		// 解析 inotify_event 序列：wd(4) mask(4) cookie(4) len(4) name(len)
		for off := 0; off+16 <= n; {
			wd := int(int32(le32(buf[off:])))
			mask := le32(buf[off+4:])
			nameLen := int(le32(buf[off+12:]))
			off += 16
			if off+nameLen > n {
				break
			}
			name := string(buf[off : off+nameLen])
			off += nameLen
			name = trimNul(name)
			// 新目录：递归时登记它，这样运行中新建的子树也被覆盖。
			if recursive && mask&unix.IN_ISDIR != 0 && mask&(unix.IN_CREATE|unix.IN_MOVED_TO) != 0 && name != "" {
				_, _ = unix.InotifyAddWatch(w.fd, filepath.Join(w.root, name), mask)
			}
			if name == "" {
				continue // 目录自身的事件，没有文件名
			}
			kind := proto.FileOther
			if mask&unix.IN_ISDIR != 0 {
				kind = proto.FileDir
			}
			pending[filepath.Join(w.root, name)] = kind
			_ = wd
		}
	}
}

// le32 读一个小端 u32。
func le32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// trimNul 去掉 inotify 名字尾部的 NUL 填充。
func trimNul(s string) string {
	for len(s) > 0 && s[len(s)-1] == 0 {
		s = s[:len(s)-1]
	}
	return s
}

// unwatch 停掉一个监听。
func (s *Server) unwatch(m proto.Msg) {
	s.mu.Lock()
	w := s.watches[m.ID]
	delete(s.watches, m.ID)
	s.mu.Unlock()
	if w != nil {
		w.stop()
	}
}

// stopWatches 停掉全部监听（agent 关闭时）。
func (s *Server) stopWatches() {
	s.mu.Lock()
	all := s.watches
	s.watches = map[string]*watcher{}
	s.mu.Unlock()
	for _, w := range all {
		w.stop()
	}
}

// stop 通知监听循环退出；可重复调用。
func (w *watcher) stop() {
	select {
	case <-w.done:
	default:
		close(w.done)
	}
}
