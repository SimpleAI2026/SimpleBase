// 文件说明：fs.go — agent 的文件服务：在沙盒视角读写、列目录、删除、改名
//
// 用法
//
//	(s *Server) fsOp(m proto.Msg)  // Stat / List / Read / Remove / Mkdir / Rename
//	(s *Server) fsWrite(m proto.Msg) error  // 写入；帧后跟 Size 字节裸数据
//	同包调用：fsOp() → agent.go；fsWrite() → agent.go
//
// 调用架构
//
//	[上游] gosandbox/agent（读循环）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/agent/fs.go
//	         │
//	         ▼
//	[下游] gosandbox/proto、标准库
//	[同包] agent.go
//
// 主要符号
//
//	(s *Server) fsOp(m proto.Msg)
//	(s *Server) fsWrite(m proto.Msg) error

package agent

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// maxReadChunk 是单次 FsRead 返回的上限。
// 与 fsview 的 MAX_READ 同量级，保证一帧装得下。
const maxReadChunk = 512 << 10

// maxListEntries 是单次 FsList 返回的条目上限，超出截断而不是报错。
const maxListEntries = 5000

// fsOp 处理不需要裸数据的文件请求。
//
// 回复里的 Seq 必须原样带回请求里的值：宿主用它找会等待中的调用方，
// 用 agent 自己的计数器会让每个文件请求都等到超时。
func (s *Server) fsOp(m proto.Msg) {
	seq := m.Seq
	switch m.Kind {
	case proto.KindFsStat:
		s.fsStat(seq, m)
	case proto.KindFsList:
		s.fsList(seq, m)
	case proto.KindFsRead:
		s.fsRead(seq, m)
	case proto.KindFsRemove:
		s.fsReply(seq, os.Remove(m.Path))
	case proto.KindFsMkdir:
		mode := os.FileMode(m.Mode)
		if mode == 0 {
			mode = 0o755
		}
		s.fsReply(seq, os.MkdirAll(m.Path, mode))
	case proto.KindFsRename:
		s.fsReply(seq, os.Rename(m.Path, m.To))
	}
}

// fsReply 是「成功即可、无数据」类请求的回复。
func (s *Server) fsReply(seq uint32, err error) {
	if err != nil {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: seq, Err: errText(err)})
		return
	}
	_ = s.send(&proto.Msg{Kind: proto.KindAck, Seq: seq})
}

// fsStat 返回路径元数据。不跟随符号链接：宿主需要知道它本身是链接。
func (s *Server) fsStat(seq uint32, m proto.Msg) {
	st, err := os.Lstat(m.Path)
	if err != nil {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: seq, Err: errText(err)})
		return
	}
	reply := protocolEntry(seq, st)
	if m.Path != "" && st.Mode()&fs.ModeSymlink != 0 {
		if target, lerr := os.Readlink(m.Path); lerr == nil {
			reply.Link = target
		}
	}
	_ = s.send(&reply)
}

// projectEntry 把 os.FileInfo 转成协议里的 FsStatReply。
func protocolEntry(seq uint32, st os.FileInfo) proto.Msg {
	return proto.Msg{
		Kind:    proto.KindFsStatReply,
		Seq:     seq,
		FileKnd: fileKindOf(st.Mode()),
		Size:    st.Size(),
		Mode:    uint32(st.Mode().Perm()),
		ModNS:   st.ModTime().UnixNano(),
	}
}

// fileKindOf 把 Go 的文件类型映射成协议的 FileKind。
func fileKindOf(m fs.FileMode) proto.FileKind {
	switch {
	case m.IsRegular():
		return proto.FileRegular
	case m.IsDir():
		return proto.FileDir
	case m&fs.ModeSymlink != 0:
		return proto.FileSymlink
	}
	return proto.FileOther
}

