package agent

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// sendStats 读本进程所在 cgroup 的用量。读不到就报 0：
// 指标缺一项不该让整个请求失败。
func (s *Server) sendStats() {
	dir, err := ownCgroupDir()
	if err != nil {
		_ = s.send(&proto.Msg{Kind: proto.KindStats})
		return
	}
	st := proto.Stats{
		MemoryBytes:    readInt(os.ReadFile, dir, "memory.current"),
		MemoryPeak:     readInt(os.ReadFile, dir, "memory.peak"),
		Pids:           readInt(os.ReadFile, dir, "pids.current"),
		OOMKills:       readKeyedInt(os.ReadFile, dir, "memory.events", "oom_kill"),
		CPUUsec:        readKeyedInt(os.ReadFile, dir, "cpu.stat", "usage_usec"),
		DiskReadBytes:  readKeyedInt(os.ReadFile, dir, "io.stat", "rbytes"),
		DiskWriteBytes: readKeyedInt(os.ReadFile, dir, "io.stat", "wbytes"),
	}
	_ = s.send(&proto.Msg{Kind: proto.KindStats, Stats: st})
}

// ownCgroupDir 返回本进程所在的 cgroup v2 目录。
func ownCgroupDir() (string, error) {
	b, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(line, "0::"); ok {
			return "/sys/fs/cgroup" + strings.TrimSpace(rest), nil
		}
	}
	return "", fmt.Errorf("cgroup v2 not in use")
}

// readInt 读一个整数文件；读不到返回 0。
func readInt(read func(string) ([]byte, error), dir, name string) int64 {
	b, err := read(dir + "/" + name)
	if err != nil {
		return 0
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// readKeyedInt 读逐行文件里的某个 key，出现多次时累加。
//
// 兼容两种排版：cpu.stat / memory.events 是 "key value"，io.stat 是
// "8:0 rbytes=4096 wbytes=8192"（字段名与值用 = 连接，且每设备一行）。
func readKeyedInt(read func(string) ([]byte, error), dir, name, key string) int64 {
	b, err := read(dir + "/" + name)
	if err != nil {
		return 0
	}
	var total int64
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		// "key value" 形式：key 在第 0 列。
		if len(f) >= 2 && f[0] == key {
			if v, perr := strconv.ParseInt(f[1], 10, 64); perr == nil {
				total += v
			}
		}
		// "k=v" 形式：任意列都可能。
		for _, field := range f {
			k, v, ok := strings.Cut(field, "=")
			if !ok || k != key {
				continue
			}
			if n, perr := strconv.ParseInt(v, 10, 64); perr == nil {
				total += n
			}
		}
	}
	return total
}

// sendTop 列出沙盒内的进程，每行格式与 docker top 一致：
// uid pid ppid c stime tty time cmd。
func (s *Server) sendTop(m proto.Msg) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		_ = s.send(&proto.Msg{Kind: proto.KindError, Err: errText(err)})
		return
	}
	lines := make([]string, 0, 32)
	for _, e := range entries {
		pid, perr := strconv.Atoi(e.Name())
		if perr != nil {
			continue
		}
		line, ok := procLine(pid)
		if !ok {
			continue
		}
		lines = append(lines, line)
	}
	_ = s.send(&proto.Msg{Kind: proto.KindTop, Seq: m.Seq, Lines: lines})
}

// procLine 组装一个进程的信息行；进程在读取过程中消失时返回 false。
func procLine(pid int) (string, bool) {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", false
	}
	// stat 的 comm 可能含空格与括号，从最后一个 ')' 之后开始切。
	text := string(stat)
	i := strings.LastIndex(text, ")")
	if i < 0 || i+2 >= len(text) {
		return "", false
	}
	fields := strings.Fields(text[i+2:])
	// 从 state 开始：state ppid ... tty_nr ... utime stime
	if len(fields) < 13 {
		return "", false
	}
	ppid := fields[1]
	tty := fields[4]
	utime, _ := strconv.ParseInt(fields[11], 10, 64)
	stime, _ := strconv.ParseInt(fields[12], 10, 64)
	comm := strings.TrimSpace(text[strings.Index(text, "(")+1 : i])
	uid := procUID(pid)
	return fmt.Sprintf("%s\t%d\t%s\t0\t0\t%s\t%d:%02d\t%s", uid, pid, ppid, tty,
		(utime+stime)/100, (utime+stime)%100, comm), true
}

// procUID 从 /proc/<pid>/status 的 Uid: 行取真实 uid；读不到返回 "0"。
func procUID(pid int) string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return "0"
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(line, "Uid:"); ok {
			if f := strings.Fields(rest); len(f) > 0 {
				return f[0]
			}
		}
	}
	return "0"
}

