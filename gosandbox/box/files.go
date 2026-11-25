// 文件说明：files.go — gosandbox/box 中 FileInfo 等符号的实现
//
// 用法
//
//	type FileInfo struct{…}  // FileInfo 是一个路径的元数据
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/files.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backpressure_test.go, copytree.go, deadline.go, egress_e2e_test.go, handle.go 等 26 个
//
// 主要符号
//
//	const fileTimeout
//	type FileInfo struct{…}
//	(s *Session) fsRequest(m *proto.Msg) (proto.Msg, error)
//	(s *Session) Stat(path string) (FileInfo, error)
//	(s *Session) ListDir(path string) ([]proto.Entry, error)
//	(s *Session) ReadFile(path string, maxBytes int) ([]byte, error)
//	(s *Session) WriteFile(path string, data []byte, perm uint32) error
//	(s *Session) Remove(path string) error
//	... 另有 5 个顶层符号

package box

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// 文件请求的默认超时。沙盒内的目录可能很大，比普通 API 调用宽一些。
const fileTimeout = 30 * time.Second

// FileInfo 是一个路径的元数据。
type FileInfo struct {
	Kind    proto.FileKind
	Size    int64
	Mode    uint32
	ModTime time.Time
	Link    string
}

// fsRequest 发一个文件请求并等回复。
func (s *Session) fsRequest(m *proto.Msg) (proto.Msg, error) {
	return s.request(m, fileTimeout)
}

// Stat 返回路径元数据；不跟随符号链接。
func (s *Session) Stat(path string) (FileInfo, error) {
	r, err := s.fsRequest(&proto.Msg{Kind: proto.KindFsStat, ID: "fs", Path: path})
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{Kind: r.FileKnd, Size: r.Size, Mode: r.Mode,
		ModTime: time.Unix(0, r.ModNS), Link: r.Link}, nil
}

// ListDir 列一层目录。
func (s *Session) ListDir(path string) ([]proto.Entry, error) {
	r, err := s.fsRequest(&proto.Msg{Kind: proto.KindFsList, ID: "fs", Path: path})
	if err != nil {
		return nil, err
	}
	return r.Entries, nil
}

// ReadFile 读一段文件。maxBytes <= 0 表示由 agent 决定单次上限（512 KiB）。
//
// 只读一次：调用方要完整文件时用 io.ReadAll(io.LimitReader(...)) 组合多次读，
// 或者直接用 maxBytes 指定所需长度。
func (s *Session) ReadFile(path string, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = 512 << 10
	}
	r, err := s.fsRequest(&proto.Msg{Kind: proto.KindFsRead, ID: "fs", Path: path, Size: int64(maxBytes)})
	if err != nil {
		return nil, err
	}
	return r.Data, nil
}

// WriteFile 写文件，按需创建父目录。offset 为 0 时截断。
func (s *Session) WriteFile(path string, data []byte, perm uint32) error {
	if len(data) > proto.MaxFrame {
		return fmt.Errorf("box: write of %d bytes exceeds one frame", len(data))
	}
	// 帧与裸字节必须连续发出：中间不能插别的帧，否则对端会把它们当文件内容。
	// outFrame 的 raw 字段让 writer 在同一次循环里写出两者。
	seq := s.nextSeq()
	ch := make(chan proto.Msg, 1)
	s.mu.Lock()
	s.pending[seq] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, seq)
		s.mu.Unlock()
	}()

	select {
	case s.out <- outFrame{
		msg: &proto.Msg{Kind: proto.KindFsWrite, ID: "fs", Seq: seq, Path: path, Size: int64(len(data)), Mode: perm},
		raw: data,
	}:
	case <-s.done:
		return io.ErrClosedPipe
	}

	select {
	case r := <-ch:
		if r.Kind == proto.KindError {
			return errors.New(r.Err)
		}
		return nil
	case <-time.After(fileTimeout):
		return errors.New("box: write timed out")
	case <-s.done:
		return io.ErrClosedPipe
	}
}

// Remove 删除文件或空目录；不存在视为成功。
func (s *Session) Remove(path string) error {
	_, err := s.fsRequest(&proto.Msg{Kind: proto.KindFsRemove, ID: "fs", Path: path})
	return err
}

// MkdirAll 建目录。
func (s *Session) MkdirAll(path string, perm uint32) error {
	_, err := s.fsRequest(&proto.Msg{Kind: proto.KindFsMkdir, ID: "fs", Path: path, Mode: perm})
	return err
}

// Rename 改名。
func (s *Session) Rename(from, to string) error {
	_, err := s.fsRequest(&proto.Msg{Kind: proto.KindFsRename, ID: "fs", Path: from, To: to})
	return err
}

// Stats 读沙盒的用量。
func (s *Session) Stats() (proto.Stats, error) {
	if err := s.send(&proto.Msg{Kind: proto.KindStatsQuery}); err != nil {
		return proto.Stats{}, err
	}
	// 回复是异步的（进 stats 缓存），这里等一小会儿。
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := s.statsPtr(); st != nil {
			return *st, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return proto.Stats{}, errors.New("box: stats timed out")
}

// Listening 返回沙盒内 LISTEN 的 TCP 端口。
func (s *Session) Listening() ([]uint16, error) {
	r, err := s.request(&proto.Msg{Kind: proto.KindListenQuery, ID: "listen"}, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return r.Ports, nil
}

// DialPort 经 agent 建一条到沙盒内 127.0.0.1:port 的连接。
//
// 这是 E2B getHost(port) 的实现方式：宿主不需要任何到沙盒的网络路径，
// 也不需要沙盒有网络栈。
func (s *Session) DialPort(ctx context.Context, port uint16) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id := s.nextSeq()
	rc := newRelay(s, id)
	rc.port = port
	s.mu.Lock()
	s.relays[id] = rc
	s.mu.Unlock()
	connected := false
	defer func() {
		if !connected {
			_ = rc.Close()
		}
	}()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	if s.closed.Load() {
		return nil, io.ErrClosedPipe
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, io.ErrClosedPipe
	case <-timer.C:
		return nil, errors.New("box: relay did not connect")
	case s.out <- outFrame{msg: &proto.Msg{Kind: proto.KindRelay, ConnID: id, Port: port}}:
	}
	select {
	case <-rc.ready:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := rc.err(); err != nil {
			return nil, err
		}
		connected = true
		go rc.watchContext(ctx)
		return rc, nil
	case <-timer.C:
		return nil, errors.New("box: relay did not connect")
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, io.ErrClosedPipe
	}
}