// fsList 列一层目录。条目按名字排序（ReadDir 已排序）。
func (s *Server) fsList(seq uint32, m proto.Msg) {
	entries, err := os.ReadDir(m.Path)
	if err != nil {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: seq, Err: errText(err)})
		return
	}
	out := make([]proto.Entry, 0, min(len(entries), 256))
	for i, e := range entries {
		if i >= maxListEntries {
			break
		}
		info, ierr := e.Info()
		if ierr != nil {
			// 条目在列目录与 stat 之间消失：跳过，不让整个列表失败。
			continue
		}
		entry := proto.Entry{
			Name:  e.Name(),
			Kind:  fileKindOf(e.Type()),
			Mode:  uint32(info.Mode().Perm()),
			ModNS: info.ModTime().UnixNano(),
		}
		if info.Mode().IsRegular() {
			entry.Size = info.Size()
		}
		if e.Type()&fs.ModeSymlink != 0 {
			if target, lerr := os.Readlink(filepath.Join(m.Path, e.Name())); lerr == nil {
				entry.Target = target
			}
		}
		out = append(out, entry)
	}
	_ = s.send(&proto.Msg{Kind: proto.KindFsListReply, Seq: seq, Entries: out})
}

// fsRead 读一段文件。
//
// 只读普通文件：沙盒内的命令可能留下 FIFO 或设备文件，O_RDONLY 打开会阻塞。
func (s *Server) fsRead(seq uint32, m proto.Msg) {
	n := int(m.Size)
	if n <= 0 || n > maxReadChunk {
		n = maxReadChunk
	}
	f, err := os.OpenFile(m.Path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: seq, Err: errText(err)})
		return
	}
	defer f.Close()
	if st, serr := f.Stat(); serr != nil || !st.Mode().IsRegular() {
		if serr == nil {
			serr = errors.New("not a regular file")
		}
		_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: seq, Err: errText(serr)})
		return
	}
	if m.Offset > 0 {
		if _, serr := f.Seek(int64(m.Offset), io.SeekStart); serr != nil {
			_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: seq, Err: errText(serr)})
			return
		}
	}
	buf := make([]byte, n)
	read, rerr := io.ReadFull(f, buf)
	if rerr != nil && !errors.Is(rerr, io.EOF) && !errors.Is(rerr, io.ErrUnexpectedEOF) {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: seq, Err: errText(rerr)})
		return
	}
	_ = s.send(&proto.Msg{Kind: proto.KindFsReadReply, Seq: seq, Data: buf[:read]})
}

// fsWrite 处理写入：帧里给了 Size，之后紧跟 Size 字节裸数据。
//
// 必须先把裸数据从连接里读干净，否则后续帧会被当成文件内容。
func (s *Server) fsWrite(m proto.Msg) error {
	size := int(m.Size)
	if size < 0 || size > proto.MaxFrame {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: m.Seq, Err: "write size out of range"})
		return nil
	}
	data := make([]byte, size)
	if size > 0 {
		if _, err := io.ReadFull(s.in, data); err != nil {
			// 连接坏了，没法再谈下去。
			return err
		}
	}
	if m.Path == "" {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: m.Seq, Err: "empty path"})
		return nil
	}
	mode := os.FileMode(m.Mode)
	if mode == 0 {
		mode = 0o644
	}
	// 父目录不存在时创建：宿主的写文件语义是「按需建父目录」。
	if dir := filepath.Dir(m.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: m.Seq, Err: errText(err)})
			return nil
		}
	}
	flags := os.O_WRONLY | os.O_CREATE
	if m.Offset == 0 {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(m.Path, flags, mode)
	if err != nil {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: m.Seq, Err: errText(err)})
		return nil
	}
	defer f.Close()
	if m.Offset > 0 {
		if _, serr := f.Seek(int64(m.Offset), io.SeekStart); serr != nil {
			_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: m.Seq, Err: errText(serr)})
			return nil
		}
	}
	if _, werr := f.Write(data); werr != nil {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Seq: m.Seq, Err: errText(werr)})
		return nil
	}
	_ = s.send(&proto.Msg{Kind: proto.KindFsWriteAck, Seq: m.Seq})
	return nil
}

// errText 是给宿主的错误文本：只给原因，不带栈。
func errText(err error) string {
	if pe, ok := err.(*os.PathError); ok {
		return pe.Op + " " + pe.Path + ": " + pe.Err.Error()
	}
	return err.Error()
}