// sendListening 解析 /proc/net/tcp{,6} 里 LISTEN 的端口。
func (s *Server) sendListening(m proto.Msg) {
	var ports []uint16
	seen := map[uint16]bool{}
	for _, name := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		b, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(b), "\n") {
			if i == 0 || line == "" {
				continue
			}
			f := strings.Fields(line)
			// 第 2 列是 local_address，第 4 列是 state；0A = LISTEN。
			if len(f) < 4 || f[3] != "0A" {
				continue
			}
			_, portHex, ok := strings.Cut(f[1], ":")
			if !ok {
				continue
			}
			v, perr := strconv.ParseUint(portHex, 16, 16)
			if perr != nil || seen[uint16(v)] {
				continue
			}
			seen[uint16(v)] = true
			ports = append(ports, uint16(v))
		}
	}
	_ = s.send(&proto.Msg{Kind: proto.KindListening, Seq: m.Seq, Ports: ports})
}

// relayConn 是一条入站中继连接。
type relayConn struct {
	conn net.Conn
	// mu 保证 Close 与 Data 不会竞争写同一个连接。
	mu     sync.Mutex
	closed bool
	// wroteFin 表示宿主已经半关闭写方向；之后的 Data 不该再来，
	// 来了也丢弃（连接可能已经被对端半关闭）。
	wroteFin bool
}

// openRelay 把宿主的入站连接接到沙盒内的 127.0.0.1:Port。
//
// 这是 E2B getHost(port) 的实现方式：沙盒不需要任何网络栈，
// 平台反向代理收到请求后经控制通道开一条中继。
func (s *Server) openRelay(m proto.Msg) {
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(m.Port))))
	if err != nil {
		// 连不上必须明确告诉宿主：只说「关掉了」会被当成「连上了但立刻 EOF」，
		// 上层就无法区分「端口没人 listen」与「服务端空响应」。
		_ = s.send(&proto.Msg{Kind: proto.KindRelayClose, ConnID: m.ConnID, Flags: proto.RelayConnectFailed})
		return
	}
	rc := &relayConn{conn: conn}
	s.addRelay(m.ConnID, rc)
	// 连上就先回一个空的数据帧：宿主据此让 DialPort 立刻返回。
	// 没有它，请求/响应型服务（客户端先写请求）会让 DialPort 一直等到
	// 服务端开口，5 秒后误报「relay did not connect」。
	_ = s.send(&proto.Msg{Kind: proto.KindRelayData, ConnID: m.ConnID})
	go s.pumpRelay(m.ConnID, rc)
}

// addRelay 登记一条中继。
func (s *Server) addRelay(id uint32, rc *relayConn) {
	s.relayMu.Lock()
	s.relays[id] = rc
	s.relayMu.Unlock()
}

// dropRelay 注销一条中继。
func (s *Server) dropRelay(id uint32) {
	s.relayMu.Lock()
	delete(s.relays, id)
	s.relayMu.Unlock()
}

// getRelay 按 id 取中继。
func (s *Server) getRelay(id uint32) *relayConn {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	return s.relays[id]
}

// pumpRelay 读沙盒内进程的输出，转成 RelayData 发给宿主。
func (s *Server) pumpRelay(id uint32, rc *relayConn) {
	buf := make([]byte, proto.MaxOutputChunk)
	defer func() {
		rc.close()
		s.dropRelay(id)
		_ = s.send(&proto.Msg{Kind: proto.KindRelayClose, ConnID: id})
	}()
	for {
		n, err := rc.conn.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			if serr := s.send(&proto.Msg{Kind: proto.KindRelayData, ConnID: id, Data: data}); serr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// relayData 把宿主的字节写进沙盒内的连接。
func (s *Server) relayData(m proto.Msg) {
	rc := s.getRelay(m.ConnID)
	if rc == nil {
		return
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.closed || rc.wroteFin {
		return
	}
	_, _ = rc.conn.Write(m.Data)
}

// relayHalfClose 处理半关闭。
//
// 宿主说「我不再发数据」时，把沙盒内连接的写方向关掉（TCP 的 FIN）：
// 沙盒内的进程据此知道请求结束，但仍然可以回写响应。
// 这是隧道模式最关键的一步——没有它，HTTP/1.1 的服务端会一直等
// 请求体结束，keep-alive 也永远等不到下一轮。
func (s *Server) relayHalfClose(m proto.Msg) {
	rc := s.getRelay(m.ConnID)
	if rc == nil {
		return
	}
	if m.Flags&1 != 0 { // 宿主 → 沙盒方向结束
		rc.mu.Lock()
		rc.wroteFin = true
		closed := rc.closed
		rc.mu.Unlock()
		if closed {
			return
		}
		// 只关写方向：net.TCPConn.CloseWrite。
		if cw, ok := rc.conn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		} else {
			_ = rc.conn.Close()
		}
	}
	if m.Flags&2 != 0 { // 宿主不再读：沙盒内的输出没人要，直接断开
		rc.close()
	}
}

// relayClose 关掉一条中继。
func (s *Server) relayClose(m proto.Msg) {
	if rc := s.getRelay(m.ConnID); rc != nil {
		rc.close()
	}
}

func (r *relayConn) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.closed = true
		_ = r.conn.Close()
	}
}
